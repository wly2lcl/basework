package llm

import (
	"errors"
	"fmt"
)

// ErrorType 错误类型
type ErrorType string

const (
	ErrorTypeRateLimit       ErrorType = "rate_limit"
	ErrorTypeContextOverflow ErrorType = "context_overflow"
	ErrorTypeAuth            ErrorType = "auth"
	ErrorTypeNetwork         ErrorType = "network"
	ErrorTypeModelNotFound   ErrorType = "model_not_found"
	ErrorTypeInternal        ErrorType = "internal"
)

// Error 是 LLM 提供商的错误
type Error struct {
	Type          ErrorType
	Message       string
	StatusCode    int
	ProviderError string // 原始 provider 错误信息
}

// Error implements the error interface
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s (status=%d)", e.Type, e.Message, e.StatusCode)
}

// Unwrap provides compatibility with errors.As
func (e *Error) Unwrap() error {
	return nil
}

// IsContextOverflow 检查错误是否为上下文溢出
func IsContextOverflow(err error) bool {
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr.Type == ErrorTypeContextOverflow
	}
	return false
}

// IsRateLimit 检查错误是否为速率限制
func IsRateLimit(err error) bool {
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr.Type == ErrorTypeRateLimit
	}
	return false
}
