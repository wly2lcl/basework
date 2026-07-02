package hook

import (
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Hook 是生命周期钩子接口
type Hook interface {
	BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error)
	AfterLLM(resp *llm.Response, err error)
	BeforeTool(call llm.ToolCall) (*llm.ToolCall, error)
	AfterTool(call llm.ToolCall, result *tool.Result, err error)
}

// NopHook 空实现，嵌入用
type NopHook struct{}

func (NopHook) BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	return messages, nil
}

func (NopHook) AfterLLM(resp *llm.Response, err error) {}

func (NopHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
	return &call, nil
}

func (NopHook) AfterTool(call llm.ToolCall, result *tool.Result, err error) {}

// FuncHook 函数式 hook，便捷构造
type FuncHook struct {
	BeforeLLMFn  func([]llm.ChatMessage) ([]llm.ChatMessage, error)
	AfterLLMFn   func(*llm.Response, error)
	BeforeToolFn func(llm.ToolCall) (*llm.ToolCall, error)
	AfterToolFn  func(llm.ToolCall, *tool.Result, error)
}

func (h *FuncHook) BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	if h.BeforeLLMFn != nil {
		return h.BeforeLLMFn(messages)
	}
	return messages, nil
}

func (h *FuncHook) AfterLLM(resp *llm.Response, err error) {
	if h.AfterLLMFn != nil {
		h.AfterLLMFn(resp, err)
	}
}

func (h *FuncHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
	if h.BeforeToolFn != nil {
		return h.BeforeToolFn(call)
	}
	return &call, nil
}

func (h *FuncHook) AfterTool(call llm.ToolCall, result *tool.Result, err error) {
	if h.AfterToolFn != nil {
		h.AfterToolFn(call, result, err)
	}
}