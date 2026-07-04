// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// 消息类型定义

// UserInputMsg 表示用户输入消息
type UserInputMsg struct {
	Text string
}

// AgentResponseMsg 表示 Agent 响应消息
type AgentResponseMsg struct {
	Text string
}

// ToolCallMsg 表示工具调用消息
type ToolCallMsg struct {
	ToolName   string
	Args       string
	Result     string
	IsError    bool
}

// ErrorMsg 表示错误消息
type ErrorMsg struct {
	Err error
}

// 布局常量
const (
	statusBarHeight = 1
	inputAreaHeight = 5
)

// 颜色样式
var (
	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleUserMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")) // 蓝色

	styleAssistantMsg = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")) // 白色

	styleToolCall = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")) // 橙色

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // 红色
			Bold(true)

	styleThinking = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")). // 灰色
			Italic(true)
)

// Message 表示一条消息记录
type Message struct {
	Role    string // "user" / "assistant" / "tool" / "error" / "thinking"
	Content string
	ToolMsg *ToolCallMsg // Role 为 "tool" 时填充
}

// App 是 TUI 应用的主结构体，实现了 Bubble Tea Model 接口
type App struct {
	// 消息历史
	Messages []Message

	// 子组件
	Input     *InputView
	StatusBar *StatusBarView
	Streaming *StreamingView

	// 布局尺寸
	Width  int
	Height int

	// 状态
	IsStreaming bool
	ShowCursor  bool
}

// NewApp 创建新的 TUI 应用
func NewApp(modelName, provider, sessionID string) *App {
	return &App{
		Messages:  make([]Message, 0),
		Input:     NewInputView(),
		StatusBar: NewStatusBarView(modelName, provider, sessionID),
		Streaming: NewStreamingView(),
		Width:     80,
		Height:    24,
	}
}

// Init 实现 Bubble Tea Model 接口
func (m *App) Init() tea.Cmd {
	return nil
}

// Update 实现 Bubble Tea Model 接口
func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		// 更新 streaming view 宽度
		m.Streaming.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		// Ctrl+C 退出程序
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// Ctrl+D 退出程序（仅当输入区为空时）
		if msg.String() == "ctrl+d" {
			if strings.TrimSpace(m.Input.Text()) == "" {
				return m, tea.Quit
			}
		}

		// Enter 提交输入
		if msg.String() == "enter" {
			text := strings.TrimSpace(m.Input.Text())
			if text != "" {
				// 将用户消息加入历史
				m.Messages = append(m.Messages, Message{
					Role:    "user",
					Content: text,
				})

				// 清空输入区
				m.Input.Reset()

				// 返回用户输入消息，由外部处理
				return m, func() tea.Msg {
					return UserInputMsg{Text: text}
				}
			}
			return m, nil
		}

		// 其他按键事件传给输入组件
		var inputCmd tea.Cmd
		m.Input, inputCmd = m.Input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}

	default:
		// 将消息也传给输入组件
		var inputCmd tea.Cmd
		m.Input, inputCmd = m.Input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View 实现 Bubble Tea Model 接口
func (m *App) View() tea.View {
	// 计算各区域高度
	msgAreaHeight := m.Height - statusBarHeight - inputAreaHeight
	if msgAreaHeight < 1 {
		msgAreaHeight = 1
	}

	// 渲染状态栏
	statusBar := m.StatusBar.Render(m.Width)

	// 渲染消息区
	msgContent := m.renderMessages(msgAreaHeight)

	// 渲染输入区
	inputContent := m.Input.Render(m.Width)

	// 组装布局
	content := fmt.Sprintf("%s\n%s\n%s",
		statusBar,
		msgContent,
		inputContent,
	)

	return tea.NewView(content)
}

// renderMessages 渲染消息区域
func (m *App) renderMessages(height int) string {
	var buf strings.Builder

	// 显示消息历史
	for _, msg := range m.Messages {
		buf.WriteString(m.renderMessage(msg))
		buf.WriteString("\n")
	}

	// 如果正在流式输出，渲染 streaming view
	if m.IsStreaming {
		buf.WriteString(m.Streaming.Render(m.Width))
		buf.WriteString("\n")
	}

	// 限制消息区高度，取最后 height 行
	lines := strings.Split(buf.String(), "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}

	return strings.Join(lines, "\n")
}

// renderMessage 渲染单条消息
func (m *App) renderMessage(msg Message) string {
	switch msg.Role {
	case "user":
		return styleUserMsg.Render("> " + msg.Content)
	case "assistant":
		return styleAssistantMsg.Render(msg.Content)
	case "tool":
		if msg.ToolMsg != nil {
			tm := msg.ToolMsg
			prefix := fmt.Sprintf("🔧 %s(%s)", tm.ToolName, tm.Args)
			result := tm.Result
			if tm.IsError {
				return styleError.Render(prefix + " => " + result)
			}
			return styleToolCall.Render(prefix + " => " + result)
		}
		return styleToolCall.Render(msg.Content)
	case "error":
		return styleError.Render(msg.Content)
	case "thinking":
		return styleThinking.Render(msg.Content)
	default:
		return msg.Content
	}
}

// AddUserMessage 添加用户消息
func (m *App) AddUserMessage(text string) {
	m.Messages = append(m.Messages, Message{
		Role:    "user",
		Content: text,
	})
}

// AddAssistantMessage 添加助手消息
func (m *App) AddAssistantMessage(text string) {
	m.Messages = append(m.Messages, Message{
		Role:    "assistant",
		Content: text,
	})
}

// AddToolCall 添加工具调用记录
func (m *App) AddToolCall(toolName, args, result string, isError bool) {
	m.Messages = append(m.Messages, Message{
		Role: "tool",
		ToolMsg: &ToolCallMsg{
			ToolName: toolName,
			Args:     args,
			Result:   result,
			IsError:  isError,
		},
	})
}

// AddErrorMessage 添加错误消息
func (m *App) AddErrorMessage(text string) {
	m.Messages = append(m.Messages, Message{
		Role:    "error",
		Content: text,
	})
}

// AddThinking 添加思考过程消息
func (m *App) AddThinking(text string) {
	m.Messages = append(m.Messages, Message{
		Role:    "thinking",
		Content: text,
	})
}

// StartStreaming 开始流式输出
func (m *App) StartStreaming() {
	m.IsStreaming = true
	m.Streaming.Start()
}

// StopStreaming 停止流式输出
func (m *App) StopStreaming() {
	m.IsStreaming = false
	m.Streaming.Stop()
}

// UpdateStreamingText 更新流式输出文本
func (m *App) UpdateStreamingText(text string) {
	m.Streaming.UpdateText(text)
}