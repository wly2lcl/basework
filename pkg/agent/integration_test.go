package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// TestIntegration_FullAgentLoop 端到端测试 — mock Model + real Session + real Tool + Hook 链
func TestIntegration_FullAgentLoop(t *testing.T) {
	// 1. 创建真实的 MemoryStore
	store := session.NewMemoryStore()

	// 2. 注册真实工具
	registry := tool.NewRegistry()
	registry.Register(&mockTool{
		name:        "calculator",
		description: "计算两个数之和",
		executeFunc: func(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "42"}, nil
		},
	})

	// 3. 创建 Hook 链（追踪调用）
	var hookCalls []string
	testHook := &hook.FuncHook{
		BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
			hookCalls = append(hookCalls, "BeforeLLM")
			return msgs, nil
		},
		AfterLLMFn: func(resp *llm.Response, err error) {
			hookCalls = append(hookCalls, "AfterLLM")
		},
		BeforeToolFn: func(call llm.ToolCall) (*llm.ToolCall, error) {
			hookCalls = append(hookCalls, "BeforeTool:"+call.Name)
			return &call, nil
		},
		AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
			hookCalls = append(hookCalls, "AfterTool:"+call.Name)
		},
	}

	// 4. Mock Model: 先返回 tool call，再返回文本
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "calculator", ArgsJSON: `{"a":20,"b":22}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "答案是 42"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28},
			},
		},
	}

	// 5. 构造 agent
	a, err := New(
		WithModel(model),
		WithToolRegistry(registry),
		WithSession(store),
		WithHook(testHook),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	// 6. 执行
	resp, err := a.HandleMessage(context.Background(), "20+22等于多少？")
	if err != nil {
		t.Fatalf("HandleMessage 失败: %v", err)
	}

	// 7. 验证响应
	if resp == nil {
		t.Fatal("期望非 nil 响应")
	}

	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "答案是 42" {
		t.Errorf("期望文本='答案是 42', 得到 '%s'", text)
	}

	if len(resp.ToolCalls) != 1 {
		t.Errorf("期望 1 条工具调用记录, 得到 %d", len(resp.ToolCalls))
	}

	if resp.Usage.TotalTokens != 28 {
		t.Errorf("期望 TotalTokens=28, 得到 %d", resp.Usage.TotalTokens)
	}

	// 8. 验证 Hook 调用顺序
	expectedHookOrder := []string{"BeforeLLM", "AfterLLM", "BeforeTool:calculator", "AfterTool:calculator", "BeforeLLM", "AfterLLM"}
	if len(hookCalls) != len(expectedHookOrder) {
		t.Errorf("期望 %d 次 Hook 调用, 得到 %d: %v", len(expectedHookOrder), len(hookCalls), hookCalls)
	} else {
		for i, expected := range expectedHookOrder {
			if hookCalls[i] != expected {
				t.Errorf("Hook 调用 #%d: 期望 '%s', 得到 '%s'", i, expected, hookCalls[i])
			}
		}
	}

	// 9. 验证 Session 中有事件（使用 agent 创建的 session ID）
	events, err := store.Events(session.EventFilter{SessionID: resp.SessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}
	if len(events) == 0 {
		t.Error("期望 session 中有事件")
	}

	// 验证有 Prompted 事件
	hasPrompted := false
	hasToolCalled := false
	hasTextDelta := false
	for _, e := range events {
		switch e.Type {
		case session.EventPrompted:
			hasPrompted = true
		case session.EventToolCalled:
			hasToolCalled = true
		case session.EventTextDelta:
			hasTextDelta = true
		}
	}
	if !hasPrompted {
		t.Error("期望有 Prompted 事件")
	}
	if !hasToolCalled {
		t.Error("期望有 ToolCalled 事件")
	}
	if !hasTextDelta {
		t.Error("期望有 TextDelta 事件")
	}
}

// TestIntegration_MultiTurnConversation 多轮对话测试
func TestIntegration_MultiTurnConversation(t *testing.T) {
	store := session.NewMemoryStore()

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第一轮回答"}},
				},
				Usage: llm.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第二轮回答"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
			},
		},
	}

	a, err := New(WithModel(model), WithSession(store))
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	// 第一轮
	resp1, err := a.HandleMessage(context.Background(), "问题一")
	if err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	text1 := resp1.Message.Content[0].Text
	if text1 != "第一轮回答" {
		t.Errorf("第一轮: 期望 '第一轮回答', 得到 '%s'", text1)
	}

	// 第二轮（model idx 自动递增）
	resp2, err := a.HandleMessage(context.Background(), "问题二")
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	text2 := resp2.Message.Content[0].Text
	if text2 != "第二轮回答" {
		t.Errorf("第二轮: 期望 '第二轮回答', 得到 '%s'", text2)
	}

	// 验证 session 中有多轮消息（使用 agent 创建的 session ID）
	msgs, err := store.Events(session.EventFilter{SessionID: resp2.SessionID})
	if err != nil {
		t.Fatalf("获取事件失败: %v", err)
	}

	// 统计事件类型
	userMsgs := 0
	for _, e := range msgs {
		switch e.Type {
		case session.EventPrompted:
			userMsgs++
		case session.EventTextDelta, session.EventTextEnded:
			// 这些事件表示 assistant 输出
		}
	}
	// 简单验证：有 Prompted 事件说明消息被记录
	if userMsgs < 2 {
		t.Errorf("期望至少 2 次 Prompted 事件, 得到 %d", userMsgs)
	}
}

// TestIntegration_ToolErrorIsolation 工具错误隔离测试
func TestIntegration_ToolErrorIsolation(t *testing.T) {
	registry := tool.NewRegistry()

	// 一个会失败的工具
	registry.Register(&mockTool{
		name: "failing_tool",
		executeFunc: func(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "工具执行失败", IsError: true}, nil
		},
	})

	// 一个成功的工具
	registry.Register(&mockTool{
		name: "success_tool",
		executeFunc: func(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
			return &tool.Result{Content: "成功"}, nil
		},
	})

	// Model: 同时调用两个工具，一个失败一个成功，然后返回最终文本
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "failing_tool", ArgsJSON: `{}`},
						{ID: "call_2", Name: "success_tool", ArgsJSON: `{}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "处理完成"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 5, TotalTokens: 25},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithToolRegistry(registry),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "同时执行两个工具")
	if err != nil {
		t.Fatalf("HandleMessage 失败: %v", err)
	}

	// 两个工具都应该被记录
	if len(resp.ToolCalls) != 2 {
		t.Errorf("期望 2 条工具调用记录, 得到 %d", len(resp.ToolCalls))
	}

	// 验证失败工具的记录
	for _, tc := range resp.ToolCalls {
		if tc.Call.Name == "failing_tool" {
			if tc.Result == nil || !tc.Result.IsError {
				t.Error("期望 failing_tool 的 IsError 为 true")
			}
		}
		if tc.Call.Name == "success_tool" {
			if tc.Result == nil || tc.Result.IsError {
				t.Error("期望 success_tool 的 IsError 为 false")
			}
		}
	}
}

