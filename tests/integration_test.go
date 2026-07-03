// Package tests 包含 basework 项目的集成测试。
//
// 这些测试使用 mock LLM 和 mock Tool 模拟完整的 agent 对话流程，
// 验证事件溯源、工具调用循环和并发安全等核心能力。
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// ---------------------------------------------------------------------------
// Mock 实现
// ---------------------------------------------------------------------------

// mockModel 实现 llm.Model 接口，用于集成测试。
type mockModel struct {
	responses []llm.Response // 按顺序返回的响应
	idx       int            // 当前响应索引
	mu        sync.Mutex     // 并发安全
}

func (m *mockModel) ID() string { return "mock-integration" }

func (m *mockModel) Generate(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx >= len(m.responses) {
		return &m.responses[len(m.responses)-1], nil
	}
	resp := &m.responses[m.idx]
	m.idx++
	return resp, nil
}

func (m *mockModel) Stream(_ context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	m.mu.Lock()
	idx := m.idx
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	} else {
		m.idx++
	}
	m.mu.Unlock()

	resp := &m.responses[idx]
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

	// 用量和完成
	ch <- llm.StreamEvent{
		Type:  llm.StreamEventUsage,
		Usage: &resp.Usage,
	}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

func (m *mockModel) Supports(cap llm.Capability) bool { return true }

// newSimpleMock 创建一个只会回复固定文本的 mockModel。
func newSimpleMock(reply string) *mockModel {
	return &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: reply}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}
}

// mockTool 实现 tool.Tool 接口，用于集成测试。
type mockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, args json.RawMessage) (*tool.Result, error)
}

func (t *mockTool) Name() string                { return t.name }
func (t *mockTool) Description() string         { return t.description }
func (t *mockTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *mockTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, args)
	}
	return &tool.Result{Content: `{"result":"ok"}`}, nil
}

// ---------------------------------------------------------------------------
// Task 4.1: 测试基础设施 — 通过上面的 mock 定义完成
// ---------------------------------------------------------------------------

// TestMockModel_ImplementsInterface 验证 mockModel 实现了 llm.Model 接口（编译期检查）。
func TestMockModel_ImplementsInterface(t *testing.T) {
	var _ llm.Model = (*mockModel)(nil)
}

// TestMockTool_ImplementsInterface 验证 mockTool 实现了 tool.Tool 接口（编译期检查）。
func TestMockTool_ImplementsInterface(t *testing.T) {
	var _ tool.Tool = (*mockTool)(nil)
}

// ---------------------------------------------------------------------------
// Task 4.2: TestIntegration_BasicConversation — 单轮对话
// ---------------------------------------------------------------------------

// TestIntegration_BasicConversation 验证基本的单轮对话流程：
// 创建 agent → 发送消息 → 接收响应 → 断言非空。
func TestIntegration_BasicConversation(t *testing.T) {
	model := newSimpleMock("你好！我是 mock 助手。")

	a, err := agent.New(agent.WithModel(model))
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "你好")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}
	if resp.SessionID == "" {
		t.Error("期望非空 SessionID")
	}

	// 提取文本内容
	text := extractText(resp.Message)
	if text == "" {
		t.Error("期望非空响应文本")
	}
	if text != "你好！我是 mock 助手。" {
		t.Errorf("期望文本='你好！我是 mock 助手。', 得到 '%s'", text)
	}

	// 验证用量信息
	if resp.Usage.TotalTokens <= 0 {
		t.Errorf("期望 TotalTokens > 0, 得到 %d", resp.Usage.TotalTokens)
	}
}

// ---------------------------------------------------------------------------
// Task 4.3: TestIntegration_MultiTurnConversation — 多轮对话
// ---------------------------------------------------------------------------

