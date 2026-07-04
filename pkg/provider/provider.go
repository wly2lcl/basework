// Package provider 提供 LLM Provider 的统一抽象和实现
package provider

import (
	"context"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Provider 是 LLM 提供商的高级抽象接口
type Provider interface {
	// Name 返回 Provider 的名称标识
	Name() string

	// Chat 发送非流式聊天请求并返回完整响应
	Chat(ctx context.Context, req *llm.Request) (*llm.Response, error)

	// ChatStream 发送流式聊天请求并返回响应事件通道
	ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error)

	// Models 返回该 Provider 支持的模型列表
	Models() []string
}

// ModelProvider 是同时实现了 Provider 和 llm.Model 的组合接口
type ModelProvider interface {
	Provider
	llm.Model
}