package compaction

import "github.com/wly2lcl/basework/pkg/llm"

// EstimateTokens 基于字符数估算 token 数（1 token ≈ 4 字符）
func EstimateTokens(text string) int {
	tokens := len(text) / 4
	if len(text)%4 != 0 {
		tokens++
	}
	return tokens
}

// EstimateConversationTokens 估算整个对话的 token 数
func EstimateConversationTokens(messages []llm.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			total += EstimateTokens(part.Text)
		}
	}
	return total
}