package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// requestCaptureModel 记录每次 Stream 的完整请求（messages + tools）。
//
// 与 steering_test.go 的 captureModel 不同：那个只需要 messages，
// 而校验请求指纹必须连 tools 一起拿到。
type requestCaptureModel struct {
	mockModel
	requests []*llm.Request
}

func (m *requestCaptureModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	cp := &llm.Request{
		Messages: append([]llm.ChatMessage(nil), req.Messages...),
		Tools:    append([]llm.ToolDefinition(nil), req.Tools...),
	}
	m.requests = append(m.requests, cp)
	return m.mockModel.Stream(ctx, req)
}

// textResponse 构造一条纯文本 assistant 响应。
func textResponse(text string) llm.Response {
	return llm.Response{
		Message: llm.ChatMessage{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: text}},
		},
		Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

// sessionEvents 读取测试 agent 会话的全部事件。
func sessionEvents(t *testing.T, a *AgentLoop) []session.Event {
	t.Helper()
	events, err := a.session.Events(session.EventFilter{SessionID: a.sessionID})
	if err != nil {
		t.Fatalf("读取会话事件失败: %v", err)
	}
	return events
}

// loopOf 把 Agent 接口断言回 *AgentLoop，便于测试读取会话内部状态。
func loopOf(t *testing.T, a Agent) *AgentLoop {
	t.Helper()
	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatalf("New 返回了非 *AgentLoop 的实现: %T", a)
	}
	return loop
}

// requestBuilt 是解码后的 request.built 事件。
type requestBuilt struct {
	Seq  int64
	Data session.RequestBuiltData
}

// requestBuiltEvents 按写入顺序返回日志里全部 request.built 事件。
func requestBuiltEvents(t *testing.T, events []session.Event) []requestBuilt {
	t.Helper()
	var out []requestBuilt
	for _, e := range events {
		if e.Type != session.EventRequestBuilt {
			continue
		}
		d, err := session.DecodeData(e)
		if err != nil {
			t.Fatalf("解码 request.built 失败: %v", err)
		}
		data, ok := d.(*session.RequestBuiltData)
		if !ok {
			t.Fatalf("request.built 解出了意外类型 %T", d)
		}
		out = append(out, requestBuilt{Seq: e.Seq, Data: *data})
	}
	return out
}

// eventsUpTo 返回 Seq 不大于 seq 的事件——即「那一刻」日志的样子。
func eventsUpTo(events []session.Event, seq int64) []session.Event {
	var out []session.Event
	for _, e := range events {
		if e.Seq <= seq {
			out = append(out, e)
		}
	}
	return out
}

// TestRequestInvariant_SingleTurn 单轮：实际请求必须等于事件日志重建的请求。
func TestRequestInvariant_SingleTurn(t *testing.T) {
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("好的")}

	a, err := New(WithModel(model), WithSystemPrompt("你是助手"))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if _, err := a.HandleMessage(context.Background(), "你好"); err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("期望 1 次请求，实际 %d 次", len(model.requests))
	}

	events := sessionEvents(t, loopOf(t, a))
	built := requestBuiltEvents(t, events)
	if len(built) != 1 {
		t.Fatalf("期望 1 条 request.built 事件，实际 %d 条", len(built))
	}

	// 校验必须拿「请求那一刻」的日志：请求发出后，assistant 的回复与 turn.ended
	// 还会继续落盘，用最终日志去比必然多出消息。
	if err := CheckRequestInvariant(eventsUpTo(events, built[0].Seq), model.requests[0].Messages); err != nil {
		t.Fatalf("请求与事件日志不一致: %v", err)
	}

	// system prompt 必须在最前，且内容来自事件而不是配置。
	if got := model.requests[0].Messages[0]; got.Role != llm.RoleSystem || messageText(got) != "你是助手" {
		t.Errorf("首条消息 = %q/%q，期望 system/你是助手", got.Role, messageText(got))
	}
}

