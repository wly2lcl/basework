package compaction

import (
	"context"
	"fmt"
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Summarizer 是摘要生成器接口，用于压缩旧消息
type Summarizer interface {
	// Summarize 生成文本摘要
	Summarize(ctx context.Context, text string) (string, error)
}

// LLMSummarizer 使用 LLM Model 实现摘要生成
type LLMSummarizer struct {
	model llm.Model
}

// NewLLMSummarizer 创建 LLM 摘要生成器
func NewLLMSummarizer(model llm.Model) *LLMSummarizer {
	return &LLMSummarizer{model: model}
}

// Summarize 调用 LLM 生成摘要
func (s *LLMSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	sysPrompt := "你是一个对话摘要助手。请总结以下对话的关键信息，保持简洁但完整，保留所有重要细节和决策。"

	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{
				Role:    llm.RoleSystem,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: sysPrompt}},
			},
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: text}},
			},
		},
	}

	resp, err := s.model.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("摘要生成失败: %w", err)
	}

	var summary string
	for _, part := range resp.Message.Content {
		summary += part.Text
	}
	return summary, nil
}

// SummarizationStrategy 使用 LLM 摘要压缩旧消息。
// 将较旧的消息合并为一条摘要，保留系统消息和较新的消息。
type SummarizationStrategy struct {
	summarizer Summarizer
	// 触发摘要的最小消息数
	MinMessages int
	// 保留的最近消息比例（0.0-1.0），默认 0.5
	KeepRatio float64
}

// NewSummarizationStrategy 创建摘要压缩策略
func NewSummarizationStrategy(summarizer Summarizer) *SummarizationStrategy {
	return &SummarizationStrategy{
		summarizer:  summarizer,
		MinMessages: 5,
		KeepRatio:   0.5,
	}
}

// Name 返回策略名称
func (s *SummarizationStrategy) Name() string {
	return "summarization"
}

// Compact 执行摘要压缩
func (s *SummarizationStrategy) Compact(messages []llm.ChatMessage, targetTokens int) ([]llm.ChatMessage, error) {
	sysMsgs := preserveSystemMessages(messages)
	nonSys := filterNonSystem(messages)

	// 如果消息太少，不需要压缩
	if len(nonSys) < s.MinMessages {
		return messages, nil
	}

	// 确定分割点：保留后半部分，摘要前半部分
	splitPoint := int(float64(len(nonSys)) * s.KeepRatio)
	if splitPoint < 1 {
		splitPoint = 1
	}
	toSummarize := nonSys[:splitPoint]
	toKeep := nonSys[splitPoint:]

	// 构建摘要文本
	var sb strings.Builder
	for _, msg := range toSummarize {
		content := extractText(msg)
		if content != "" {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
		}
	}

	summary, err := s.summarizer.Summarize(context.Background(), sb.String())
	if err != nil {
		return nil, fmt.Errorf("压缩摘要生成失败: %w", err)
	}

	// 用摘要消息替换旧消息
	summaryMsg := llm.ChatMessage{
		Role:    llm.RoleUser,
		Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "之前的对话摘要: " + summary}},
	}

	result := make([]llm.ChatMessage, 0, len(sysMsgs)+1+len(toKeep))
	result = append(result, sysMsgs...)
	result = append(result, summaryMsg)
	result = append(result, toKeep...)

	return result, nil
}