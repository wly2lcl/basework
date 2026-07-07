package provider

import (
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// contextOverflowKeywords are checked against the response body to detect context overflow errors
var contextOverflowKeywords = []string{
	"context_length_exceeded",
	"maximum context length",
	"too many tokens",
	"prompt is too long",
	"prompt_too_long",
	"token limit",
	"content too long",
	"reduce your prompt",
}

// mapHTTPError maps an HTTP status code and optional response body to an llm.Error
func mapHTTPError(statusCode int, body []byte, providerName string) *llm.Error {
	bodyStr := string(body)

	// Check for context overflow first (can be any status code)
	for _, keyword := range contextOverflowKeywords {
		if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(keyword)) {
			return &llm.Error{
				Type:          llm.ErrorTypeContextOverflow,
				Message:       "context length exceeded",
				StatusCode:    statusCode,
				ProviderError: bodyStr,
			}
		}
	}

	// Map by status code
	switch statusCode {
	case 401:
		return &llm.Error{Type: llm.ErrorTypeAuth, Message: "authentication failed", StatusCode: statusCode, ProviderError: bodyStr}
	case 403:
		return &llm.Error{Type: llm.ErrorTypeAuth, Message: "forbidden", StatusCode: statusCode, ProviderError: bodyStr}
	case 404:
		return &llm.Error{Type: llm.ErrorTypeModelNotFound, Message: "model not found", StatusCode: statusCode, ProviderError: bodyStr}
	case 429:
		return &llm.Error{Type: llm.ErrorTypeRateLimit, Message: "rate limit exceeded", StatusCode: statusCode, ProviderError: bodyStr}
	case 500, 502, 503, 504:
		return &llm.Error{Type: llm.ErrorTypeInternal, Message: "provider internal error", StatusCode: statusCode, ProviderError: bodyStr}
	default:
		return &llm.Error{Type: llm.ErrorTypeInternal, Message: "unexpected error", StatusCode: statusCode, ProviderError: bodyStr}
	}
}
