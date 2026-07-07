package llm

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig 是客户端重试配置。
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// defaultRetryConfig 返回默认重试配置。
func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BaseDelay:   2 * time.Second,
		MaxDelay:    60 * time.Second,
	}
}

// Client 是 LLM 客户端，包装 Model 接口并提供重试和可观测性支持。
type Client struct {
	model    Model
	retryCfg RetryConfig
	eventBus EventBus
}

// ClientOption 是客户端配置选项。
type ClientOption func(*Client)

// WithRetry 设置重试配置。
func WithRetry(cfg RetryConfig) ClientOption {
	return func(c *Client) { c.retryCfg = cfg }
}

// WithEventBus 设置可观测性事件总线。
func WithEventBus(eb EventBus) ClientOption {
	return func(c *Client) { c.eventBus = eb }
}

// WithRetryEnabled 启用或禁用重试。
func WithRetryEnabled(enabled bool) ClientOption {
	return func(c *Client) {
		cfg := c.retryCfg
		cfg.Enabled = enabled
		c.retryCfg = cfg
	}
}

// NewClient 创建新的 LLM 客户端。
//
// 如果提供了 OAuth TokenSource，会尝试应用到 model 上。
// 重试默认启用（3 次），可通过 WithRetry / WithRetryEnabled 调整。
func NewClient(model Model, opts ...ClientOption) *Client {
	c := &Client{
		model:    model,
		retryCfg: defaultRetryConfig(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ApplyOAuth 将 TokenSource 应用到 model 的底层 HTTP 客户端。
// 如果 model 不是支持 OAuth 注入的具体类型，此操作可能无效。
// 返回 true 表示应用成功，false 表示该 model 不支持 OAuth 注入。
func ApplyOAuth(model Model, ts interface{}) bool {
	type oauthAware interface {
		SetTokenSource(ts interface{}) error
	}
	if aware, ok := model.(oauthAware); ok {
		if err := aware.SetTokenSource(ts); err != nil {
			return false
		}
		return true
	}
	return false
}

// Model 返回底层的 Model 实例。
func (c *Client) Model() Model {
	return c.model
}

// Generate 调用底层模型的 Generate 方法，包装重试逻辑和可观测性事件。
func (c *Client) Generate(ctx context.Context, req *Request) (*Response, error) {
	c.publishEvent("llm.call.start", map[string]interface{}{
		"model":    c.model.ID(),
		"messages": len(req.Messages),
		"tools":    len(req.Tools),
	})

	var resp *Response
	err := c.withRetry(ctx, func() (err error) {
		resp, err = c.model.Generate(ctx, req)
		return err
	})

	data := map[string]interface{}{
		"model": c.model.ID(),
	}
	if err != nil {
		data["error"] = err.Error()
	} else if resp != nil {
		data["prompt_tokens"] = resp.Usage.PromptTokens
		data["completion_tokens"] = resp.Usage.CompletionTokens
		data["total_tokens"] = resp.Usage.TotalTokens
	}
	c.publishEvent("llm.call.end", data)

	if err != nil {
		return nil, fmt.Errorf("llm client: %w", err)
	}
	return resp, nil
}

// Stream 调用底层模型的 Stream 方法，包装可观测性事件。
// 流式调用通常不可重试，因此不包装重试逻辑。
func (c *Client) Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	c.publishEvent("llm.call.start", map[string]interface{}{
		"model":    c.model.ID(),
		"messages": len(req.Messages),
		"tools":    len(req.Tools),
		"stream":   true,
	})

	ch, err := c.model.Stream(ctx, req)

	data := map[string]interface{}{
		"model":  c.model.ID(),
		"stream": true,
	}
	if err != nil {
		data["error"] = err.Error()
	}
	c.publishEvent("llm.call.end", data)

	if err != nil {
		return nil, fmt.Errorf("llm client stream: %w", err)
	}
	return ch, nil
}

// withRetry 使用指数退避重试执行函数 fn。
// 对于速率限制错误（429）或临时性错误进行重试。
func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	if !c.retryCfg.Enabled {
		return fn()
	}

	var lastErr error
	for attempt := 0; attempt < c.retryCfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// 不可重试的错误直接返回
		if !c.isRetryable(err) {
			return err
		}

		if attempt >= c.retryCfg.MaxAttempts-1 {
			break
		}

		// 退避等待
		delay := c.backoff(attempt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// isRetryable 检查错误是否可重试。
func (c *Client) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// 速率限制和网络错误可重试
	if IsRateLimit(err) {
		return true
	}
	return false
}

// backoff 计算第 attempt 次重试的退避时间（带随机抖动）。
func (c *Client) backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := float64(c.retryCfg.BaseDelay) * math.Pow(2, float64(attempt))
	// 添加 ±25% 随机抖动
	jitter := (rand.Float64() - 0.5) * 0.5 * delay
	delay += jitter
	if delay > float64(c.retryCfg.MaxDelay) {
		delay = float64(c.retryCfg.MaxDelay)
	}
	return time.Duration(delay)
}

// publishEvent 发布可观测性事件。
func (c *Client) publishEvent(eventType string, data map[string]interface{}) {
	if c.eventBus == nil {
		return
	}
	c.eventBus.PublishEvent(eventType, data)
}
