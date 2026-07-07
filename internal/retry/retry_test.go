package retry

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ========== IsRetryable ==========

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// 可重试错误
		{
			name: "网络超时可重试",
			err: &net.DNSError{
				IsTimeout: true,
			},
			want: true,
		},
		{
			name: "临时性网络错误可重试",
			err: &fakeNetError{
				timeout:   false,
				temporary: true,
			},
			want: true,
		},
		{
			name: "LLM 5xx 内部错误可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeInternal,
				StatusCode: 500,
				Message:    "Internal Server Error",
			},
			want: true,
		},
		{
			name: "LLM 503 可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeInternal,
				StatusCode: 503,
				Message:    "Service Unavailable",
			},
			want: true,
		},
		{
			name: "LLM 速率限制(429)可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeRateLimit,
				StatusCode: 429,
				Message:    "Too Many Requests",
			},
			want: true,
		},
		{
			name: "LLM 网络错误可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeNetwork,
				StatusCode: 0,
				Message:    "connection refused",
			},
			want: true,
		},
		// 不可重试错误
		{
			name: "nil 错误不可重试",
			err:  nil,
			want: false,
		},
		{
			name: "LLM 认证错误(401)不可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeAuth,
				StatusCode: 401,
				Message:    "Unauthorized",
			},
			want: false,
		},
		{
			name: "LLM 模型不存在不可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeModelNotFound,
				StatusCode: 404,
				Message:    "Model not found",
			},
			want: false,
		},
		{
			name: "LLM 上下文溢出不可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeContextOverflow,
				StatusCode: 400,
				Message:    "Context length exceeded",
			},
			want: false,
		},
		{
			name: "LLM 400 错误不可重试",
			err: &llm.Error{
				Type:       llm.ErrorTypeInternal,
				StatusCode: 400,
				Message:    "Bad Request",
			},
			want: false,
		},
		// 兜底：无类型但含有关键字
		{
			name: "错误消息含 timeout 关键字可重试（兜底）",
			err:  errors.New("connection timeout"),
			want: true,
		},
		{
			name: "普通错误不可重试",
			err:  errors.New("some random error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, 期望 %v", tt.err, got, tt.want)
			}
		})
	}
}

// fakeNetError 模拟 net.Error 接口用于测试
type fakeNetError struct {
	timeout   bool
	temporary bool
}

func (e *fakeNetError) Error() string   { return "fake net error" }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return e.temporary }

// ========== Backoff ==========