// TestRequestInvariant_RequestPrecedesAssistantReply 记录一个容易搞错的时间点：
// request.built 写在 Stream 之前，因此它的 Seq 严格小于本轮 assistant 回复事件的 Seq。
//
// 这正是「用最终日志校验历史请求」会失败的原因，也是必须按 Seq 截断日志的原因。
func TestRequestInvariant_RequestPrecedesAssistantReply(t *testing.T) {
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("回复")}

	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if _, err := a.HandleMessage(context.Background(), "你好"); err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	events := sessionEvents(t, loopOf(t, a))
	built := requestBuiltEvents(t, events)
	if len(built) != 1 {
		t.Fatalf("期望 1 条 request.built 事件，实际 %d 条", len(built))
	}

	var replySeq int64
	for _, e := range events {
		if e.Type == session.EventTextDelta {
			replySeq = e.Seq
		}
	}
	if replySeq == 0 {
		t.Fatal("未找到 assistant 回复事件")
	}
	if built[0].Seq >= replySeq {
		t.Errorf("request.built Seq=%d 应早于回复 Seq=%d", built[0].Seq, replySeq)
	}

	// 用最终日志校验会失败——这是预期行为，不是 bug。
	if err := CheckRequestInvariant(events, model.requests[0].Messages); err == nil {
		t.Error("用最终日志校验历史请求本该失败（回复已入日志），却通过了")
	}
}

// TestRequestInvariant_SurvivesSecondTurn 多轮 + steering + system prompt：
// 「请求 = 日志的纯函数」这条等式必须每一轮都成立，而不只是第一轮。
//
// 之所以要逐轮校验：后期的日志会包含之后的 turn，直接拿最终日志比第一轮请求
// 必然不等。所以这里用每条 request.built 事件的 Seq 把日志截回那一刻。
func TestRequestInvariant_SurvivesSecondTurn(t *testing.T) {
	sm := NewSteeringManager()
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("第一轮"), textResponse("第二轮")}

	a, err := New(
		WithModel(model),
		WithSteeringManager(sm),
		WithSystemPrompt("你是助手"),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if err := sm.InjectMessage(context.Background(), "请用中文回答", Queue); err != nil {
		t.Fatalf("注入 steering 失败: %v", err)
	}

	ctx := context.Background()
	if _, err := a.HandleMessage(ctx, "你好"); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if _, err := a.HandleMessage(ctx, "继续"); err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}

	if len(model.requests) != 2 {
		t.Fatalf("期望 2 次请求，实际 %d 次", len(model.requests))
	}

	events := sessionEvents(t, loopOf(t, a))
	built := requestBuiltEvents(t, events)
	if len(built) != 2 {
		t.Fatalf("期望 2 条 request.built 事件，实际 %d 条", len(built))
	}

	for i, b := range built {
		req := model.requests[i]
		// ① 请求可由「那一刻」的日志完整重建
		if err := CheckRequestInvariant(eventsUpTo(events, b.Seq), req.Messages); err != nil {
			t.Errorf("第 %d 轮请求与当时的日志不一致: %v", i+1, err)
		}
		// ② 日志里留下的指纹必须与真实请求一致
		want := RequestFingerprint(req.Messages, req.Tools)
		if b.Data.Hash != want {
			t.Errorf("第 %d 轮指纹不符：日志 %s，实际 %s", i+1, b.Data.Hash, want)
		}
		if b.Data.MsgCount != len(req.Messages) || b.Data.ToolCount != len(req.Tools) {
			t.Errorf("第 %d 轮条数不符：日志 %d/%d，实际 %d/%d",
				i+1, b.Data.MsgCount, b.Data.ToolCount, len(req.Messages), len(req.Tools))
		}
	}

	// steering 是「一轮注入、之后仍在」的：第二轮重建出的请求里也必须还看得到它。
	second := model.requests[1].Messages
	found := false
	for _, msg := range second {
		if msg.Role == llm.RoleSystem && messageText(msg) == "请用中文回答" {
			found = true
		}
	}
	if !found {
		t.Error("第二轮请求里丢失了 steering 消息——它已落盘，就不该消失")
	}

	// 来源描述必须点明 system prompt 与 steering 都来自日志。
	var kinds []string
	for _, s := range built[1].Data.Sources {
		kinds = append(kinds, s.Kind)
	}
	if !contains(kinds, SourceSystemPrompt) || !contains(kinds, SourceSteering) {
		t.Errorf("第二轮来源描述 = %v，期望同时包含 %s 与 %s",
			kinds, SourceSystemPrompt, SourceSteering)
	}
}

