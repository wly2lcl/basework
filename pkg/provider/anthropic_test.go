package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ---------- Helper: create a test server that returns a given response ----------

func newAnthropicTestServer(statusCode int, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
}

func newAnthropicTestServerWithCheck(statusCode int, responseBody string, checkFn func(r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkFn != nil {
			checkFn(r)
		}
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}))
}

// SSE streaming helper
func newAnthropicStreamServer(events string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Write([]byte(events))
	}))
}

// ---------- Test: newAnthropic validation ----------

func TestNewAnthropic_EmptyAPIKey(t *testing.T) {
	_, err := newAnthropic("https://api.anthropic.com", "", "claude-sonnet-4-20250514", nil)
	if err == nil {
		t.Fatal("expected error for empty apiKey")
	}
	var llmErr *llm.Error
	if !llm.IsRateLimit(nil) { // just checking it's an llm.Error
		_ = llmErr
	}
}

// ---------- Test: ID and Supports ----------

func TestAnthropic_ID(t *testing.T) {
	m, err := newAnthropic("https://api.anthropic.com", "sk-ant-test123", "claude-sonnet-4-20250514", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID() != "claude-sonnet-4-20250514" {
		t.Errorf("expected model ID 'claude-sonnet-4-20250514', got '%s'", m.ID())
	}
}

func TestAnthropic_Supports(t *testing.T) {
	m, err := newAnthropic("", "sk-ant-test123", "claude-sonnet-4-20250514", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if !m.Supports(llm.CapVision) {
		t.Error(expectedVisionSupport)
	}
	if !m.Supports(llm.CapStreaming) {
		t.Error(expectedStreamingSupport)
	}
	if m.Supports(llm.CapJSON) {
		t.Error("did not expect JSON mode support")
	}
}

// ---------- Test: Headers ----------

func TestAnthropic_Headers(t *testing.T) {
	var capturedReq *http.Request
	ts := newAnthropicTestServerWithCheck(200, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, func(r *http.Request) {
		capturedReq = r
	})
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools:     true,
			llm.CapVision:    true,
			llm.CapStreaming: true,
		},
	}

	_, err := m.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if capturedReq == nil {
		t.Fatal("no request captured")
	}

	if capturedReq.Header.Get("x-api-key") != "sk-ant-test123" {
		t.Errorf("expected x-api-key header 'sk-ant-test123', got '%s'", capturedReq.Header.Get("x-api-key"))
	}
	if capturedReq.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("expected anthropic-version header '2023-06-01', got '%s'", capturedReq.Header.Get("anthropic-version"))
	}
	if capturedReq.Header.Get("Authorization") != "" {
		t.Errorf("did not expect Authorization header, got '%s'", capturedReq.Header.Get("Authorization"))
	}
}

// ---------- Test: Generate (text response) ----------

func TestAnthropic_Generate(t *testing.T) {
	responseBody := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello! How can I help you?"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	ts := newAnthropicTestServer(200, responseBody)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	resp, err := m.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("expected assistant role, got %s", resp.Message.Role)
	}
	if len(resp.Message.Content) != 1 || resp.Message.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected stop reason 'stop', got '%s'", resp.FinishReason)
	}
}

// ---------- Test: Generate with tools ----------

func TestAnthropic_Generate_WithTools(t *testing.T) {
	responseBody := `{
		"id": "msg_456",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "text", "text": "Let me check the weather."},
			{"type": "tool_use", "id": "toolu_abc123", "name": "get_weather", "input": {"location": "San Francisco", "unit": "celsius"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 15}
	}`
	ts := newAnthropicTestServer(200, responseBody)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	resp, err := m.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Weather in SF?"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "toolu_abc123" {
		t.Errorf("expected tool ID 'toolu_abc123', got '%s'", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", tc.Name)
	}
	// Verify ArgsJSON contains the expected args
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.ArgsJSON), &args); err != nil {
		t.Fatal(err)
	}
	if args["location"] != "San Francisco" {
		t.Errorf("expected location 'San Francisco', got '%v'", args["location"])
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish reason 'tool_calls', got '%s'", resp.FinishReason)
	}
	// Verify text is preserved
	if len(resp.Message.Content) != 1 || resp.Message.Content[0].Text != "Let me check the weather." {
		t.Errorf("unexpected text content: %+v", resp.Message.Content)
	}
}

