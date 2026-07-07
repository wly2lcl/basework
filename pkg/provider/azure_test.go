package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestAzure_NewProvider 验证创建 Azure Provider
func TestAzure_NewProvider(t *testing.T) {
	p, err := NewAzureProvider(AzureConfigOpts{
		Resource:   "my-resource",
		Deployment: "gpt-4",
		APIKey:     "test-key",
	})
	if err != nil {
		t.Fatalf("NewAzureProvider failed: %v", err)
	}
	if p.Name() != "azure" {
		t.Errorf("expected name 'azure', got '%s'", p.Name())
	}
	if p.ID() != "gpt-4" {
		t.Errorf("expected ID 'gpt-4', got '%s'", p.ID())
	}
}

// TestAzure_NewProvider_Validation 验证必填字段
func TestAzure_NewProvider_Validation(t *testing.T) {
	tests := []struct {
		name string
		cfg  AzureConfigOpts
	}{
		{"empty resource", AzureConfigOpts{Resource: "", Deployment: "d", APIKey: "k"}},
		{"empty deployment", AzureConfigOpts{Resource: "r", Deployment: "", APIKey: "k"}},
		{"empty api key", AzureConfigOpts{Resource: "r", Deployment: "d", APIKey: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAzureProvider(tt.cfg)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

// TestAzure_EndpointURL 验证端点 URL 构造
func TestAzure_EndpointURL(t *testing.T) {
	p, err := NewAzureProvider(AzureConfigOpts{
		Resource:   "my-resource",
		Deployment: "gpt-4",
		APIKey:     "test-key",
		APIVersion: "2024-03-01",
	})
	if err != nil {
		t.Fatalf("NewAzureProvider failed: %v", err)
	}
	expected := "https://my-resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-03-01"
	if p.endpointURL != expected {
		t.Errorf("expected '%s', got '%s'", expected, p.endpointURL)
	}
}

// TestAzure_EndpointURL_DefaultVersion 验证默认 API 版本
func TestAzure_EndpointURL_DefaultVersion(t *testing.T) {
	p, err := NewAzureProvider(AzureConfigOpts{
		Resource:   "my-resource",
		Deployment: "gpt-4",
		APIKey:     "test-key",
	})
	if err != nil {
		t.Fatalf("NewAzureProvider failed: %v", err)
	}
	if !strings.Contains(p.endpointURL, "api-version=2024-02-01") {
		t.Errorf("expected default api-version, got '%s'", p.endpointURL)
	}
}

// TestAzure_Generate 验证请求构造和认证头
func TestAzure_Generate(t *testing.T) {
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("api-key")
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		json.Unmarshal(body, &reqMap)
		if reqMap["stream"] != false {
			t.Errorf("expected stream=false")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123456,
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello from Azure!"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`)
	}))
	defer srv.Close()

	p, err := NewAzureProvider(AzureConfigOpts{
		Resource:   "test",
		Deployment: "gpt-4",
		APIKey:     "azure-key-123",
	})
	if err != nil {
		t.Fatalf("NewAzureProvider failed: %v", err)
	}
	// 覆盖 client 以使用测试服务器
	p.client = srv.Client()
	// 覆盖 endpoint URL 到测试服务器
	p.endpointURL = srv.URL

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if capturedKey != "azure-key-123" {
		t.Errorf("expected api-key header 'azure-key-123', got '%s'", capturedKey)
	}
	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "Hello from Azure!" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
}

// TestAzure_Stream 验证流式请求
func TestAzure_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "azure-key-123" {
			t.Errorf("expected api-key header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" Azure\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p, err := NewAzureProvider(AzureConfigOpts{
		Resource:   "test",
		Deployment: "gpt-4",
		APIKey:     "azure-key-123",
	})
	if err != nil {
		t.Fatalf("NewAzureProvider failed: %v", err)
	}
	p.client = srv.Client()
	p.endpointURL = srv.URL

	events, err := p.ChatStream(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var texts []string
	gotDone := false
	for evt := range events {
		switch evt.Type {
		case llm.StreamEventText:
			texts = append(texts, evt.Delta)
		case llm.StreamEventDone:
			gotDone = true
		}
	}

	fullText := strings.Join(texts, "")
	if fullText != "Hello Azure" {
		t.Errorf("expected 'Hello Azure', got '%s'", fullText)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// TestAzure_Models 验证模型列表
func TestAzure_Models(t *testing.T) {
	p, _ := NewAzureProvider(AzureConfigOpts{
		Resource:   "r",
		Deployment: "gpt-4",
		APIKey:     "k",
	})
	models := p.Models()
	if len(models) != 1 || models[0] != "gpt-4" {
		t.Errorf("expected ['gpt-4'], got %v", models)
	}
}

// TestAzure_Supports 验证能力支持
func TestAzure_Supports(t *testing.T) {
	p, _ := NewAzureProvider(AzureConfigOpts{
		Resource:   "r",
		Deployment: "gpt-4",
		APIKey:     "k",
	})
	if !p.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if !p.Supports(llm.CapStreaming) {
		t.Error("expected streaming support")
	}
}
