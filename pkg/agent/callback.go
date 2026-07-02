package agent

import (
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Callback 是 agent 生命周期回调接口，用于流式输出和事件通知
type Callback interface {
	OnTextDelta(delta string)
	OnToolCallStart(call llm.ToolCall)
	OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error)
	OnThinkingDelta(delta string)
	OnTurnEnd(resp *Response)
	OnError(err error)
}

// NopCallback 是 Callback 的空实现，所有方法均为 no-op
type NopCallback struct{}

func (NopCallback) OnTextDelta(string)          {}
func (NopCallback) OnToolCallStart(llm.ToolCall) {}
func (NopCallback) OnToolCallEnd(llm.ToolCall, *tool.Result, error) {}
func (NopCallback) OnThinkingDelta(string)       {}
func (NopCallback) OnTurnEnd(*Response)          {}
func (NopCallback) OnError(error)                {}

// CallbackFuncs 是函数式的 Callback 实现，未设置的字段自动回退到无操作
type CallbackFuncs struct {
	TextDelta     func(string)
	ToolCallStart func(llm.ToolCall)
	ToolCallEnd   func(llm.ToolCall, *tool.Result, error)
	ThinkingDelta func(string)
	TurnEnd       func(*Response)
	Error         func(error)
}

func (f *CallbackFuncs) OnTextDelta(delta string) {
	if f.TextDelta != nil {
		f.TextDelta(delta)
	}
}

func (f *CallbackFuncs) OnToolCallStart(call llm.ToolCall) {
	if f.ToolCallStart != nil {
		f.ToolCallStart(call)
	}
}

func (f *CallbackFuncs) OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error) {
	if f.ToolCallEnd != nil {
		f.ToolCallEnd(call, result, err)
	}
}

func (f *CallbackFuncs) OnThinkingDelta(delta string) {
	if f.ThinkingDelta != nil {
		f.ThinkingDelta(delta)
	}
}

func (f *CallbackFuncs) OnTurnEnd(resp *Response) {
	if f.TurnEnd != nil {
		f.TurnEnd(resp)
	}
}

func (f *CallbackFuncs) OnError(err error) {
	if f.Error != nil {
		f.Error(err)
	}
}