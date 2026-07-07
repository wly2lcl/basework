package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Default Gemini API base URL
const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

// geminiModel implements llm.Model for Google Gemini API
type geminiModel struct {
	baseURL            string
	apiKey             string
	modelID            string
	client             *http.Client
	capabilities       map[llm.Capability]bool
	promptCacheEnabled bool
}

// newGemini creates a new Gemini model provider
func newGemini(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	if apiKey == "" {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: "Gemini API key is required",
		}
	}

	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}

	capabilities := map[llm.Capability]bool{
		llm.CapTools:     true,
		llm.CapVision:    true,
		llm.CapStreaming: true,
	}

	promptCacheEnabled := true
	if opts != nil {
		if v, ok := opts["prompt_cache_enabled"]; ok {
			promptCacheEnabled, _ = v.(bool)
		}
	}

	return &geminiModel{
		baseURL:            strings.TrimRight(baseURL, "/"),
		apiKey:            apiKey,
		modelID:           modelID,
		client:            newHTTPClient(60 * time.Second),
		capabilities:      capabilities,
		promptCacheEnabled: promptCacheEnabled,
	}, nil
}

// ID returns the model ID
func (m *geminiModel) ID() string {
	return m.modelID
}

// Supports checks if the model supports a given capability
func (m *geminiModel) Supports(cap llm.Capability) bool {
	return m.capabilities[cap]
}

// Generate sends a non-streaming request to the Gemini API
func (m *geminiModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", m.baseURL, m.modelID, m.apiKey)

	contents, systemInstruction := toGeminiContents(req.Messages, m.promptCacheEnabled)
	body := buildGeminiRequest(req, systemInstruction, m.promptCacheEnabled)
	body["contents"] = contents

	respBody, statusCode, err := jsonRequest(ctx, m.client, url, nil, body)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "gemini")
	}

	return fromGeminiResponse(respBody)
}

// Stream sends a streaming request to the Gemini API and returns a channel of events
func (m *geminiModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", m.baseURL, m.modelID, m.apiKey)

	contents, systemInstruction := toGeminiContents(req.Messages, m.promptCacheEnabled)
	body := buildGeminiRequest(req, systemInstruction, m.promptCacheEnabled)
	body["contents"] = contents

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("gemini create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini stream request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, respBody, "gemini")
	}

	return parseGeminiStream(ctx, resp), nil
}

// toGeminiContents converts internal ChatMessage slice to Gemini API contents array
// and extracts system instruction.
func toGeminiContents(msgs []llm.ChatMessage, cacheEnabled bool) ([]map[string]any, string) {
	var contents []map[string]any
	var systemParts []string
	userContentCount := 0

	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText && part.Text != "" {
					systemParts = append(systemParts, part.Text)
				}
			}

		case llm.RoleUser:
			var parts []map[string]any
			for _, part := range msg.Content {
				switch part.Type {
				case llm.ContentTypeText:
					if part.Text != "" {
						parts = append(parts, map[string]any{"text": part.Text})
					}
				case llm.ContentTypeImage:
					mimeType, data := parseImageDataURL(part.ImageURL)
					parts = append(parts, map[string]any{
						"inlineData": map[string]any{
							"mimeType": mimeType,
							"data":     data,
						},
					})
				}
			}
			if len(parts) > 0 {
				content := map[string]any{
					"role":  "user",
					"parts": parts,
				}
				// 缓存标记：前 maxUserContentsToCache 条 user content 添加 cachedContent
				if cacheEnabled && userContentCount < maxUserContentsToCache {
					content["cachedContent"] = true
					userContentCount++
				}
				contents = append(contents, content)
			}

		case llm.RoleAssistant:
			var parts []map[string]any
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText && part.Text != "" {
					parts = append(parts, map[string]any{"text": part.Text})
				}
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.ArgsJSON), &args); err != nil {
					args = map[string]any{}
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": args,
					},
				})
			}
			if len(parts) == 0 {
				parts = append(parts, map[string]any{"text": ""})
			}
			contents = append(contents, map[string]any{
				"role":  "model",
				"parts": parts,
			})

		case llm.RoleTool:
			content := ""
			if len(msg.Content) > 0 {
				content = msg.Content[0].Text
			}
			toolName := msg.Name
			if toolName == "" {
				toolName = "unknown_tool"
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name": toolName,
							"response": map[string]any{
								"content": content,
							},
						},
					},
				},
			})
		}
	}

	systemInstruction := strings.Join(systemParts, "\n")
	return contents, systemInstruction
}