// TestRequestInvariant_PicksLatestSystemPrompt 多条 system.prompt_set 时只认最后一条。
func TestRequestInvariant_PicksLatestSystemPrompt(t *testing.T) {
	events := []session.Event{
		{Seq: 1, Type: session.EventSystemPromptSet, Data: mustEncode(t, &session.SystemPromptSetData{Content: "旧人格"})},
		{Seq: 2, Type: session.EventSystemPromptSet, Data: mustEncode(t, &session.SystemPromptSetData{Content: "新人格"})},
		{Seq: 3, Type: session.EventPrompted, Data: mustEncode(t, &session.PromptedData{Content: "你好"})},
	}

	msgs := BuildRequestMessages(events)
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息（system + user），得到 %d", len(msgs))
	}
	if got := messageText(msgs[0]); got != "新人格" {
		t.Errorf("system prompt = %q，期望最后一条 %q", got, "新人格")
	}
	if msgs[1].Role != llm.RoleUser {
		t.Errorf("第二条消息角色 = %q，期望 user", msgs[1].Role)
	}
}

// TestRequestInvariant_OrderSystemThenSteeringThenHistory 组装顺序必须是
// system prompt → steering → 历史，否则 system 消息会落到对话中间，多数 provider 会拒收。
func TestRequestInvariant_OrderSystemThenSteeringThenHistory(t *testing.T) {
	events := []session.Event{
		{Seq: 1, Type: session.EventSystemPromptSet, Data: mustEncode(t, &session.SystemPromptSetData{Content: "人格"})},
		{Seq: 2, Type: session.EventPrompted, Data: mustEncode(t, &session.PromptedData{Content: "历史问题"})},
		{Seq: 3, Type: session.EventTextDelta, Data: mustEncode(t, &session.TextDeltaData{Delta: "历史回答"})},
		{Seq: 4, Type: session.EventTextEnded},
		{Seq: 5, Type: session.EventSteered, Data: mustEncode(t, &session.SteeredData{Messages: []string{"插话"}})},
	}

	msgs := BuildRequestMessages(events)
	want := []struct {
		role llm.Role
		text string
	}{
		{llm.RoleSystem, "人格"},
		{llm.RoleSystem, "插话"},
		{llm.RoleUser, "历史问题"},
		{llm.RoleAssistant, "历史回答"},
	}
	if len(msgs) != len(want) {
		t.Fatalf("消息数 = %d，期望 %d", len(msgs), len(want))
	}
	for i, w := range want {
		if msgs[i].Role != w.role || messageText(msgs[i]) != w.text {
			t.Errorf("msgs[%d] = %q/%q，期望 %q/%q", i, msgs[i].Role, messageText(msgs[i]), w.role, w.text)
		}
	}
}

// TestCheckRequestInvariant_DetectsDrift 校验器必须真的能发现漂移，
// 否则「不变量」只是句口号。
func TestCheckRequestInvariant_DetectsDrift(t *testing.T) {
	events := []session.Event{
		{Seq: 1, Type: session.EventSystemPromptSet, Data: mustEncode(t, &session.SystemPromptSetData{Content: "人格"})},
		{Seq: 2, Type: session.EventPrompted, Data: mustEncode(t, &session.PromptedData{Content: "你好"})},
	}

	t.Run("长度不符", func(t *testing.T) {
		actual := BuildRequestMessages(events)
		actual = append(actual, llm.ChatMessage{Role: llm.RoleUser})
		if err := CheckRequestInvariant(events, actual); err == nil {
			t.Error("多出一条消息时应报错")
		}
	})

	t.Run("内容被改写", func(t *testing.T) {
		actual := BuildRequestMessages(events)
		actual[0].Content = []llm.ContentPart{{Type: llm.ContentTypeText, Text: "被 hook 改过的人格"}}
		if err := CheckRequestInvariant(events, actual); err == nil {
			t.Error("system prompt 被改写时应报错")
		}
	})

	t.Run("一致时不报错", func(t *testing.T) {
		if err := CheckRequestInvariant(events, BuildRequestMessages(events)); err != nil {
			t.Errorf("一致时不应报错，得到 %v", err)
		}
	})
}

