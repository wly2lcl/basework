package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func newGeminiModel(srv *httptest.Server) llm.Model {
	model, err := newGemini(srv.URL, "test-api-key", "gemini-pro", nil)
	if err != nil {
		panic(err)
	}
	return model
}

func readRequest(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	return string(body)
}

func writeResponse(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	w.Write([]byte(body))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// ============================================================
// TestGemini_Generate
// ============================================================

func TestGemini_Generate(t *testing.T) {
	respBody := `{
		"candidates": [{
			"content": {
				"role": "model",
				"parts": [{"text": "Hello from Gemini!"}]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"totalTokenCount": 15
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body to ensure server processes it
		readRequest(r)
		writeResponse(w, 200, respBody)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	resp, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Text != "Hello from Gemini!" {
		t.Errorf("expected 'Hello from Gemini!', got '%s'", resp.Message.Content[0].Text)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected completion tokens 5, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

// ============================================================
// TestGemini_Generate_WithTools
// ============================================================

func TestGemini_Generate_WithTools(t *testing.T) {
	respBody := `{
		"candidates": [{
			"content": {
				"role": "model",
				"parts": [
					{"text": "I'll run that command."},
					{"functionCall": {"name": "bash", "args": {"command": "ls -la"}}}
				]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 20,
			"candidatesTokenCount": 10,
			"totalTokenCount": 30
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		writeResponse(w, 200, respBody)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	resp, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "List files"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Text != "I'll run that command." {
		t.Errorf("expected text, got '%s'", resp.Message.Content[0].Text)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "bash" {
		t.Errorf("expected tool name 'bash', got '%s'", resp.Message.ToolCalls[0].Name)
	}
	if !strings.Contains(resp.Message.ToolCalls[0].ID, "call_bash") {
		t.Errorf("expected ID containing 'call_bash', got '%s'", resp.Message.ToolCalls[0].ID)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(resp.Message.ToolCalls[0].ArgsJSON), &args); err != nil {
		t.Fatalf("failed to unmarshal args: %v", err)
	}
	if args["command"] != "ls -la" {
		t.Errorf("expected command 'ls -la', got '%v'", args["command"])
	}
}

// ============================================================
// TestGemini_Stream
// ============================================================

func TestGemini_Stream(t *testing.T) {
	// Two incremental chunks then a final chunk with the full text
	chunk1 := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello \"}],\"role\":\"model\"}}]}\n\n"
	chunk2 := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello world\"}],\"role\":\"model\"}}]}\n\n"
	chunk3 := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello world!\"}],\"role\":\"model\"}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":3,\"totalTokenCount\":8}}\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		w.WriteHeader(200)
		w.Write([]byte(chunk1))
		w.(http.Flusher).Flush()
		w.Write([]byte(chunk2))
		w.(http.Flusher).Flush()
		w.Write([]byte(chunk3))
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	ch, err := model.Stream(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []llm.StreamEvent
	for evt := range ch {
		events = append(events, evt)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	var fullText string
	for _, evt := range events {
		if evt.Type == llm.StreamEventText {
			fullText += evt.Delta
		}
	}
	if fullText != "Hello world!" {
		t.Errorf("expected full text 'Hello world!', got '%s'", fullText)
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Type != llm.StreamEventDone {
		t.Errorf("expected last event type 'done', got '%s'", lastEvent.Type)
	}
}

// ============================================================
// TestGemini_Stream_WithToolCalls
// ============================================================

func TestGemini_Stream_WithToolCalls(t *testing.T) {
	chunk1 := "data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Let me check.\"}], \"role\": \"model\"}}]}\n\n"
	chunk2 := "data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Let me check.\"}, {\"functionCall\": {\"name\": \"get_weather\", \"args\": {\"city\": \"Beijing\"}}}], \"role\": \"model\"}}]}\n\n"
	chunk3 := "data: {\"usageMetadata\": {\"promptTokenCount\": 10, \"candidatesTokenCount\": 8, \"totalTokenCount\": 18}}\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		w.WriteHeader(200)
		w.Write([]byte(chunk1))
		w.(http.Flusher).Flush()
		w.Write([]byte(chunk2))
		w.(http.Flusher).Flush()
		w.Write([]byte(chunk3))
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	ch, err := model.Stream(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Weather?"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolCallFound, usageFound, textFound bool
	for evt := range ch {
		switch evt.Type {
		case llm.StreamEventText:
			textFound = true
		case llm.StreamEventToolCall:
			toolCallFound = true
			if evt.ToolCall == nil {
				t.Fatal("expected non-nil ToolCall")
			}
			if evt.ToolCall.Name != "get_weather" {
				t.Errorf("expected tool name 'get_weather', got '%s'", evt.ToolCall.Name)
			}
		case llm.StreamEventUsage:
			usageFound = true
			if evt.Usage == nil || evt.Usage.TotalTokens != 18 {
				t.Errorf("expected total tokens 18, got %v", evt.Usage)
			}
		}
	}

	if !textFound {
		t.Error("expected text event")
	}
	if !toolCallFound {
		t.Error("expected tool call event")
	}
	if !usageFound {
		t.Error("expected usage event")
	}
}

// ============================================================
// TestGemini_APIKeyInURL
// ============================================================

func TestGemini_APIKeyInURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)

		// Verify API key is in URL query param, not Authorization header
		if r.URL.Query().Get("key") != "test-api-key" {
			t.Errorf("expected 'test-api-key' in URL query, got '%s'", r.URL.Query().Get("key"))
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got '%s'", auth)
		}
		writeResponse(w, 200, `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	_, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================
// TestGemini_Error_RateLimit
// ============================================================

func TestGemini_Error_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		writeResponse(w, 429, `{"error": {"message": "Rate limit exceeded"}}`)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	_, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !llm.IsRateLimit(err) {
		t.Errorf("expected rate limit error, got: %v", err)
	}
}

// ============================================================
// TestGemini_Error_ContextOverflow
// ============================================================

func TestGemini_Error_ContextOverflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		writeResponse(w, 400, `{"error": {"message": "token limit exceeded. Please reduce your prompt."}}`)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	_, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !llm.IsContextOverflow(err) {
		t.Errorf("expected context overflow error, got: %v", err)
	}
}

// ============================================================
// TestGemini_Generate_Error (non-200 response)
// ============================================================

func TestGemini_Generate_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readRequest(r)
		writeResponse(w, 500, `{"error": {"message": "internal error"}}`)
	}))
	defer srv.Close()

	model := newGeminiModel(srv)
	_, err := model.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================
// TestGemini_EmptyAPIKey
// ============================================================

func TestGemini_EmptyAPIKey(t *testing.T) {
	_, err := newGemini("", "", "gemini-pro", nil)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
}

// ============================================================
// TestGemini_Supports
// ============================================================

func TestGemini_Supports(t *testing.T) {
	model, err := newGemini("https://example.com", "test-key", "gemini-pro", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !model.Supports(llm.CapTools) {
		t.Error("expected Supports(CapTools) = true")
	}
	if !model.Supports(llm.CapVision) {
		t.Error("expected Supports(CapVision) = true")
	}
	if !model.Supports(llm.CapStreaming) {
		t.Error("expected Supports(CapStreaming) = true")
	}
}

// ============================================================
// TestGemini_ID
// ============================================================

func TestGemini_ID(t *testing.T) {
	model, err := newGemini("https://example.com", "test-key", "gemini-2.0-flash", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.ID() != "gemini-2.0-flash" {
		t.Errorf("expected 'gemini-2.0-flash', got '%s'", model.ID())
	}
}

// ============================================================
// TestGemini_MessageFormat (unit tests for toGeminiContents)
// ============================================================

func TestGemini_MessageFormat(t *testing.T) {
	t.Run("system message extraction", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are helpful."}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		}
		contents, systemInstruction := toGeminiContents(msgs, false)

		if systemInstruction != "You are helpful." {
			t.Errorf("expected 'You are helpful.', got '%s'", systemInstruction)
		}
		if len(contents) != 1 {
			t.Fatalf("expected 1 content, got %d", len(contents))
		}
		if contents[0]["role"] != "user" {
			t.Errorf("expected role 'user', got '%v'", contents[0]["role"])
		}
	})

	t.Run("role model not assistant", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Sure!"}}},
		}
		contents, _ := toGeminiContents(msgs, false)

		if len(contents) != 1 {
			t.Fatalf("expected 1 content, got %d", len(contents))
		}
		if contents[0]["role"] != "model" {
			t.Errorf("expected role 'model', got '%v'", contents[0]["role"])
		}
	})

	t.Run("tool result has user role", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{Role: llm.RoleTool, Name: "bash", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "file1.txt\nfile2.txt"}}},
		}
		contents, _ := toGeminiContents(msgs, false)

		if len(contents) != 1 {
			t.Fatalf("expected 1 content, got %d", len(contents))
		}
		if contents[0]["role"] != "user" {
			t.Errorf("expected role 'user', got '%v'", contents[0]["role"])
		}

		parts, _ := contents[0]["parts"].([]map[string]any)
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}
		fr, ok := parts[0]["functionResponse"].(map[string]any)
		if !ok {
			t.Fatal("expected functionResponse")
		}
		if fr["name"] != "bash" {
			t.Errorf("expected name 'bash', got '%v'", fr["name"])
		}
	})

	t.Run("tool call has functionCall parts", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{Name: "bash", ArgsJSON: `{"command": "ls"}`},
				},
			},
		}
		contents, _ := toGeminiContents(msgs, false)

		if len(contents) != 1 {
			t.Fatalf("expected 1 content, got %d", len(contents))
		}
		if contents[0]["role"] != "model" {
			t.Errorf("expected role 'model', got '%v'", contents[0]["role"])
		}

		parts, _ := contents[0]["parts"].([]map[string]any)
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}
		fc, ok := parts[0]["functionCall"].(map[string]any)
		if !ok {
			t.Fatal("expected functionCall")
		}
		if fc["name"] != "bash" {
			t.Errorf("expected name 'bash', got '%v'", fc["name"])
		}
	})

	t.Run("image content", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{
				Role: llm.RoleUser,
				Content: []llm.ContentPart{
					{Type: llm.ContentTypeText, Text: "What's in this image?"},
					{Type: llm.ContentTypeImage, ImageURL: "data:image/jpeg;base64,/9j/4AAQ=="},
				},
			},
		}
		contents, _ := toGeminiContents(msgs, false)

		if len(contents) != 1 {
			t.Fatalf("expected 1 content, got %d", len(contents))
		}

		parts, ok := contents[0]["parts"].([]map[string]any)
		if !ok {
			t.Fatalf("parts is not []map[string]any, got %T", contents[0]["parts"])
		}
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(parts))
		}

		if parts[0]["text"] != "What's in this image?" {
			t.Errorf("expected text, got '%v'", parts[0]["text"])
		}

		inlineData, ok := parts[1]["inlineData"].(map[string]any)
		if !ok {
			t.Fatal("expected inlineData")
		}
		if inlineData["mimeType"] != "image/jpeg" {
			t.Errorf("expected mimeType 'image/jpeg', got '%v'", inlineData["mimeType"])
		}
		if inlineData["data"] != "/9j/4AAQ==" {
			t.Errorf("expected data, got '%v'", inlineData["data"])
		}
	})
}

// ============================================================
// TestGemini_ToolDefinition (unit test for toGeminiTools)
// ============================================================

func TestGemini_ToolDefinition(t *testing.T) {
	tools := []llm.ToolDefinition{
		{
			Name:        "bash",
			Description: "Run shell commands",
			Parameters:  json.RawMessage(`{"type": "object", "properties": {"cmd": {"type": "string"}}}`),
		},
		{
			Name:        "read_file",
			Description: "Read file contents",
		},
	}

	result := toGeminiTools(tools)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool wrapper, got %d", len(result))
	}

	declarations, ok := result[0]["functionDeclarations"].([]map[string]any)
	if !ok {
		t.Fatalf("functionDeclarations is not []map[string]any, got %T", result[0]["functionDeclarations"])
	}
	if len(declarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(declarations))
	}

	if declarations[0]["name"] != "bash" {
		t.Errorf("expected name 'bash', got '%v'", declarations[0]["name"])
	}
	if declarations[0]["description"] != "Run shell commands" {
		t.Errorf("expected description, got '%v'", declarations[0]["description"])
	}

	if declarations[1]["name"] != "read_file" {
		t.Errorf("expected name 'read_file', got '%v'", declarations[1]["name"])
	}
	params, ok := declarations[1]["parameters"].(map[string]any)
	if !ok {
		t.Fatal("expected parameters map")
	}
	if params["type"] != "object" {
		t.Errorf("expected params type 'object', got '%v'", params["type"])
	}
}

// ============================================================
// TestGemini_CacheInjection - 验证 Gemini 缓存标记（3 个场景）
// ============================================================

func TestGemini_CacheInjection(t *testing.T) {
	t.Run("systemInstruction with cachedContent when cache enabled", func(t *testing.T) {
		req := &llm.Request{
			Messages: []llm.ChatMessage{
				{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are Gemini."}}},
				{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
			},
		}
		contents, systemInstruction := toGeminiContents(req.Messages, true)
		body := buildGeminiRequest(req, systemInstruction, true)

		sysInst, ok := body["systemInstruction"].(map[string]any)
		if !ok {
			t.Fatal("expected systemInstruction in body")
		}
		parts, ok := sysInst["parts"].([]map[string]any)
		if !ok {
			t.Fatalf("expected parts []map[string]any, got %T", sysInst["parts"])
		}
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}
		if parts[0]["cachedContent"] != true {
			t.Error("expected cachedContent: true on systemInstruction part")
		}
		_ = contents
	})

	t.Run("first 2 user contents get cachedContent when cache enabled", func(t *testing.T) {
		msgs := []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "First"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Response"}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Second"}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Third"}}},
		}
		contents, _ := toGeminiContents(msgs, true)

		// First user content (index 0) should have cachedContent
		user1 := contents[0]
		if user1["cachedContent"] != true {
			t.Error("expected cachedContent: true on first user content")
		}

		// Second user content is at index 2 (after assistant)
		user2 := contents[2]
		if user2["cachedContent"] != true {
			t.Error("expected cachedContent: true on second user content")
		}

		// Third user content (index 3) - should NOT have cachedContent
		user3 := contents[3]
		if user3["cachedContent"] != nil {
			t.Error("expected no cachedContent on third user content")
		}
	})

	t.Run("cache disabled - no cachedContent on systemInstruction or user content", func(t *testing.T) {
		req := &llm.Request{
			Messages: []llm.ChatMessage{
				{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are Gemini."}}},
				{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
			},
		}
		contents, systemInstruction := toGeminiContents(req.Messages, false)
		body := buildGeminiRequest(req, systemInstruction, false)

		// systemInstruction should NOT have cachedContent
		sysInst, ok := body["systemInstruction"].(map[string]any)
		if ok {
			parts, _ := sysInst["parts"].([]map[string]any)
			if len(parts) > 0 {
				if parts[0]["cachedContent"] != nil {
					t.Error("expected no cachedContent on systemInstruction when cache disabled")
				}
			}
		}

		// User content should NOT have cachedContent
		if len(contents) > 0 {
			userContent := contents[0]
			if userContent["cachedContent"] != nil {
				t.Error("expected no cachedContent on user content when cache disabled")
			}
		}
	})
}
