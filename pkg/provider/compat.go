package provider

import (
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

// compatModel implements llm.Model by reusing OpenAI's format/SSE logic
type compatModel struct {
	baseURL      string
	apiKey       string
	modelID      string
	client       *http.Client
	capabilities map[llm.Capability]bool
	extraHeaders map[string]string // custom headers from Config.Options["headers"]
}

// newOpenAICompat creates an OpenAI-compatible model
// cfg 和 baseURL 来自 factory.go 的 Create()
func newOpenAICompat(cfg Config, baseURL string) (llm.Model, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("OpenAI-compatible provider %q requires a BaseURL", cfg.Type)
	}

	// Extract custom headers from options
	var extraHeaders map[string]string
	if cfg.Options != nil {
		if h, ok := cfg.Options["headers"].(map[string]any); ok {
			extraHeaders = make(map[string]string, len(h))
			for k, v := range h {
				extraHeaders[k] = fmt.Sprint(v)
			}
		}
	}

	return &compatModel{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       cfg.APIKey,
		modelID:      cfg.ModelID,
		client:       newHTTPClient(10 * time.Minute),
		capabilities: map[llm.Capability]bool{llm.CapTools: true, llm.CapVision: true, llm.CapStreaming: true},
		extraHeaders: extraHeaders,
	}, nil
}

// ID returns the model identifier
func (m *compatModel) ID() string {
	return m.modelID
}

// Supports checks if the model supports the given capability
func (m *compatModel) Supports(cap llm.Capability) bool {
	return m.capabilities[cap]
}

// buildHeaders 构建包含 Authorization 和自定义头的请求头
func (m *compatModel) buildHeaders() map[string]string {
	headers := map[string]string{}
	if m.apiKey != "" {
		headers["Authorization"] = "Bearer " + m.apiKey
	}
	for k, v := range m.extraHeaders {
		headers[k] = v
	}
	return headers
}

// Generate sends a non-streaming chat completion request
func (m *compatModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := buildOpenAIRequest(req, false, m.modelID, false)
	headers := m.buildHeaders()

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
		return nil, mapHTTPError(statusCode, respBody, "openai-compat")
	}

	return fromOpenAIResponse(respBody)
}

// Stream sends a streaming chat completion request and returns a channel of events
func (m *compatModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	body := buildOpenAIRequest(req, true, m.modelID, false)

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

	setHeaders(httpReq, m.apiKey, m.extraHeaders)
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
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "openai-compat")
	}

	return parseOpenAIStream(ctx, resp), nil
}
