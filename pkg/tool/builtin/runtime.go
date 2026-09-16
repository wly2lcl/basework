package builtin

import (
	"context"
	"errors"
	"time"

	"github.com/wly2lcl/basework/pkg/tool"
)

// Runtime 是工具实例级的运行时注入（CFG-003）。
//
// 背景：SetPathChecker / SetTimeoutConfig / SetEventBus 是**包级全局状态**，
// 进程内所有工具实例共享，后设置者覆盖先设置者。单实例 CLI 无感，但嵌入方
// 起两个 Agent（或产品里主/子代理配置不同）时，会出现「一个实例的配置悄悄
// 改掉另一个实例的行为」。Runtime 把这三样东西下沉到实例：
//
//   - 注入实例的（非 nil 字段）优先生效；
//   - 未注入的字段回落到包级全局（Set* 设置的值），**旧嵌入入口完全兼容**；
//   - 两者都为空时回落内置默认。
//
// 注意：本结构不改任何全局状态。它只是让「实例」有机会优先于「全局」。
type Runtime struct {
	// PathChecker 是实例级路径检查器。nil 时回落 globalPathChecker。
	PathChecker PathChecker
	// Timeout 是实例级超时配置。nil 时回落 globalTimeoutConfig。
	Timeout *TimeoutConfig
	// EventBus 是实例级事件发布器。nil 时回落 globalEventBus。
	EventBus EventPublisher
	// Environment 是 BashTool 子进程使用的完整环境。nil 时继承当前进程环境。
	// 设置后不会自动追加当前环境；调用方应先复制 os.Environ，再移除不应
	// 暴露给工具的变量。这样 Provider 凭证可以只留在模型客户端内存中。
	Environment []string
}

// resolvePathChecker 返回该实例应使用的路径检查器。
func (r *Runtime) resolvePathChecker() PathChecker {
	if r != nil && r.PathChecker != nil {
		return r.PathChecker
	}
	return getPathChecker()
}

// resolveTimeout 返回该实例应使用的超时配置。
func (r *Runtime) resolveTimeout() TimeoutConfig {
	if r != nil && r.Timeout != nil {
		return *r.Timeout
	}
	return getTimeoutConfig()
}

// resolveEventBus 返回该实例应使用的事件发布器。
func (r *Runtime) resolveEventBus() EventPublisher {
	if r != nil && r.EventBus != nil {
		return r.EventBus
	}
	return globalEventBus
}

// resolveEnvironment 返回 BashTool 应传给子进程的环境；nil 表示沿用 exec 默认继承。
func (r *Runtime) resolveEnvironment() []string {
	if r != nil && r.Environment != nil {
		return append([]string(nil), r.Environment...)
	}
	return nil
}

// AllWithRuntime 返回注入了实例运行时的全部内置工具。
//
// 与 All() 的分工：All() 不携带实例配置，行为完全由包级全局决定（旧路径，
// 保持兼容）；需要实例隔离的调用方（产品运行时、多 Agent 嵌入）用本入口。
// rt 可为 nil——nil 与 All() 等价，全部走全局回落。
func AllWithRuntime(rt *Runtime) []tool.Tool {
	tools := All()
	for _, t := range tools {
		switch typed := t.(type) {
		case *BashTool:
			typed.Runtime = rt
		case *ReadTool:
			typed.Runtime = rt
		case *WriteTool:
			typed.Runtime = rt
		case *EditTool:
			typed.Runtime = rt
		}
	}
	return tools
}

// WithTimeoutBus 是 WithTimeout 的实例级版本：超时事件发布到指定 bus，
// 而不是包级全局。WithTimeout 内部即委托到本函数（bus = globalEventBus）。
func WithTimeoutBus(ctx context.Context, toolName string, timeout time.Duration, bus EventPublisher) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

	if bus != nil {
		context.AfterFunc(timeoutCtx, func() {
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				bus.PublishEvent(
					EventToolTimeout,
					map[string]interface{}{
						"tool_name":     toolName,
						"timeout_limit": timeout.Seconds(),
						"elapsed_time":  timeout.Seconds(),
					},
				)
			}
		})
	}

	return timeoutCtx, cancel
}
