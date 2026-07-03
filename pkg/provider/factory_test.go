package provider

import (
	"strings"
	"testing"
)

// TestCreate_OpenAI 验证 type "openai" 返回 *openAIModel
func TestCreate_OpenAI(t *testing.T) {
	model, err := Create(Config{
		Type:    "openai",
		APIKey:  "sk-test",
		ModelID: "gpt-4",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, ok := model.(*openAIModel); !ok {
		t.Fatalf("expected *openAIModel, got %T", model)
	}
	if model.ID() != "gpt-4" {
		t.Errorf("expected ID 'gpt-4', got '%s'", model.ID())
	}
}

// TestCreate_Anthropic 验证 type "anthropic" 返回 *anthropicModel
func TestCreate_Anthropic(t *testing.T) {
	model, err := Create(Config{
		Type:    "anthropic",
		APIKey:  "sk-ant-test",
		ModelID: "claude-sonnet-4-20250514",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, ok := model.(*anthropicModel); !ok {
		t.Fatalf("expected *anthropicModel, got %T", model)
	}
}

// TestCreate_Gemini 验证 type "gemini" 返回 *geminiModel
func TestCreate_Gemini(t *testing.T) {
	model, err := Create(Config{
		Type:    "gemini",
		APIKey:  "gemini-key",
		ModelID: "gemini-2.0-flash",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, ok := model.(*geminiModel); !ok {
		t.Fatalf("expected *geminiModel, got %T", model)
	}
}

// TestCreate_Compat_DeepSeek 验证 type "deepseek" 返回 *compatModel
func TestCreate_Compat_DeepSeek(t *testing.T) {
	model, err := Create(Config{
		Type:    "deepseek",
		APIKey:  "ds-key",
		ModelID: "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	cm, ok := model.(*compatModel)
	if !ok {
		t.Fatalf("expected *compatModel, got %T", model)
	}
	if cm.baseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected baseURL 'https://api.deepseek.com/v1', got '%s'", cm.baseURL)
	}
	if cm.modelID != "deepseek-chat" {
		t.Errorf("expected modelID 'deepseek-chat', got '%s'", cm.modelID)
	}
}

// TestCreate_Compat_Groq 验证 type "groq" 返回 *compatModel
func TestCreate_Compat_Groq(t *testing.T) {
	model, err := Create(Config{
		Type:    "groq",
		APIKey:  "groq-key",
		ModelID: "mixtral-8x7b",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	cm, ok := model.(*compatModel)
	if !ok {
		t.Fatalf("expected *compatModel, got %T", model)
	}
	if cm.baseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("expected baseURL 'https://api.groq.com/openai/v1', got '%s'", cm.baseURL)
	}
}

// TestCreate_UnknownType 未知类型但提供 BaseURL → 返回 *compatModel
func TestCreate_UnknownType(t *testing.T) {
	model, err := Create(Config{
		Type:    "my-custom",
		APIKey:  "key",
		ModelID: "custom-model",
		BaseURL: "https://custom.example.com/v1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, ok := model.(*compatModel); !ok {
		t.Fatalf("expected *compatModel, got %T", model)
	}
}

// TestCreate_UnknownType_NoBaseURL 未知类型且没有 BaseURL → 错误
func TestCreate_UnknownType_NoBaseURL(t *testing.T) {
	_, err := Create(Config{
		Type:    "my-custom",
		APIKey:  "key",
		ModelID: "custom-model",
	})
	if err == nil {
		t.Fatal("expected error for unknown type without BaseURL, got nil")
	}
	if !strings.Contains(err.Error(), "requires a BaseURL") {
		t.Errorf("expected error about BaseURL, got: %v", err)
	}
}

// TestCreate_MissingAPIKey openai 没有 API key → 错误
func TestCreate_MissingAPIKey(t *testing.T) {
	_, err := Create(Config{
		Type:    "openai",
		ModelID: "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
	if !strings.Contains(err.Error(), "requires an API key") {
		t.Errorf("expected error about API key, got: %v", err)
	}
}

// TestCreate_CustomBaseURL 覆盖默认 BaseURL
func TestCreate_CustomBaseURL(t *testing.T) {
	model, err := Create(Config{
		Type:    "openai",
		APIKey:  "sk-test",
		ModelID: "gpt-4",
		BaseURL: "https://custom.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	om, ok := model.(*openAIModel)
	if !ok {
		t.Fatalf("expected *openAIModel, got %T", model)
	}
	if om.baseURL != "https://custom.openai.com/v1" {
		t.Errorf("expected baseURL 'https://custom.openai.com/v1', got '%s'", om.baseURL)
	}
}

// TestCreate_DefaultModelID 空的 ModelID 被透传（provider 自己处理）
func TestCreate_DefaultModelID(t *testing.T) {
	model, err := Create(Config{
		Type:   "openai",
		APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if model.ID() != "" {
		t.Errorf("expected empty ModelID, got '%s'", model.ID())
	}
}

// TestCreate_OpenAICompat_Type 验证 type "openai-compat" 返回 *compatModel
func TestCreate_OpenAICompat_Type(t *testing.T) {
	model, err := Create(Config{
		Type:    "openai-compat",
		APIKey:  "key",
		ModelID: "model",
		BaseURL: "https://example.com/v1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, ok := model.(*compatModel); !ok {
		t.Fatalf("expected *compatModel, got %T", model)
	}
}

// TestCreate_OpenAICompat_NoBaseURL "openai-compat" 没有 BaseURL → 错误
func TestCreate_OpenAICompat_NoBaseURL(t *testing.T) {
	_, err := Create(Config{
		Type:    "openai-compat",
		APIKey:  "key",
		ModelID: "model",
	})
	if err == nil {
		t.Fatal("expected error for openai-compat without BaseURL, got nil")
	}
}

// TestCreate_Error 验证 Create 的 llm.Error 类型
func TestCreate_Error(t *testing.T) {
	_, err := Create(Config{
		Type: "openai",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires an API key") {
		t.Errorf("unexpected error: %v", err)
	}
}
