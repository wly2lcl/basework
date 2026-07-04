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

// anthropicModel implements llm.Model for Anthropic Messages API
type anthropicModel struct {
	baseURL            string
	apiKey             string
	modelID            string
	client             *http.Client
	capabilities       map[llm.Capability]bool
	promptCacheEnabled bool
}

// newAnthropic creates a new Anthropic model provider
func newAnthropic(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	if apiKey == "" {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: "apiKey is required for Anthropic provider",
		}
	}

	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	promptCacheEnabled := true
	if opts != nil {
		if v, ok := opts["prompt_cache_enabled"]; ok {
			promptCacheEnabled, _ = v.(bool)
		}
	}

	return &anthropicModel{
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

func (m *anthropicModel) ID() string {
	return m.modelID
}

func (m *anthropicModel) Supports(cap llm.Capability) bool {
	return m.capabilities[cap]
}

// anthropicHeaders returns the standard headers for Anthropic API calls
func (m *anthropicModel) anthropicHeaders() map[string]string {
	return map[string]string{
		"x-api-key":         m.apiKey,
		"anthropic-version": "2023-06-01",
		"Content-Type":      "application/json",
	}
}

// Generate sends a synchronous request and returns the full response
func (m *anthropicModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	system, anthropicMessages := toAnthropicMessages(req.Messages)
	systemBlocks, systemText, anthropicMessages := markAnthropicMessagesForCache(system, anthropicMessages, m.promptCacheEnabled)
	body := buildAnthropicRequest(m.modelID, req, systemBlocks, systemText, anthropicMessages, false)

	url := m.baseURL + "/v1/messages"
	respBody, statusCode, err := jsonRequest(ctx, m.client, url, m.anthropicHeaders(), body)
	if err != nil {
		return nil, err
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "anthropic")
	}

	return fromAnthropicResponse(respBody)
}

// Stream sends a streaming request and returns a channel of stream events
func (m *anthropicModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	system, anthropicMessages := toAnthropicMessages(req.Messages)
	systemBlocks, systemText, anthropicMessages := markAnthropicMessagesForCache(system, anthropicMessages, m.promptCacheEnabled)
	body := buildAnthropicRequest(m.modelID, req, systemBlocks, systemText, anthropicMessages, true)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := m.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	for k, v := range m.anthropicHeaders() {
		httpReq.Header.Set(k, v)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, respBody, "anthropic")
	}

	return parseAnthropicStream(ctx, resp), nil
}

// toAnthropicMessages converts llm.ChatMessage slice to Anthropic API format.
// Returns the system prompt string and the messages array.
func toAnthropicMessages(msgs []llm.ChatMessage) (system string, messages []map[string]any) {
	var systemParts []string
	var result []map[string]any

	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			for _, part := range msg.Content {
				systemParts = append(systemParts, part.Text)
			}

		case llm.RoleUser:
			result = append(result, buildUserContent(msg))

		case llm.RoleAssistant:
			result = append(result, buildAssistantContent(msg))

		case llm.RoleTool:
			// Tool results become user role messages with tool_result content blocks
			toolResultBlock := buildToolResultBlock(msg)

			// Check if the last message is a user message that contains only tool_results
			// If so, merge consecutive tool results
			if len(result) > 0 {
				last := result[len(result)-1]
				if last["role"] == "user" {
					if lastContent, ok := last["content"].([]map[string]any); ok {
						// Check if all entries are tool_result type
						allToolResults := len(lastContent) > 0
						for _, c := range lastContent {
							if c["type"] != "tool_result" {
								allToolResults = false
								break
							}
						}
						if allToolResults {
							// Merge: append this tool_result to the existing content array
							last["content"] = append(lastContent, toolResultBlock)
							continue
						}
					}
				}
			}

			// Start a new tool_result-only user message
			result = append(result, map[string]any{
				"role":    "user",
				"content": []map[string]any{toolResultBlock},
			})
		}
	}

	system = strings.Join(systemParts, "\n")
	messages = result
	return
}

// buildUserContent creates an Anthropic user message from an llm message
func buildUserContent(msg llm.ChatMessage) map[string]any {
	// Check if message has image content
	hasImage := false
	for _, part := range msg.Content {
		if part.Type == llm.ContentTypeImage {
			hasImage = true
			break
		}
	}

	if !hasImage {
		// Simple text-only message
		var textBuilder strings.Builder
		for _, part := range msg.Content {
			textBuilder.WriteString(part.Text)
		}
		return map[string]any{
			"role":    "user",
			"content": textBuilder.String(),
		}
	}

	// Multi-modal message with images
	var contentBlocks []map[string]any
	for _, part := range msg.Content {
		switch part.Type {
		case llm.ContentTypeText:
			contentBlocks = append(contentBlocks, map[string]any{
				"type": "text",
				"text": part.Text,
			})
		case llm.ContentTypeImage:
			mediaType, data := parseImageURL(part.ImageURL)
			contentBlocks = append(contentBlocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mediaType,
					"data":       data,
				},
			})
		}
	}

	return map[string]any{
		"role":    "user",
		"content": contentBlocks,
	}
}

