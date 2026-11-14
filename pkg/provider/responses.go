package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// responsesModel implements the OpenAI Responses API.  The adapter is kept
// independent from the Chat Completions implementation because Responses has
// different input items, tool definitions, output items, and SSE events.
// Agnes exposes the same protocol at /v1/responses, so agnes-responses uses
// this model without any vendor-specific request rewriting.
type responsesModel struct {
	baseURL      string
	apiKey       string
	modelID      string
	client       *http.Client
	capabilities map[llm.Capability]bool
	caps         *capabilitySet
}

func newResponses(baseURL, apiKey, modelID string, _ map[string]any) (llm.Model, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("Responses provider requires a BaseURL")
	}
	caps := newCapabilitySet("responses", modelID)
	return &responsesModel{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		modelID: modelID,
		client:  newHTTPClient(10 * time.Minute),
		capabilities: map[llm.Capability]bool{
			llm.CapTools:     true,
			llm.CapStreaming: true,
		},
		caps: caps,
	}, nil
}

func (m *responsesModel) ID() string { return m.modelID }

func (m *responsesModel) Supports(cap llm.Capability) bool { return m.capabilities[cap] }

func (m *responsesModel) Capability(cap llm.Capability) llm.CapabilityDetail {
	return m.caps.Capability(cap)
}

func (m *responsesModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := buildResponsesRequest(req, false, m.modelID)
	respBody, statusCode, err := jsonRequest(ctx, m.client, m.baseURL+"/responses", map[string]string{
		"Authorization": "Bearer " + m.apiKey,
	}, body)
	if err != nil {
		return nil, &llm.Error{Type: llm.ErrorTypeNetwork, Message: fmt.Sprintf("request failed: %v", err)}
	}
	if statusCode != http.StatusOK {
		return nil, mapHTTPError(statusCode, respBody, "responses")
	}
	return fromResponsesResponse(respBody)
}

func (m *responsesModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	body := buildResponsesRequest(req, true, m.modelID)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/responses", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{Type: llm.ErrorTypeNetwork, Message: fmt.Sprintf("create request failed: %v", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	// Streaming requests are subject to the same transient 429/5xx and
	// transport failures as non-streaming requests. Recreate the body for each
	// attempt so a retry never sends an already-consumed request body.
	// A real Agent turn can issue several streaming requests in a short burst
	// (one per tool round). Give 429s a longer recovery window than the small
	// non-streaming request budget, while still respecting the caller context.
	resp, err := doWithRetry(ctx, m.client, httpReq, maxRetries+2, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonBody)), nil
	})
	if err != nil {
		return nil, &llm.Error{Type: llm.ErrorTypeNetwork, Message: fmt.Sprintf("request failed: %v", err)}
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "responses")
	}
	return parseResponsesStream(ctx, resp), nil
}

// buildResponsesRequest translates the internal full-history request into the
// stateless Responses input format.  Sending all history on every request is
// intentional: it keeps the adapter independent of server-side response state
// and makes retries/replays deterministic.
func buildResponsesRequest(req *llm.Request, stream bool, modelID string) map[string]any {
	body := map[string]any{
		"model":  modelID,
		"input":  toResponsesInput(req.Messages),
		"stream": stream,
	}
	if len(req.Tools) > 0 {
		body["tools"] = toResponsesTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		body["max_output_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	// Responses does not define Chat Completions' `stop` field. Callers that
	// target a gateway-specific extension can still pass it explicitly through
	// req.Extra; silently emitting an unsupported standard field would make
	// Agnes reject otherwise valid requests.
	for k, v := range req.Extra {
		body[k] = v
	}
	return body
}

func toResponsesInput(msgs []llm.ChatMessage) []map[string]any {
	result := make([]map[string]any, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleTool:
			output := joinContentText(msg.Content)
			// Agnes rejects an empty function_call_output even though an empty
			// tool result is valid in the internal message model (for example,
			// a successful command with no stdout). Keep the request shape
			// valid while preserving the fact that the tool produced no text.
			if output == "" {
				output = "(no output)"
			}
			result = append(result, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  output,
			})
		case llm.RoleAssistant:
			if text := joinContentText(msg.Content); text != "" {
				result = append(result, responsesMessageItem("assistant", text))
			}
			for _, call := range msg.ToolCalls {
				result = append(result, map[string]any{
					"type": "function_call",
					// Agnes' Responses deserializer requires the output item id and
					// status when replaying a prior function call. The internal
					// ToolCall ID is the call_id; using it for id keeps the mapping
					// deterministic because llm.ToolCall has no second item id.
					"id":        call.ID,
					"call_id":   call.ID,
					"name":      call.Name,
					"arguments": call.ArgsJSON,
					"status":    "completed",
				})
			}
		case llm.RoleUser:
			result = append(result, responsesContentMessage("user", msg.Content, true))
		default: // system
			result = append(result, responsesContentMessage("system", msg.Content, false))
		}
	}
	return result
}

