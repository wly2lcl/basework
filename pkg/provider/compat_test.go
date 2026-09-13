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

// TestCompat_CustomBaseURL 验证请求发送到配置的 BaseURL
func TestCompat_CustomBaseURL(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":123,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	model, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "test-key", ModelID: "gpt-4"}, srv.URL)
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(capturedURL, "/chat/completions") {
		t.Errorf("expected URL to contain /chat/completions, got %s", capturedURL)
	}
}

// TestCompat_CustomHeaders 验证自定义头包含在请求中
func TestCompat_CustomHeaders(t *testing.T) {
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":123,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	model, err := newOpenAICompat(Config{
		Type:    "openai-compat",
		APIKey:  "test-key",
		ModelID: "gpt-4",
		Options: map[string]any{
			"headers": map[string]any{
				"X-Custom-Header": "custom-value",
				"X-Another":       "another-value",
			},
		},
	}, srv.URL)
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if capturedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got '%s'", capturedHeaders.Get("X-Custom-Header"))
	}
	if capturedHeaders.Get("X-Another") != "another-value" {
		t.Errorf("expected X-Another 'another-value', got '%s'", capturedHeaders.Get("X-Another"))
	}
	if capturedHeaders.Get("Authorization") != "Bearer test-key" {
		t.Errorf("expected Authorization 'Bearer test-key', got '%s'", capturedHeaders.Get("Authorization"))
	}
}

// TestCompat_DefaultBaseURL 使用 factory.Create 并指定 "deepseek" 类型，验证使用默认 URL
func TestCompat_DefaultBaseURL(t *testing.T) {
	// 使用一个测试服务器验证请求 URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":123,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	// 使用工厂创建，但用测试服务器 URL 覆盖
	model, err := Create(Config{
		Type:    "deepseek",
		APIKey:  "test-key",
		ModelID: "deepseek-chat",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 验证返回的是 compatModel
	if _, ok := model.(*compatModel); !ok {
		t.Fatalf("expected *compatModel, got %T", model)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

// TestCompat_UnknownType 未知类型且提供 BaseURL，回退到兼容路径
func TestCompat_UnknownType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":123,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	model, err := Create(Config{
		Type:    "my-custom-provider",
		APIKey:  "test-key",
		ModelID: "my-model",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("Create with unknown type failed: %v", err)
	}

	if _, ok := model.(*compatModel); !ok {
		t.Fatalf("expected *compatModel for unknown type, got %T", model)
	}

	resp, err := model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "ok" {
		t.Errorf("expected content 'ok', got '%v'", resp.Message.Content)
	}
}

// TestCompat_Generate mock 服务器返回 OpenAI 格式响应，验证解析
func TestCompat_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求 body
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		if err := json.Unmarshal(body, &reqMap); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		if reqMap["stream"] != false {
			t.Errorf("expected stream=false, got %v", reqMap["stream"])
		}
		if reqMap["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %v", reqMap["model"])
		}

		// 返回标准 OpenAI 格式响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123456,
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you?"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`)
	}))
	defer srv.Close()

	model, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "test-key", ModelID: "gpt-4"}, srv.URL)
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}

	resp, err := model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(resp.Message.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if resp.Message.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %s", resp.Message.Content[0].Text)
	}
	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("expected role assistant, got %s", resp.Message.Role)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %s", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total_tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

// TestCompat_NoBaseURL 兼容 provider 没有 baseURL 应返回错误
func TestCompat_NoBaseURL(t *testing.T) {
	_, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "test-key", ModelID: "gpt-4"}, "")
	if err == nil {
		t.Fatal("expected error for empty baseURL, got nil")
	}
	if !strings.Contains(err.Error(), "requires a BaseURL") {
		t.Errorf("expected error about BaseURL, got: %v", err)
	}
}

// TestCompat_Stream 验证流式请求正常工作
func TestCompat_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证流式请求
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		if err := json.Unmarshal(body, &reqMap); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		if reqMap["stream"] != true {
			t.Errorf("expected stream=true, got %v", reqMap["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
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

	model, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "test-key", ModelID: "gpt-4"}, srv.URL)
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}

	events, err := model.Stream(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
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

	expected := []string{"Hello", " world"}
	if len(texts) != len(expected) {
		t.Fatalf("expected %d text events, got %d: %v", len(expected), len(texts), texts)
	}
	for i, txt := range texts {
		if txt != expected[i] {
			t.Errorf("event %d: expected %q, got %q", i, expected[i], txt)
		}
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// TestCompat_ID 验证 ID() 方法
func TestCompat_ID(t *testing.T) {
	model, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "key", ModelID: "my-model"}, "https://example.com")
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}
	if model.ID() != "my-model" {
		t.Errorf("expected ID 'my-model', got '%s'", model.ID())
	}
}

// TestCompat_Supports 验证 Supports() 方法。
//
// 注意视觉能力的语义在 REL-002 之后收紧了：openai-compat 端点上的任意模型 ID
// 不再被无条件声明为支持图片输入（历史上这里断言 vision=true，等于把未验证的
// 猜测当结论）。现在对未知模型 ID 的结论是 unknown，只能表示“没有依据”。
func TestCompat_Supports(t *testing.T) {
	model, err := newOpenAICompat(Config{Type: "openai-compat", APIKey: "key", ModelID: "m"}, "https://example.com")
	if err != nil {
		t.Fatalf("newOpenAICompat failed: %v", err)
	}
	if !model.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if model.Supports(llm.CapVision) {
		t.Error("unknown model must not be declared vision-capable")
	}
	if !model.Supports(llm.CapStreaming) {
		t.Error("expected streaming support")
	}
	if model.Supports(llm.CapJSON) {
		t.Error("did not expect JSON mode support")
	}
}
