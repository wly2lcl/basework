// Package provider — Azure OpenAI Provider
// Azure OpenAI 使用 api-key 请求头认证，端点格式为：
// https://{resource}.openai.azure.com/openai/deployments/{deployment}

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 默认 Azure API 版本
const defaultAzureAPIVersion = "2024-02-01"

// AzureProvider 实现 Provider 接口，封装 Azure OpenAI API
type AzureProvider struct {
	resource    string
	deployment  string
	apiKey      string
	apiVersion  string
	modelID     string
	client      *http.Client
	endpointURL string // 完整端点 URL（可被测试覆盖）
}

// AzureConfig 是 Azure Provider 的配置选项
type AzureConfigOpts struct {
	Resource   string
	Deployment string
	APIKey     string
	APIVersion string
	Model      string
}

// NewAzureProvider 创建新的 Azure OpenAI Provider
func NewAzureProvider(cfg AzureConfigOpts) (*AzureProvider, error) {
	if cfg.Resource == "" {
		return nil, fmt.Errorf("Azure resource name is required")
	}
	if cfg.Deployment == "" {
		return nil, fmt.Errorf("Azure deployment name is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Azure API key is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = defaultAzureAPIVersion
	}
	if cfg.Model == "" {
		cfg.Model = cfg.Deployment
	}

	return &AzureProvider{
		resource:   cfg.Resource,
		deployment: cfg.Deployment,
		apiKey:     cfg.APIKey,
		apiVersion: cfg.APIVersion,
		modelID:    cfg.Model,
		client:     newHTTPClient(10 * time.Minute),
		endpointURL: fmt.Sprintf("https://%s.openai.azure.com/openai/deployments/%s/chat/completions?api-version=%s",
			cfg.Resource, cfg.Deployment, cfg.APIVersion),
	}, nil
}

// Name 返回 Provider 名称
func (p *AzureProvider) Name() string {
	return "azure"
}

// Chat 发送非流式聊天请求
func (p *AzureProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := buildOpenAIRequest(req, false, p.modelID, false)
	headers := map[string]string{
		"api-key": p.apiKey,
	}

	url := p.endpointURL
	respBody, statusCode, err := jsonRequest(ctx, p.client, url, headers, body)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("Azure request failed: %v", err),
			StatusCode: 0,
		}
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "azure")
	}

	return fromOpenAIResponse(respBody)
}

// ChatStream 发送流式聊天请求
func (p *AzureProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	body := buildOpenAIRequest(req, true, p.modelID, false)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := p.endpointURL
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "azure")
	}

	return parseOpenAIStream(ctx, resp), nil
}

// Models 返回支持的模型列表（Azure 使用部署名）
func (p *AzureProvider) Models() []string {
	return []string{p.deployment}
}

// ID 返回模型标识（实现 llm.Model）
func (p *AzureProvider) ID() string {
	return p.modelID
}

// Supports 检查能力支持（实现 llm.Model）
func (p *AzureProvider) Supports(cap llm.Capability) bool {
	switch cap {
	case llm.CapTools, llm.CapVision, llm.CapStreaming:
		return true
	default:
		return false
	}
}

// Generate 非流式生成（实现 llm.Model）
func (p *AzureProvider) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.Chat(ctx, req)
}

// Stream 流式生成（实现 llm.Model）
func (p *AzureProvider) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.ChatStream(ctx, req)
}

// newAzure 是 factory.Create 使用的构造函数（返回 llm.Model）
func newAzure(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	resource := ""
	deployment := ""
	apiVersion := defaultAzureAPIVersion

	if opts != nil {
		if v, ok := opts["resource"].(string); ok {
			resource = v
		}
		if v, ok := opts["deployment"].(string); ok {
			deployment = v
		}
		if v, ok := opts["api_version"].(string); ok {
			apiVersion = v
		}
	}

	// 从 baseURL 提取 resource
	if resource == "" && baseURL != "" {
		// https://{resource}.openai.azure.com
		parts := strings.Split(baseURL, ".")
		if len(parts) >= 2 {
			resource = strings.TrimPrefix(parts[0], "https://")
		}
	}
	if resource == "" {
		return nil, fmt.Errorf("Azure resource name is required (set via options[\"resource\"] or baseURL)")
	}
	if deployment == "" {
		deployment = modelID
	}
	if deployment == "" {
		return nil, fmt.Errorf("Azure deployment name is required (set via options[\"deployment\"] or modelID)")
	}

	return NewAzureProvider(AzureConfigOpts{
		Resource:   resource,
		Deployment: deployment,
		APIKey:     apiKey,
		APIVersion: apiVersion,
		Model:      modelID,
	})
}

// 编译时验证
var _ ModelProvider = (*AzureProvider)(nil)
