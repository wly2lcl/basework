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

// TestOllama_NewProvider 验证创建 Ollama Provider
func TestOllama_NewProvider(t *testing.T) {
	p, err := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: "http://localhost:11434",
		Model:    "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider failed: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got '%s'", p.Name())
	}
	if p.ID() != "llama3" {
		t.Errorf("expected ID 'llama3', got '%s'", p.ID())
	}
}

// TestOllama_DefaultEndpoint 验证默认端点
func TestOllama_DefaultEndpoint(t *testing.T) {
	p, err := NewOllamaProvider(OllamaConfigOpts{Model: "llama3"})
	if err != nil {
		t.Fatalf("NewOllamaProvider failed: %v", err)
	}
	expected := "http://localhost:11434/v1/chat/completions"
	if p.chatURL() != expected {
		t.Errorf("expected '%s', got '%s'", expected, p.chatURL())
	}
}

// TestOllama_DiscoverModels 验证模型发现
func TestOllama_DiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			fmt.Fprint(w, `{
				"models": [
					{"name": "llama3:latest", "modified_at": "2024-01-01"},
					{"name": "mistral:latest", "modified_at": "2024-01-01"},
					{"name": "codellama:latest", "modified_at": "2024-01-01"}
				]
			}`)
		}
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: srv.URL,
		Model:    "llama3",
	})

	models, err := p.DiscoverModels(t.Context())
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(models), models)
	}
	if models[0] != "llama3:latest" {
		t.Errorf("expected 'llama3:latest', got '%s'", models[0])
	}
}

// TestOllama_DiscoverModels_Error 验证模型发现失败
func TestOllama_DiscoverModels_Error(t *testing.T) {
	p, _ := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: "http://localhost:1",
		Model:    "llama3",
	})

	_, err := p.DiscoverModels(t.Context())
	if err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}

// TestOllama_Generate_NoAuth 验证无需认证
func TestOllama_Generate_NoAuth(t *testing.T) {
	var authHeader, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		json.Unmarshal(body, &reqMap)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123,
			"model": "llama3",
			"choices": [{"index":0,"message":{"role":"assistant","content":"Hello from Ollama!"},"finish_reason":"stop"}],
			"usage": {"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`)
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: srv.URL,
		Model:    "llama3",
	})

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if authHeader != "" {
		t.Errorf("expected no Authorization header, got '%s'", authHeader)
	}
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "Hello from Ollama!" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
}

// TestOllama_Stream 验证流式请求
func TestOllama_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" Ollama\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(OllamaConfigOpts{
		Endpoint: srv.URL,
		Model:    "llama3",
	})

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
	if fullText != "Hello Ollama" {
		t.Errorf("expected 'Hello Ollama', got '%s'", fullText)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// TestOllama_Models 验证模型列表
func TestOllama_Models(t *testing.T) {
	p, _ := NewOllamaProvider(OllamaConfigOpts{Model: "llama3"})
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
}

// TestOllama_Supports 验证能力支持
func TestOllama_Supports(t *testing.T) {
	p, _ := NewOllamaProvider(OllamaConfigOpts{Model: "llama3"})
	if !p.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if !p.Supports(llm.CapStreaming) {
		t.Error("expected streaming support")
	}
}

// TestOllama_NoModel 验证未指定模型也能工作
func TestOllama_NoModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123,
			"model": "llama3",
			"choices": [{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}]
		}`)
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(OllamaConfigOpts{Endpoint: srv.URL})
	p.models = []string{"llama3"} // 模拟已发现的模型

	_, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
}