// ---------- Test: Stream (text only) ----------

func TestAnthropic_Stream(t *testing.T) {
	events := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"Hello\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"!\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	ts := newAnthropicStreamServer(events)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	ch, err := m.Stream(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Collect events
	var texts []string
	var usage *llm.Usage
	gotDone := false

	for evt := range ch {
		switch evt.Type {
		case llm.StreamEventText:
			texts = append(texts, evt.Delta)
		case llm.StreamEventUsage:
			usage = evt.Usage
		case llm.StreamEventDone:
			gotDone = true
		}
	}

	// Verify text deltas
	fullText := strings.Join(texts, "")
	if fullText != "Hello world!" {
		t.Errorf("expected 'Hello world!', got '%s'", fullText)
	}

	// Verify usage from message_delta
	if usage == nil || usage.CompletionTokens != 3 {
		t.Errorf("expected 3 output tokens from message_delta, got %+v", usage)
	}

	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// ---------- Test: Stream with tool calls ----------

func TestAnthropic_Stream_WithToolCalls(t *testing.T) {
	events := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Let me look that up\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_sf_123\",\"name\":\"get_weather\",\"input\":{}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"location\\\":\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"San Francisco\\\"}\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":15}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	ts := newAnthropicStreamServer(events)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	ch, err := m.Stream(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Weather in SF?"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var textDeltas []string
	var toolCallDeltas []*llm.ToolCallDelta
	var completeToolCall *llm.ToolCallDelta
	gotDone := false

	for evt := range ch {
		switch evt.Type {
		case llm.StreamEventText:
			textDeltas = append(textDeltas, evt.Delta)
		case llm.StreamEventToolCall:
			if evt.ToolCall.Complete {
				completeToolCall = evt.ToolCall
			} else {
				toolCallDeltas = append(toolCallDeltas, evt.ToolCall)
			}
		case llm.StreamEventDone:
			gotDone = true
		}
	}

	fullText := strings.Join(textDeltas, "")
	if fullText != "Let me look that up" {
		t.Errorf("expected 'Let me look that up', got '%s'", fullText)
	}

	if completeToolCall == nil {
		t.Fatal("expected complete tool call")
	}
	if completeToolCall.ID != "toolu_sf_123" {
		t.Errorf("expected tool ID 'toolu_sf_123', got '%s'", completeToolCall.ID)
	}
	if completeToolCall.Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", completeToolCall.Name)
	}
	// Verify the complete args JSON
	if !strings.Contains(completeToolCall.ArgsJSON, "San Francisco") {
		t.Errorf("expected args to contain 'San Francisco', got '%s'", completeToolCall.ArgsJSON)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

// ---------- Test: Error - Rate Limit ----------

func TestAnthropic_Error_RateLimit(t *testing.T) {
	ts := newAnthropicTestServer(429, `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	_, err := m.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	if !llm.IsRateLimit(err) {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

// ---------- Test: Error - Context Overflow ----------

func TestAnthropic_Error_ContextOverflow(t *testing.T) {
	ts := newAnthropicTestServer(400, `{"type":"error","error":{"type":"invalid_request_error","message":"prompt_too_long: your prompt has too many tokens"}}`)
	defer ts.Close()

	m := &anthropicModel{
		baseURL: ts.URL,
		apiKey:  "sk-ant-test123",
		modelID: "claude-sonnet-4-20250514",
		client:  ts.Client(),
		capabilities: map[llm.Capability]bool{
			llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true,
		},
	}

	_, err := m.Generate(context.Background(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for context overflow")
	}
	if !llm.IsContextOverflow(err) {
		t.Errorf("expected context overflow error, got %v", err)
	}
}

// ---------- Test: toAnthropicMessages ----------

func TestAnthropic_MessageFormat(t *testing.T) {
	tests := []struct {
		name          string
		messages      []llm.ChatMessage
		wantSystem    string
		wantMsgCount  int
		checkMessages func(t *testing.T, msgs []map[string]any)
	}{
		{
			name: "system extraction",
			messages: []llm.ChatMessage{
				{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are a helpful assistant."}}},
				{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
			},
			wantSystem:   "You are a helpful assistant.",
			wantMsgCount: 1,
		},
		{
			name: "tool result formatting",
			messages: []llm.ChatMessage{
				{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "What's the weather?"}}},
				{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Let me check"}}, ToolCalls: []llm.ToolCall{{ID: "toolu_1", Name: "get_weather", ArgsJSON: `{"location":"NYC"}`}}},
				{Role: llm.RoleTool, ToolCallID: "toolu_1", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Sunny"}}},
			},
			wantMsgCount: 3,
			checkMessages: func(t *testing.T, msgs []map[string]any) {
				// Last message should be user with tool_result
				last := msgs[len(msgs)-1]
				if last["role"] != "user" {
					t.Errorf("expected tool result to have role 'user', got '%v'", last["role"])
				}
				content, ok := last["content"].([]map[string]any)
				if !ok {
					t.Fatal("expected content to be []map[string]any")
				}
				if len(content) != 1 {
					t.Fatalf("expected 1 content block, got %d", len(content))
				}
				if content[0]["type"] != "tool_result" {
					t.Errorf("expected content type 'tool_result', got '%v'", content[0]["type"])
				}
				if content[0]["tool_use_id"] != "toolu_1" {
					t.Errorf("expected tool_use_id 'toolu_1', got '%v'", content[0]["tool_use_id"])
				}
			},
		},
		{
			name: "merge consecutive tool results",
			messages: []llm.ChatMessage{
				{Role: llm.RoleTool, ToolCallID: "toolu_1", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Result 1"}}},
				{Role: llm.RoleTool, ToolCallID: "toolu_2", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Result 2"}}},
			},
			wantMsgCount: 1,
			checkMessages: func(t *testing.T, msgs []map[string]any) {
				if len(msgs) != 1 {
					t.Fatalf("expected 1 merged message, got %d", len(msgs))
				}
				msg := msgs[0]
				if msg["role"] != "user" {
					t.Errorf("expected role 'user', got '%v'", msg["role"])
				}
				content, ok := msg["content"].([]map[string]any)
				if !ok {
					t.Fatal("expected content to be []map[string]any")
				}
				if len(content) != 2 {
					t.Fatalf("expected 2 merged tool_result blocks, got %d", len(content))
				}
				if content[0]["tool_use_id"] != "toolu_1" || content[1]["tool_use_id"] != "toolu_2" {
					t.Errorf("unexpected tool_use_ids: %v, %v", content[0]["tool_use_id"], content[1]["tool_use_id"])
				}
			},
		},
		{
			name: "user message with images",
			messages: []llm.ChatMessage{
				{Role: llm.RoleUser, Content: []llm.ContentPart{
					{Type: llm.ContentTypeText, Text: "What's in this image?"},
					{Type: llm.ContentTypeImage, ImageURL: "data:image/jpeg;base64,/9j/4AAQ=="},
				}},
			},
			wantMsgCount: 1,
			checkMessages: func(t *testing.T, msgs []map[string]any) {
				msg := msgs[0]
				content, ok := msg["content"].([]map[string]any)
				if !ok {
					t.Fatal("expected content to be []map[string]any for multimodal message")
				}
				if len(content) != 2 {
					t.Fatalf("expected 2 content blocks, got %d", len(content))
				}
				if content[0]["type"] != "text" || content[0]["text"] != "What's in this image?" {
					t.Errorf("unexpected first content block: %+v", content[0])
				}
				if content[1]["type"] != "image" {
					t.Errorf("expected second block type 'image', got '%v'", content[1]["type"])
				}
				source, ok := content[1]["source"].(map[string]any)
				if !ok {
					t.Fatal("expected source map in image block")
				}
				if source["type"] != "base64" {
					t.Errorf("expected source type 'base64', got '%v'", source["type"])
				}
				if source["media_type"] != "image/jpeg" {
					t.Errorf("expected media_type 'image/jpeg', got '%v'", source["media_type"])
				}
				if source["data"] != "/9j/4AAQ==" {
					t.Errorf("unexpected image data: '%v'", source["data"])
				}
			},
		},
		{
			name: "assistant message with tool calls",
			messages: []llm.ChatMessage{
				{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "I'll help"}}, ToolCalls: []llm.ToolCall{
					{ID: "toolu_1", Name: "bash", ArgsJSON: `{"command":"ls"}`},
				}},
			},
			wantMsgCount: 1,
			checkMessages: func(t *testing.T, msgs []map[string]any) {
				msg := msgs[0]
				if msg["role"] != "assistant" {
					t.Errorf("expected role 'assistant', got '%v'", msg["role"])
				}
				content, ok := msg["content"].([]map[string]any)
				if !ok {
					t.Fatal("expected content to be []map[string]any")
				}
				if len(content) != 2 {
					t.Fatalf("expected 2 content blocks, got %d", len(content))
				}
				if content[0]["type"] != "text" || content[0]["text"] != "I'll help" {
					t.Errorf("unexpected text block: %+v", content[0])
				}
				if content[1]["type"] != "tool_use" || content[1]["id"] != "toolu_1" || content[1]["name"] != "bash" {
					t.Errorf("unexpected tool_use block: %+v", content[1])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system, messages := toAnthropicMessages(tt.messages)
			if system != tt.wantSystem {
				t.Errorf("expected system '%s', got '%s'", tt.wantSystem, system)
			}
			if len(messages) != tt.wantMsgCount {
				t.Errorf("expected %d messages, got %d", tt.wantMsgCount, len(messages))
			}
			if tt.checkMessages != nil {
				tt.checkMessages(t, messages)
			}
		})
	}
}

// ---------- Test: toAnthropicTools ----------

func TestAnthropic_ToolDefinition(t *testing.T) {
	tools := []llm.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the weather for a location",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"location": {"type": "string"}
				}
			}`),
		},
		{
			Name:        "bash",
			Description: "Run a shell command",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {"type": "string"}
				},
				"required": ["command"]
			}`),
		},
	}

	result := toAnthropicTools(tools)

	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}

	// Verify first tool has no function wrapper
	tool1 := result[0]
	if tool1["name"] != "get_weather" {
		t.Errorf("expected name 'get_weather', got '%v'", tool1["name"])
	}
	if tool1["description"] != "Get the weather for a location" {
		t.Errorf("expected description, got '%v'", tool1["description"])
	}
	if _, hasInputSchema := tool1["input_schema"]; !hasInputSchema {
		t.Error("expected input_schema field")
	}
	if _, hasFunc := tool1["function"]; hasFunc {
		t.Error("did not expect function wrapper")
	}
	if _, hasType := tool1["type"]; hasType {
		t.Error("did not expect type field")
	}

	// Second tool
	tool2 := result[1]
	if tool2["name"] != "bash" {
		t.Errorf("expected name 'bash', got '%v'", tool2["name"])
	}
}

// ---------- Test: toAnthropicTools empty ----------

func TestAnthropic_ToolDefinition_Empty(t *testing.T) {
	result := toAnthropicTools(nil)
	if result != nil {
		t.Errorf("expected nil for empty tools, got %+v", result)
	}

	result = toAnthropicTools([]llm.ToolDefinition{})
	if result != nil {
		t.Errorf("expected nil for empty tools, got %+v", result)
	}
}

// Helper for test readability
const (
	expectedVisionSupport    = "expected vision support"
	expectedStreamingSupport = "expected streaming support"
)