func responsesMessageItem(role, text string) map[string]any {
	return map[string]any{"role": role, "content": text}
}

func responsesContentMessage(role string, parts []llm.ContentPart, allowImages bool) map[string]any {
	if !allowImages && !hasImageContent(parts) {
		return responsesMessageItem(role, joinContentText(parts))
	}
	content := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case llm.ContentTypeImage:
			if allowImages {
				content = append(content, map[string]any{"type": "input_image", "image_url": part.ImageURL})
			}
		case llm.ContentTypeText:
			content = append(content, map[string]any{"type": "input_text", "text": part.Text})
		}
	}
	return map[string]any{"role": role, "content": content}
}

func toResponsesTools(tools []llm.ToolDefinition) []map[string]any {
	result := make([]map[string]any, len(tools))
	for i, tool := range tools {
		var params map[string]any
		if len(tool.Parameters) > 0 {
			_ = json.Unmarshal(tool.Parameters, &params)
		}
		if params == nil {
			params = map[string]any{}
		}
		result[i] = map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  params,
		}
	}
	return result
}

type responsesResponse struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Output []json.RawMessage `json:"output"`
	Usage  *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	OutputText string `json:"output_text"`
}

type responsesOutputItem struct {
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
	ID      string            `json:"id"`
	CallID  string            `json:"call_id"`
	Name    string            `json:"name"`
	Args    string            `json:"arguments"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func fromResponsesResponse(body []byte) (*llm.Response, error) {
	var raw responsesResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse Responses response: %w", err)
	}
	msg := llm.ChatMessage{Role: llm.RoleAssistant}
	for _, itemRaw := range raw.Output {
		var item responsesOutputItem
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			return nil, fmt.Errorf("parse Responses output item: %w", err)
		}
		switch item.Type {
		case "message":
			for _, contentRaw := range item.Content {
				var content responsesContentItem
				if err := json.Unmarshal(contentRaw, &content); err == nil && content.Type == "output_text" {
					msg.Content = append(msg.Content, llm.ContentPart{Type: llm.ContentTypeText, Text: content.Text})
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = item.ID
			}
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: id, Name: item.Name, ArgsJSON: item.Args})
		}
	}
	if len(msg.Content) == 0 && raw.OutputText != "" {
		msg.Content = []llm.ContentPart{{Type: llm.ContentTypeText, Text: raw.OutputText}}
	}
	result := &llm.Response{Message: msg, FinishReason: raw.Status}
	if raw.Usage != nil {
		result.Usage = llm.Usage{PromptTokens: raw.Usage.InputTokens, CompletionTokens: raw.Usage.OutputTokens, TotalTokens: raw.Usage.TotalTokens}
	}
	if len(raw.Output) == 0 && len(msg.Content) == 0 && len(msg.ToolCalls) == 0 {
		return nil, fmt.Errorf("Responses response: empty output")
	}
	return result, nil
}

type responsesStreamState struct {
	toolCalls map[int]*responsesStreamToolCall
}

type responsesStreamToolCall struct {
	ID        string
	Name      string
	Arguments string
	Completed bool
}

// parseResponsesStream accepts the standard event: + data: SSE framing.  A
// few compatible gateways omit event: and put type in data, so data.type is
// also used as a fallback.
func parseResponsesStream(ctx context.Context, resp *http.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		state := responsesStreamState{toolCalls: make(map[int]*responsesStreamToolCall)}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		eventName := ""
		emit := func(event llm.StreamEvent) bool {
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var payload struct {
				Type        string          `json:"type"`
				Delta       string          `json:"delta"`
				Text        string          `json:"text"`
				OutputIndex int             `json:"output_index"`
				ItemID      string          `json:"item_id"`
				Item        json.RawMessage `json:"item"`
				Response    json.RawMessage `json:"response"`
				Usage       *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				if !emit(llm.StreamEvent{Type: llm.StreamEventUsage, Error: fmt.Errorf("parse Responses SSE event: %w", err)}) {
					return
				}
				continue
			}
			typeName := payload.Type
			if typeName == "" {
				typeName = eventName
			}
			eventName = ""
			switch typeName {
			case "response.output_text.delta":
				if payload.Delta != "" && !emit(llm.StreamEvent{Type: llm.StreamEventText, Delta: payload.Delta}) {
					return
				}
			case "response.function_call_arguments.delta":
				call := state.toolCalls[payload.OutputIndex]
				if call == nil {
					call = &responsesStreamToolCall{ID: payload.ItemID}
					state.toolCalls[payload.OutputIndex] = call
				}
				call.Arguments += payload.Delta
				if !emit(llm.StreamEvent{Type: llm.StreamEventToolCall, ToolCall: &llm.ToolCallDelta{Index: payload.OutputIndex, ID: call.ID, Name: call.Name, ArgsJSON: payload.Delta}}) {
					return
				}
			case "response.function_call_arguments.done":
				call := state.toolCalls[payload.OutputIndex]
				if call == nil {
					call = &responsesStreamToolCall{ID: payload.ItemID}
					state.toolCalls[payload.OutputIndex] = call
				}
				call.Arguments, call.Completed = payload.Text, true
				if !emit(llm.StreamEvent{Type: llm.StreamEventToolCall, ToolCall: &llm.ToolCallDelta{Index: payload.OutputIndex, ID: call.ID, Name: call.Name, ArgsJSON: payload.Text, Complete: true}}) {
					return
				}
			case "response.output_item.added":
				var item responsesOutputItem
				if json.Unmarshal(payload.Item, &item) == nil && item.Type == "function_call" {
					id := item.CallID
					if id == "" {
						id = item.ID
					}
					state.toolCalls[payload.OutputIndex] = &responsesStreamToolCall{ID: id, Name: item.Name, Arguments: item.Args}
				}
			case "response.completed", "response.incomplete":
				usage := payload.Usage
				if len(payload.Response) > 0 {
					var completed responsesResponse
					if json.Unmarshal(payload.Response, &completed) == nil {
						if usage == nil && completed.Usage != nil {
							usage = &struct {
								InputTokens  int `json:"input_tokens"`
								OutputTokens int `json:"output_tokens"`
								TotalTokens  int `json:"total_tokens"`
							}{
								InputTokens: completed.Usage.InputTokens, OutputTokens: completed.Usage.OutputTokens, TotalTokens: completed.Usage.TotalTokens,
							}
						}
						for i, itemRaw := range completed.Output {
							var item responsesOutputItem
							if json.Unmarshal(itemRaw, &item) == nil && item.Type == "function_call" {
								call := state.toolCalls[i]
								if call == nil || !call.Completed {
									id := item.CallID
									if id == "" {
										id = item.ID
									}
									args := item.Args
									if call != nil && args == "" {
										args = call.Arguments
									}
									if !emit(llm.StreamEvent{Type: llm.StreamEventToolCall, ToolCall: &llm.ToolCallDelta{Index: i, ID: id, Name: item.Name, ArgsJSON: args, Complete: true}}) {
										return
									}
								}
							}
						}
					}
				}
				if usage != nil {
					if !emit(llm.StreamEvent{Type: llm.StreamEventUsage, Usage: &llm.Usage{PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens}}) {
						return
					}
				}
				if !emit(llm.StreamEvent{Type: llm.StreamEventDone}) {
					return
				}
				return
			case "response.failed", "error":
				if !emit(llm.StreamEvent{Type: llm.StreamEventUsage, Error: fmt.Errorf("Responses stream failed: %s", data)}) {
					return
				}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = emit(llm.StreamEvent{Type: llm.StreamEventUsage, Error: err})
		}
	}()
	return ch
}
