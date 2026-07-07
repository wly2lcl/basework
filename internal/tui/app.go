// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wly2lcl/basework/internal/tui/command"
	"github.com/wly2lcl/basework/internal/tui/dialog"
	"github.com/wly2lcl/basework/internal/tui/keymap"
	"github.com/wly2lcl/basework/internal/tui/plugin"
	"github.com/wly2lcl/basework/internal/tui/theme"
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

// 主题感知的样式创建函数
func makeStatusBarStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorStatusFg))).
		Padding(0, 1)
}

func makeUserMsgStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorUserMsg)))
}

func makeAssistantMsgStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorAssistMsg)))
}

func makeToolCallStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorToolCall)))
}

func makeErrorStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorError))).
		Bold(true)
}

func makeThinkingStyle(t *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorThinking))).
		Italic(true)
}

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

	// 主题系统
	Theme *theme.Theme

	// 命令面板
	CommandRegistry *command.Registry
	CommandPanel    *command.PanelModel

	// 键盘绑定
	KeyResolver *keymap.Resolver

	// 对话框
	DialogMgr *dialog.Manager

	// 插件
	MountedPlugins []plugin.TUIPlugin

	// 生命周期管理
	ctx    context.Context
	cancel context.CancelFunc
}

// NewApp 创建新的 TUI 应用
func NewApp(modelName, provider, sessionID string) *App {
	// 加载主题配置
	themeCfg, err := theme.LoadThemeConfig()
	activeTheme := theme.DefaultTheme
	if err == nil {
		if t, ok := theme.GetTheme(themeCfg.Name); ok {
			activeTheme = t
		}
	}

	// 检测终端颜色能力并适配
	depth := theme.DetectColorDepth()
	activeTheme = theme.AdaptTheme(activeTheme, depth)

	// 创建命令注册表
	cmdRegistry := command.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		Messages:        make([]Message, 0),
		Input:           NewInputView(),
		StatusBar:       NewStatusBarView(modelName, provider, sessionID),
		Streaming:       NewStreamingView(),
		Width:           80,
		Height:          24,
		Theme:           activeTheme,
		CommandRegistry: cmdRegistry,
		CommandPanel:    command.NewPanelModel(cmdRegistry),
		KeyResolver:     keymap.NewResolver(keymap.DefaultBindings),
		DialogMgr:       dialog.NewManager(),
		MountedPlugins:  make([]plugin.TUIPlugin, 0),
		ctx:             ctx,
		cancel:          cancel,
	}

	// 注册内置命令
	command.RegisterBuiltinCommands(cmdRegistry, app)

	// 挂载已注册的插件
	app.MountedPlugins = plugin.List()

	return app
}

// 命令面板 App 接口实现

// SetTheme 切换主题
func (m *App) SetTheme(name string) error {
	t, ok := theme.GetTheme(name)
	if !ok {
		return fmt.Errorf("未知主题: %s，可用主题: %s", name, strings.Join(theme.ListThemes(), ", "))
	}
	m.Theme = t
	return theme.SetTheme(name)
}

// ListThemes 列出所有可用主题
func (m *App) ListThemes() []string {
	return theme.ListThemes()
}

// ClearMessages 清空消息历史
func (m *App) ClearMessages() {
	m.Messages = nil
}

// Quit 退出程序
func (m *App) Quit() {
	m.cleanup()
}

