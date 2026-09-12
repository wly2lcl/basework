package compaction

import "github.com/wly2lcl/basework/pkg/llm"

// Result 是一次压缩的产物。
type Result struct {
	// Messages 是压缩后的消息列表。
	Messages []llm.ChatMessage
	// Summary 是被压缩掉的旧消息的摘要文本。
	//
	// 为空表示本次压缩未产出摘要——例如 sliding_window 直接丢弃旧消息。
	// 非空时调用方**必须**把它持久化（写进 Compacted 事件）：请求是由事件
	// 日志投影出来的，摘要只留在内存里等于没生成，"压缩"会退化成"静默丢消息"，
	// 模型不知道被丢掉了什么。
	Summary string
}

// Strategy 是压缩策略接口
type Strategy interface {
	// Compact 对消息列表执行压缩，目标 token 数为 targetTokens
	Compact(messages []llm.ChatMessage, targetTokens int) (Result, error)

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
