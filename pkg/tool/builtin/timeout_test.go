package builtin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/observability"
)

// --- TimeoutConfig 测试 ---

func TestGetTimeout_Default(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	timeout := cfg.GetTimeout("read")
	if timeout != 30*time.Second {
		t.Fatalf("期望 30s，得到 %v", timeout)
	}
}

func TestGetTimeout_Override(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	timeout := cfg.GetTimeout("bash")
	if timeout != 60*time.Second {
		t.Fatalf("期望 60s，得到 %v", timeout)
	}
}

func TestGetTimeout_OverrideZero(t *testing.T) {
	cfg := TimeoutConfig{
		DefaultTimeout: 30,
		Overrides:      map[string]int{"bash": 0},
	}
	timeout := cfg.GetTimeout("bash")
	if timeout != 0 {
		t.Fatalf("超时禁用时应返回 0，得到 %v", timeout)
	}
}

func TestGetTimeout_DefaultZero(t *testing.T) {
	cfg := TimeoutConfig{
		DefaultTimeout: 0,
		Overrides:      nil,
	}
	timeout := cfg.GetTimeout("unknown")
	if timeout != 0 {
		t.Fatalf("默认 0 时应返回 0，得到 %v", timeout)
	}
}

func TestGetTimeout_OverrideForUnknownTool(t *testing.T) {
	cfg := TimeoutConfig{
		DefaultTimeout: 30,
		Overrides:      map[string]int{"bash": 60},
	}
	timeout := cfg.GetTimeout("nonexistent")
	if timeout != 30*time.Second {
		t.Fatalf("未知工具应返回默认 30s，得到 %v", timeout)
	}
}

// --- WithTimeout 测试 ---

func TestWithTimeout_ReturnsOriginalOnZero(t *testing.T) {
	ctx := context.Background()
	wrappedCtx, cancel := WithTimeout(ctx, "test", 0)
	defer cancel()

	if wrappedCtx != ctx {
		t.Fatal("timeout <= 0 时应返回原始 context")
	}
}

func TestWithTimeout_TimeoutFires(t *testing.T) {
	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test", 10*time.Millisecond)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		if !errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			t.Fatalf("应返回 DeadlineExceeded，得到 %v", timeoutCtx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("超时应在 10ms 内触发")
	}
}

func TestWithTimeout_NoTimeout(t *testing.T) {
	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test", time.Minute)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		t.Fatal("不应在 10ms 内超时")
	case <-time.After(10 * time.Millisecond):
		// 正确：未超时
	}
}

func TestWithTimeout_CancelPreventsTimeoutPublish(t *testing.T) {
	bus := observability.NewEventBus()
	SetEventBus(bus)
	defer SetEventBus(nil)

	received := make(chan bool, 1)
	bus.Subscribe(observability.EventToolTimeout, func(e observability.Event) {
		received <- true
	})

	ctx := context.Background()
	_, cancel := WithTimeout(ctx, "test", 100*time.Millisecond)
	cancel() // 取消后超时不应触发

	select {
	case <-received:
		t.Fatal("取消后不应发布超时事件")
	case <-time.After(200 * time.Millisecond):
		// 正确：没有事件
	}
}

// --- IsTimeoutError 测试 ---

func TestIsTimeoutError_DeadlineExceeded(t *testing.T) {
	if !IsTimeoutError(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded 应被识别为超时错误")
	}
}

func TestIsTimeoutError_Canceled(t *testing.T) {
	if IsTimeoutError(context.Canceled) {
		t.Fatal("Canceled 不应被识别为超时错误")
	}
}

func TestIsTimeoutError_Nil(t *testing.T) {
	if IsTimeoutError(nil) {
		t.Fatal("nil 不应被识别为超时错误")
	}
}

func TestIsTimeoutError_WrappedDeadlineExceeded(t *testing.T) {
	wrapped := errors.Join(context.DeadlineExceeded, errors.New("context deadline exceeded"))
	if !IsTimeoutError(wrapped) {
		t.Fatal("包装的 DeadlineExceeded 应被识别为超时错误")
	}
}

// --- 超时事件发布测试 ---

func TestTimeoutEventPublished(t *testing.T) {
	bus := observability.NewEventBus()
	SetEventBus(bus)
	defer SetEventBus(nil)

	received := make(chan observability.Event, 1)
	bus.Subscribe(observability.EventToolTimeout, func(e observability.Event) {
		received <- e
	})

	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test_tool", 10*time.Millisecond)
	defer cancel()

	// 等待超时触发
	<-timeoutCtx.Done()

	select {
	case e := <-received:
		if e.Type != observability.EventToolTimeout {
			t.Fatalf("事件类型应为 %q，得到 %q", observability.EventToolTimeout, e.Type)
		}
		toolName, _ := e.Data["tool_name"].(string)
		if toolName != "test_tool" {
			t.Fatalf("工具名应为 test_tool，得到 %v", toolName)
		}
	case <-time.After(time.Second):
		t.Fatal("超时事件未发布")
	}
}

// --- SetTimeoutConfig/Save/Restore 测试 ---

func TestSetTimeoutConfig(t *testing.T) {
	original := DefaultTimeoutConfig()
	SetTimeoutConfig(TimeoutConfig{
		DefaultTimeout: 10,
		Overrides:      map[string]int{"bash": 5},
	})

	cfg := getTimeoutConfig()
	if cfg.DefaultTimeout != 10 {
		t.Fatalf("期望 DefaultTimeout=10，得到 %d", cfg.DefaultTimeout)
	}
	bashTimeout := cfg.GetTimeout("bash")
	if bashTimeout != 5*time.Second {
		t.Fatalf("期望 bash=5s，得到 %v", bashTimeout)
	}

	// 恢复
	SetTimeoutConfig(original)
}

// --- 集成测试：WithTimeout 与真实 EventBus ---

func TestWithTimeoutAndBus_NoBusDoesNotPanic(t *testing.T) {
	SetEventBus(nil) // 确保 bus 为 nil

	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test", 10*time.Millisecond)
	defer cancel()

	<-timeoutCtx.Done()
	// bus 为 nil 时不应 panic
}
