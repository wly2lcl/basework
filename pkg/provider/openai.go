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

// openAIModel implements llm.Model for OpenAI Chat Completions API
type openAIModel struct {
	baseURL            string
	apiKey             string
	modelID            string
	client             *http.Client
	capabilities       map[llm.Capability]bool
	promptCacheEnabled bool
}

// newOpenAI creates a new OpenAI model instance
func newOpenAI(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	promptCacheEnabled := true
	if opts != nil {
		if v, ok := opts["prompt_cache_enabled"]; ok {
			promptCacheEnabled, _ = v.(bool)
		}
	}
	return &openAIModel{
		baseURL: baseURL,
		apiKey:  apiKey,
		modelID: modelID,
		client:  newHTTPClient(10 * time.Minute),
		capabilities: map[llm.Capability]bool{
			llm.CapTools:     true,
			llm.CapVision:    true,
			llm.CapStreaming: true,
		},
		promptCacheEnabled: promptCacheEnabled,
	}, nil
}

// ID returns the model identifier
func (m *openAIModel) ID() string {
	return m.modelID
}

// Supports checks if the model supports the given capability
func (m *openAIModel) Supports(cap llm.Capability) bool {
	return m.capabilities[cap]
}

// Generate sends a non-streaming chat completion request
func (m *openAIModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := buildOpenAIRequest(req, false, m.modelID, m.promptCacheEnabled)
	headers := map[string]string{
		"Authorization": "Bearer " + m.apiKey,
	}

	url := m.baseURL + "/chat/completions"
	respBody, statusCode, err := jsonRequest(ctx, m.client, url, headers, body)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "openai")
	}

	return fromOpenAIResponse(respBody)
}

// Stream sends a streaming chat completion request and returns a channel of events
func (m *openAIModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	body := buildOpenAIRequest(req, true, m.modelID, m.promptCacheEnabled)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := m.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "openai")
	}

	return parseOpenAIStream(ctx, resp), nil
}

// ---------------------------------------------------------------------------
// Message format translation
// ---------------------------------------------------------------------------

// toOpenAIMessages converts internal ChatMessage slice to OpenAI API message format
func toOpenAIMessages(msgs []llm.ChatMessage, cacheEnabled bool) []map[string]any {
	result := make([]map[string]any, len(msgs))
	for i, msg := range msgs {
		m := map[string]any{
			"role": string(msg.Role),
		}

		switch msg.Role {
		case llm.RoleTool:
			m["tool_call_id"] = msg.ToolCallID
			m["content"] = joinContentText(msg.Content)

		case llm.RoleAssistant:
			m["content"] = joinContentText(msg.Content)
			if len(msg.ToolCalls) > 0 {
				tcs := make([]map[string]any, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					tcs[j] = map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": tc.ArgsJSON,
						},
					}
				}
				m["tool_calls"] = tcs
			}

		case llm.RoleUser:
			if cacheEnabled {
				m["content"] = markOpenAIContentForCache(msg.Content)
			} else if hasImageContent(msg.Content) {
				m["content"] = toOpenAIContentParts(msg.Content)
			} else {
				m["content"] = joinContentText(msg.Content)
			}

		default: // system
			m["content"] = joinContentText(msg.Content)
		}

		result[i] = m
	}
	return result
}

func joinContentText(parts []llm.ContentPart) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

func hasImageContent(parts []llm.ContentPart) bool {
	for _, p := range parts {
		if p.Type == llm.ContentTypeImage {
			return true
		}
	}
	return false
}