func TestBackoff(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		base    time.Duration
		max     time.Duration
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "attempt 0: base * 2^0 + jitter = base + [0,1s]",
			attempt: 0,
			base:    2 * time.Second,
			max:     60 * time.Second,
			wantMin: 2 * time.Second,
			wantMax: 3 * time.Second,
		},
		{
			name:    "attempt 1: base * 2^1 + jitter = 2*base + [0,1s]",
			attempt: 1,
			base:    2 * time.Second,
			max:     60 * time.Second,
			wantMin: 4 * time.Second,
			wantMax: 5 * time.Second,
		},
		{
			name:    "attempt 2: base * 2^2 + jitter = 4*base + [0,1s]",
			attempt: 2,
			base:    2 * time.Second,
			max:     60 * time.Second,
			wantMin: 8 * time.Second,
			wantMax: 9 * time.Second,
		},
		{
			name:    "达到最大延迟上限 60s",
			attempt: 6, // 2*2^6 = 128 > 60
			base:    2 * time.Second,
			max:     60 * time.Second,
			wantMin: 60 * time.Second,
			wantMax: 61 * time.Second,
		},
		{
			name:    "负 attempt 当作 0 处理",
			attempt: -1,
			base:    2 * time.Second,
			max:     60 * time.Second,
			wantMin: 2 * time.Second,
			wantMax: 3 * time.Second,
		},
		{
			name:    "自定义 base=1s, max=30s",
			attempt: 0,
			base:    1 * time.Second,
			max:     30 * time.Second,
			wantMin: 1 * time.Second,
			wantMax: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Backoff(tt.attempt, tt.base, tt.max)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Backoff(%d, %v, %v) = %v, 期望在 [%v, %v] 范围内",
					tt.attempt, tt.base, tt.max, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// ========== WithRetry ==========

// errRetryable 可重试的错误哨兵
var errRetryable = &llm.Error{
	Type:       llm.ErrorTypeRateLimit,
	StatusCode: 429,
	Message:    "rate limited",
}

// errNonRetryable 不可重试的错误哨兵
var errNonRetryable = &llm.Error{
	Type:       llm.ErrorTypeAuth,
	StatusCode: 401,
	Message:    "unauthorized",
}

func TestWithRetry_Success(t *testing.T) {
	// 场景：fn 第一次就成功
	t.Run("第一次调用成功", func(t *testing.T) {
		callCount := 0
		err := WithRetry(context.Background(), DefaultConfig(), func() error {
			callCount++
			return nil
		})
		if err != nil {
			t.Errorf("WithRetry() 返回错误: %v", err)
		}
		if callCount != 1 {
			t.Errorf("函数被调用了 %d 次, 期望 1 次", callCount)
		}
	})
}

func TestWithRetry_RetryThenSuccess(t *testing.T) {
	// 场景：前 2 次失败，第 3 次成功
	t.Run("重试后成功", func(t *testing.T) {
		callCount := 0
		attempts := 3
		err := WithRetry(context.Background(), Config{
			Enabled:     true,
			MaxAttempts: attempts,
			BaseDelay:   time.Millisecond, // 缩短等待时间
			MaxDelay:    10 * time.Millisecond,
		}, func() error {
			callCount++
			if callCount < attempts {
				return errRetryable
			}
			return nil
		})
		if err != nil {
			t.Errorf("WithRetry() 返回错误: %v", err)
		}
		if callCount != attempts {
			t.Errorf("函数被调用了 %d 次, 期望 %d 次", callCount, attempts)
		}
	})
}

func TestWithRetry_MaxAttempts(t *testing.T) {
	// 场景：达到最大重试次数返回最后一次错误
	t.Run("达到最大重试次数", func(t *testing.T) {
		callCount := 0
		maxAttempts := 3
		err := WithRetry(context.Background(), Config{
			Enabled:     true,
			MaxAttempts: maxAttempts,
			BaseDelay:   time.Millisecond,
			MaxDelay:    10 * time.Millisecond,
		}, func() error {
			callCount++
			return errRetryable
		})
		if !errors.Is(err, errRetryable) {
			t.Errorf("WithRetry() 返回错误 %v, 期望 %v", err, errRetryable)
		}
		if callCount != maxAttempts {
			t.Errorf("函数被调用了 %d 次, 期望 %d 次", callCount, maxAttempts)
		}
	})
}

func TestWithRetry_NonRetryable(t *testing.T) {
	// 场景：不可重试错误立即返回
	t.Run("不可重试错误不重试", func(t *testing.T) {
		callCount := 0
		err := WithRetry(context.Background(), DefaultConfig(), func() error {
			callCount++
			return errNonRetryable
		})
		if !errors.Is(err, errNonRetryable) {
			t.Errorf("WithRetry() 返回错误 %v, 期望 %v", err, errNonRetryable)
		}
		if callCount != 1 {
			t.Errorf("函数被调用了 %d 次, 期望 1 次", callCount)
		}
	})
}

func TestWithRetry_ContextCancel(t *testing.T) {
	// 场景：上下文取消时停止重试
	t.Run("context 取消停止重试", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		callCount := 0
		var mu sync.Mutex

		errCh := make(chan error, 1)
		go func() {
			errCh <- WithRetry(ctx, Config{
				Enabled:     true,
				MaxAttempts: 10,
				BaseDelay:   100 * time.Millisecond, // 较长的延迟，确保取消生效
				MaxDelay:    1 * time.Second,
			}, func() error {
				mu.Lock()
				callCount++
				mu.Unlock()
				return errRetryable
			})
		}()

		// 等待第一次重试开始后取消
		time.Sleep(50 * time.Millisecond)
		cancel()

		err := <-errCh
		if !errors.Is(err, context.Canceled) {
			t.Errorf("WithRetry() 返回错误 %v, 期望 context.Canceled", err)
		}
	})
}

