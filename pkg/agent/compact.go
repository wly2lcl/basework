package agent

import (
	"context"
	"fmt"

	"github.com/wly2lcl/basework/pkg/llm"
)

// CompactStrategy 压缩策略
type CompactStrategy string

const (
	// StrategySummary 使用 LLM 生成摘要进行压缩
	StrategySummary CompactStrategy = "summary"
	// StrategyTruncate 直接截断旧消息
	StrategyTruncate CompactStrategy = "truncate"
)

// CompactConfig 压缩配置
type CompactConfig struct {
	// TriggerThreshold token 占比阈值（默认 0.8）
	TriggerThreshold float64
	// KeepRecent 保留最近消息数（默认 10）
	KeepRecent int
	// Strategy 压缩策略
	Strategy CompactStrategy
	// MaxTokens 模型最大 token 数（用于计算占比）
	MaxTokens int
}

// ShouldCompact 检查是否触发压缩
// 当 usage.TotalTokens / cfg.MaxTokens > cfg.TriggerThreshold 时返回 true
func ShouldCompact(usage llm.Usage, cfg CompactConfig) bool {
	if cfg.MaxTokens <= 0 {
		return false
	}
	threshold := cfg.TriggerThreshold
	if threshold <= 0 {
		threshold = 0.8
	}
	return float64(usage.TotalTokens)/float64(cfg.MaxTokens) > threshold
}

// CompactSummary 使用 LLM 生成对话摘要
// 构造 prompt 让 LLM 总结对话，调用 model.Generate
func CompactSummary(ctx context.Context, model llm.Model, messages []llm.ChatMessage) (string, error) {
	if model == nil {
		return "", fmt.Errorf("compact: model 不能为 nil")
	}

	// 将消息序列化为文本
	var conversationText string
	for _, msg := range messages {
		role := string(msg.Role)
		text := ""
		for _, part := range msg.Content {
			if part.Type == llm.ContentTypeText {
				text += part.Text
			}
		}
		if text != "" {
			conversationText += fmt.Sprintf("%s: %s\n", role, text)
		}
		for _, tc := range msg.ToolCalls {
			conversationText += fmt.Sprintf("assistant (tool call %s): %s(%s)\n", tc.ID, tc.Name, tc.ArgsJSON)
		}
	}

	prompt := fmt.Sprintf(`请总结以下对话的核心内容，保留重要的上下文信息，以便后续继续对话：

%s

请用简洁的语言总结这段对话。`, conversationText)

	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: prompt}},
			},
		},
	}

	resp, err := model.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compact: LLM 摘要生成失败: %w", err)
	}

	// 提取文本
	summary := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			summary += part.Text
		}
	}
	return summary, nil
}

// TruncateMessages 截断消息，只保留最近 n 条
func TruncateMessages(messages []llm.ChatMessage, keepRecent int) []llm.ChatMessage {
	if keepRecent <= 0 {
		keepRecent = 10
	}
	if len(messages) <= keepRecent {
		return messages
	}
	// 保留最后 keepRecent 条
	truncated := make([]llm.ChatMessage, keepRecent)
	copy(truncated, messages[len(messages)-keepRecent:])
	return truncated
}