func toOpenAIContentParts(parts []llm.ContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case llm.ContentTypeText:
			out = append(out, map[string]any{
				"type": "text",
				"text": p.Text,
			})
		case llm.ContentTypeImage:
			out = append(out, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": p.ImageURL,
				},
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tool definition translation
// ---------------------------------------------------------------------------

// toOpenAITools converts internal ToolDefinition slice to OpenAI tool format
func toOpenAITools(tools []llm.ToolDefinition) []map[string]any {
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		var params map[string]any
		if t.Parameters != nil {
			// best-effort unmarshal; ignore errors
			_ = json.Unmarshal(t.Parameters, &params)
		}
		if params == nil {
			params = map[string]any{}
		}

		result[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Request body builder
// ---------------------------------------------------------------------------

// buildOpenAIRequest builds the full request body map for OpenAI Chat API
func buildOpenAIRequest(req *llm.Request, stream bool, modelID string, cacheEnabled bool) map[string]any {
	body := map[string]any{
		"model":    modelID,
		"messages": toOpenAIMessages(req.Messages, cacheEnabled),
		"stream":   stream,
	}

	if len(req.Tools) > 0 {
		body["tools"] = toOpenAITools(req.Tools)
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}
	// Merge extra params
	for k, v := range req.Extra {
		body[k] = v
	}

	return body
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// openAIResponse is the minimal structure needed to parse a non-streaming response
type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int `json:"index"`
		Message      struct {
			Role      string `json:"role"`
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens              int `json:"prompt_tokens"`
		CompletionTokens          int `json:"completion_tokens"`
		TotalTokens               int `json:"total_tokens"`
		CacheCreationInputTokens  int `json:"cache_creation_input_tokens,omitempty"`
		CacheReadInputTokens      int `json:"cache_read_input_tokens,omitempty"`
	} `json:"usage"`
}

// fromOpenAIResponse parses a raw OpenAI API response body into an llm.Response
func fromOpenAIResponse(body []byte) (*llm.Response, error) {
	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openai response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai response: empty choices")
	}

	choice := resp.Choices[0]
	msg := llm.ChatMessage{
		Role: llm.RoleAssistant,
	}

	// Content may be nil when tool_calls is present
	if choice.Message.Content != nil {
		msg.Content = []llm.ContentPart{
			{Type: llm.ContentTypeText, Text: *choice.Message.Content},
		}
	}

	// Parse tool calls
	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
			ID:       tc.ID,
			Name:     tc.Function.Name,
			ArgsJSON: tc.Function.Arguments,
		})
	}

	result := &llm.Response{
		Message:      msg,
		FinishReason: choice.FinishReason,
	}

	if resp.Usage != nil {
		result.Usage = llm.Usage{
			PromptTokens:              resp.Usage.PromptTokens,
			CompletionTokens:          resp.Usage.CompletionTokens,
			TotalTokens:               resp.Usage.TotalTokens,
			CacheCreationInputTokens:  resp.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:      resp.Usage.CacheReadInputTokens,
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// SSE stream parser
// ---------------------------------------------------------------------------

type accumulatedToolCall struct {
	ID       string
	Name     string
	ArgsJSON string
}

// parseOpenAIStream reads an SSE stream from the OpenAI API and emits events
func parseOpenAIStream(ctx context.Context, resp *http.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer for large JSON lines (e.g. long tool call args)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		acc := make(map[int]*accumulatedToolCall)

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// End-of-stream marker
			if line == "data: [DONE]" {
				ch <- llm.StreamEvent{Type: llm.StreamEventDone}
				return
			}

			// Must start with "data: "
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			var sseEvent struct {
				Choices []struct {
					Index int `json:"index"`
					Delta struct {
						Role      *string `json:"role"`
						Content   *string `json:"content"`
						ToolCalls []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens              int `json:"prompt_tokens"`
					CompletionTokens          int `json:"completion_tokens"`
					TotalTokens               int `json:"total_tokens"`
					CacheCreationInputTokens  int `json:"cache_creation_input_tokens,omitempty"`
					CacheReadInputTokens      int `json:"cache_read_input_tokens,omitempty"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &sseEvent); err != nil {
				ch <- llm.StreamEvent{Error: err}
				continue
			}

			// Handle usage if present (non-streaming usage at end)
			if sseEvent.Usage != nil {
				ch <- llm.StreamEvent{
					Type: llm.StreamEventUsage,
					Usage: &llm.Usage{
						PromptTokens:              sseEvent.Usage.PromptTokens,
						CompletionTokens:          sseEvent.Usage.CompletionTokens,
						TotalTokens:               sseEvent.Usage.TotalTokens,
						CacheCreationInputTokens:  sseEvent.Usage.CacheCreationInputTokens,
						CacheReadInputTokens:      sseEvent.Usage.CacheReadInputTokens,
					},
				}
			}

			if len(sseEvent.Choices) == 0 {
				continue
			}

			delta := sseEvent.Choices[0].Delta

			// Handle tool call deltas
			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					entry, ok := acc[tc.Index]
					if !ok {
						entry = &accumulatedToolCall{}
						acc[tc.Index] = entry
					}
					if tc.ID != "" {
						entry.ID = tc.ID
					}
					if tc.Function.Name != "" {
						entry.Name = tc.Function.Name
					}
					entry.ArgsJSON += tc.Function.Arguments

					// Mark complete when current delta has empty args but we've accumulated something
					complete := tc.Function.Arguments == "" && entry.ArgsJSON != ""

					ch <- llm.StreamEvent{
						Type: llm.StreamEventToolCall,
						ToolCall: &llm.ToolCallDelta{
							Index:    tc.Index,
							ID:       entry.ID,
							Name:     entry.Name,
							ArgsJSON: entry.ArgsJSON,
							Complete: complete,
						},
					}

					if complete {
						delete(acc, tc.Index)
					}
				}
				continue
			}

			// Handle text content delta
			if delta.Content != nil && *delta.Content != "" {
				ch <- llm.StreamEvent{
					Type:  llm.StreamEventText,
					Delta: *delta.Content,
				}
			}
		}

		// Check for scanner error
		if err := scanner.Err(); err != nil {
			// Only send error if context is not done (avoid race with cancellation)
			if ctx.Err() == nil {
				ch <- llm.StreamEvent{Error: err}
			}
		}
	}()

	return ch
}
