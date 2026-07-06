// Package tui 测试
package tui

import (
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// TestNewMessageView 测试创建消息渲染器
func TestNewMessageView(t *testing.T) {
	mv := NewMessageView()
	if mv == nil {
		t.Fatal("NewMessageView() 返回了 nil")
	}
}

// TestRenderUserMessage 测试用户消息渲染
func TestRenderUserMessage(t *testing.T) {
	mv := NewMessageView()
	result := mv.RenderUserMessage("你好世界", theme.DefaultTheme)
	if !strings.Contains(result, "你好世界") {
		t.Fatalf("用户消息渲染应包含文本，得到: %s", result)
	}
	if !strings.Contains(result, ">") {
		t.Fatalf("用户消息渲染应包含 > 前缀，得到: %s", result)
	}
}

// TestRenderAssistantMessage 测试助手消息渲染
func TestRenderAssistantMessage(t *testing.T) {
	mv := NewMessageView()

	// 纯文本消息
	result := mv.RenderAssistantMessage("Hello World")
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "World") {
		t.Fatalf("助手消息渲染应包含文本，得到: %s", result)
	}

	// Markdown 消息
	mdResult := mv.RenderAssistantMessage("# Title\n\nSome **bold** text")
	if !strings.Contains(mdResult, "Title") {
		t.Fatalf("Markdown 渲染应包含标题，得到: %s", mdResult)
	}
}

// TestRenderToolCall 测试工具调用渲染
func TestRenderToolCall(t *testing.T) {
	mv := NewMessageView()

	// 成功调用
	result := mv.RenderToolCall("web_fetch", "url=https://example.com", "获取成功", false, theme.DefaultTheme)
	if !strings.Contains(result, "web_fetch") {
		t.Fatalf("工具调用渲染应包含工具名，得到: %s", result)
	}

	// 失败调用
	errResult := mv.RenderToolCall("web_search", "query=golang", "超时错误", true, theme.DefaultTheme)
	if !strings.Contains(errResult, "超时错误") {
		t.Fatalf("错误工具调用应包含错误信息，得到: %s", errResult)
	}
}

// TestRenderError 测试错误消息渲染
func TestRenderError(t *testing.T) {
	mv := NewMessageView()
	result := mv.RenderError("连接失败", theme.DefaultTheme)
	if !strings.Contains(result, "连接失败") {
		t.Fatalf("错误消息渲染应包含错误文本，得到: %s", result)
	}
}

// TestRenderThinking 测试思考过程渲染
func TestRenderThinking(t *testing.T) {
	mv := NewMessageView()
	result := mv.RenderThinking("分析用户问题中...", theme.DefaultTheme)
	if !strings.Contains(result, "分析用户问题中...") {
		t.Fatalf("思考过程渲染应包含文本，得到: %s", result)
	}
}

// TestSetWidth 测试设置渲染宽度
func TestSetWidth(t *testing.T) {
	mv := NewMessageView()
	// 设置宽度不应崩溃
	mv.SetWidth(60)
	mv.SetWidth(120)
}

// TestRenderEmpty 测试空消息渲染
func TestRenderAssistantMessageEmpty(t *testing.T) {
	mv := NewMessageView()
	result := mv.RenderAssistantMessage("")
	if result != "" {
		t.Fatalf("空消息应返回空字符串，得到: %q", result)
	}
}