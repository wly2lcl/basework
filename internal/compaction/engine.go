package compaction

import (
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
)

// Ensure Engine implements agent.Compactor.
var _ agent.Compactor = (*Engine)(nil)

// Config 是上下文压缩模块的配置
type Config struct {
	// Enabled 是否启用压缩
	Enabled bool `json:"enabled"`
	// Strategy 压缩策略名称：sliding_window / summarization / selective
	Strategy string `json:"strategy"`
	// Threshold 自动触发阈值（0.0-1.0），默认 0.8
	Threshold float64 `json:"threshold"`
	// WindowSize 滑动窗口大小，默认 10
	WindowSize int `json:"window_size"`
}

// Engine 是自动压缩引擎，负责检测是否需要压缩并执行压缩
type Engine struct {
	config   Config
	strategy Strategy
}

// NewEngine 创建压缩引擎
func NewEngine(config Config, strategy Strategy) *Engine {
	if config.Threshold <= 0 || config.Threshold > 1.0 {
		config.Threshold = 0.8
	}
	if config.WindowSize <= 0 {
		config.WindowSize = 10
	}
	return &Engine{
		config:   config,
		strategy: strategy,
	}
}

// ShouldCompact 检查是否需要压缩。
// 当 token 使用率达到配置阈值时返回 true。
func (e *Engine) ShouldCompact(messages []llm.ChatMessage, maxTokens int) bool {
	if !e.config.Enabled {
		return false
	}
	if maxTokens <= 0 {
		return false
	}
	current := EstimateConversationTokens(messages)
	threshold := int(float64(maxTokens) * e.config.Threshold)
	return current >= threshold
}

// Compact 执行压缩。将当前消息压缩到约 60% 的大小。
func (e *Engine) Compact(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	if !e.config.Enabled {
		return messages, nil
	}
	current := EstimateConversationTokens(messages)
	// 目标 token 数：压缩到当前大小的 60%
	target := int(float64(current) * 0.6)
	if target < 1 {
		target = 1
	}
	return e.strategy.Compact(messages, target)
}
