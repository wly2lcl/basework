package hook

import (
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// 工具函数：快速构造消息
func makeMsg(role llm.Role, content string) llm.ChatMessage {
	return llm.ChatMessage{
		Role:    role,
		Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: content}},
	}
}

// TestSingleHookBeforeLLM 测试单 Hook 修改消息
func TestSingleHookBeforeLLM(t *testing.T) {
	chain := NewChain()
	chain.Add(&FuncHook{
		BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
			msgs = append(msgs, makeMsg(llm.RoleUser, "appended"))
			return msgs, nil
		},
	})

	msgs := []llm.ChatMessage{makeMsg(llm.RoleUser, "hello")}
	result, err := chain.RunBeforeLLM(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Content[0].Text != "appended" {
		t.Errorf("expected 'appended', got '%s'", result[1].Content[0].Text)
	}
}

// TestMultiHookBeforeLLMChain 测试多 Hook 链式传递
func TestMultiHookBeforeLLMChain(t *testing.T) {
	chain := NewChain()
	chain.Add(
		&FuncHook{
			BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
				return append(msgs, makeMsg(llm.RoleUser, "hook1")), nil
			},
		},
		&FuncHook{
			BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
				return append(msgs, makeMsg(llm.RoleUser, "hook2")), nil
			},
		},
	)

	msgs := []llm.ChatMessage{makeMsg(llm.RoleUser, "start")}
	result, err := chain.RunBeforeLLM(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[1].Content[0].Text != "hook1" || result[2].Content[0].Text != "hook2" {
		t.Errorf("chain order incorrect: got %s, %s", result[1].Content[0].Text, result[2].Content[0].Text)
	}
}

// TestAfterLLMReverseOrder 测试 AfterLLM 逆序执行
func TestAfterLLMReverseOrder(t *testing.T) {
	var order []string

	chain := NewChain()
	chain.Add(
		&FuncHook{
			AfterLLMFn: func(resp *llm.Response, err error) {
				order = append(order, "hook1")
			},
		},
		&FuncHook{
			AfterLLMFn: func(resp *llm.Response, err error) {
				order = append(order, "hook2")
			},
		},
		&FuncHook{
			AfterLLMFn: func(resp *llm.Response, err error) {
				order = append(order, "hook3")
			},
		},
	)

	chain.RunAfterLLM(nil, nil)
	if len(order) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(order))
	}
	// 逆序：hook3 先执行，hook1 最后执行
	if order[0] != "hook3" || order[1] != "hook2" || order[2] != "hook1" {
		t.Errorf("expected reverse order [hook3, hook2, hook1], got %v", order)
	}
}

// TestBeforeToolErrorAbort 测试 BeforeTool 链中某 Hook 返回 error 时后续 Hook 不被调用
func TestBeforeToolErrorAbort(t *testing.T) {
	var calledAfter bool

	chain := NewChain()
	chain.Add(
		&FuncHook{
			BeforeToolFn: func(call llm.ToolCall) (*llm.ToolCall, error) {
				return nil, errors.New("abort")
			},
		},
		&FuncHook{
			BeforeToolFn: func(call llm.ToolCall) (*llm.ToolCall, error) {
				calledAfter = true
				return &call, nil
			},
		},
	)

	_, err := chain.RunBeforeTool(llm.ToolCall{Name: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calledAfter {
		t.Error("second hook should not have been called")
	}
}

// TestAfterToolReverseOrder 测试 AfterTool 逆序执行
func TestAfterToolReverseOrder(t *testing.T) {
	var order []string

	chain := NewChain()
	chain.Add(
		&FuncHook{
			AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
				order = append(order, "hook1")
			},
		},
		&FuncHook{
			AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
				order = append(order, "hook2")
			},
		},
	)

	chain.RunAfterTool(llm.ToolCall{}, nil, nil)
	if len(order) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(order))
	}
	if order[0] != "hook2" || order[1] != "hook1" {
		t.Errorf("expected reverse order [hook2, hook1], got %v", order)
	}
}
