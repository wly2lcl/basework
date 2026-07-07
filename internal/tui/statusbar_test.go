// Package tui 测试
package tui

import (
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// TestNewStatusBarView 测试创建状态栏
func TestNewStatusBarView(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")
	if sb == nil {
		t.Fatal("NewStatusBarView() 返回了 nil")
	}
	if sb.ModelName != "gpt-4" {
		t.Fatalf("期望 ModelName=gpt-4，得到 %s", sb.ModelName)
	}
	if sb.Provider != "openai" {
		t.Fatalf("期望 Provider=openai，得到 %s", sb.Provider)
	}
}

// TestStatusBarViewRender 测试渲染
func TestStatusBarViewRender(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")
	result := sb.Render(80, theme.DefaultTheme)
	if result == "" {
		t.Fatal("Render 返回了空内容")
	}
	// 应包含模型名
	if !strings.Contains(result, "gpt-4") {
		t.Fatalf("状态栏应包含模型名 'gpt-4'，得到: %s", result)
	}
}

// TestStatusBarViewUpdateTokens 测试更新 token 用量
func TestStatusBarViewUpdateTokens(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")
	sb.UpdateTokens(100, 50, 150)

	if sb.PromptTokens != 100 {
		t.Fatalf("期望 PromptTokens=100，得到 %d", sb.PromptTokens)
	}
	if sb.TotalTokens != 150 {
		t.Fatalf("期望 TotalTokens=150，得到 %d", sb.TotalTokens)
	}

	// 渲染应包含 token 信息
	result := sb.Render(80, theme.DefaultTheme)
	if !strings.Contains(result, "150") {
		t.Fatalf("状态栏应包含 token 数，得到: %s", result)
	}
}

// TestStatusBarViewBusyIdle 测试忙闲状态切换
func TestStatusBarViewBusyIdle(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")

	// 空闲状态
	sb.SetBusy(false)
	result := sb.Render(80, theme.DefaultTheme)
	if !strings.Contains(result, "空闲") {
		t.Fatalf("空闲状态应显示 '空闲'，得到: %s", result)
	}

	// 忙碌状态
	sb.SetBusy(true)
	result = sb.Render(80, theme.DefaultTheme)
	if !strings.Contains(result, "忙") {
		t.Fatalf("忙碌状态应显示 '忙'，得到: %s", result)
	}
}

// TestStatusBarViewMCPConnection 测试 MCP 连接状态
func TestStatusBarViewMCPConnection(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")

	// 已连接
	sb.SetMCPConnected(true)
	result := sb.Render(80, theme.DefaultTheme)
	if !strings.Contains(result, "MCP") {
		t.Fatalf("MCP 状态应显示在状态栏，得到: %s", result)
	}

	// 断开
	sb.SetMCPConnected(false)
	result = sb.Render(80, theme.DefaultTheme)
	if !strings.Contains(result, "MCP") {
		t.Fatalf("MCP 断开时也应显示，得到: %s", result)
	}
}

// TestStatusBarViewModelInfo 测试更新模型信息
func TestStatusBarViewModelInfo(t *testing.T) {
	sb := NewStatusBarView("old-model", "old-provider", "sess-12345")
	sb.SetModelInfo("new-model", "new-provider")

	if sb.ModelName != "new-model" {
		t.Fatalf("期望 ModelName=new-model，得到 %s", sb.ModelName)
	}
	if sb.Provider != "new-provider" {
		t.Fatalf("期望 Provider=new-provider，得到 %s", sb.Provider)
	}
}

// TestStatusBarViewTick 测试心跳
func TestStatusBarViewTick(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")
	initialIdx := sb.spinnerIdx
	sb.Tick()
	if sb.spinnerIdx != initialIdx+1 {
		t.Fatalf("Tick 后 spinnerIdx 应增加 1")
	}
}

// TestFormatTokens 测试 token 格式化
func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0 tok"},
		{500, "500 tok"},
		{1000, "1.0k tok"},
		{1500, "1.5k tok"},
		{1000000, "1.0M tok"},
	}

	for _, tt := range tests {
		result := formatTokens(tt.input)
		if result != tt.expected {
			t.Fatalf("formatTokens(%d) = %q，期望 %q", tt.input, result, tt.expected)
		}
	}
}

// TestStatusBarViewRenderWidth 测试不同宽度渲染不崩溃
func TestStatusBarViewRenderWidth(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "sess-12345")
	sb.SetBusy(true)
	sb.SetMCPConnected(true)
	sb.UpdateTokens(100, 50, 150)

	// 在不同宽度下渲染不应崩溃
	sb.Render(40, theme.DefaultTheme)
	sb.Render(80, theme.DefaultTheme)
	sb.Render(120, theme.DefaultTheme)
}

// TestStatusBarViewEmptySessionID 测试空会话 ID
func TestStatusBarViewEmptySessionID(t *testing.T) {
	sb := NewStatusBarView("gpt-4", "openai", "")
	result := sb.Render(80, theme.DefaultTheme)
	if result == "" {
		t.Fatal("即使会话 ID 为空也应有渲染内容")
	}
}

// TestStatusBarViewEmptyModelInfo 测试空模型信息
func TestStatusBarViewEmptyModelInfo(t *testing.T) {
	sb := NewStatusBarView("", "", "sess-12345")
	result := sb.Render(80, theme.DefaultTheme)
	if result == "" {
		t.Fatal("即使模型信息为空也应有渲染内容")
	}
}
