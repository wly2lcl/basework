package hook

import (
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Chain 管理多个 Hook
type Chain struct {
	hooks []Hook
}

// NewChain 创建新的 Chain
func NewChain() *Chain {
	return &Chain{}
}

// Add 注册一个或多个 Hook
func (c *Chain) Add(hooks ...Hook) {
	c.hooks = append(c.hooks, hooks...)
}

// RunBeforeLLM 按注册顺序执行，前一个的输出作为后一个的输入
func (c *Chain) RunBeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	var err error
	for _, h := range c.hooks {
		messages, err = h.BeforeLLM(messages)
		if err != nil {
			return messages, err
		}
	}
	return messages, nil
}

// RunAfterLLM 按注册逆序执行
func (c *Chain) RunAfterLLM(resp *llm.Response, err error) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i].AfterLLM(resp, err)
	}
}

// RunBeforeTool 按注册顺序执行，某 Hook 返回 error 则立即中止
func (c *Chain) RunBeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
	for _, h := range c.hooks {
		callPtr, err := h.BeforeTool(call)
		if err != nil {
			return nil, err
		}
		if callPtr != nil {
			call = *callPtr
		}
	}
	return &call, nil
}

// RunAfterTool 按注册逆序执行
func (c *Chain) RunAfterTool(call llm.ToolCall, result *tool.Result, err error) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i].AfterTool(call, result, err)
	}
}
