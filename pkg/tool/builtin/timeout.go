package builtin

import (
	"context"
	"errors"
	"time"

	"github.com/wly2lcl/basework/internal/observability"
)

// 包级全局变量
var (
	globalTimeoutConfig = DefaultTimeoutConfig()
	globalEventBus      *observability.EventBus
)

// SetTimeoutConfig 设置全局超时配置
func SetTimeoutConfig(cfg TimeoutConfig) {
	globalTimeoutConfig = cfg
}

// getTimeoutConfig 获取当前超时配置
func getTimeoutConfig() TimeoutConfig {
	return globalTimeoutConfig
}

// SetEventBus 设置事件总线，用于发布超时事件
func SetEventBus(bus *observability.EventBus) {
	globalEventBus = bus
}

// TimeoutConfig 工具超时配置
type TimeoutConfig struct {
	// DefaultTimeout 全局默认超时（秒）
	DefaultTimeout int
	// Overrides 按工具名称覆盖的超时（秒）
	Overrides map[string]int
}

// DefaultTimeoutConfig 返回默认超时配置
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		DefaultTimeout: 30,
		Overrides: map[string]int{
			"bash": 60,
		},
	}
}

// GetTimeout 获取指定工具的超时时间
// 优先级：overrides[toolName] > default > 0（不超时）
func (c TimeoutConfig) GetTimeout(toolName string) time.Duration {
	if seconds, ok := c.Overrides[toolName]; ok {
		if seconds == 0 {
			return 0 // 禁用超时
		}
		return time.Duration(seconds) * time.Second
	}
	if c.DefaultTimeout == 0 {
		return 0
	}
	return time.Duration(c.DefaultTimeout) * time.Second
}

// TimeoutEvent 超时事件数据
type TimeoutEvent struct {
	ToolName     string        `json:"tool_name"`
	ElapsedTime  time.Duration `json:"elapsed_time"`
	TimeoutLimit time.Duration `json:"timeout_limit"`
}

// WithTimeout 包装 context，添加超时控制
// 返回包装后的 context 和 cancel 函数
// 如果 timeout <= 0，返回原始 context
// 超时触发时通过全局 event bus 发布 tool.timeout 事件
func WithTimeout(ctx context.Context, toolName string, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

	// 超时后通过 bus 发布事件
	if globalEventBus != nil {
		context.AfterFunc(timeoutCtx, func() {
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				globalEventBus.Publish(observability.NewEvent(
					observability.EventToolTimeout,
					map[string]interface{}{
						"tool_name":     toolName,
						"timeout_limit": timeout.Seconds(),
						"elapsed_time":  timeout.Seconds(),
					},
				))
			}
		})
	}

	return timeoutCtx, cancel
}

// IsTimeoutError 检查错误是否为超时错误
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}