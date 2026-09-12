package builtin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockEventPublisher 是 EventPublisher 的测试替身。
// 用它替代原先对 internal/observability 的依赖，使 pkg 层测试不再需要导入 internal。
type mockEventPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	Type string
	Data map[string]interface{}
}

func (m *mockEventPublisher) PublishEvent(eventType string, data map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{Type: eventType, Data: data})
}

// count 返回匹配指定事件类型的已发布事件数。
func (m *mockEventPublisher) count(eventType string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.events {
		if e.Type == eventType {
			n++
		}
	}
	return n
}

// first 返回首个匹配指定事件类型的事件。
func (m *mockEventPublisher) first(eventType string) (publishedEvent, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.events {
		if e.Type == eventType {
			return e, true
		}
	}
	return publishedEvent{}, false
}

// waitFor 轮询等待直到匹配事件数达到 want，超时返回 false。
func (m *mockEventPublisher) waitFor(eventType string, want int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if m.count(eventType) >= want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return m.count(eventType) >= want
}

// 编译期断言：测试替身满足 EventPublisher 接口。
var _ EventPublisher = (*mockEventPublisher)(nil)

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
	mock := &mockEventPublisher{}
	SetEventBus(mock)
	defer SetEventBus(nil)

	ctx := context.Background()
	_, cancel := WithTimeout(ctx, "test", 100*time.Millisecond)
	cancel() // 取消后超时不应触发

	time.Sleep(200 * time.Millisecond)
	if n := mock.count(EventToolTimeout); n != 0 {
		t.Fatalf("取消后不应发布超时事件，实际发布了 %d 次", n)
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
	mock := &mockEventPublisher{}
	SetEventBus(mock)
	defer SetEventBus(nil)

	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test_tool", 10*time.Millisecond)
	defer cancel()

	// 等待超时触发
	<-timeoutCtx.Done()

	if !mock.waitFor(EventToolTimeout, 1, time.Second) {
		t.Fatal("超时事件未发布")
	}
	e, ok := mock.first(EventToolTimeout)
	if !ok {
		t.Fatal("超时事件未发布")
	}
	if e.Type != EventToolTimeout {
		t.Fatalf("事件类型应为 %q，得到 %q", EventToolTimeout, e.Type)
	}
	// 事件类型字符串必须保持历史值，避免破坏已订阅方。
	if e.Type != "tool.timeout" {
		t.Fatalf("事件类型字符串应保持为 %q，得到 %q", "tool.timeout", e.Type)
	}
	toolName, _ := e.Data["tool_name"].(string)
	if toolName != "test_tool" {
		t.Fatalf("工具名应为 test_tool，得到 %v", toolName)
	}
	if _, ok := e.Data["timeout_limit"]; !ok {
		t.Fatal("事件 payload 应包含 timeout_limit")
	}
	if _, ok := e.Data["elapsed_time"]; !ok {
		t.Fatal("事件 payload 应包含 elapsed_time")
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

// --- 集成测试：WithTimeout 与事件发布器 ---

func TestWithTimeoutAndBus_NoBusDoesNotPanic(t *testing.T) {
	SetEventBus(nil) // 确保 bus 为 nil

	ctx := context.Background()
	timeoutCtx, cancel := WithTimeout(ctx, "test", 10*time.Millisecond)
	defer cancel()

	<-timeoutCtx.Done()
	// bus 为 nil 时不应 panic
}