func TestWithRetry_Disabled(t *testing.T) {
	// 场景：重试禁用时直接执行并返回
	t.Run("重试禁用时直接执行", func(t *testing.T) {
		callCount := 0
		err := WithRetry(context.Background(), Config{
			Enabled: false,
		}, func() error {
			callCount++
			return errRetryable
		})
		if !errors.Is(err, errRetryable) {
			t.Errorf("WithRetry() 返回错误 %v, 期望 %v", err, errRetryable)
		}
		if callCount != 1 {
			t.Errorf("函数被调用了 %d 次, 期望 1 次", callCount)
		}
	})
}

func TestWithRetry_RetryAfterPriority(t *testing.T) {
	// 场景：错误携带 Retry-After 信息时优先使用
	t.Run("Retry-After 优先于退避时间", func(t *testing.T) {
		callCount := 0
		maxAttempts := 2
		start := time.Now()
		err := WithRetry(context.Background(), Config{
			Enabled:     true,
			MaxAttempts: maxAttempts,
			BaseDelay:   time.Millisecond,      // 基础退避很短
			MaxDelay:    10 * time.Millisecond, // 最大退避也很短
		}, func() error {
			callCount++
			if callCount < maxAttempts {
				// 返回一个既包含 timeout（可重试）又包含 RetryAfter 的错误
				return &retryableWithAfterError{
					msg:   "rate limit timeout, retry later",
					after: 50 * time.Millisecond,
				}
			}
			return nil
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("WithRetry() 返回错误: %v", err)
		}
		if callCount != maxAttempts {
			t.Errorf("函数被调用了 %d 次, 期望 %d 次", callCount, maxAttempts)
		}
		// 等待时间应 >= Retry-After 指定的 50ms
		if elapsed < 45*time.Millisecond {
			t.Errorf("等待时间 %v 太短, 应至少 50ms", elapsed)
		}
	})
}

// retryableWithAfterError 既包含 timeout 关键字（可被 IsRetryable 识别），又实现 RetryAfter 接口
type retryableWithAfterError struct {
	msg   string
	after time.Duration
}

func (e *retryableWithAfterError) Error() string             { return e.msg }
func (e *retryableWithAfterError) RetryAfter() time.Duration { return e.after }

// ========== NewRetryAfterError / RetryAfterFromLLMError ==========

func TestRetryAfterFromLLMError(t *testing.T) {
	tests := []struct {
		name string
		err  *llm.Error
		want bool // 是否应该返回非 nil 错误
	}{
		{
			name: "速率限制错误返回 Retry-After 错误",
			err: &llm.Error{
				Type:       llm.ErrorTypeRateLimit,
				StatusCode: 429,
			},
			want: true,
		},
		{
			name: "非速率限制错误返回 nil",
			err: &llm.Error{
				Type:       llm.ErrorTypeAuth,
				StatusCode: 401,
			},
			want: false,
		},
		{
			name: "nil 错误返回 nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryAfterFromLLMError(tt.err)
			if tt.want {
				if got == nil {
					t.Error("RetryAfterFromLLMError() = nil, 期望非 nil")
				} else {
					var ra RetryAfter
					if !errors.As(got, &ra) {
						t.Error("返回的错误未实现 RetryAfter 接口")
					}
				}
			} else {
				if got != nil {
					t.Errorf("RetryAfterFromLLMError() = %v, 期望 nil", got)
				}
			}
		})
	}
}

func TestNewRetryAfterError_RetryAfter(t *testing.T) {
	duration := 5 * time.Second
	err := NewRetryAfterError(duration)
	var ra RetryAfter
	if !errors.As(err, &ra) {
		t.Fatal("NewRetryAfterError 返回的错误未实现 RetryAfter 接口")
	}
	if ra.RetryAfter() != duration {
		t.Errorf("RetryAfter() = %v, 期望 %v", ra.RetryAfter(), duration)
	}
}

// ========== DefaultConfig ==========

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("DefaultConfig().Enabled 应为 true")
	}
	if cfg.MaxAttempts != 3 {
		t.Errorf("DefaultConfig().MaxAttempts = %d, 期望 3", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != 2*time.Second {
		t.Errorf("DefaultConfig().BaseDelay = %v, 期望 2s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 60*time.Second {
		t.Errorf("DefaultConfig().MaxDelay = %v, 期望 60s", cfg.MaxDelay)
	}
}
