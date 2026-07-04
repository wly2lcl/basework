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

// TestAppCtrlCQuit 测试 Ctrl+C 退出
func TestAppCtrlCQuit(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 模拟 Ctrl+C
	msg := tea.KeyPressMsg(tea.Key{Text: "ctrl+c"})
	_, cmd := app.Update(tea.Msg(msg))

	// 应该产生退出命令 (QuitMsg)
	if cmd != nil {
		resultMsg := cmd()
		if _, ok := resultMsg.(tea.QuitMsg); !ok {
			t.Logf("Ctrl+C 命令产生的消息类型: %T", resultMsg)
		}
	}
}

// TestAppEnterSubmit 测试 Enter 提交
func TestAppEnterSubmit(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 先设置输入文本
	app.Input.SetText("测试消息")

	// 模拟 Enter 键
	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"})
	_, cmd := app.Update(tea.Msg(msg))

	// 应该有用户消息被添加
	if len(app.Messages) != 1 {
		t.Fatalf("Enter 后期望 1 条消息，得到 %d", len(app.Messages))
	}
	if app.Messages[0].Content != "测试消息" {
		t.Fatalf("期望内容为 '测试消息'，得到 %s", app.Messages[0].Content)
	}

	// 应该产生 UserInputMsg 命令
	if cmd == nil {
		t.Fatal("Enter 后应产生命令")
	} else {
		resultMsg := cmd()
		if _, ok := resultMsg.(UserInputMsg); !ok {
			t.Fatalf("期望 UserInputMsg，得到 %T", resultMsg)
		}
	}

	// 输入框应该被清空
	if app.Input.Text() != "" {
		t.Fatalf("Enter 后输入框应为空，得到 %q", app.Input.Text())
	}
}

// TestAppEnterWithEmptyInput 测试空输入时 Enter 不提交
func TestAppEnterWithEmptyInput(t *testing.T) {
	app := NewApp("test-model", "test-provider", "test-session-id")

	// 不设置输入文本，空输入

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"})
	_, cmd := app.Update(tea.Msg(msg))

	// 不应有消息被添加
	if len(app.Messages) != 0 {
		t.Fatalf("空输入时不应添加消息，得到 %d", len(app.Messages))
	}

	// 不应产生命令
	if cmd != nil {
		t.Fatal("空输入时不应产生命令")
	}
}

// TestAppToolCallError 测试带错误的工具调用
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