// parseImageURL extracts media type and base64 data from a data URI
func parseImageURL(imageURL string) (mediaType, data string) {
	// Expected format: data:image/jpeg;base64,/9j/4AAQ...
	if strings.HasPrefix(imageURL, "data:") {
		// Split at the first comma
		if idx := strings.Index(imageURL, ","); idx >= 0 {
			header := imageURL[5:idx] // skip "data:"
			data = imageURL[idx+1:]
			// header is like "image/jpeg;base64" or "image/png;base64"
			if semiIdx := strings.Index(header, ";"); semiIdx >= 0 {
				mediaType = header[:semiIdx]
			} else {
				mediaType = header
			}
			return
		}
	}
	// Fallback
	return "image/png", imageURL
}

// buildAssistantContent creates an Anthropic assistant message from an llm message
func buildAssistantContent(msg llm.ChatMessage) map[string]any {
	hasText := false
	for _, part := range msg.Content {
		if part.Text != "" {
			hasText = true
			break
		}
	}

	hasToolCalls := len(msg.ToolCalls) > 0

	if !hasText && !hasToolCalls {
		// Empty assistant message - Anthropic still needs content
		return map[string]any{
			"role":    "assistant",
			"content": "",
		}
	}

	if hasText && !hasToolCalls {
		// Simple text-only message
		var textBuilder strings.Builder
		for _, part := range msg.Content {
			textBuilder.WriteString(part.Text)
		}
		return map[string]any{
			"role":    "assistant",
			"content": textBuilder.String(),
		}
	}

	// Combined text + tool_use, or just tool_use
	var contentBlocks []map[string]any

	// Add text content if present
	var textBuilder strings.Builder
	for _, part := range msg.Content {
		textBuilder.WriteString(part.Text)
	}
	if textBuilder.Len() > 0 {
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "text",
			"text": textBuilder.String(),
		})
	}

	// Add tool_use blocks
	for _, tc := range msg.ToolCalls {
		// Parse ArgsJSON into a map
		var input map[string]any
		if err := json.Unmarshal([]byte(tc.ArgsJSON), &input); err != nil {
			input = map[string]any{}
		}
		contentBlocks = append(contentBlocks, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": input,
		})
	}

	return map[string]any{
		"role":    "assistant",
		"content": contentBlocks,
	}
}

// buildToolResultBlock creates a tool_result content block for Anthropic
func buildToolResultBlock(msg llm.ChatMessage) map[string]any {
	var contentBuilder strings.Builder
	for _, part := range msg.Content {
		contentBuilder.WriteString(part.Text)
	}

	return map[string]any{
		"type":        "tool_result",
		"tool_use_id": msg.ToolCallID,
		"content":     contentBuilder.String(),
	}
}

// toAnthropicTools converts tool definitions to Anthropic API format.
// Anthropic tools have no "function" wrapper — just name, description, input_schema directly.
func toAnthropicTools(tools []llm.ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var inputSchema map[string]any
		if t.Parameters != nil {
			if err := json.Unmarshal(t.Parameters, &inputSchema); err != nil {
				inputSchema = map[string]any{}
			}
		}

		result = append(result, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": inputSchema,
		})
	}
	return result
}

// buildAnthropicRequest builds the JSON request body for the Anthropic Messages API
func buildAnthropicRequest(modelID string, req *llm.Request, systemBlocks []map[string]any, systemText string, messages []map[string]any, stream bool) map[string]any {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	result := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"stream":     stream,
		"messages":   messages,
	}

	if len(systemBlocks) > 0 {
		// 缓存启用时，system 使用对象数组格式
		result["system"] = systemBlocks
	} else if systemText != "" {
		// 缓存未启用时，system 使用普通字符串格式
		result["system"] = systemText
	}

	if tools := toAnthropicTools(req.Tools); len(tools) > 0 {
		result["tools"] = tools
	}

	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}

	if len(req.Stop) > 0 {
		result["stop_sequences"] = req.Stop
	}

	return result
}

