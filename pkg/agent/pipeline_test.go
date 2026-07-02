package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// mockModel 是用于测试的 LLM Model 模拟实现
type mockModel struct {
	responses []llm.Response // 按顺序返回
	idx       int
}

func (m *mockModel) ID() string { return "mock" }

func (m *mockModel) Generate(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if m.idx >= len(m.responses) {
		return &m.responses[len(m.responses)-1], nil
	}
	resp := &m.responses[m.idx]
	m.idx++
	return resp, nil
}

func (m *mockModel) Stream(_ context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	if m.idx >= len(m.responses) {
		resp := &m.responses[len(m.responses)-1]
		return eventsFromResponse(resp), nil
	}
	resp := &m.responses[m.idx]
	m.idx++
	return eventsFromResponse(resp), nil
}

func (m *mockModel) Supports(cap llm.Capability) bool { return true }

// eventsFromResponse 将 Response 转为 stream event 序列
func eventsFromResponse(resp *llm.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, 10)

	// 发送文本 delta
	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "" {
		ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: text}
	}

	// 发送 tool call 事件
	for i, tc := range resp.Message.ToolCalls {
		// 分多次 delta 模拟流式
		ch <- llm.StreamEvent{
			Type: llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{
				Index: i,
				ID:    tc.ID,
				Name:  tc.Name,
			},
		}
		ch <- llm.StreamEvent{
			Type: llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{
				Index:    i,
				ArgsJSON: tc.ArgsJSON,
				Complete: true,
			},
		}
	}

	// 发送用量
	ch <- llm.StreamEvent{
		Type:  llm.StreamEventUsage,
		Usage: &resp.Usage,
	}

	// 发送完成
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch
}

// mockSession 是 session.Store 的测试封装，绑定 session ID
type mockSession struct {
	session.Store
	sessionID string
}

func (s *mockSession) Messages() ([]llm.ChatMessage, error) {
	return s.Store.Messages()
}

// mockTool 是工具测试实现
type mockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, args json.RawMessage) (*tool.Result, error)
}

func (t *mockTool) Name() string                     { return t.name }
func (t *mockTool) Description() string              { return t.description }
func (t *mockTool) Parameters() json.RawMessage      { return json.RawMessage(`{}`) }
func (t *mockTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, args)
	}
	return &tool.Result{Content: "ok"}, nil
}

// testTurnD 是 TurnD 的测试实现
type testTurnD struct {
	sysPrompt  string
	maxSteps   int
	model      llm.Model
	session    session.Store
	sessionID  string
	history    []llm.ChatMessage
	hooks      *hook.Chain
	registry   *tool.Registry
	callback   Callback
}

func (d *testTurnD) SystemPrompt() string                     { return d.sysPrompt }
func (d *testTurnD) MaxSteps() int                             { return d.maxSteps }
func (d *testTurnD) Model() llm.Model                          { return d.model }
func (d *testTurnD) Session() session.Store                    { return d.session }
func (d *testTurnD) SessionID() string                         { return d.sessionID }
func (d *testTurnD) History() ([]llm.ChatMessage, error)       { return d.history, nil }
func (d *testTurnD) Hooks() *hook.Chain                        { return d.hooks }
func (d *testTurnD) ToolRegistry() *tool.Registry              { return d.registry }
func (d *testTurnD) Callback() Callback                        { return d.callback }

