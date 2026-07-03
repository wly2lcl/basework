package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Config 是重试机制的配置。
type Config struct {
	Enabled     bool          `json:"enabled"`
	MaxAttempts int           `json:"max_attempts"` // 最大重试次数，默认 3
	BaseDelay   time.Duration `json:"base_delay"`   // 基础延迟，默认 2s
	MaxDelay    time.Duration `json:"max_delay"`    // 最大延迟，默认 60s
}

// DefaultConfig 返回默认重试配置。
//
// 默认值：
//   - Enabled: true
//   - MaxAttempts: 3
//   - BaseDelay: 2s
//   - MaxDelay: 60s
func DefaultConfig() Config {
	return Config{
		Enabled:     true,
		MaxAttempts: 3,
		BaseDelay:   2 * time.Second,
		MaxDelay:    60 * time.Second,
	}
}

// RetryAfter 接口用于从错误中提取 Retry-After 信息。
// 如果错误实现了此接口，重试包装器会优先使用指定的延迟时间。
type RetryAfter interface {
	RetryAfter() time.Duration
}

// retryAfterError 是实现了 RetryAfter 接口的错误类型。
type retryAfterError struct {
	duration time.Duration
}

// RetryAfter 返回 Retry-After 延迟时间。
func (e *retryAfterError) RetryAfter() time.Duration {
	return e.duration
}

func (e *retryAfterError) Error() string {
	return fmt.Sprintf("retry after %s", e.duration)
}

// NewRetryAfterError 创建一个携带 Retry-After 信息的错误。
func NewRetryAfterError(d time.Duration) error {
	return &retryAfterError{duration: d}
}

// WithRetry 使用指数退避重试执行函数 fn。
//
// 参数：
//   - ctx: 上下文，支持取消和超时
//   - config: 重试配置
//   - fn: 要执行的函数
//
// 行为：
//   - 如果重试被禁用（config.Enabled == false），直接执行 fn 并返回结果
//   - 遇到不可重试错误立即返回，不再重试
//   - 达到最大重试次数返回最后一次的错误
//   - context 取消或超时时立即返回 context 错误
//   - 如果错误实现了 RetryAfter 接口，优先使用其指定的延迟时间
func WithRetry(ctx context.Context, config Config, fn func() error) error {
	// 重试被禁用，直接执行
	if !config.Enabled {
		return fn()
	}

	var lastErr error

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// 每次重试前检查 context 是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 执行目标函数
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否为不可重试错误
		if !IsRetryable(err) {
			return err
		}

		// 已用完重试次数，返回最后错误
		if attempt >= config.MaxAttempts-1 {
			break
		}

		// 计算退避时间
		delay := Backoff(attempt, config.BaseDelay, config.MaxDelay)

		// 若错误携带了 Retry-After 信息，优先使用
		var ra RetryAfter
		if errors.As(err, &ra) {
			if raDuration := ra.RetryAfter(); raDuration > delay {
				delay = raDuration
			}
		}

		// 等待退避时间，同时监听 context 取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// RetryAfterFromLLMError 从 llm.Error 中提取 Retry-After 信息。
// 对于速率限制错误（429），返回 5 秒的标准退避时间。
func RetryAfterFromLLMError(err *llm.Error) error {
	if err != nil && err.Type == llm.ErrorTypeRateLimit {
		return NewRetryAfterError(5 * time.Second)
	}
	return nil
}