// toGeminiTools converts internal ToolDefinition slice to Gemini API tools format.
func toGeminiTools(tools []llm.ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	var declarations []map[string]any
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = json.RawMessage(`{"type": "object"}`)
		}
		var paramsMap map[string]any
		if err := json.Unmarshal(params, &paramsMap); err != nil {
			paramsMap = map[string]any{"type": "object"}
		}

		declarations = append(declarations, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  paramsMap,
		})
	}

	return []map[string]any{
		{"functionDeclarations": declarations},
	}
}

// buildGeminiRequest constructs the Gemini API request body from an internal request.
func buildGeminiRequest(req *llm.Request, systemInstruction string, cacheEnabled bool) map[string]any {
	body := map[string]any{}

	if systemInstruction != "" {
		sysParts := []map[string]any{
			{"text": systemInstruction},
		}
		if cacheEnabled {
			sysParts[0]["cachedContent"] = true
		}
		body["systemInstruction"] = map[string]any{
			"parts": sysParts,
		}
	}

	if len(req.Tools) > 0 {
		body["tools"] = toGeminiTools(req.Tools)
	}

	genConfig := map[string]any{}
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if len(req.Stop) > 0 {
		genConfig["stopSequences"] = req.Stop
	}
	if len(genConfig) > 0 {
		body["generationConfig"] = genConfig
	}

	return body
}

// fromGeminiResponse parses a Gemini API response into an internal Response.
func fromGeminiResponse(body []byte) (*llm.Response, error) {
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("gemini parse response failed: %w", err)
	}

	// Check for error in response
	if errObj, ok := result["error"]; ok {
		errMap, _ := errObj.(map[string]any)
		msg, _ := errMap["message"].(string)
		return nil, &llm.Error{
			Type:          llm.ErrorTypeInternal,
			Message:       "gemini API error",
			ProviderError: msg,
		}
	}

	resp := &llm.Response{
		Message: llm.ChatMessage{
			Role: llm.RoleAssistant,
		},
	}

	// Parse candidates
	candidates, _ := result["candidates"].([]any)
	if len(candidates) > 0 {
		candidate, _ := candidates[0].(map[string]any)

		// Parse finish reason
		if finishReason, ok := candidate["finishReason"].(string); ok {
			resp.FinishReason = mapGeminiFinishReason(finishReason)
		}

		// Parse content
		content, _ := candidate["content"].(map[string]any)
		if content != nil {
			parts, _ := content["parts"].([]any)
			for i, p := range parts {
				part, _ := p.(map[string]any)

				if text, ok := part["text"].(string); ok && text != "" {
					resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
						Type: llm.ContentTypeText,
						Text: text,
					})
				}

				if fc, ok := part["functionCall"]; ok {
					fcMap, _ := fc.(map[string]any)
					name, _ := fcMap["name"].(string)
					args, _ := fcMap["args"].(map[string]any)

					argsJSON, _ := json.Marshal(args)
					id := fmt.Sprintf("call_%s_%d", name, i)

					resp.Message.ToolCalls = append(resp.Message.ToolCalls, llm.ToolCall{
						ID:       id,
						Name:     name,
						ArgsJSON: string(argsJSON),
					})
				}
			}
		}
	}

	// Parse usage metadata
	usageMeta, _ := result["usageMetadata"].(map[string]any)
	if usageMeta != nil {
		promptTokens, _ := toInt(usageMeta["promptTokenCount"])
		candidateTokens, _ := toInt(usageMeta["candidatesTokenCount"])
		totalTokens, _ := toInt(usageMeta["totalTokenCount"])
		resp.Usage = llm.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: candidateTokens,
			TotalTokens:      totalTokens,
		}
	}

	return resp, nil
}

