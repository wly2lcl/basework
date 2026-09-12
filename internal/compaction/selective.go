package compaction

import "github.com/wly2lcl/basework/pkg/llm"

// SelectiveStrategy 选择性压缩策略。
// 保留高优先级消息（system, user），压缩低优先级消息（assistant）。
type SelectiveStrategy struct {
	// AssistantKeepCount 保留的助手消息数量，超出部分被丢弃
	AssistantKeepCount int
}

// NewSelectiveStrategy 创建选择性压缩策略，默认保留最近 5 条助手消息
func NewSelectiveStrategy(assistantKeepCount int) *SelectiveStrategy {
	if assistantKeepCount <= 0 {
		assistantKeepCount = 5
	}
	return &SelectiveStrategy{AssistantKeepCount: assistantKeepCount}
}

// Name 返回策略名称
func (s *SelectiveStrategy) Name() string {
	return "selective"
}

// Compact 执行选择性压缩。
// 该策略按角色裁剪，不产出摘要，因此 Result.Summary 恒为空。
func (s *SelectiveStrategy) Compact(messages []llm.ChatMessage, targetTokens int) (Result, error) {
	var result []llm.ChatMessage
	var assistantMsgs []llm.ChatMessage

	// 分类消息：系统消息和用户消息直接保留，助手消息收集后裁剪
	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			result = append(result, msg)
		case llm.RoleUser:
			result = append(result, msg)
		case llm.RoleAssistant:
			assistantMsgs = append(assistantMsgs, msg)
		default:
			// 其他角色（如 tool）也保留
			result = append(result, msg)
		}
	}

	// 只保留最近的 N 条助手消息
	if len(assistantMsgs) > s.AssistantKeepCount {
		assistantMsgs = assistantMsgs[len(assistantMsgs)-s.AssistantKeepCount:]
	}
	result = append(result, assistantMsgs...)

	return Result{Messages: result}, nil
}
