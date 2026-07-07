package loopdetect

import (
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// CheckRepeatedContent 检测连续 assistant 消息中是否存在重复内容。
// 比较 assistant 消息的文本内容，忽略空白差异。
// 返回 (是否循环, 连续重复次数)。
func CheckRepeatedContent(messages []llm.ChatMessage, threshold int) (bool, int) {
	if threshold <= 0 {
		threshold = 3 // 默认阈值
	}

	var assistantContents []string
	for _, msg := range messages {
		if msg.Role == llm.RoleAssistant {
			// 提取文本内容并规范化空白
			content := normalizeContent(msg.Content)
			assistantContents = append(assistantContents, content)
		}
	}

	if len(assistantContents) < threshold {
		return false, 0
	}

	// 从后向前检查连续重复
	repeatCount := 1
	for i := len(assistantContents) - 1; i > 0; i-- {
		if assistantContents[i] == assistantContents[i-1] {
			repeatCount++
			if repeatCount >= threshold {
				return true, repeatCount
			}
		} else {
			repeatCount = 1
		}
	}

	return false, repeatCount
}

// normalizeContent 从 ContentPart 列表中提取文本并规范化空白。
func normalizeContent(parts []llm.ContentPart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Type == llm.ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	// 规范化空白：去除首尾空白，合并连续空白为单个空格
	text := strings.TrimSpace(b.String())
	words := strings.Fields(text)
	return strings.Join(words, " ")
}
