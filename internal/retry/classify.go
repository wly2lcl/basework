// Package retry 提供重试机制，包括错误分类、指数退避和重试包装器。
package retry

import (
	"errors"
	"net"
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// IsRetryable 判断错误是否可重试。
//
// 可重试的错误：
//   - 网络超时（net.Error.Timeout）
//   - 5xx 服务器错误
//   - 速率限制错误（429）
//
// 不可重试的错误：
//   - 4xx 客户端错误（除 429 外）
//   - 认证失败
//   - 模型不存在
//   - 上下文溢出
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 检查网络超时错误（如 DNS、TCP 超时）
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// 检查临时性网络错误
	if errors.As(err, &netErr) && netErr.Temporary() {
		return true
	}

	// 检查 LLM 错误类型
	var llmErr *llm.Error
	if errors.As(err, &llmErr) {
		return isLLMErrorRetryable(llmErr)
	}

	// 检查错误消息中是否包含超时关键字（兜底）
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "timeout") ||
		strings.Contains(strings.ToLower(msg), "temporary") {
		return true
	}

	return false
}

// isLLMErrorRetryable 判断 LLM 错误是否可重试。
func isLLMErrorRetryable(err *llm.Error) bool {
	switch err.Type {
	case llm.ErrorTypeRateLimit:
		// 速率限制（429）可重试
		return true

	case llm.ErrorTypeNetwork:
		// 网络错误可重试
		return true

	case llm.ErrorTypeInternal:
		// 5xx 错误可重试
		return err.StatusCode >= 500

	case llm.ErrorTypeAuth:
		// 认证失败不可重试
		return false

	case llm.ErrorTypeModelNotFound:
		// 模型不存在不可重试
		return false

	case llm.ErrorTypeContextOverflow:
		// 上下文溢出不可重试
		return false

	default:
		// 根据 HTTP 状态码兜底判断
		if err.StatusCode == 429 {
			return true
		}
		if err.StatusCode >= 500 {
			return true
		}
		if err.StatusCode >= 400 && err.StatusCode < 500 {
			return false
		}
		// 无状态码时默认不可重试
		return false
	}
}