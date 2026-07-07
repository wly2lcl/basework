package agent

import (
	"context"
	"errors"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// ErrMaxStepsExceeded 表示 agent 执行步数超过最大限制
var ErrMaxStepsExceeded = errors.New("agent: 超过最大执行步数")

// Agent 是核心 agent 接口，封装了 LLM 对话处理生命周期
type Agent interface {
	// HandleMessage 处理单条文本输入
	HandleMessage(ctx context.Context, input string) (*Response, error)

	// HandleMessages 处理预构建的多条消息（支持多模态）
	HandleMessages(ctx context.Context, messages []llm.ChatMessage) (*Response, error)

	// Tools 返回已注册的工具列表
	Tools() []tool.Tool

	// Close 释放 agent 资源
	Close() error
}

// Response 是 agent 响应，包含最终消息、工具调用记录和用量
type Response struct {
	Message   llm.ChatMessage
	ToolCalls []ToolCallRecord
	Usage     llm.Usage
	SessionID string
}

// ToolCallRecord 记录一次工具调用的完整生命周期
type ToolCallRecord struct {
	Call   llm.ToolCall
	Result *tool.Result
	Err    error
}

// New 创建 agent，应用提供的选项进行配置
func New(opts ...Option) (Agent, error) {
	cfg := &config{
		maxSteps:         25,
		maxContextTokens: 128000,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.model == nil {
		return nil, errors.New("agent: 必须提供 model（使用 WithModel）")
	}

	// 创建 tool registry（如果提供了 tools 但没有 registry）
	if cfg.registry == nil {
		cfg.registry = tool.NewRegistry()
		for _, t := range cfg.tools {
			_ = cfg.registry.Register(t)
		}
	}

	// 如果提供了工具工厂，调用它注册额外工具
	if cfg.toolFactory != nil {
		cfg.toolFactory(cfg.registry)
	}

	// 创建 hook chain
	chain := hook.NewChain()
	for _, h := range cfg.hooks {
		chain.Add(h)
	}

	// 如果提供了 observer，包一层 hook
	if cfg.observer != nil {
		chain.Add(&observerHook{observer: cfg.observer})
	}

	return newAgentLoop(cfg, chain)
}