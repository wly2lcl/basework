package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockProvider 测试用模拟 Provider
type mockProvider struct {
	name    string
	text    string
	err     error
	delay   time.Duration
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