// Package provider — Ollama Provider
// Ollama 使用 OpenAI 兼容协议，无需认证
// 默认端点：http://localhost:11434
// 自动发现本地模型：调用 /api/tags 获取模型列表

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

// 默认 Ollama 端点
const defaultOllamaEndpoint = "http://localhost:11434"

// OllamaProvider 实现 Provider 接口，封装 Ollama API
type OllamaProvider struct {
	endpoint string
	modelID  string
	client   *http.Client
	models   []string
}

// OllamaConfigOpts 是 Ollama Provider 的配置选项
type OllamaConfigOpts struct {
	Endpoint string
	Model    string
}

// NewOllamaProvider 创建新的 Ollama Provider
func NewOllamaProvider(cfg OllamaConfigOpts) (*OllamaProvider, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultOllamaEndpoint
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")

	return &OllamaProvider{
		endpoint: cfg.Endpoint,
		modelID:  cfg.Model,
		client:   newHTTPClient(10 * time.Minute),
	}, nil
}

// chatURL 返回聊天端点 URL
func (p *OllamaProvider) chatURL() string {
	return p.endpoint + "/v1/chat/completions"
}

// Name 返回 Provider 名称
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// DiscoverModels 自动发现本地 Ollama 模型列表
// 调用 /api/tags 端点获取可用模型
func (p *OllamaProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	url := p.endpoint + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Ollama 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}

	var models []string
	for _, m := range tagsResp.Models {
		models = append(models, m.Name)
	}
	p.models = models
	return models, nil
}

// Chat 发送非流式聊天请求
func (p *OllamaProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	modelID := p.modelID
	if modelID == "" {
		// 使用第一个可用模型
		if len(p.models) > 0 {
			modelID = p.models[0]
		} else {
			return nil, &llm.Error{
				Type:    llm.ErrorTypeInternal,
				Message: "Ollama 未指定模型且未发现可用模型",
			}
		}
	}

	body := buildOpenAIRequest(req, false, modelID, false)
	url := p.chatURL()

	respBody, statusCode, err := jsonRequest(ctx, p.client, url, nil, body)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("Ollama request failed: %v", err),
			StatusCode: 0,
		}
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "ollama")
	}

	return fromOpenAIResponse(respBody)
}

// ChatStream 发送流式聊天请求
func (p *OllamaProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	modelID := p.modelID
	if modelID == "" {
		if len(p.models) > 0 {
			modelID = p.models[0]
		} else {
			return nil, &llm.Error{
				Type:    llm.ErrorTypeInternal,
				Message: "Ollama 未指定模型且未发现可用模型",
			}
		}
	}

	body := buildOpenAIRequest(req, true, modelID, false)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := p.chatURL()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
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
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "ollama")
	}

	return parseOpenAIStream(ctx, resp), nil
}

// Models 返回本地发现的模型列表
func (p *OllamaProvider) Models() []string {
	if len(p.models) > 0 {
		return p.models
	}
	// 如果未发现模型，返回默认值
	return []string{"llama3", "mistral", "codellama"}
}

// ID 返回模型标识（实现 llm.Model）
func (p *OllamaProvider) ID() string {
	if p.modelID != "" {
		return p.modelID
	}
	if len(p.models) > 0 {
		return p.models[0]
	}
	return "llama3"
}

// Supports 检查能力支持（实现 llm.Model）
func (p *OllamaProvider) Supports(cap llm.Capability) bool {
	switch cap {
	case llm.CapTools, llm.CapStreaming:
		return true
	default:
		return false
	}
}

// Generate 非流式生成（实现 llm.Model）
func (p *OllamaProvider) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.Chat(ctx, req)
}

// Stream 流式生成（实现 llm.Model）
func (p *OllamaProvider) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.ChatStream(ctx, req)
}

// newOllama 是 factory.Create 使用的构造函数（返回 llm.Model）
func newOllama(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	endpoint := defaultOllamaEndpoint
	if baseURL != "" {
		endpoint = baseURL
	}
	if opts != nil {
		if v, ok := opts["endpoint"].(string); ok && v != "" {
			endpoint = v
		}
	}

	p, err := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: endpoint,
		Model:    modelID,
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

// 编译时验证
var _ ModelProvider = (*OllamaProvider)(nil)
