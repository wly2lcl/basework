package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ---------------------------------------------------------------------------
// Unit tests for toOpenAIMessages
// ---------------------------------------------------------------------------

func TestOpenAI_MessageFormat(t *testing.T) {
	t.Run("system message", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are a helpful assistant."}}},
		}, false)
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0]["role"] != "system" {
			t.Errorf("expected role 'system', got %v", msgs[0]["role"])
		}
		if msgs[0]["content"] != "You are a helpful assistant." {
			t.Errorf("unexpected content: %v", msgs[0]["content"])
		}
	})

	t.Run("user message text only", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		}, false)
		content, ok := msgs[0]["content"].(string)
		if !ok {
			t.Fatalf("expected content to be string, got %T", msgs[0]["content"])
		}
		if content != "Hello" {
			t.Errorf("unexpected content: %s", content)
		}
	})

	t.Run("user message multi-modal", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{
				Role: llm.RoleUser,
				Content: []llm.ContentPart{
					{Type: llm.ContentTypeText, Text: "Describe this image"},
					{Type: llm.ContentTypeImage, ImageURL: "https://example.com/img.jpg"},
				},
			},
		}, false)
		parts, ok := msgs[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("expected content to be []map[string]any, got %T", msgs[0]["content"])
		}
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(parts))
		}
		if parts[0]["type"] != "text" {
			t.Errorf("expected part[0].type 'text', got %v", parts[0]["type"])
		}
		if parts[1]["type"] != "image_url" {
			t.Errorf("expected part[1].type 'image_url', got %v", parts[1]["type"])
		}
	})

	t.Run("assistant message with text", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Sure!"}}},
		}, false)
		if msgs[0]["role"] != "assistant" {
			t.Errorf("expected role 'assistant', got %v", msgs[0]["role"])
		}
		if msgs[0]["content"] != "Sure!" {
			t.Errorf("unexpected content: %v", msgs[0]["content"])
		}
	})

	t.Run("assistant message with tool calls", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: ""}},
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"Beijing"}`},
				},
			},
		}, false)
		tcs, ok := msgs[0]["tool_calls"].([]map[string]any)
		if !ok {
			t.Fatalf("expected tool_calls to be []map[string]any, got %T", msgs[0]["tool_calls"])
		}
		if len(tcs) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(tcs))
		}
		if tcs[0]["id"] != "call_1" {
			t.Errorf("unexpected tool call id: %v", tcs[0]["id"])
		}
		fn, _ := tcs[0]["function"].(map[string]any)
		if fn["name"] != "get_weather" {
			t.Errorf("unexpected function name: %v", fn["name"])
		}
	})

	t.Run("tool result message", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{
				Role:       llm.RoleTool,
				ToolCallID: "call_1",
				Content:    []llm.ContentPart{{Type: llm.ContentTypeText, Text: `{"temp":22}`}},
			},
		}, false)
		if msgs[0]["role"] != "tool" {
			t.Errorf("expected role 'tool', got %v", msgs[0]["role"])
		}
		if msgs[0]["tool_call_id"] != "call_1" {
			t.Errorf("expected tool_call_id 'call_1', got %v", msgs[0]["tool_call_id"])
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests for toOpenAITools
// ---------------------------------------------------------------------------

func TestOpenAI_ToolDefinition(t *testing.T) {
	rawParams := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)
	tools := toOpenAITools([]llm.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			Parameters:  rawParams,
		},
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0]["type"] != "function" {
		t.Errorf("expected type 'function', got %v", tools[0]["type"])
	}

	fn, ok := tools[0]["function"].(map[string]any)
	if !ok {
		t.Fatalf("expected function to be map[string]any, got %T", tools[0]["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("unexpected name: %v", fn["name"])
	}
	if fn["description"] != "Get weather for a city" {
		t.Errorf("unexpected description: %v", fn["description"])
	}

	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be map[string]any, got %T", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("unexpected parameters type: %v", params["type"])
	}
}

// ---------------------------------------------------------------------------
// Integration tests using httptest
// ---------------------------------------------------------------------------

func TestOpenAI_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		if err := json.Unmarshal(body, &reqMap); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		if reqMap["stream"] != false {
			t.Errorf("expected stream=false, got %v", reqMap["stream"])
		}

		// Return a simple response
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

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
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

func TestOpenAI_Generate_WithTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-456",
			"object": "chat.completion",
			"created": 123456,
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [
						{
							"id": "call_abc123",
							"type": "function",
							"function": {
								"name": "get_weather",
								"arguments": "{\"city\":\"Beijing\"}"
							}
						}
					]
				},
				"finish_reason": "tool_calls"
			}],
			"usage": {
				"prompt_tokens": 20,
				"completion_tokens": 10,
				"total_tokens": 30
			}
		}`)
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
	}

	resp, err := model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Weather?"}}},
		},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].ID != "call_abc123" {
		t.Errorf("unexpected tool call ID: %s", resp.Message.ToolCalls[0].ID)
	}
	if resp.Message.ToolCalls[0].Name != "get_weather" {
		t.Errorf("unexpected tool call name: %s", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[0].ArgsJSON != `{"city":"Beijing"}` {
		t.Errorf("unexpected args: %s", resp.Message.ToolCalls[0].ArgsJSON)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %s", resp.FinishReason)
	}
}

func TestOpenAI_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")

		// Flush to ensure all data is sent
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
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
		case llm.StreamEventToolCall, llm.StreamEventUsage:
			// not expected in this test
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

func TestOpenAI_Stream_WithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)

		// First delta: tool call ID + name
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`+"\n\n")
		// Accumulating arguments
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`+"\n\n")
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Beijing\""}}]}}]}`+"\n\n")
		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}`+"\n\n")
		// Finish
		fmt.Fprintf(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
	}

	events, err := model.Stream(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Weather?"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var toolCalls []llm.ToolCallDelta
	gotDone := false
	for evt := range events {
		switch evt.Type {
		case llm.StreamEventToolCall:
			if evt.ToolCall != nil {
				toolCalls = append(toolCalls, *evt.ToolCall)
			}
		case llm.StreamEventDone:
			gotDone = true
		}
		if evt.Error != nil {
			t.Fatalf("unexpected error: %v", evt.Error)
		}
	}

	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool call delta")
	}

	// Check the final (most accumulated) tool call
	lastTC := toolCalls[len(toolCalls)-1]
	if lastTC.ID != "call_1" {
		t.Errorf("expected ID 'call_1', got '%s'", lastTC.ID)
	}
	if lastTC.Name != "get_weather" {
		t.Errorf("expected Name 'get_weather', got '%s'", lastTC.Name)
	}
	if lastTC.ArgsJSON != `{"city":"Beijing"}` {
		t.Errorf("expected ArgsJSON '{\"city\":\"Beijing\"}', got '%s'", lastTC.ArgsJSON)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

func TestOpenAI_Error_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error": {"message": "rate limit exceeded"}}`)
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !llm.IsRateLimit(err) {
		t.Errorf("expected IsRateLimit to be true, got false: %v", err)
	}
}