func TestCheckRequestInvariant_DetectsEveryMessageField(t *testing.T) {
	const secret = "sensitive-payload-marker"
	events := []session.Event{
		{Seq: 1, Type: session.EventPrompted, Data: mustEncode(t, &session.PromptedData{Content: "你好"})},
	}
	base := BuildRequestMessages(events)[0]

	tests := []struct {
		name       string
		wantReason string
		mutate     func(*llm.ChatMessage)
	}{
		{name: "role", wantReason: "role 不一致", mutate: func(m *llm.ChatMessage) { m.Role = llm.RoleAssistant }},
		{name: "name", wantReason: "name 不一致", mutate: func(m *llm.ChatMessage) { m.Name = secret }},
		{name: "tool call id", wantReason: "tool_call_id 不一致", mutate: func(m *llm.ChatMessage) { m.ToolCallID = secret }},
		{name: "content type", wantReason: "content[0].type 不一致", mutate: func(m *llm.ChatMessage) { m.Content[0].Type = llm.ContentTypeImage }},
		{name: "content text", wantReason: "content[0].text 不一致", mutate: func(m *llm.ChatMessage) { m.Content[0].Text = secret }},
		{name: "image url", wantReason: "content[0].image_url 不一致", mutate: func(m *llm.ChatMessage) { m.Content[0].ImageURL = "https://example.invalid/" + secret }},
		{name: "cache control", wantReason: "content[0].cache_control presence 不一致", mutate: func(m *llm.ChatMessage) { m.Content[0].CacheControl = &llm.CacheControl{Type: secret} }},
		{
			name:       "content segment boundary",
			wantReason: "content 分段数量不一致",
			mutate: func(m *llm.ChatMessage) {
				m.Content = []llm.ContentPart{
					{Type: llm.ContentTypeText, Text: "你"},
					{Type: llm.ContentTypeText, Text: "好"},
				}
			},
		},
		{name: "tool calls", wantReason: "tool_calls 数量不一致", mutate: func(m *llm.ChatMessage) { m.ToolCalls = []llm.ToolCall{{ID: secret}} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := base
			actual.Content = append([]llm.ContentPart(nil), base.Content...)
			actual.ToolCalls = append([]llm.ToolCall(nil), base.ToolCalls...)
			tt.mutate(&actual)

			err := CheckRequestInvariant(events, []llm.ChatMessage{actual})
			if err == nil {
				t.Fatal("字段发生差异时应报错")
			}
			if !strings.Contains(err.Error(), tt.wantReason) {
				t.Errorf("差异原因 %q 不包含 %q", err, tt.wantReason)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("差异原因泄露了消息内容: %q", err)
			}
		})
	}

	t.Run("tool call fields", func(t *testing.T) {
		toolEvent := session.Event{
			Seq:  1,
			Type: session.EventToolCalled,
			Data: mustEncode(t, &session.ToolCalledData{ToolCall: llm.ToolCall{ID: "logged-id", Name: "logged-name", ArgsJSON: `{"key":"logged-value"}`}}),
		}
		for _, field := range []struct {
			name       string
			wantReason string
			mutate     func(*llm.ToolCall)
		}{
			{name: "id", wantReason: "tool_calls[0].id 不一致", mutate: func(tc *llm.ToolCall) { tc.ID = secret }},
			{name: "name", wantReason: "tool_calls[0].name 不一致", mutate: func(tc *llm.ToolCall) { tc.Name = secret }},
			{name: "arguments", wantReason: "tool_calls[0].args_json 不一致", mutate: func(tc *llm.ToolCall) { tc.ArgsJSON = secret }},
		} {
			t.Run(field.name, func(t *testing.T) {
				actual := BuildRequestMessages([]session.Event{toolEvent})
				field.mutate(&actual[0].ToolCalls[0])
				err := CheckRequestInvariant([]session.Event{toolEvent}, actual)
				if err == nil || !strings.Contains(err.Error(), field.wantReason) {
					t.Fatalf("差异原因 = %v，期望包含 %q", err, field.wantReason)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("差异原因泄露了消息内容: %q", err)
				}
			})
		}
	})

	t.Run("cache control type", func(t *testing.T) {
		logged := llm.ChatMessage{Content: []llm.ContentPart{{
			Type:         llm.ContentTypeText,
			Text:         "内容",
			CacheControl: &llm.CacheControl{Type: "logged-cache-type"},
		}}}
		actual := llm.ChatMessage{Content: []llm.ContentPart{{
			Type:         llm.ContentTypeText,
			Text:         "内容",
			CacheControl: &llm.CacheControl{Type: "actual-cache-type"},
		}}}
		reason := diffMessage(logged, actual)
		if !strings.Contains(reason, "content[0].cache_control.type 不一致") {
			t.Fatalf("差异原因 = %q", reason)
		}
		if strings.Contains(reason, "logged-cache-type") || strings.Contains(reason, "actual-cache-type") {
			t.Errorf("差异原因泄露了缓存控制值: %q", reason)
		}
	})
}

// mustEncode 是测试用的 EncodeData 包装，编码失败直接 Fatal。
func mustEncode(t *testing.T, v interface{}) []byte {
	t.Helper()
	raw, err := session.EncodeData(v)
	if err != nil {
		t.Fatalf("EncodeData 失败: %v", err)
	}
	return raw
}

// contains 判断字符串切片是否含目标值。
func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