// cleanup 清理资源：取消后台 goroutine、卸载插件等
func (m *App) cleanup() {
	// 清空插件注册表
	plugin.Clear()
	m.MountedPlugins = nil

	// 取消 context，停止所有派生 goroutine
	if m.cancel != nil {
		m.cancel()
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
		m.Streaming.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()

		// 1. 先检查对话框（模态对话框拦截所有按键）
		if m.DialogMgr.HasDialog() {
			cmd := m.DialogMgr.Update(msg)
			return m, cmd
		}

		// 2. 检查命令面板
		if m.CommandPanel.IsVisible() {
			_, panelCmd := m.CommandPanel.Update(msg)
			return m, panelCmd
		}

		// 3. 通过 KeyResolver 解析按键动作
		if action, found := m.KeyResolver.Resolve(key, keymap.LayerApp); found {
			switch action {
			case keymap.ActionQuit:
				m.cleanup()
				return m, tea.Quit

			case keymap.ActionSubmit:
				text := strings.TrimSpace(m.Input.Text())
				if text != "" {
					// 检查是否以 / 开头（命令）
					if strings.HasPrefix(text, "/") {
						cmdStr := strings.TrimPrefix(text, "/")
						err := m.CommandRegistry.Execute(cmdStr)
						if err != nil {
							m.AddErrorMessage(err.Error())
						}
						m.Input.Reset()
						return m, nil
					}

					m.Messages = append(m.Messages, Message{
						Role:    "user",
						Content: text,
					})
					m.Input.Reset()
					return m, func() tea.Msg {
						return UserInputMsg{Text: text}
					}
				}
				return m, nil

			case keymap.ActionCommandPalette:
				m.CommandPanel.Open()
				return m, nil

			case keymap.ActionEscape:
				return m, nil

			default:
				// 将按键转发给输入组件
				var inputCmd tea.Cmd
				m.Input, inputCmd = m.Input.Update(msg)
				if inputCmd != nil {
					cmds = append(cmds, inputCmd)
				}
			}
		} else {
			// 未绑定——将按键转发给输入组件
			var inputCmd tea.Cmd
			m.Input, inputCmd = m.Input.Update(msg)
			if inputCmd != nil {
				cmds = append(cmds, inputCmd)
			}
		}

	case command.ExecuteCommandMsg:
		err := m.CommandRegistry.Execute(msg.Command + " " + msg.Args)
		if err != nil {
			m.AddErrorMessage(err.Error())
		}

	default:
		// 将消息传给输入组件
		var inputCmd tea.Cmd
		m.Input, inputCmd = m.Input.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}

		// 将消息传给插件
		for i, p := range m.MountedPlugins {
			updated, cmd := p.Update(msg)
			// 更新插件引用
			m.MountedPlugins[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
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

	// 计算顶层插件高度
	topPluginHeight := 0
	for _, p := range m.MountedPlugins {
		if p.Slot() == plugin.SlotTop {
			topPluginHeight++
		}
	}
	msgAreaHeight -= topPluginHeight

	// 渲染状态栏
	statusBar := m.StatusBar.Render(m.Width, m.Theme)

	// 渲染顶层插件
	var topPluginContent string
	if topPluginHeight > 0 {
		var pluginBuf strings.Builder
		for _, p := range m.MountedPlugins {
			if p.Slot() == plugin.SlotTop {
				pluginBuf.WriteString(p.View(m.Width, 1))
				pluginBuf.WriteString("\n")
			}
		}
		topPluginContent = strings.TrimRight(pluginBuf.String(), "\n")
	}

	// 渲染消息区
	msgContent := m.renderMessages(msgAreaHeight)

	// 渲染输入区
	inputContent := m.Input.Render(m.Width, m.Theme)

	// 渲染底层插件
	var bottomPluginContent string
	for _, p := range m.MountedPlugins {
		if p.Slot() == plugin.SlotBottom {
			bottomPluginContent += p.View(m.Width, 1) + "\n"
		}
	}

	// 组装布局
	content := statusBar
	if topPluginContent != "" {
		content += "\n" + topPluginContent
	}
	content += "\n" + msgContent
	if bottomPluginContent != "" {
		content += "\n" + strings.TrimRight(bottomPluginContent, "\n")
	}
	content += "\n" + inputContent

	// 命令面板叠加
	if m.CommandPanel.IsVisible() {
		panelView := m.CommandPanel.View(m.Width)
		content += "\n" + panelView
	}

	// 对话框叠加
	if m.DialogMgr.HasDialog() {
		dialogView := m.DialogMgr.View(m.Width, m.Height)
		if dialogView != "" {
			content += "\n" + dialogView
		}
	}

	return tea.NewView(content)
}

// renderMessages 渲染消息区域
func (m *App) renderMessages(height int) string {
	var buf strings.Builder

	for _, msg := range m.Messages {
		buf.WriteString(m.renderMessage(msg))
		buf.WriteString("\n")
	}

	if m.IsStreaming {
		buf.WriteString(m.Streaming.Render(m.Width, m.Theme))
		buf.WriteString("\n")
	}

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
		return makeUserMsgStyle(m.Theme).Render("> " + msg.Content)
	case "assistant":
		return makeAssistantMsgStyle(m.Theme).Render(msg.Content)
	case "tool":
		if msg.ToolMsg != nil {
			tm := msg.ToolMsg
			prefix := fmt.Sprintf("🔧 %s(%s)", tm.ToolName, tm.Args)
			result := tm.Result
			if tm.IsError {
				return makeErrorStyle(m.Theme).Render(prefix + " => " + result)
			}
			return makeToolCallStyle(m.Theme).Render(prefix + " => " + result)
		}
		return makeToolCallStyle(m.Theme).Render(msg.Content)
	case "error":
		return makeErrorStyle(m.Theme).Render(msg.Content)
	case "thinking":
		return makeThinkingStyle(m.Theme).Render(msg.Content)
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