// TestIntegration_SystemPrompt 系统提示测试
func TestIntegration_SystemPrompt(t *testing.T) {
	store := session.NewMemoryStore()
	var capturedMessages []llm.ChatMessage

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "好的"}},
				},
				Usage: llm.Usage{TotalTokens: 10},
			},
		},
	}

	// 使用 BeforeLLM Hook 捕获发送给 LLM 的 messages
	a, err := New(
		WithModel(model),
		WithSession(store),
		WithSystemPrompt("你是一个编程助手"),
		WithHook(&hook.FuncHook{
			BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
				capturedMessages = msgs
				return msgs, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "你好")
	if err != nil {
		t.Fatalf("HandleMessage 失败: %v", err)
	}

	// 验证第一条消息是 system prompt
	if len(capturedMessages) == 0 {
		t.Fatal("期望捕获到 messages")
	}
	if capturedMessages[0].Role != llm.RoleSystem {
		t.Errorf("期望第一条消息 Role=system, 得到 %s", capturedMessages[0].Role)
	}
	if len(capturedMessages[0].Content) == 0 || !strings.Contains(capturedMessages[0].Content[0].Text, "编程助手") {
		t.Error("期望 system prompt 包含 '编程助手'")
	}
}

// TestIntegration_CallbackStreaming 流式回调测试
func TestIntegration_CallbackStreaming(t *testing.T) {
	var (
		textDeltas  []string
		toolStarted []string
		toolEnded   []string
		turnEnded   bool
	)

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`},
					},
				},
				Usage: llm.Usage{TotalTokens: 10},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "最终结果"}},
				},
				Usage: llm.Usage{TotalTokens: 20},
			},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(&mockTool{name: "test_tool"})

	a, err := New(
		WithModel(model),
		WithToolRegistry(registry),
		WithCallback(&CallbackFuncs{
			TextDelta: func(d string) {
				textDeltas = append(textDeltas, d)
			},
			ToolCallStart: func(call llm.ToolCall) {
				toolStarted = append(toolStarted, call.Name)
			},
			ToolCallEnd: func(call llm.ToolCall, result *tool.Result, err error) {
				toolEnded = append(toolEnded, call.Name)
			},
			TurnEnd: func(r *Response) {
				turnEnded = true
			},
		}),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "执行工具")
	if err != nil {
		t.Fatalf("HandleMessage 失败: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil 响应")
	}

	// 验证文本 delta
	if len(textDeltas) == 0 {
		t.Error("期望有文本 delta")
	}

	// 验证工具回调
	if len(toolStarted) != 1 || toolStarted[0] != "test_tool" {
		t.Errorf("期望 toolStarted=['test_tool'], 得到 %v", toolStarted)
	}
	if len(toolEnded) != 1 || toolEnded[0] != "test_tool" {
		t.Errorf("期望 toolEnded=['test_tool'], 得到 %v", toolEnded)
	}

	// 验证 turn 结束
	if !turnEnded {
		t.Error("期望 TurnEnd 被调用")
	}
}
