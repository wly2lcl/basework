// Package tui 测试
package tui

import (
	"testing"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// TestNewInputView 测试创建输入组件
func TestNewInputView(t *testing.T) {
	iv := NewInputView()
	if iv == nil {
		t.Fatal("NewInputView() 返回了 nil")
	}
	if iv.Text() != "" {
		t.Fatalf("期望空文本，得到 %q", iv.Text())
	}
}

// TestInputViewInit 测试 Init 返回 nil
func TestInputViewInit(t *testing.T) {
	iv := NewInputView()
	cmd := iv.Init()
	if cmd != nil {
		t.Fatal("Init() 应返回 nil")
	}
}

// TestInputViewRender 测试渲染
func TestInputViewRender(t *testing.T) {
	iv := NewInputView()
	result := iv.Render(80, theme.DefaultTheme)
	if result == "" {
		t.Fatal("Render() 返回了空内容")
	}
}

// TestInputViewSetText 测试设置文本
func TestInputViewSetText(t *testing.T) {
	iv := NewInputView()
	iv.SetText("hello")
	if iv.Text() != "hello" {
		t.Fatalf("期望 'hello'，得到 %q", iv.Text())
	}
}

// TestInputViewReset 测试重置
func TestInputViewReset(t *testing.T) {
	iv := NewInputView()
	iv.SetText("some input")
	iv.Reset()
	if iv.Text() != "" {
		t.Fatalf("Reset 后期望空文本，得到 %q", iv.Text())
	}
	// 历史应包含之前的输入
	if len(iv.history) != 1 {
		t.Fatalf("Reset 后期望历史长度为 1，得到 %d", len(iv.history))
	}
}

// TestInputViewHistoryNavigation 测试历史导航
func TestInputViewHistoryNavigation(t *testing.T) {
	iv := NewInputView()

	// 输入并重置多次
	iv.SetText("第一条消息")
	iv.Reset()
	iv.SetText("第二条消息")
	iv.Reset()

	if len(iv.history) != 2 {
		t.Fatalf("期望历史长度为 2，得到 %d", len(iv.history))
	}

	// 导航到第一条历史
	iv.navigateHistory(-1) // 上箭头
	if iv.Text() != "第二条消息" {
		t.Fatalf("期望 '第二条消息'，得到 %q", iv.Text())
	}

	iv.navigateHistory(-1) // 再上箭头
	if iv.Text() != "第一条消息" {
		t.Fatalf("期望 '第一条消息'，得到 %q", iv.Text())
	}

	// 向下导航
	iv.navigateHistory(1)
	if iv.Text() != "第二条消息" {
		t.Fatalf("向下导航后期望 '第二条消息'，得到 %q", iv.Text())
	}

	iv.navigateHistory(1)
	if iv.Text() != "" {
		t.Fatalf("向下导航到最新后期望空文本，得到 %q", iv.Text())
	}
}

// TestFindPathCompletions 测试文件路径补全
func TestFindPathCompletions(t *testing.T) {
	// 查找当前目录下的路径补全
	completions := findPathCompletions("")
	if completions != nil && len(completions) == 0 {
		t.Fatal("空前缀不应返回空补全列表")
	}
}

// TestInputViewRenderWidth 测试不同宽度渲染
func TestInputViewRenderWidth(t *testing.T) {
	iv := NewInputView()
	iv.SetText("test")
	r1 := iv.Render(40, theme.DefaultTheme)
	r2 := iv.Render(100, theme.DefaultTheme)
	if r1 == "" || r2 == "" {
		t.Fatal("Render 返回了空内容")
	}
}

// TestInputViewTextAfterOperations 测试操作后的文本
func TestInputViewTextAfterOperations(t *testing.T) {
	iv := NewInputView()

	// 多行文本
	iv.SetText("第一行\n第二行")
	if iv.Text() != "第一行\n第二行" {
		t.Fatalf("期望多行文本，得到 %q", iv.Text())
	}
}