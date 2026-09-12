package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

type snapshotTestCompactor struct {
	snapshot []llm.ChatMessage
	summary  string
}

func (*snapshotTestCompactor) ShouldCompact([]llm.ChatMessage, int) bool { return true }

func (c *snapshotTestCompactor) Compact([]llm.ChatMessage) ([]llm.ChatMessage, error) {
	return c.snapshot, nil
}

func (c *snapshotTestCompactor) CompactWithReport([]llm.ChatMessage) (CompactReport, error) {
	return CompactReport{Messages: c.snapshot, Summary: c.summary}, nil
}

type failOnceEventStore struct {
	session.Store
	mu        sync.Mutex
	failType  session.EventType
	failCount int
}

func (s *failOnceEventStore) AppendEvent(event session.Event) error {
	s.mu.Lock()
	if event.Type == s.failType && s.failCount > 0 {
		s.failCount--
		s.mu.Unlock()
		return fmt.Errorf("injected %s append failure", event.Type)
	}
	s.mu.Unlock()
	return s.Store.AppendEvent(event)
}

func chatText(role llm.Role, text string) llm.ChatMessage {
	return llm.ChatMessage{
		Role:    role,
		Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: text}},
	}
}

func TestAgentLoopPersistsExactCompactionSnapshot(t *testing.T) {
	store := session.NewMemoryStore()
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("完成")}

	input := make([]llm.ChatMessage, 10)
	for i := range input {
		input[i] = chatText(llm.RoleUser, fmt.Sprintf("历史 %d", i))
	}
	snapshot := []llm.ChatMessage{chatText(llm.RoleUser, session.SummaryMessagePrefix+"已总结历史 0 到 4")}
	snapshot = append(snapshot, input[5:]...)
	compactor := &snapshotTestCompactor{snapshot: snapshot, summary: "已总结历史 0 到 4"}

	a, err := New(WithModel(model), WithSession(store), WithCompactor(compactor))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()
	loop := loopOf(t, a)
	for i := 0; i < 9; i++ {
		data, err := session.EncodeData(&session.PromptedData{Content: fmt.Sprintf("历史 %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendEvent(session.Event{SessionID: loop.sessionID, Type: session.EventPrompted, Data: data}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := a.HandleMessage(context.Background(), "历史 9"); err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("模型调用次数 = %d，期望 1", len(model.requests))
	}

	events := sessionEvents(t, loop)
	var compactEvent session.Event
	var built requestBuilt
	for _, event := range events {
		if event.Type == session.EventCompacted {
			compactEvent = event
		}
	}
	requests := requestBuiltEvents(t, events)
	if len(requests) != 1 {
		t.Fatalf("request.built 事件数 = %d，期望 1", len(requests))
	}
	built = requests[0]
	if compactEvent.Seq == 0 || compactEvent.Seq >= built.Seq {
		t.Fatalf("压缩事件 Seq=%d 应先于 request.built Seq=%d", compactEvent.Seq, built.Seq)
	}

	decoded, err := session.DecodeData(compactEvent)
	if err != nil {
		t.Fatalf("解码压缩事件失败: %v", err)
	}
	data := decoded.(*session.CompactedData)
	if !reflect.DeepEqual(data.Snapshot, snapshot) {
		t.Fatalf("持久化快照与压缩器输出不同：\n got: %#v\nwant: %#v", data.Snapshot, snapshot)
	}
	projected := session.ProjectMessages(eventsUpTo(events, built.Seq))
	if !reflect.DeepEqual(projected, snapshot) {
		t.Fatalf("日志投影与压缩器输出不同：\n got: %#v\nwant: %#v", projected, snapshot)
	}
	if !reflect.DeepEqual(model.requests[0].Messages, snapshot) {
		t.Fatalf("实际请求与压缩器输出不同：\n got: %#v\nwant: %#v", model.requests[0].Messages, snapshot)
	}
	if err := CheckRequestInvariant(eventsUpTo(events, built.Seq), model.requests[0].Messages); err != nil {
		t.Fatalf("请求无法由当时的事件日志重建: %v", err)
	}
}

func TestAgentLoopPersistsSameLengthReorderedSnapshot(t *testing.T) {
	store := session.NewMemoryStore()
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("完成")}
	input := []llm.ChatMessage{
		chatText(llm.RoleUser, "第一条"),
		chatText(llm.RoleAssistant, "第二条"),
		chatText(llm.RoleUser, "第三条"),
	}
	snapshot := []llm.ChatMessage{input[2], input[1], input[0]}
	compactor := &snapshotTestCompactor{snapshot: snapshot}
	a, err := New(WithModel(model), WithSession(store), WithCompactor(compactor))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()
	loop := loopOf(t, a)
	for i := 0; i < 2; i++ {
		data, err := session.EncodeData(&session.PromptedData{Content: []string{"第一条", "第二条"}[i]})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendEvent(session.Event{SessionID: loop.sessionID, Type: session.EventPrompted, Data: data}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := a.HandleMessage(context.Background(), "第三条"); err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
	if len(model.requests) != 1 || !reflect.DeepEqual(model.requests[0].Messages, snapshot) {
		t.Fatalf("等长重排的快照没有进入实际请求：got %#v, want %#v", model.requests, snapshot)
	}
}

func TestSystemPromptPersistenceFailureStopsAndRetries(t *testing.T) {
	store := &failOnceEventStore{
		Store:     session.NewMemoryStore(),
		failType:  session.EventSystemPromptSet,
		failCount: 1,
	}
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("恢复后")}
	a, err := New(WithModel(model), WithSession(store), WithSystemPrompt("必须保留的 system prompt"))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if _, err := a.HandleMessage(context.Background(), "第一次"); err == nil {
		t.Fatal("system.prompt_set 写失败时应阻断本轮")
	}
	if len(model.requests) != 0 {
		t.Fatalf("写失败后模型调用次数 = %d，期望 0", len(model.requests))
	}
	if _, err := a.HandleMessage(context.Background(), "重试"); err != nil {
		t.Fatalf("重试仍失败: %v", err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) == 0 ||
		messageText(model.requests[0].Messages[0]) != "必须保留的 system prompt" {
		t.Fatalf("重试请求没有恢复 system prompt: %#v", model.requests)
	}
}

func TestSteeringPersistenceFailureRestoresQueueAndStops(t *testing.T) {
	store := &failOnceEventStore{
		Store:     session.NewMemoryStore(),
		failType:  session.EventSteered,
		failCount: 1,
	}
	steering := NewSteeringManager()
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("恢复后")}
	a, err := New(WithModel(model), WithSession(store), WithSteeringManager(steering))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()
	if err := steering.InjectMessage(context.Background(), "不要丢掉我", Queue); err != nil {
		t.Fatal(err)
	}

	if _, err := a.HandleMessage(context.Background(), "第一次"); err == nil {
		t.Fatal("steered 事件写失败时应阻断本轮")
	}
	if len(model.requests) != 0 {
		t.Fatalf("写失败后模型调用次数 = %d，期望 0", len(model.requests))
	}
	if _, err := a.HandleMessage(context.Background(), "重试"); err != nil {
		t.Fatalf("重试仍失败: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("重试后的模型调用次数 = %d，期望 1", len(model.requests))
	}
	found := false
	for _, msg := range model.requests[0].Messages {
		if msg.Role == llm.RoleSystem && messageText(msg) == "不要丢掉我" {
			found = true
		}
	}
	if !found {
		t.Fatalf("steering 未在重试请求中恢复: %#v", model.requests[0].Messages)
	}
}

func TestSystemPromptCanBeClearedInEventLog(t *testing.T) {
	model := &requestCaptureModel{}
	model.mockModel.responses = []llm.Response{textResponse("第一轮"), textResponse("第二轮")}
	a, err := New(WithModel(model), WithSystemPrompt("旧 prompt"))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if _, err := a.HandleMessage(context.Background(), "第一轮输入"); err != nil {
		t.Fatal(err)
	}
	loop := loopOf(t, a)
	loop.cfg.systemPrompt = ""
	if _, err := a.HandleMessage(context.Background(), "第二轮输入"); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("模型调用次数 = %d，期望 2", len(model.requests))
	}
	for _, msg := range model.requests[1].Messages {
		if msg.Role == llm.RoleSystem {
			t.Fatalf("清空后的请求仍包含 system prompt: %#v", msg)
		}
	}
	events := sessionEvents(t, loop)
	latest := "not-cleared"
	for _, event := range events {
		if event.Type != session.EventSystemPromptSet {
			continue
		}
		var data session.SystemPromptSetData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		latest = data.Content
	}
	if latest != "" {
		t.Fatalf("最后一条 system.prompt_set = %q，期望空串表示清除", latest)
	}
}

func TestInitialEmptySystemPromptClearsPersistedPrompt(t *testing.T) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "existing session"})
	if err != nil {
		t.Fatal(err)
	}
	oldPrompt, err := session.EncodeData(&session.SystemPromptSetData{Content: "过期 prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventSystemPromptSet,
		Data:      oldPrompt,
	}); err != nil {
		t.Fatal(err)
	}

	a := &AgentLoop{session: store, sessionID: info.ID, cfg: &config{}}
	if err := a.persistSystemPrompt(); err != nil {
		t.Fatalf("持久化空 system prompt 失败: %v", err)
	}
	events, err := store.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range BuildRequestMessages(events) {
		if msg.Role == llm.RoleSystem {
			t.Fatalf("空配置应清除已存在的 system prompt，实际请求仍包含: %#v", msg)
		}
	}
}