func setupTestTurnD(t *testing.T, model llm.Model, opts ...func(*testTurnD)) *testTurnD {
	t.Helper()

	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "test"})
	if err != nil {
		t.Fatalf("创建 session 失败: %v", err)
	}

	reg := tool.NewRegistry()
	chain := hook.NewChain()

	d := &testTurnD{
		maxSteps:  25,
		model:     model,
		session:   store,
		sessionID: info.ID,
		hooks:     chain,
		registry:  reg,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func TestPipeline_SetupTurn(t *testing.T) {
	model := &mockModel{}
	td := setupTestTurnD(t, model)
	p := NewPipeline(td)

	msgs, tools, err := p.setupTurn(context.Background())
	if err != nil {
		t.Fatalf("setupTurn 返回错误: %v", err)
	}

	// 空 session 返回空消息列表
	if len(msgs) != 0 {
		t.Errorf("期望空 messages, 得到 %d 条", len(msgs))
	}
	if tools == nil {
		t.Error("setupTurn 返回 nil tools")
	}
}

func TestPipeline_SetupTurn_WithSystemPrompt(t *testing.T) {
	model := &mockModel{}
	td := setupTestTurnD(t, model, func(d *testTurnD) {
		d.sysPrompt = "你是一个助手"
	})
	p := NewPipeline(td)

	msgs, _, err := p.setupTurn(context.Background())
	if err != nil {
		t.Fatalf("setupTurn 返回错误: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("期望至少有一条消息（system prompt）")
	}
	if msgs[0].Role != llm.RoleSystem {
		t.Errorf("第一条消息期望 Role=system, 得到 %s", msgs[0].Role)
	}
	if len(msgs[0].Content) == 0 || msgs[0].Content[0].Text != "你是一个助手" {
		t.Errorf("system prompt 内容不匹配: %+v", msgs[0].Content)
	}
}

func TestPipeline_CallLLM_StreamText(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好，有什么可以帮助你的？"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}
	td := setupTestTurnD(t, model)
	p := NewPipeline(td)

	resp, err := p.callLLM(context.Background(), []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}, nil)
	if err != nil {
		t.Fatalf("callLLM 返回错误: %v", err)
	}

	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("期望 Role=assistant, 得到 %s", resp.Message.Role)
	}
	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "你好，有什么可以帮助你的？" {
		t.Errorf("期望文本='你好，有什么可以帮助你的？', 得到 '%s'", text)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("期望 TotalTokens=15, 得到 %d", resp.Usage.TotalTokens)
	}

	// 验证事件已存储到 session
	events, err := td.session.Events(session.EventFilter{SessionID: td.sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	hasTextDelta := false
	for _, e := range events {
		if e.Type == session.EventTextDelta {
			hasTextDelta = true
			break
		}
	}
	if !hasTextDelta {
		t.Error("期望 session 中有 EventTextDelta 事件")
	}
}

func TestPipeline_CallLLM_WithToolCalls(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"北京"}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
			},
		},
	}
	td := setupTestTurnD(t, model)
	p := NewPipeline(td)

	resp, err := p.callLLM(context.Background(), []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "北京天气"}}},
	}, nil)
	if err != nil {
		t.Fatalf("callLLM 返回错误: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("期望 1 个 tool call, 得到 %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "get_weather" {
		t.Errorf("期望 tool name=get_weather, 得到 %s", resp.Message.ToolCalls[0].Name)
	}

	// 验证事件
	events, err := td.session.Events(session.EventFilter{SessionID: td.sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	hasToolCalled := false
	for _, e := range events {
		if e.Type == session.EventToolCalled {
			hasToolCalled = true
			break
		}
	}
	if !hasToolCalled {
		t.Error("期望 session 中有 EventToolCalled 事件")
	}
}

func TestPipeline_ExecuteTools_Success(t *testing.T) {
	model := &mockModel{}
	td := setupTestTurnD(t, model, func(d *testTurnD) {
		d.registry.Register(&mockTool{name: "get_weather", executeFunc: func(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: `{"温度": 22}`}, nil
		}})
	})
	p := NewPipeline(td)

	calls := []llm.ToolCall{{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"北京"}`}}
	records, err := p.executeTools(context.Background(), calls)
	if err != nil {
		t.Fatalf("executeTools 返回错误: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("期望 1 条记录, 得到 %d", len(records))
	}
	if records[0].Err != nil {
		t.Errorf("期望无错误, 得到 %v", records[0].Err)
	}
	if records[0].Result == nil || records[0].Result.Content != `{"温度": 22}` {
		t.Errorf("期望结果内容='{\"温度\": 22}', 得到 %+v", records[0].Result)
	}

	// 验证成功事件
	events, err := td.session.Events(session.EventFilter{SessionID: td.sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	hasSuccess := false
	for _, e := range events {
		if e.Type == session.EventToolSuccess {
			hasSuccess = true
			break
		}
	}
	if !hasSuccess {
		t.Error("期望 session 中有 EventToolSuccess 事件")
	}
}

func TestPipeline_ExecuteTools_ErrorIsolation(t *testing.T) {
	model := &mockModel{}
	td := setupTestTurnD(t, model, func(d *testTurnD) {
		d.registry.Register(&mockTool{name: "good_tool", executeFunc: func(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "success"}, nil
		}})
		d.registry.Register(&mockTool{name: "bad_tool", executeFunc: func(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
			return nil, errors.New("工具执行失败")
		}})
	})
	p := NewPipeline(td)

	calls := []llm.ToolCall{
		{ID: "call_1", Name: "good_tool", ArgsJSON: `{}`},
		{ID: "call_2", Name: "bad_tool", ArgsJSON: `{}`},
	}
	records, err := p.executeTools(context.Background(), calls)
	if err != nil {
		t.Fatalf("executeTools 不应该返回顶层错误: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("期望 2 条记录, 得到 %d", len(records))
	}
	if records[0].Err != nil {
		t.Errorf("good_tool 不应有错误, 得到 %v", records[0].Err)
	}
	if records[1].Err == nil {
		t.Error("bad_tool 应有错误")
	}

	// 验证事件
	events, err := td.session.Events(session.EventFilter{SessionID: td.sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	successCount, failCount := 0, 0
	for _, e := range events {
		switch e.Type {
		case session.EventToolSuccess:
			successCount++
		case session.EventToolFailed:
			failCount++
		}
	}
	if successCount != 1 {
		t.Errorf("期望 1 个 EventToolSuccess, 得到 %d", successCount)
	}
	if failCount != 1 {
		t.Errorf("期望 1 个 EventToolFailed, 得到 %d", failCount)
	}
}

func TestPipeline_ExecuteTools_HookIntegration(t *testing.T) {
	model := &mockModel{}
	var hookCalled bool
	testHook := &hook.FuncHook{
		BeforeToolFn: func(call llm.ToolCall) (*llm.ToolCall, error) {
			hookCalled = true
			return &call, nil
		},
		AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {},
	}
	td := setupTestTurnD(t, model, func(d *testTurnD) {
		d.registry.Register(&mockTool{name: "test_tool"})
		d.hooks.Add(testHook)
	})
	p := NewPipeline(td)

	calls := []llm.ToolCall{{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`}}
	_, err := p.executeTools(context.Background(), calls)
	if err != nil {
		t.Fatalf("executeTools 返回错误: %v", err)
	}
	if !hookCalled {
		t.Error("期望 BeforeTool hook 被调用")
	}
}

func TestPipeline_Finalize(t *testing.T) {
	p := &Pipeline{}

	// 有 tool calls
	records := []ToolCallRecord{
		{Call: llm.ToolCall{ID: "call_1"}, Result: &tool.Result{Content: "ok"}},
	}
	result, err := p.finalize(context.Background(), &llm.Response{
		Message: llm.ChatMessage{Role: llm.RoleAssistant},
		Usage:   llm.Usage{TotalTokens: 10},
	}, records)
	if err != nil {
		t.Fatalf("finalize 返回错误: %v", err)
	}
	if !result.HasToolCalls {
		t.Error("期望 HasToolCalls=true")
	}
	if len(result.ToolCalls) != 1 {
		t.Errorf("期望 1 条 tool call 记录, 得到 %d", len(result.ToolCalls))
	}

	// 无 tool calls
	records2 := []ToolCallRecord{}
	result2, err := p.finalize(context.Background(), &llm.Response{
		Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "hello"}}},
	}, records2)
	if err != nil {
		t.Fatalf("finalize 返回错误: %v", err)
	}
	if result2.HasToolCalls {
		t.Error("无 tool calls 时期望 HasToolCalls=false")
	}
}

func TestPipeline_Finalize_AllToolsFailed(t *testing.T) {
	p := &Pipeline{}
	records := []ToolCallRecord{
		{Call: llm.ToolCall{ID: "call_1"}, Err: errors.New("失败")},
		{Call: llm.ToolCall{ID: "call_2"}, Err: errors.New("失败")},
	}
	result, err := p.finalize(context.Background(), &llm.Response{}, records)
	if err != nil {
		t.Fatalf("finalize 返回错误: %v", err)
	}
	if result.HasToolCalls {
		t.Error("所有工具失败时，HasToolCalls 应为 false，表示无需继续循环")
	}
}