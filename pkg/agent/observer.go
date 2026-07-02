package agent

import (
	"context"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Observer 是 agent 运行时观测接口，用于记录 agent 的完整执行过程
type Observer interface {
	OnAgentStart(ctx context.Context, input string)
	OnAgentEnd(ctx context.Context, resp *Response, err error)
	OnLLMRequest(ctx context.Context, req *llm.Request)
	OnLLMResponse(ctx context.Context, resp *llm.Response, err error)
	OnToolExecution(ctx context.Context, call llm.ToolCall, result *tool.Result, err error)
}

// observerHook 将 Observer 适配为 hook.Hook，插入到 hook 链中
type observerHook struct {
	observer Observer
}

func (h *observerHook) BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	if h.observer != nil {
		h.observer.OnLLMRequest(context.Background(), &llm.Request{Messages: messages})
	}
	return messages, nil
}

func (h *observerHook) AfterLLM(resp *llm.Response, err error) {
	if h.observer != nil {
		h.observer.OnLLMResponse(context.Background(), resp, err)
	}
}

func (h *observerHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
	return &call, nil
}

func (h *observerHook) AfterTool(call llm.ToolCall, result *tool.Result, err error) {
	if h.observer != nil {
		h.observer.OnToolExecution(context.Background(), call, result, err)
	}
}