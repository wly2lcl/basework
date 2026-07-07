package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestOpenCode_NewProvider 验证创建 OpenCode Provider
func TestOpenCode_NewProvider(t *testing.T) {
	p, err := NewOpenCodeProvider("test-key", "big-pickle", nil)
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}
	if p.Name() != "opencode" {
		t.Errorf("expected name 'opencode', got '%s'", p.Name())
	}
	if p.ID() != "big-pickle" {
		t.Errorf("expected ID 'big-pickle', got '%s'", p.ID())
	}
	if !p.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
}

// TestOpenCode_NewProvider_EmptyKey 验证空 API Key 报错
func TestOpenCode_NewProvider_EmptyKey(t *testing.T) {
	_, err := NewOpenCodeProvider("", "big-pickle", nil)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
}

// TestOpenCode_NewProvider_DefaultModel 验证默认模型
func TestOpenCode_NewProvider_DefaultModel(t *testing.T) {
	p, err := NewOpenCodeProvider("test-key", "", nil)
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}
	if p.ID() != "big-pickle" {
		t.Errorf("expected default model 'big-pickle', got '%s'", p.ID())
	}
}

// TestOpenCode_Models 验证模型列表
func TestOpenCode_Models(t *testing.T) {
	p, err := NewOpenCodeProvider("test-key", "big-pickle", nil)
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
	found := false
	for _, m := range models {
		if m == "big-pickle" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'big-pickle' in model list, got %v", models)
	}
}

// TestOpenCode_Generate 验证非流式请求构造和响应解析
func TestOpenCode_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		if err := json.Unmarshal(body, &reqMap); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		if reqMap["model"] != "big-pickle" {
			t.Errorf("expected model 'big-pickle', got %v", reqMap["model"])
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization header, got '%s'", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123456,
			"model": "big-pickle",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello from OpenCode!"
				},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`)
	}))
	defer srv.Close()

	p, err := NewOpenCodeProvider("test-key", "big-pickle", map[string]any{"baseURL": srv.URL})
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if len(resp.Message.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if resp.Message.Content[0].Text != "Hello from OpenCode!" {
		t.Errorf("unexpected content: %s", resp.Message.Content[0].Text)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

// TestOpenCode_Stream 验证流式请求
func TestOpenCode_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	p, err := NewOpenCodeProvider("test-key", "big-pickle", map[string]any{"baseURL": srv.URL})
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}

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
		if evt.Error != nil {
			t.Fatalf("unexpected error: %v", evt.Error)
		}
	}

	fullText := strings.Join(texts, "")
	if fullText != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", fullText)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// TestOpenCode_Error_Handling 验证错误处理
func TestOpenCode_Error_Handling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error": {"message": "invalid API key"}}`)
	}))
	defer srv.Close()

	p, err := NewOpenCodeProvider("test-key", "big-pickle", map[string]any{"baseURL": srv.URL})
	if err != nil {
		t.Fatalf("NewOpenCodeProvider failed: %v", err)
	}

	_, err = p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var llmErr *llm.Error
	if !errors.As(err, &llmErr) || llmErr.Type != llm.ErrorTypeAuth {
		t.Errorf("expected auth error, got %v", err)
	}
}