// parseGeminiStream parses Gemini SSE stream and returns a channel of stream events.
func parseGeminiStream(ctx context.Context, resp *http.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for potentially large Gemini responses
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		accumulatedText := ""

		for scanner.Scan() {
			line := scanner.Text()

			// Skip non-data lines
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			// Skip empty data
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			var chunk map[string]any
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// Check for error in chunk
			if errObj, ok := chunk["error"]; ok {
				errMap, _ := errObj.(map[string]any)
				msg, _ := errMap["message"].(string)
				ch <- llm.StreamEvent{
					Error: &llm.Error{
					Type:          llm.ErrorTypeInternal,
					Message:       "gemini stream error",
					ProviderError: msg,
				},
			}
			return
		}

			// Parse usage metadata (usually in the final chunk)
			if usageMeta, ok := chunk["usageMetadata"].(map[string]any); ok {
				promptTokens, _ := toInt(usageMeta["promptTokenCount"])
				candidateTokens, _ := toInt(usageMeta["candidatesTokenCount"])
				totalTokens, _ := toInt(usageMeta["totalTokenCount"])
				ch <- llm.StreamEvent{
					Type: llm.StreamEventUsage,
					Usage: &llm.Usage{
						PromptTokens:     promptTokens,
						CompletionTokens: candidateTokens,
						TotalTokens:      totalTokens,
					},
				}
			}

			// Parse candidates
			candidates, _ := chunk["candidates"].([]any)
			if len(candidates) == 0 {
				continue
			}

			candidate, _ := candidates[0].(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			if content == nil {
				continue
			}

			parts, _ := content["parts"].([]any)
			currentText := ""

			for i, p := range parts {
				part, _ := p.(map[string]any)

				if text, ok := part["text"].(string); ok && text != "" {
					currentText = text
				}

				if fc, ok := part["functionCall"]; ok {
					fcMap, _ := fc.(map[string]any)
					name, _ := fcMap["name"].(string)
					args, _ := fcMap["args"].(map[string]any)

					argsJSON, _ := json.Marshal(args)
					id := fmt.Sprintf("call_%s_%d", name, i)

					ch <- llm.StreamEvent{
						Type: llm.StreamEventToolCall,
						ToolCall: &llm.ToolCallDelta{
							Index:    i,
							ID:       id,
							Name:     name,
							ArgsJSON: string(argsJSON),
							Complete: true,
						},
					}
				}
			}

			// Emit text delta
			if currentText != "" {
				delta := ""
				if strings.HasPrefix(currentText, accumulatedText) {
					delta = strings.TrimPrefix(currentText, accumulatedText)
				} else if currentText != accumulatedText {
					// If text changed entirely, emit the full text
					delta = currentText
				}
				if delta != "" {
					ch <- llm.StreamEvent{
						Type:  llm.StreamEventText,
						Delta: delta,
					}
				}
				accumulatedText = currentText
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- llm.StreamEvent{
				Error: &llm.Error{
					Type:    llm.ErrorTypeNetwork,
					Message: fmt.Sprintf("gemini stream read error: %v", err),
				},
			}
			return
		}

		// Signal done
		ch <- llm.StreamEvent{
			Type: llm.StreamEventDone,
		}
	}()

	return ch
}

// parseImageDataURL parses a data URL into mime type and base64 data.
// Supports formats like: data:image/png;base64,iVBOR...
func parseImageDataURL(url string) (string, string) {
	if strings.HasPrefix(url, "data:") {
		parts := strings.SplitN(url, ",", 2)
		if len(parts) == 2 {
			header := parts[0]
			data := parts[1]
			// Extract mime type: data:image/png;base64
			mimeType := strings.TrimPrefix(header, "data:")
			if idx := strings.Index(mimeType, ";"); idx >= 0 {
				mimeType = mimeType[:idx]
			}
			if mimeType == "" {
				mimeType = "image/png"
			}
			return mimeType, data
		}
	}
	// Assume it's raw base64 data
	return "image/png", url
}

// mapGeminiFinishReason maps Gemini finish reasons to internal format.
func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	case "OTHER":
		return "other"
	default:
		return reason
	}
}

// toInt converts a value from JSON unmarshaling to int.
func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case int64:
		return int(val), true
	default:
		return 0, false
	}
}