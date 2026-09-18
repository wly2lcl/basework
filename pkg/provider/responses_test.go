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

func TestResponsesRequestTranslation(t *testing.T) {
	temperature := 0.25
	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "system"}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "look"}, {Type: llm.ContentTypeImage, ImageURL: "https://example.test/a.png"}}},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "read", ArgsJSON: `{"path":"a.go"}`}}},
			{Role: llm.RoleTool, ToolCallID: "call_1", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "contents"}}},
		},
		Tools:       []llm.ToolDefinition{{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)}},
		MaxTokens:   123,
		Temperature: &temperature,
		Extra:       map[string]any{"top_p": 0.9},
	}
	body := buildResponsesRequest(req, true, "agnes-3.0-flash")
	if body["model"] != "agnes-3.0-flash" || body["stream"] != true {
		t.Fatalf("unexpected top-level request: %#v", body)
	}
	if body["max_output_tokens"] != 123 || body["temperature"] != temperature || body["top_p"] != 0.9 {
		t.Fatalf("request options were not translated: %#v", body)
	}
	input := body["input"].([]map[string]any)
	if len(input) != 4 {
		t.Fatalf("expected system, user, assistant call, and tool output input items, got %d: %#v", len(input), input)
	}
	if input[2]["type"] != "function_call" || input[2]["id"] != "call_1" || input[2]["call_id"] != "call_1" || input[2]["status"] != "completed" {
		t.Fatalf("assistant tool call was not translated: %#v", input[2])
	}
	if input[3]["type"] != "function_call_output" || input[3]["output"] != "contents" {
		t.Fatalf("tool result was not translated: %#v", input[3])
	}
	userContent := input[1]["content"].([]map[string]any)
	if userContent[0]["type"] != "input_text" || userContent[1]["type"] != "input_image" {
		t.Fatalf("multimodal Responses input was not translated: %#v", userContent)
	}
	tools := body["tools"].([]map[string]any)
	if tools[0]["type"] != "function" || tools[0]["name"] != "read" {
		t.Fatalf("Responses tool shape is wrong: %#v", tools[0])
	}
}

func TestResponsesGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer agnes-key" {
			t.Errorf("Authorization = %q", got)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["model"] != "agnes-3.0-flash" || request["stream"] != false {
			t.Errorf("unexpected request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{\"path\":\"a.go\"}"}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}}`)
	}))
	defer server.Close()

	model, err := newResponses(server.URL+"/v1", "agnes-key", "agnes-3.0-flash", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.Generate(t.Context(), &llm.Request{Messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "hi"}}}}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := response.Message.Content[0].Text; got != "hello" {
		t.Errorf("text = %q", got)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].ID != "call_1" || response.Message.ToolCalls[0].Name != "read" {
		t.Fatalf("tool call = %#v", response.Message.ToolCalls)
	}
	if response.Usage.TotalTokens != 12 || response.FinishReason != "completed" {
		t.Errorf("response metadata = %#v", response)
	}
}

func TestResponsesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
		_, _ = io.WriteString(w, "event: response.output_item.added\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"read\"}}\n\n")
		_, _ = io.WriteString(w, "event: response.output_text.delta\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
		_, _ = io.WriteString(w, "event: response.function_call_arguments.delta\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"item_id\":\"call_1\",\"delta\":\"{\\\"path\\\":\"}\n\n")
		_, _ = io.WriteString(w, "event: response.function_call_arguments.done\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.done\",\"output_index\":1,\"item_id\":\"call_1\",\"text\":\"{\\\"path\\\":\\\"a.go\\\"}\"}\n\n")
		_, _ = io.WriteString(w, "event: response.completed\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n")
	}))
	defer server.Close()

	model, err := newResponses(server.URL+"/v1", "key", "model", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := model.Stream(t.Context(), &llm.Request{Messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "hi"}}}}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var text string
	var deltas []llm.ToolCallDelta
	var usage *llm.Usage
	var done bool
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error: %v", event.Error)
		}
		switch event.Type {
		case llm.StreamEventText:
			text += event.Delta
		case llm.StreamEventToolCall:
			if event.ToolCall != nil {
				deltas = append(deltas, *event.ToolCall)
			}
		case llm.StreamEventUsage:
			usage = event.Usage
		case llm.StreamEventDone:
			done = true
		}
	}
	if text != "hello" || len(deltas) != 2 || deltas[0].ArgsJSON != `{"path":` || !deltas[1].Complete || deltas[1].ArgsJSON != `{"path":"a.go"}` {
		t.Fatalf("stream output text=%q tool_deltas=%#v", text, deltas)
	}
	if deltas[0].ID != "call_1" || deltas[0].Name != "read" || usage == nil || usage.TotalTokens != 5 || !done {
		t.Fatalf("stream metadata: deltas=%#v usage=%#v done=%v", deltas, usage, done)
	}
}

func TestResponsesFactoryAliases(t *testing.T) {
	for _, providerType := range []string{"responses", "openai-responses", "agnes-responses"} {
		model, err := Create(Config{Type: providerType, APIKey: "key", ModelID: "model"})
		if err != nil {
			t.Fatalf("Create(%q): %v", providerType, err)
		}
		if _, ok := model.(*responsesModel); !ok {
			t.Fatalf("Create(%q) returned %T", providerType, model)
		}
	}
	if got := APIKeyEnvVar("agnes-responses"); got != "AGNES_API_KEY" {
		t.Fatalf("Agnes Responses key env = %q", got)
	}
}

func TestResponsesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"code":"invalid_api_key"}}`)
	}))
	defer server.Close()
	model, err := newResponses(server.URL, "bad", "model", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.Generate(t.Context(), &llm.Request{})
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error, got %v", err)
	}
}
