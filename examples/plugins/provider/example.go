//go:build example_provider

// Package main 展示如何实现一个 Provider 插件
package main

import (
	"context"
	"fmt"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/provider"
)

// ExampleProvider 示例 Provider
type ExampleProvider struct{}

func (p *ExampleProvider) ID() string { return "example" }
func (p *ExampleProvider) Create(cfg provider.Config) (llm.Model, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("example provider requires an API key")
	}
	return &ExampleModel{apiKey: cfg.APIKey, model: cfg.ModelID}, nil
}

// ExampleModel 示例 Model 实现
type ExampleModel struct {
	apiKey string
	model  string
}

func (m *ExampleModel) ID() string {
	return m.model
}

func (m *ExampleModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	// 示例实现：直接返回一个固定响应
	return &llm.Response{
		Message: llm.ChatMessage{
			Role: llm.RoleAssistant,
			Content: []llm.ContentPart{
				{Type: llm.ContentTypeText, Text: "这是来自 Example Provider 的响应"},
			},
		},
		Usage: llm.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		FinishReason: "stop",
	}, nil
}

func (m *ExampleModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		ch <- llm.StreamEvent{
			Type:  "content",
			Delta: "这是来自 Example Provider 的流式响应",
		}
		ch <- llm.StreamEvent{
			Type: llm.StreamEventDone,
		}
	}()
	return ch, nil
}

func (m *ExampleModel) Supports(cap llm.Capability) bool {
	switch cap {
	case llm.CapTools, llm.CapStreaming:
		return true
	default:
		return false
	}
}

func init() {
	provider.RegisterPlugin(&ExampleProvider{})
}

func main() {
	// 列出所有已注册的插件
	plugins := provider.ListPlugins()
	fmt.Println("已注册的 Provider 插件:", plugins)
}