// fromAnthropicResponse parses a sync Anthropic API response into llm.Response
func fromAnthropicResponse(body []byte) (*llm.Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string          `json:"type"`
			Text string          `json:"text,omitempty"`
			ID   string          `json:"id,omitempty"`
			Name string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens            int `json:"input_tokens"`
			OutputTokens           int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	usage := llm.Usage{
		PromptTokens:              raw.Usage.InputTokens,
		CompletionTokens:          raw.Usage.OutputTokens,
		TotalTokens:               raw.Usage.InputTokens + raw.Usage.OutputTokens,
		CacheCreationInputTokens:  raw.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:      raw.Usage.CacheReadInputTokens,
	}

	var textBuilder strings.Builder
	var toolCalls []llm.ToolCall

	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			textBuilder.WriteString(block.Text)
		case "tool_use":
			var argsMap map[string]any
			if block.Input != nil {
				if err := json.Unmarshal(block.Input, &argsMap); err != nil {
					argsMap = map[string]any{}
				}
			}
			argsJSON, _ := json.Marshal(argsMap)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:       block.ID,
				Name:     block.Name,
				ArgsJSON: string(argsJSON),
			})
		}
	}

	var finishReason string
	switch raw.StopReason {
	case "end_turn":
		finishReason = "stop"
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	case "stop_sequence":
		finishReason = "stop"
	default:
		finishReason = raw.StopReason
	}

	message := llm.ChatMessage{
		Role:      llm.RoleAssistant,
		ToolCalls: toolCalls,
	}
	if textBuilder.Len() > 0 {
		message.Content = []llm.ContentPart{
			{Type: llm.ContentTypeText, Text: textBuilder.String()},
		}
	}

	return &llm.Response{
		Message:      message,
		Usage:        usage,
		FinishReason: finishReason,
	}, nil
}

