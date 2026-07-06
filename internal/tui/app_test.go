// Package tui 测试
package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestNewApp 测试创建新 App
func TestNewApp(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")
	if app == nil {
		t.Fatal("NewApp() 返回了 nil")
	}
	if app.Input == nil {
		t.Fatal("Input 组件未初始化")
	}
	if app.StatusBar == nil {
		t.Fatal("StatusBar 组件未初始化")
	}
	if app.Streaming == nil {
		t.Fatal("Streaming 组件未初始化")
	}
	if len(app.Messages) != 0 {
		t.Fatalf("期望消息数为 0，得到 %d", len(app.Messages))
	}
	if app.Theme == nil {
		t.Fatal("Theme 组件未初始化")
	}
	if app.CommandRegistry == nil {
		t.Fatal("CommandRegistry 组件未初始化")
	}
	if app.CommandPanel == nil {
		t.Fatal("CommandPanel 组件未初始化")
	}
	if app.KeyResolver == nil {
		t.Fatal("KeyResolver 组件未初始化")
	}
	if app.DialogMgr == nil {
		t.Fatal("DialogMgr 组件未初始化")
	}
}

// TestAppInit 测试 App.Init
func TestAppInit(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")
	cmd := app.Init()
	// Init() 可以返回 nil（无可执行命令）
	// 只要不 panic 即可
	_ = cmd
}

// TestAppView 测试 View 渲染
func TestAppView(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")
	app.Width = 80
	app.Height = 24
	view := app.View()
	if view.Content == "" {
		t.Fatal("View() 返回了空内容")
	}
	// 应包含状态栏和输入区等
	if len(view.Content) < 10 {
		t.Fatalf("View 内容太短: %q", view.Content)
	}
}

// TestAppUpdateWindowSize 测试窗口调整大小
func TestAppUpdateWindowSize(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")
	msg := tea.WindowSizeMsg{Width: 100, Height: 40}
	model, cmd := app.Update(msg)
	if cmd != nil {
		t.Fatal("WindowSizeMsg 不应产生命令")
	}
	updatedApp, ok := model.(*App)
	if !ok {
		t.Fatal("Update 返回的不是 *App 类型")
	}
	if updatedApp.Width != 100 {
		t.Fatalf("期望 Width=100，得到 %d", updatedApp.Width)
	}
	if updatedApp.Height != 40 {
		t.Fatalf("期望 Height=40，得到 %d", updatedApp.Height)
	}
}

// TestAppAddMessages 测试添加各类消息
func TestAppAddMessages(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 添加用户消息
	app.AddUserMessage("你好")
	if len(app.Messages) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(app.Messages))
	}
	if app.Messages[0].Role != "user" {
		t.Fatalf("期望 role=user，得到 %s", app.Messages[0].Role)
	}

	// 添加助手消息
	app.AddAssistantMessage("你好！有什么可以帮助你的？")
	if len(app.Messages) != 2 {
		t.Fatalf("期望 2 条消息，得到 %d", len(app.Messages))
	}

	// 添加工具调用
	app.AddToolCall("web_fetch", "url=https://example.com", "页面内容：...", false)
	if len(app.Messages) != 3 {
		t.Fatalf("期望 3 条消息，得到 %d", len(app.Messages))
	}
	if app.Messages[2].Role != "tool" {
		t.Fatalf("期望 role=tool，得到 %s", app.Messages[2].Role)
	}
	if app.Messages[2].ToolMsg == nil {
		t.Fatal("ToolMsg 不应为 nil")
	}
	if app.Messages[2].ToolMsg.ToolName != "web_fetch" {
		t.Fatalf("期望 ToolName=web_fetch，得到 %s", app.Messages[2].ToolMsg.ToolName)
	}

	// 添加错误消息
	app.AddErrorMessage("发生了一个错误")
	if len(app.Messages) != 4 {
		t.Fatalf("期望 4 条消息，得到 %d", len(app.Messages))
	}
	if app.Messages[3].Role != "error" {
		t.Fatalf("期望 role=error，得到 %s", app.Messages[3].Role)
	}

	// 添加思考消息
	app.AddThinking("分析中...")
	if len(app.Messages) != 5 {
		t.Fatalf("期望 5 条消息，得到 %d", len(app.Messages))
	}
	if app.Messages[4].Role != "thinking" {
		t.Fatalf("期望 role=thinking，得到 %s", app.Messages[4].Role)
	}
}

// TestAppStreaming 测试流式输出控制
func TestAppStreaming(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 开始流式输出
	app.StartStreaming()
	if !app.IsStreaming {
		t.Fatal("StartStreaming 后 IsStreaming 应为 true")
	}

	// 更新文本
	app.UpdateStreamingText("Hello")
	if app.Streaming.FullText() != "Hello" {
		t.Fatalf("期望文本为 Hello，得到 %s", app.Streaming.FullText())
	}

	// 停止流式输出
	app.StopStreaming()
	if app.IsStreaming {
		t.Fatal("StopStreaming 后 IsStreaming 应为 false")
	}
}

// TestAppCommandPalette 测试命令面板触发
func TestAppCommandPalette(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 打开命令面板
	app.CommandPanel.Open()
	if !app.CommandPanel.IsVisible() {
		t.Fatal("命令面板应可见")
	}

	// 关闭
	app.CommandPanel.Close()
	if app.CommandPanel.IsVisible() {
		t.Fatal("命令面板应不可见")
	}
}

// TestAppThemeCommands 测试主题命令
func TestAppThemeCommands(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 主题命令参数
	themes := app.ListThemes()
	if len(themes) == 0 {
		t.Fatal("至少应有一个内置主题")
	}
}

// TestAppAddMessages 测试添加各类消息
func TestAppToolCallError(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	app.AddToolCall("web_search", "query=golang", "API 调用失败", true)
	if len(app.Messages) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(app.Messages))
	}
	if !app.Messages[0].ToolMsg.IsError {
		t.Fatal("期望 IsError=true")
	}
}

// TestAppMultipleMessages 测试多条消息管理
func TestAppMultipleMessages(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 模拟一个完整对话
	app.AddUserMessage("你好")
	app.AddAssistantMessage("你好！有什么可以帮助你的？")
	app.AddToolCall("web_fetch", "url=https://example.com", "获取成功", false)
	app.AddAssistantMessage("这是获取到的内容...")
	app.AddErrorMessage("连接超时")

	if len(app.Messages) != 5 {
		t.Fatalf("期望 5 条消息，得到 %d", len(app.Messages))
	}

	// 验证顺序
	roles := []string{"user", "assistant", "tool", "assistant", "error"}
	for i, role := range roles {
		if app.Messages[i].Role != role {
			t.Fatalf("消息[%d] 期望 role=%s，得到 %s", i, role, app.Messages[i].Role)
		}
	}
}

// TestAppRenderWithTheme 测试主题化渲染
func TestAppRenderWithTheme(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")
	app.Width = 80
	app.Height = 24

	// 加入一些消息
	app.AddUserMessage("测试用户消息")
	app.AddAssistantMessage("测试助手回复")

	view := app.View()
	if view.Content == "" {
		t.Fatal("View 不应为空")
	}
}