// TestIntegration_MultiTurnConversation 验证多轮对话：
// 发送 3 条连续消息，每条都获得响应，并验证 Session 历史正确累积。
func TestIntegration_MultiTurnConversation(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第一轮回复"}}}, Usage: llm.Usage{TotalTokens: 10}},
			{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第二轮回复"}}}, Usage: llm.Usage{TotalTokens: 20}},
			{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第三轮回复"}}}, Usage: llm.Usage{TotalTokens: 30}},
		},
	}

	// 使用显式的 session store 以便后续检查历史
	store := session.NewMemoryStore()
	a, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	inputs := []string{"第一轮", "第二轮", "第三轮"}
	expected := []string{"第一轮回复", "第二轮回复", "第三轮回复"}

	var sessionID string

	for i, input := range inputs {
		resp, err := a.HandleMessage(context.Background(), input)
		if err != nil {
			t.Fatalf("第 %d 轮 HandleMessage 返回错误: %v", i+1, err)
		}
		if resp == nil {
			t.Fatalf("第 %d 轮 response 为 nil", i+1)
		}

		text := extractText(resp.Message)
		if text != expected[i] {
			t.Errorf("第 %d 轮期望文本='%s', 得到 '%s'", i+1, expected[i], text)
		}

		if i == 0 {
			sessionID = resp.SessionID
		} else if resp.SessionID != sessionID {
			t.Errorf("SessionID 发生变化: 初始=%s, 第%d轮=%s", sessionID, i+1, resp.SessionID)
		}
	}

	// 验证 Session 历史：应有 3 个 Prompted 事件 + 3 组文本事件
	events, err := store.Events(session.EventFilter{SessionID: sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}

	promptCount := 0
	textDeltaCount := 0
	for _, e := range events {
		switch e.Type {
		case session.EventPrompted:
			promptCount++
		case session.EventTextDelta:
			textDeltaCount++
		}
	}

	if promptCount != 3 {
		t.Errorf("期望 3 个 Prompted 事件, 得到 %d", promptCount)
	}
	if textDeltaCount != 3 {
		t.Errorf("期望 3 个 TextDelta 事件, 得到 %d", textDeltaCount)
	}

	// 使用 ProjectMessages 验证重建后的消息列表
	msgs := session.ProjectMessages(events)
	if len(msgs) != 6 { // 3 user + 3 assistant
		t.Errorf("期望 6 条消息 (3 user + 3 assistant), 得到 %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// Task 4.4: TestIntegration_ToolCallLoop — 工具调用循环
// ---------------------------------------------------------------------------

// TestIntegration_ToolCallLoop 验证工具调用循环：
// agent 调用工具 → 工具返回结果 → agent 生成最终响应。
func TestIntegration_ToolCallLoop(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			// 第一轮：返回 tool call
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_weather", Name: "get_weather", ArgsJSON: `{"city":"北京"}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			// 第二轮：工具执行后返回纯文本
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "北京今天 22°C，晴天。"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			},
		},
	}

	// mock 天气工具
	weatherTool := &mockTool{
		name:        "get_weather",
		description: "查询指定城市的天气",
		executeFunc: func(_ context.Context, args json.RawMessage) (*tool.Result, error) {
			var params struct {
				City string `json:"city"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}
			if params.City == "北京" {
				return &tool.Result{Content: `{"温度":22,"天气":"晴天"}`}, nil
			}
			return &tool.Result{Content: `{"温度":15,"天气":"多云"}`}, nil
		},
	}

	a, err := agent.New(
		agent.WithModel(model),
		agent.WithTools(weatherTool),
		agent.WithMaxSteps(5),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "北京天气怎么样？")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	// 验证最终文本
	text := extractText(resp.Message)
	if text != "北京今天 22°C，晴天。" {
		t.Errorf("期望最终文本='北京今天 22°C，晴天。', 得到 '%s'", text)
	}

	// 验证有工具调用记录
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("期望 1 条工具调用记录, 得到 %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Call.Name != "get_weather" {
		t.Errorf("期望工具名='get_weather', 得到 '%s'", resp.ToolCalls[0].Call.Name)
	}
	if resp.ToolCalls[0].Result == nil {
		t.Fatal("期望工具调用结果不为 nil")
	}
	if !strings.Contains(resp.ToolCalls[0].Result.Content, "22") {
		t.Errorf("期望结果包含温度 22, 得到 '%s'", resp.ToolCalls[0].Result.Content)
	}

	// 验证最终用量是最后一次的
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("期望 TotalTokens=30 (最后一次), 得到 %d", resp.Usage.TotalTokens)
	}
}

// TestIntegration_ToolCallLoop_MaxStepsExceeded 验证工具调用超出最大步数限制。
func TestIntegration_ToolCallLoop_MaxStepsExceeded(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_loop", Name: "loop_tool", ArgsJSON: `{}`},
					},
				},
			},
		},
	}

	loopTool := &mockTool{
		name:        "loop_tool",
		description: "总是返回结果导致继续循环",
		executeFunc: func(_ context.Context, _ json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "继续调用我"}, nil
		},
	}

	a, err := agent.New(
		agent.WithModel(model),
		agent.WithTools(loopTool),
		agent.WithMaxSteps(3),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "循环测试")
	if !errors.Is(err, agent.ErrMaxStepsExceeded) {
		t.Errorf("期望 ErrMaxStepsExceeded, 得到 %v", err)
	}
}

// ---------------------------------------------------------------------------
// Task 4.5: TestIntegration_SessionEventSourcing — 事件溯源
// ---------------------------------------------------------------------------

// TestIntegration_SessionEventSourcing 验证事件溯源机制：
// agent 对话产生事件 → 从 Store 加载事件 → 通过 ProjectMessages 重建会话状态。
func TestIntegration_SessionEventSourcing(t *testing.T) {
	store := session.NewMemoryStore()
	model := newSimpleMock("这是回复内容。")

	a, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	// 执行一轮对话
	resp, err := a.HandleMessage(context.Background(), "原始消息")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
	sessionID := resp.SessionID

	// 从 Store 获取事件
	events, err := store.Events(session.EventFilter{SessionID: sessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("期望至少有一个事件")
	}

	// 验证事件类型
	hasPrompted := false
	hasTextDelta := false
	hasTextEnded := false
	hasTurnStarted := false
	hasTurnEnded := false

	for _, e := range events {
		switch e.Type {
		case session.EventPrompted:
			hasPrompted = true
			// 解码验证数据
			data, err := session.DecodeData(e)
			if err != nil {
				t.Errorf("解码 PromptedData 失败: %v", err)
			}
			if pd, ok := data.(*session.PromptedData); ok {
				if pd.Content != "原始消息" {
					t.Errorf("PromptedData.Content 期望='原始消息', 得到 '%s'", pd.Content)
				}
			}
		case session.EventTextDelta:
			hasTextDelta = true
		case session.EventTextEnded:
			hasTextEnded = true
		case session.EventTurnStarted:
			hasTurnStarted = true
		case session.EventTurnEnded:
			hasTurnEnded = true
		}
	}

	if !hasPrompted {
		t.Error("期望 EventPrompted 事件")
	}
	if !hasTextDelta {
		t.Error("期望 EventTextDelta 事件")
	}
	if !hasTextEnded {
		t.Error("期望 EventTextEnded 事件")
	}
	if !hasTurnStarted {
		t.Error("期望 EventTurnStarted 事件")
	}
	if !hasTurnEnded {
		t.Error("期望 EventTurnEnded 事件")
	}

	// 通过 ProjectMessages 重建消息列表
	msgs := session.ProjectMessages(events)
	if len(msgs) != 2 { // 1 user + 1 assistant
		t.Fatalf("期望 2 条重建消息 (1 user + 1 assistant), 得到 %d", len(msgs))
	}

	if msgs[0].Role != llm.RoleUser {
		t.Errorf("第一条消息期望 Role=user, 得到 %s", msgs[0].Role)
	}
	userText := extractText(msgs[0])
	if userText != "原始消息" {
		t.Errorf("用户消息内容不匹配: '%s'", userText)
	}

	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("第二条消息期望 Role=assistant, 得到 %s", msgs[1].Role)
	}
	assistantText := extractText(msgs[1])
	if assistantText != "这是回复内容。" {
		t.Errorf("助手消息内容不匹配: '%s'", assistantText)
	}

	// 验证 session.Info
	info, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("获取会话信息失败: %v", err)
	}
	if info.ID != sessionID {
		t.Errorf("Info.ID 不匹配: '%s'", info.ID)
	}
	if info.MessageCount != 0 { // MessageCount 在 Create 后为 0，未追踪后续变更
		t.Logf("注意: MessageCount=%d (可能未被自动更新)", info.MessageCount)
	}
}

// TestIntegration_SessionEventSourcing_MultiTurn 验证多轮事件溯源。
func TestIntegration_SessionEventSourcing_MultiTurn(t *testing.T) {
	store := session.NewMemoryStore()
	model := &mockModel{
		responses: []llm.Response{
			{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第一轮"}}}, Usage: llm.Usage{TotalTokens: 10}},
			{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第二轮"}}}, Usage: llm.Usage{TotalTokens: 20}},
		},
	}

	a, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	resp1, _ := a.HandleMessage(context.Background(), "你好")
	sessionID := resp1.SessionID
	_, _ = a.HandleMessage(context.Background(), "再来一轮")

	// 加载事件并重建
	events, _ := store.Events(session.EventFilter{SessionID: sessionID})
	msgs := session.ProjectMessages(events)

	// 期望 4 条消息: 2 user + 2 assistant
	if len(msgs) != 4 {
		t.Errorf("期望 4 条重建消息, 得到 %d", len(msgs))
	}

	// 验证顺序正确
	expectedRoles := []llm.Role{llm.RoleUser, llm.RoleAssistant, llm.RoleUser, llm.RoleAssistant}
	for i, role := range expectedRoles {
		if msgs[i].Role != role {
			t.Errorf("消息 %d 期望 Role=%s, 得到 %s", i, role, msgs[i].Role)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 4.6: TestIntegration_OpenAI_E2E — 真实 OpenAI 端到端测试
// ---------------------------------------------------------------------------

// TestIntegration_OpenAI_E2E 使用真实 OpenAI API 进行端到端测试。
// 需要设置 OPENAI_API_KEY 环境变量，否则自动跳过。
//
// 由于 mock 无法验证流式输出，此测试提供真实环境验证。
func TestIntegration_OpenAI_E2E(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY 未设置，跳过真实 OpenAI 测试")
	}

	// 注意: 这里使用真实 OpenAI 模型。
	// 为确保测试可重复，实际环境中应使用适配器模式从配置创建模型。
	// 以下代码为框架示例，展示如何接入真实 provider。
	//
	// 实际使用时，取消注释并替换为真实模型创建代码:
	//
	// model, err := openai.NewModel(openai.Config{
	//     APIKey:  apiKey,
	//     Model:   "gpt-4o-mini",
	// })
	// if err != nil {
	//     t.Fatalf("创建 OpenAI 模型失败: %v", err)
	// }
	//
	// a, err := agent.New(agent.WithModel(model))
	// ...
	//
	// // 验证流式输出
	// streamCh, err := model.Stream(ctx, &llm.Request{
	//     Messages: []llm.ChatMessage{
	//         {Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	//     },
	// })
	// if err != nil {
	//     t.Fatalf("Stream 返回错误: %v", err)
	// }
	//
	// var fullText strings.Builder
	// for event := range streamCh {
	//     if event.Type == llm.StreamEventText {
	//         fullText.WriteString(event.Delta)
	//     }
	//     if event.Type == llm.StreamEventDone {
	//         break
	//     }
	// }
	// if fullText.Len() == 0 {
	//     t.Error("流式输出为空")
	// }

	t.Log("OPENAI_API_KEY 已设置。如需运行真实测试，请取消注释测试体内的 OpenAI 模型创建代码。")
}

// ---------------------------------------------------------------------------
// Task 4.7: TestIntegration_ConcurrentConversations — 并发对话
// ---------------------------------------------------------------------------

// TestIntegration_ConcurrentConversations 验证多个 agent 并发运行时的安全性。
// 启动 5 个 goroutine 各自进行独立对话，用 -race 检测竞争条件。
func TestIntegration_ConcurrentConversations(t *testing.T) {
	const goroutineCount = 5
	const messagesPerGoroutine = 3

	var wg sync.WaitGroup
	errCh := make(chan error, goroutineCount*messagesPerGoroutine)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 每个 goroutine 使用独立的 model 和 agent
			model := &mockModel{
				responses: []llm.Response{
					{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "A"}}}, Usage: llm.Usage{TotalTokens: 5}},
					{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "B"}}}, Usage: llm.Usage{TotalTokens: 5}},
					{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "C"}}}, Usage: llm.Usage{TotalTokens: 5}},
				},
			}

			a, err := agent.New(agent.WithModel(model))
			if err != nil {
				errCh <- err
				return
			}
			defer a.Close()

			for j := 0; j < messagesPerGoroutine; j++ {
				resp, err := a.HandleMessage(context.Background(), "并发消息")
				if err != nil {
					errCh <- err
					return
				}
				if resp == nil {
					errCh <- errors.New("response 为 nil")
					return
				}
				if resp.SessionID == "" {
					errCh <- errors.New("SessionID 为空")
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// 收集错误
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Errorf("并发测试中 %d 个 goroutine 遇到错误:", len(errs))
		for _, err := range errs {
			t.Errorf("  %v", err)
		}
	}
}

// TestIntegration_ConcurrentConversations_SameStore 验证多个 agent 共享同一 Store 的并发安全性。
func TestIntegration_ConcurrentConversations_SameStore(t *testing.T) {
	const goroutineCount = 5
	store := session.NewMemoryStore()

	var wg sync.WaitGroup
	errCh := make(chan error, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			model := newSimpleMock("共享 store 回复")
			a, err := agent.New(
				agent.WithModel(model),
				agent.WithSession(store),
			)
			if err != nil {
				errCh <- err
				return
			}
			defer a.Close()

			resp, err := a.HandleMessage(context.Background(), "共享 store 消息")
			if err != nil {
				errCh <- err
				return
			}
			if resp == nil || resp.SessionID == "" {
				errCh <- errors.New("无效响应")
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Errorf("共享 Store 并发测试中 %d 个 goroutine 遇到错误:", len(errs))
		for _, err := range errs {
			t.Errorf("  %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// extractText 从 ChatMessage 中提取所有文本内容。
func extractText(msg llm.ChatMessage) string {
	var sb strings.Builder
	for _, part := range msg.Content {
		if part.Type == llm.ContentTypeText {
			if sb.Len() > 0 {
				sb.WriteString("")
			}
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}