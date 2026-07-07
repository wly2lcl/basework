package compaction

import "github.com/wly2lcl/basework/pkg/llm"

// Strategy 是压缩策略接口
type Strategy interface {
	// Compact 对消息列表执行压缩，目标 token 数为 targetTokens
	Compact(messages []llm.ChatMessage, targetTokens int) ([]llm.ChatMessage, error)

	// Name 返回策略名称
	Name() string
}

// preserveSystemMessages 从消息列表中提取系统消息
func preserveSystemMessages(messages []llm.ChatMessage) []llm.ChatMessage {
	var sysMsgs []llm.ChatMessage
	for _, msg := range messages {
		if msg.Role == llm.RoleSystem {
			sysMsgs = append(sysMsgs, msg)
		}
	}
	return sysMsgs
}

// filterNonSystem 返回非系统消息
func filterNonSystem(messages []llm.ChatMessage) []llm.ChatMessage {
	var result []llm.ChatMessage
	for _, msg := range messages {
		if msg.Role != llm.RoleSystem {
			result = append(result, msg)
		}
	}
	return result
}

// extractText 从消息中提取纯文本内容
func extractText(msg llm.ChatMessage) string {
	var text string
	for _, part := range msg.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	return text
}
