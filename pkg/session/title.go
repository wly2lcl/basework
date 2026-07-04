package session

import (
	"context"
	"fmt"
	"strings"
)

// LLMClient 是用于生成标题的 LLM 客户端接口。
type LLMClient interface {
	// Generate 发送请求并返回完整响应。
	Generate(ctx context.Context, req interface{}) (interface{}, error)
}

// simpleTitleClient 是简化版的 LLM 客户端适配，用于标题生成。
// 实际使用时需要包装 pkg/llm.Client。
type simpleTitleClient struct {
	generate func(ctx context.Context, prompt string) (string, error)
}

func (c *simpleTitleClient) Generate(ctx context.Context, req interface{}) (interface{}, error) {
	return nil, fmt.Errorf("simpleTitleClient 不支持直接调用 Generate")
}

// GenerateTitle 使用 LLM 客户端根据第一条用户消息生成会话标题。
//
// 参数：
//   - ctx: 上下文
//   - client: LLM 客户端（实现了 Generate 方法）
//   - firstMessage: 第一条用户消息的文本内容
//   - titleModel: 用于生成标题的模型名（可选，为空时使用 client 默认模型）
//
// 返回：
//   - 生成的标题（10-20 字）
//   - 如果 LLM 调用失败，回退到使用前 50 个字符作为标题
func GenerateTitle(ctx context.Context, client *simpleTitleClient, firstMessage string) (string, error) {
	if client == nil || client.generate == nil {
		return fallbackTitle(firstMessage), nil
	}

	prompt := fmt.Sprintf("用一句话总结这个对话的主题（10-20字）：%s", firstMessage)

	title, err := client.generate(ctx, prompt)
	if err != nil {
		return fallbackTitle(firstMessage), nil
	}

	title = strings.TrimSpace(title)
	title = strings.Trim(title, `"'「」『』【】《》`)
	if title == "" {
		return fallbackTitle(firstMessage), nil
	}

	return title, nil
}

// fallbackTitle 在 LLM 调用失败时使用前 50 个字符作为标题。
func fallbackTitle(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 50 {
		// 按 UTF-8 字符截断，避免截断多字节字符
		runes := []rune(msg)
		if len(runes) > 50 {
			return string(runes[:50]) + "..."
		}
		return msg
	}
	return msg
}

// NewSimpleTitleClient 创建一个简单的标题生成客户端。
// generate 函数接收 context 和 prompt，返回生成的文本。
func NewSimpleTitleClient(generate func(ctx context.Context, prompt string) (string, error)) *simpleTitleClient {
	return &simpleTitleClient{generate: generate}
}