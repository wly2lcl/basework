package compaction

import "github.com/wly2lcl/basework/pkg/llm"

// SlidingWindowStrategy 滑动窗口压缩策略。
// 保留最近 N 条非系统消息，丢弃较早的消息，系统消息始终保留。
type SlidingWindowStrategy struct {
	WindowSize int
}

// NewSlidingWindowStrategy 创建滑动窗口策略，默认窗口大小为 10
func NewSlidingWindowStrategy(windowSize int) *SlidingWindowStrategy {
	if windowSize <= 0 {
		windowSize = 10
	}
	return &SlidingWindowStrategy{WindowSize: windowSize}
}

// Name 返回策略名称
func (s *SlidingWindowStrategy) Name() string {
	return "sliding_window"
}

// Compact 执行滑动窗口压缩
func (s *SlidingWindowStrategy) Compact(messages []llm.ChatMessage, targetTokens int) ([]llm.ChatMessage, error) {
	sysMsgs := preserveSystemMessages(messages)
	nonSys := filterNonSystem(messages)

	// 如果消息数量未超过窗口大小，无需压缩
	if len(nonSys) <= s.WindowSize {
		return messages, nil
	}

	// 保留最近 N 条非系统消息
	start := len(nonSys) - s.WindowSize
	kept := nonSys[start:]

	// 合并系统消息和保留的消息
	result := make([]llm.ChatMessage, 0, len(sysMsgs)+len(kept))
	result = append(result, sysMsgs...)
	result = append(result, kept...)

	return result, nil
}