func TestOpenAI_Error_ContextOverflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error": {"message": "context_length_exceeded"}}`)
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !llm.IsContextOverflow(err) {
		t.Errorf("expected IsContextOverflow to be true, got false: %v", err)
	}
}

func TestOpenAI_Error_Auth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error": {"message": "invalid API key"}}`)
	}))
	defer srv.Close()

	model, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI failed: %v", err)
	}

	_, err = model.Generate(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var llmErr *llm.Error
	if !errors.As(err, &llmErr) {
		t.Fatal("expected error to be *llm.Error")
	}
	if llmErr.Type != llm.ErrorTypeAuth {
		t.Errorf("expected auth error, got type=%s: %v", llmErr.Type, err)
	}
}

// ============================================================
// TestOpenAI_CacheInjection - 验证 OpenAI 缓存字段（3 个场景）
// ============================================================

func TestOpenAI_CacheInjection(t *testing.T) {
	t.Run("text-only user message with cache enabled - uses content parts", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		}, true)

		// Should use content parts format instead of string
		content, ok := msgs[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("expected content to be []map[string]any when cache enabled, got %T", msgs[0]["content"])
		}
		if len(content) != 1 {
			t.Fatalf("expected 1 content part, got %d", len(content))
		}
		if content[0]["type"] != "text" {
			t.Errorf("expected type 'text', got '%v'", content[0]["type"])
		}
		if content[0]["cache_control"] == nil {
			t.Error("expected cache_control on text content part")
		}
	})

	t.Run("multi-modal user message with cache enabled - text gets cache_control", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{
				Role: llm.RoleUser,
				Content: []llm.ContentPart{
					{Type: llm.ContentTypeText, Text: "Describe this"},
					{Type: llm.ContentTypeImage, ImageURL: "https://example.com/img.jpg"},
				},
			},
		}, true)

		parts, ok := msgs[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("expected content to be []map[string]any")
		}
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(parts))
		}
		// Text part should have cache_control
		if parts[0]["cache_control"] == nil {
			t.Error("expected cache_control on text part")
		}
		// Image part should NOT have cache_control
		if parts[1]["cache_control"] != nil {
			t.Error("expected no cache_control on image part")
		}
		if parts[1]["type"] != "image_url" {
			t.Errorf("expected type 'image_url', got '%v'", parts[1]["type"])
		}
	})

	t.Run("cache disabled - user text remains string", func(t *testing.T) {
		msgs := toOpenAIMessages([]llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		}, false)

		content, ok := msgs[0]["content"].(string)
		if !ok {
			t.Fatalf("expected content to be string when cache disabled, got %T", msgs[0]["content"])
		}
		if content != "Hello" {
			t.Errorf("expected content 'Hello', got '%s'", content)
		}
	})
}