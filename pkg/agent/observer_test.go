package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// testObserver 测试用 Observer 实现
type testObserver struct {
	mu              sync.Mutex
	agentStartCalls []string
	agentEndCalls   int
	llmReqCalls     int
	llmRespCalls    int
	toolExecCalls   int
}

func (o *testObserver) OnAgentStart(_ context.Context, input string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentStartCalls = append(o.agentStartCalls, input)
}

func (o *testObserver) OnAgentEnd(_ context.Context, _ *Response, _ error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentEndCalls++
}

func (o *testObserver) OnLLMRequest(_ context.Context, _ *llm.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.llmReqCalls++
}

func (o *testObserver) OnLLMResponse(_ context.Context, _ *llm.Response, _ error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.llmRespCalls++
}

func (o *testObserver) OnToolExecution(_ context.Context, _ llm.ToolCall, _ *tool.Result, _ error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.toolExecCalls++
}

func TestObserver_OnAgentStartEnd(t *testing.T) {
	obs := &testObserver{}

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "回复"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model), WithObserver(obs))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "测试")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	obs.mu.Lock()
	startCalls := len(obs.agentStartCalls)
	endCalls := obs.agentEndCalls
	obs.mu.Unlock()

	if startCalls != 1 {
		t.Errorf("期望 OnAgentStart 被调用 1 次, 得到 %d", startCalls)
	}
	if endCalls != 1 {
		t.Errorf("期望 OnAgentEnd 被调用 1 次, 得到 %d", endCalls)
	}
}

func TestObserver_OnLLMRequestResponse(t *testing.T) {
	obs := &testObserver{}

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model), WithObserver(obs))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "你好")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	obs.mu.Lock()
	reqCalls := obs.llmReqCalls
	respCalls := obs.llmRespCalls
	obs.mu.Unlock()

	if reqCalls != 1 {
		t.Errorf("期望 OnLLMRequest 被调用 1 次, 得到 %d", reqCalls)
	}
	if respCalls != 1 {
		t.Errorf("期望 OnLLMResponse 被调用 1 次, 得到 %d", respCalls)
	}
}

func TestObserver_OnToolExecution(t *testing.T) {
	obs := &testObserver{}

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "完成"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithTools(&mockTool{name: "test_tool"}),
		WithObserver(obs),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "执行工具")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	obs.mu.Lock()
	toolCalls := obs.toolExecCalls
	obs.mu.Unlock()

	if toolCalls != 1 {
		t.Errorf("期望 OnToolExecution 被调用 1 次, 得到 %d", toolCalls)
	}
}

func TestObserver_WithObserverInjection(t *testing.T) {
	// 验证 WithObserver 注入后，Observer 方法通过 Hook 链被调用
	obs := &testObserver{}

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "测试"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model), WithObserver(obs))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	// 验证 observer 被正确存储到 config
	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}
	if loop.observer == nil {
		t.Error("期望 observer 不为 nil")
	}

	// 执行 HandleMessage 触发 Observer 调用
	_, err = a.HandleMessage(context.Background(), "测试输入")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	obs.mu.Lock()
	startInput := ""
	if len(obs.agentStartCalls) > 0 {
		startInput = obs.agentStartCalls[0]
	}
	obs.mu.Unlock()

	if startInput != "测试输入" {
		t.Errorf("期望 OnAgentStart 输入='测试输入', 得到 '%s'", startInput)
	}
}

func TestObserver_NopObserver(t *testing.T) {
	// 不设置 observer，不应 panic
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "ok"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "test")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
}