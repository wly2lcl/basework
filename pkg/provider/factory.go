package provider

import (
	"fmt"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Config 是 provider 创建配置
type Config struct {
	Type    string // "openai", "anthropic", "gemini", "openai-compat", 或兼容 provider 名称
	APIKey  string
	BaseURL string // 可选，自定义端点
	ModelID string
	Options map[string]any // provider 特定选项
}

// protocolMeta 存储每个 provider 的默认配置
type protocolMeta struct {
	defaultBaseURL string
	allowEmptyKey  bool
}

// protocols 是已知 provider 的元数据映射
var protocols = map[string]protocolMeta{
	"openai":        {defaultBaseURL: "https://api.openai.com/v1", allowEmptyKey: false},
	"anthropic":     {defaultBaseURL: "https://api.anthropic.com", allowEmptyKey: false},
	"gemini":        {defaultBaseURL: "https://generativelanguage.googleapis.com", allowEmptyKey: false},
	"openai-compat": {defaultBaseURL: "", allowEmptyKey: false},
	// 常见的 OpenAI 兼容 provider
	"deepseek":   {defaultBaseURL: "https://api.deepseek.com/v1", allowEmptyKey: false},
	"groq":       {defaultBaseURL: "https://api.groq.com/openai/v1", allowEmptyKey: false},
	"together":   {defaultBaseURL: "https://api.together.xyz/v1", allowEmptyKey: false},
	"openrouter": {defaultBaseURL: "https://openrouter.ai/api/v1", allowEmptyKey: false},
	"xai":        {defaultBaseURL: "https://api.x.ai/v1", allowEmptyKey: false},
	"mistral":    {defaultBaseURL: "https://api.mistral.ai/v1", allowEmptyKey: false},
	// Phase 20 新增 Provider
	"opencode": {defaultBaseURL: "https://opencode.ai/zen/v1", allowEmptyKey: false},
	"bedrock":  {defaultBaseURL: "", allowEmptyKey: true},
	"azure":    {defaultBaseURL: "", allowEmptyKey: false},
	"copilot":  {defaultBaseURL: "", allowEmptyKey: true},
	"ollama":   {defaultBaseURL: "http://localhost:11434", allowEmptyKey: true},
}

// compatProviders 列出所有已知走 OpenAI-compatible 路径的 provider 类型
var compatProviders = map[string]bool{
	"openai-compat": true,
	"deepseek":      true,
	"groq":          true,
	"together":      true,
	"openrouter":    true,
	"xai":           true,
	"mistral":       true,
	"opencode":      true,
	"ollama":        true,
}

// Create 根据配置创建 Model
func Create(cfg Config) (llm.Model, error) {
	// 查找元数据
	meta, known := protocols[cfg.Type]

	// 确定 BaseURL
	baseURL := cfg.BaseURL
	if baseURL == "" && known {
		baseURL = meta.defaultBaseURL
	}

	// 验证 API key
	if cfg.APIKey == "" && known && !meta.allowEmptyKey {
		return nil, fmt.Errorf("provider %q requires an API key", cfg.Type)
	}

	// 4 分支 switch
	switch cfg.Type {
	case "anthropic":
		return newAnthropic(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "gemini":
		return newGemini(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "openai":
		return newOpenAI(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "bedrock":
		return newBedrock(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "azure":
		return newAzure(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "copilot":
		return newCopilot(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "opencode":
		return newOpenCode(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	case "ollama":
		return newOllama(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
	default:
		// 所有其他类型走 OpenAI-compatible 路径
		return newOpenAICompat(cfg, baseURL)
	}
}
