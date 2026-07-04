// Package provider — Provider 注册中心
// Registry 管理所有 Provider 实例，支持按名称和模型查找

package provider

import (
	"fmt"
	"sync"
)

// Registry 管理所有 Provider 实例
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register 注册 Provider
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get 根据名称获取 Provider
func (r *Registry) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[name]
}

// List 返回所有已注册的 Provider 名称列表
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// GetByModel 根据模型名查找可提供该模型的 Provider
func (r *Registry) GetByModel(model string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.providers {
		for _, m := range p.Models() {
			if m == model {
				return p
			}
		}
	}
	return nil
}

// DefaultRegistry 返回默认注册中心，包含所有内置 Provider
// 注意：需要注入 API key 等配置
func DefaultRegistry() *Registry {
	r := NewRegistry()
	// 注册 OpenCode Zen（需 API key）
	// 注册 Bedrock（需 AWS 凭证）
	// 注册 Azure（需资源名和 API key）
	// 注册 Copilot（需 OAuth 认证）
	// 注册 Ollama（无需认证，本地端点）
	// 注意：这些 provider 需要配置后才完全可用
	return r
}

// MustRegister 注册 Provider，如果名称冲突则 panic
func (r *Registry) MustRegister(p Provider) {
	if existing := r.Get(p.Name()); existing != nil {
		panic(fmt.Sprintf("provider %q already registered", p.Name()))
	}
	r.Register(p)
}

// NewConfiguredRegistry 根据配置创建并注册所有可用的 Provider
func NewConfiguredRegistry(opts map[string]any) *Registry {
	r := NewRegistry()

	// 尝试创建并注册每个 Provider
	// OpenCode
	if apiKey, ok := opts["opencode_api_key"].(string); ok && apiKey != "" {
		model, _ := opts["opencode_model"].(string)
		if p, err := NewOpenCodeProvider(apiKey, model, nil); err == nil {
			r.Register(p)
		}
	}

	// Bedrock
	if accessKey, ok := opts["bedrock_access_key"].(string); ok && accessKey != "" {
		secretKey, _ := opts["bedrock_secret_key"].(string)
		region, _ := opts["bedrock_region"].(string)
		model, _ := opts["bedrock_model"].(string)
		if p, err := NewBedrockProvider(BedrockConfigOpts{
			AccessKey: accessKey,
			SecretKey: secretKey,
			Region:    region,
			Model:     model,
		}); err == nil {
			r.Register(p)
		}
	}

	// Azure
	if resource, ok := opts["azure_resource"].(string); ok && resource != "" {
		deployment, _ := opts["azure_deployment"].(string)
		apiKey, _ := opts["azure_api_key"].(string)
		apiVersion, _ := opts["azure_api_version"].(string)
		if p, err := NewAzureProvider(AzureConfigOpts{
			Resource:   resource,
			Deployment: deployment,
			APIKey:     apiKey,
			APIVersion: apiVersion,
		}); err == nil {
			r.Register(p)
		}
	}

	// Copilot
	if tokenPath, ok := opts["copilot_token_path"].(string); ok {
		model, _ := opts["copilot_model"].(string)
		if p, err := NewCopilotProvider(CopilotConfigOpts{
			Model:     model,
			TokenPath: tokenPath,
		}); err == nil {
			r.Register(p)
		}
	}

	// Ollama — 始终注册（无需认证）
	endpoint, _ := opts["ollama_endpoint"].(string)
	model, _ := opts["ollama_model"].(string)
	if p, err := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: endpoint,
		Model:    model,
	}); err == nil {
		r.Register(p)
	}

	return r
}