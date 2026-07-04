// Package provider — OpenCode Zen Provider
// OpenCode Zen 使用 OpenAI 兼容协议，可复用 compatModel 的 HTTP 客户端逻辑

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 默认 OpenCode Zen 端点
const defaultOpenCodeBaseURL = "https://opencode.ai/zen/v1"

// 免费模型列表
var openCodeFreeModels = []string{
	"big-pickle",
	"deepseek-v4-flash-free",
	"mimo-v2.5-free",
}

// OpenCodeProvider 实现 Provider 接口，封装 OpenCode Zen API
type OpenCodeProvider struct {
	model *compatModel
	apiKey string
	modelID string
}

// NewOpenCodeProvider 创建新的 OpenCode Zen Provider
func NewOpenCodeProvider(apiKey, modelID string, opts map[string]any) (*OpenCodeProvider, error) {
	if apiKey == "" {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: "OpenCode Zen API key is required",
		}
	}
	if modelID == "" {
		modelID = "big-pickle"
	}

	baseURL := defaultOpenCodeBaseURL
	if opts != nil {
		if v, ok := opts["baseURL"].(string); ok && v != "" {
			baseURL = v
		}
	}

	model, err := newOpenAICompat(Config{
		Type:    "opencode",
		APIKey:  apiKey,
		ModelID: modelID,
		Options: opts,
	}, baseURL)
	if err != nil {
		return nil, fmt.Errorf("创建 OpenCode 模型失败: %w", err)
	}

	return &OpenCodeProvider{
		model:   model.(*compatModel),
		apiKey:  apiKey,
		modelID: modelID,
	}, nil
}

// Name 返回 Provider 名称
func (p *OpenCodeProvider) Name() string {
	return "opencode"
}

// Chat 发送非流式聊天请求
func (p *OpenCodeProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.model.Generate(ctx, req)
}

// ChatStream 发送流式聊天请求
func (p *OpenCodeProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.model.Stream(ctx, req)
}

// Models 返回支持的模型列表
func (p *OpenCodeProvider) Models() []string {
	return openCodeFreeModels
}

// ID 返回模型标识（实现 llm.Model）
func (p *OpenCodeProvider) ID() string {
	return p.modelID
}

// Supports 检查能力支持（实现 llm.Model）
func (p *OpenCodeProvider) Supports(cap llm.Capability) bool {
	return p.model.Supports(cap)
}

// Generate 非流式生成（实现 llm.Model）
func (p *OpenCodeProvider) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.Chat(ctx, req)
}

// Stream 流式生成（实现 llm.Model）
func (p *OpenCodeProvider) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.ChatStream(ctx, req)
}

// newOpenCode 是 factory.Create 使用的构造函数（返回 llm.Model）
func newOpenCode(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	if modelID == "" {
		modelID = "big-pickle"
	}
	if baseURL == "" {
		baseURL = defaultOpenCodeBaseURL
	}
	return newOpenAICompat(Config{
		Type:    "opencode",
		APIKey:  apiKey,
		ModelID: modelID,
		Options: opts,
	}, strings.TrimRight(baseURL, "/"))
}

// openCodeModel 结构体 — 嵌入 compatModel 以实现 llm.Model
type openCodeModel struct {
	*compatModel
	apiKey  string
	modelID string
}

// 确保编译时验证接口实现
var _ Provider = (*OpenCodeProvider)(nil)
var _ llm.Model = (*openCodeModel)(nil)
var _ time.Duration // 避免未使用导入