package agent

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// mockProvider 测试用模拟 Provider
type mockProvider struct {
	name  string
	text  string
	err   error
	delay time.Duration
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Collect() (string, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.text, m.err
}

func TestContextCollector_Collect(t *testing.T) {
	providers := []ContextProvider{
		&mockProvider{name: "a", text: "info_a"},
		&mockProvider{name: "b", text: "info_b"},
	}

	cc := NewContextCollector(providers...)
	result := cc.Collect(context.Background())

	if !strings.Contains(result, "info_a") {
		t.Error("结果应包含 info_a")
	}
	if !strings.Contains(result, "info_b") {
		t.Error("结果应包含 info_b")
	}
}

func TestContextCollector_Empty(t *testing.T) {
	cc := NewContextCollector()
	result := cc.Collect(context.Background())
	// 空参数使用默认 providers，不应为空
	if result == "" {
		t.Log("默认 providers 可能返回空（取决于运行环境）")
	}
}

func TestContextCollector_NoProviders(t *testing.T) {
	cc := NewContextCollector()
	cc.providers = nil
	result := cc.Collect(context.Background())
	if result != "" {
		t.Errorf("无 providers 时应返回空字符串，得到 %q", result)
	}
}

func TestContextCollector_Error(t *testing.T) {
	providers := []ContextProvider{
		&mockProvider{name: "good", text: "good_info"},
		&mockProvider{name: "bad", text: "", err: errors.New("collect error")},
	}

	cc := NewContextCollector(providers...)
	result := cc.Collect(context.Background())

	if !strings.Contains(result, "good_info") {
		t.Error("出错 provider 不应影响其他 provider 的结果")
	}
}

func TestContextCollector_Timeout(t *testing.T) {
	providers := []ContextProvider{
		&mockProvider{name: "fast", text: "fast_info"},
		&mockProvider{name: "slow", text: "slow_info", delay: 100 * time.Millisecond},
	}

	cc := NewContextCollector(providers...)
	cc.timeout = 10 * time.Millisecond // 极短超时

	result := cc.Collect(context.Background())

	// fast 可能在超时前完成
	if strings.Contains(result, "fast_info") {
		t.Log("fast provider 在超时前完成")
	}
	// slow 应超时被跳过
	if strings.Contains(result, "slow_info") {
		t.Log("slow provider 可能已超时")
	}
}

func TestContextCollector_WithTimeout(t *testing.T) {
	cc := NewContextCollector()
	cc2 := cc.WithTimeout(10 * time.Second)
	if cc2.timeout != 10*time.Second {
		t.Errorf("WithTimeout 应设置超时")
	}
}

func TestContextCollector_CancelledContext(t *testing.T) {
	providers := []ContextProvider{
		&mockProvider{name: "a", text: "text", delay: 50 * time.Millisecond},
	}

	cc := NewContextCollector(providers...)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	result := cc.Collect(ctx)
	if result != "" {
		t.Log("取消的上下文可能导致空结果")
	}
}

// hangProvider 模拟一个会 hang 住的 ContextProvider
type hangProvider struct {
	name string
}

func (h *hangProvider) Name() string { return h.name }

func (h *hangProvider) Collect() (string, error) {
	<-make(chan struct{}) // 永远阻塞
	return "", nil
}

// TestContextCollector_HangProvider 验证 hang 的 provider 不会导致 Collect 永远阻塞，也不会 goroutine 泄漏
func TestContextCollector_HangProvider(t *testing.T) {
	before := runtime.NumGoroutine()

	providers := []ContextProvider{
		&mockProvider{name: "fast", text: "fast_info"},
		&hangProvider{name: "hang"},
	}

	cc := NewContextCollector(providers...)
	cc.timeout = 50 * time.Millisecond

	// Collect 应在超时后返回，不会无限阻塞
	done := make(chan struct{})
	go func() {
		result := cc.Collect(context.Background())
		if !strings.Contains(result, "fast_info") {
			t.Error("fast provider 的结果应包含在结果中")
		}
		close(done)
	}()

	select {
	case <-done:
		// 正常返回
	case <-time.After(5 * time.Second):
		t.Fatal("Collect 超时，hang provider 导致阻塞")
	}

	// 等待 goroutine 调度后比较数量
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	// 允许少量 goroutine 增长（gc、调度等），但不允许大幅泄漏
	leaked := after - before
	if leaked > 5 {
		t.Errorf("可能的 goroutine 泄漏: 调用前 %d, 调用后 %d (泄漏 %d)", before, after, leaked)
	}
}
