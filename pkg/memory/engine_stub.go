//go:build !memory

package memory

import (
	"context"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Engine 是记忆引擎的桩实现（无 memory 构建标签时使用）。
// 所有方法均为空操作，不做任何记忆处理。
type Engine struct{}

// NewEngine 创建一个新的记忆引擎桩。
func NewEngine() *Engine {
	return &Engine{}
}

// Compress 压缩对话历史。桩实现：返回原始消息不变。
func (e *Engine) Compress(_ context.Context, messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	return messages, nil
}

// Retrieve 检索相关记忆。桩实现：返回空切片。
func (e *Engine) Retrieve(_ context.Context, _ []llm.ChatMessage, _ int) ([]Entry, error) {
	return []Entry{}, nil
}

// Assemble 将记忆组装到提示中。桩实现：返回原始消息不变。
func (e *Engine) Assemble(_ string, _ []Entry, messages []llm.ChatMessage) []llm.ChatMessage {
	return messages
}