// parseAnthropicStream parses Server-Sent Events from Anthropic streaming API
func parseAnthropicStream(ctx context.Context, resp *http.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for potentially long lines
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var currentEventType string
		// Track tool_use blocks by index for delta accumulation
		type toolBlockState struct {
			Index int
			ID    string
			Name  string
			Args  strings.Builder
		}
		var toolBlocks []*toolBlockState

		for scanner.Scan() {
			line := scanner.Text()

			// Check for context cancellation
			select {
			case <-ctx.Done():
				ch <- llm.StreamEvent{
					Type:  llm.StreamEventDone,
					Error: ctx.Err(),
				}
				return
			default:
			}

			if strings.HasPrefix(line, "event: ") {
				currentEventType = strings.TrimPrefix(line, "event: ")
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if data == "" {
					continue
				}

				switch currentEventType {
				case "message_start":
					var msgStart struct {
						Type    string `json:"type"`
						Message struct {
							Usage struct {
								InputTokens              int `json:"input_tokens"`
								OutputTokens             int `json:"output_tokens"`
								CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
								CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
							} `json:"usage"`
						} `json:"message"`
					}
					if err := json.Unmarshal([]byte(data), &msgStart); err == nil {
						if msgStart.Message.Usage.InputTokens > 0 || msgStart.Message.Usage.OutputTokens > 0 {
							usage := &llm.Usage{
								PromptTokens:              msgStart.Message.Usage.InputTokens,
								CompletionTokens:          msgStart.Message.Usage.OutputTokens,
								TotalTokens:               msgStart.Message.Usage.InputTokens + msgStart.Message.Usage.OutputTokens,
								CacheCreationInputTokens:  msgStart.Message.Usage.CacheCreationInputTokens,
								CacheReadInputTokens:      msgStart.Message.Usage.CacheReadInputTokens,
							}
							ch <- llm.StreamEvent{
								Type:  llm.StreamEventUsage,
								Usage: usage,
							}
						}
					}
					// Reset state
					toolBlocks = nil

				case "content_block_start":
					var blockStart struct {
						Index        int             `json:"index"`
						ContentBlock json.RawMessage `json:"content_block"`
					}
					if err := json.Unmarshal([]byte(data), &blockStart); err != nil {
						continue
					}

					// Parse content_block to determine type
					var typeOnly struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(blockStart.ContentBlock, &typeOnly); err != nil {
						continue
					}

					if typeOnly.Type == "tool_use" {
						var toolBlock struct {
							Type string `json:"type"`
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						if err := json.Unmarshal(blockStart.ContentBlock, &toolBlock); err != nil {
							continue
						}
						tb := &toolBlockState{
							Index: blockStart.Index,
							ID:    toolBlock.ID,
							Name:  toolBlock.Name,
						}
						toolBlocks = append(toolBlocks, tb)
					} else if typeOnly.Type == "text" {
						// Emit initial text from content_block_start
						var textBlock struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}
						if err := json.Unmarshal(blockStart.ContentBlock, &textBlock); err == nil && textBlock.Text != "" {
							ch <- llm.StreamEvent{
								Type:  llm.StreamEventText,
								Delta: textBlock.Text,
							}
						}
					}

				case "content_block_delta":
					var delta struct {
						Index int             `json:"index"`
						Delta json.RawMessage `json:"delta"`
					}
					if err := json.Unmarshal([]byte(data), &delta); err != nil {
						continue
					}

					var deltaType struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(delta.Delta, &deltaType); err != nil {
						continue
					}

					switch deltaType.Type {
					case "text_delta":
						var textDelta struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}
						if err := json.Unmarshal(delta.Delta, &textDelta); err != nil {
							continue
						}
						ch <- llm.StreamEvent{
							Type:  llm.StreamEventText,
							Delta: textDelta.Text,
						}

					case "input_json_delta":
						var inputDelta struct {
							Type        string `json:"type"`
							PartialJSON string `json:"partial_json"`
						}
						if err := json.Unmarshal(delta.Delta, &inputDelta); err != nil {
							continue
						}
						// Find the matching tool block
						for _, tb := range toolBlocks {
							if tb.Index == delta.Index {
								tb.Args.WriteString(inputDelta.PartialJSON)
								ch <- llm.StreamEvent{
									Type: llm.StreamEventToolCall,
									ToolCall: &llm.ToolCallDelta{
										Index:    tb.Index,
										ID:       tb.ID,
										Name:     tb.Name,
										ArgsJSON: inputDelta.PartialJSON,
									},
								}
								break
							}
						}
					}

				case "content_block_stop":
					var blockStop struct {
						Index int `json:"index"`
					}
					if err := json.Unmarshal([]byte(data), &blockStop); err != nil {
						continue
					}

					// Find the matching tool block and emit complete tool call
					for _, tb := range toolBlocks {
						if tb.Index == blockStop.Index {
							ch <- llm.StreamEvent{
								Type: llm.StreamEventToolCall,
								ToolCall: &llm.ToolCallDelta{
									Index:    tb.Index,
									ID:       tb.ID,
									Name:     tb.Name,
									ArgsJSON: tb.Args.String(),
									Complete: true,
								},
							}
							break
						}
					}

				case "message_delta":
					var msgDelta struct {
						Type  string `json:"type"`
						Delta struct {
							StopReason   string `json:"stop_reason"`
							StopSequence string `json:"stop_sequence"`
						} `json:"delta"`
						Usage struct {
							OutputTokens int `json:"output_tokens"`
						} `json:"usage"`
					}
					if err := json.Unmarshal([]byte(data), &msgDelta); err == nil {
						if msgDelta.Usage.OutputTokens > 0 {
							ch <- llm.StreamEvent{
								Type: llm.StreamEventUsage,
								Usage: &llm.Usage{
									CompletionTokens: msgDelta.Usage.OutputTokens,
								},
							}
						}
					}

				case "message_stop":
					ch <- llm.StreamEvent{
						Type: llm.StreamEventDone,
					}
					return

				case "ping":
					// Heartbeat, ignore

				default:
					// Unknown event type, ignore
				}

				continue
			}

			// Handle potential error events in the data stream
			if currentEventType == "" && strings.HasPrefix(line, "{") {
				// Check if this is an error response
				var errResponse struct {
					Type  string `json:"type"`
					Error struct {
						Type    string `json:"type"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(line), &errResponse); err == nil && errResponse.Type == "error" {
					ch <- llm.StreamEvent{
						Type: llm.StreamEventDone,
						Error: &llm.Error{
							Type:    mapAnthropicErrorType(errResponse.Error.Type),
							Message: errResponse.Error.Message,
						},
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- llm.StreamEvent{
				Type:  llm.StreamEventDone,
				Error: fmt.Errorf("stream read error: %w", err),
			}
			return
		}

		// If we exit without message_stop, emit done
		ch <- llm.StreamEvent{
			Type: llm.StreamEventDone,
		}
	}()

	return ch
}

// mapAnthropicErrorType maps Anthropic error types to llm.ErrorType
func mapAnthropicErrorType(anthropicType string) llm.ErrorType {
	switch anthropicType {
	case "rate_limit_error":
		return llm.ErrorTypeRateLimit
	case "invalid_request_error":
		return llm.ErrorTypeContextOverflow
	case "authentication_error":
		return llm.ErrorTypeAuth
	case "permission_error":
		return llm.ErrorTypeAuth
	case "not_found_error":
		return llm.ErrorTypeModelNotFound
	case "api_error", "overloaded_error":
		return llm.ErrorTypeInternal
	default:
		return llm.ErrorTypeInternal
	}
}