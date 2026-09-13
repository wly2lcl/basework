// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// ApprovalRequestMsg 是权限审批请求（UI-002）。由运行 goroutine 经 broker
// 通知转发而来；Respond 闭包把决定按请求 ID 送回 broker（过期请求返回
// false，由调用方忽略——「批准不会错误应用到下一请求」的机制保证）。
type ApprovalRequestMsg struct {
	ID         string
	ToolName   string
	Purpose    string
	Paths      []string
	Diff       string
	RiskReason string
	// Respond 提交决定；approved=false 覆盖拒绝与关闭窗口两种情况。
	Respond func(approved bool) bool
	// Timeout 是审批等待上限；对话框超时自动拒绝（与 broker 侧一致）。
	Timeout time.Duration
}

// approvalExpiredMsg 内部消息：审批等待超时，自动拒绝并关窗。
type approvalExpiredMsg struct{ requestID string }

// RunEventMsg 是运行服务的生命周期事件（RUN-003，瞬时消息：不落盘、不重放）。
// 携带 RunID/SessionID，App.Update 按 SessionID 路由——不属于当前会话的
// 事件被忽略，旧会话的运行收尾不会漏进新会话的界面。
type RunEventMsg struct {
	RunID     string
	SessionID string
	// Kind 是事件类别（run.started / run.finished）。
	Kind string
	// Err 是运行结束时的错误文本（成功为空）。
	Err string
}

// AgentResponseMsg 表示 Agent 响应消息
type AgentResponseMsg struct {
	Text string
}

// ToolCallMsg 表示工具调用消息
type ToolCallMsg struct {
	ToolName string
	Args     string
	Result   string
	IsError  bool
}

// ErrorMsg 表示错误消息
type ErrorMsg struct {
	Err error
}

// InputHandler 处理用户输入并返回助手回复。
type InputHandler func(ctx context.Context, text string) (string, error)

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
	ctx          context.Context
	cancel       context.CancelFunc
	inputHandler InputHandler

	// SessionID 是当前会话（RUN-003 的路由键）。空表示未绑定（全部接受）。
	SessionID string
	// Jobs 是后台任务状态卡片（UI-001）。
	Jobs *JobsView
	// Resume 是重启后的恢复进度面板（UI-003）。
	Resume *ResumePanel
	// jobsReader/jobsCancel 是任务卡片运行时钩子，SwitchSession 重建
	// Jobs 时需要复用（UI-003）。
	jobsReader func(jobID string, offset int64) JobOutputMsg
	jobsCancel func(jobID string) error
	// lastRunEvent 是最近一条通过路由的 RunEventMsg（测试观察点；
	// UI 呈现属 UI-001 的状态卡片，这里只做路由与记录）。
	lastRunEvent *RunEventMsg
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
		SessionID:       sessionID,
		Jobs:            NewJobsView(nil, nil),
		Resume:          NewResumePanel(),
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

// SetInputHandler 设置用户输入处理函数。
func (m *App) SetInputHandler(handler InputHandler) {
	m.inputHandler = handler
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

	case UserInputMsg:
		if m.inputHandler == nil {
			m.AddErrorMessage("Agent 未连接")
			return m, nil
		}
		m.StartStreaming()
		text := msg.Text
		return m, func() tea.Msg {
			reply, err := m.inputHandler(m.ctx, text)
			if err != nil {
				return ErrorMsg{Err: err}
			}
			return AgentResponseMsg{Text: reply}
		}

	case AgentResponseMsg:
		m.StopStreaming()
		if strings.TrimSpace(msg.Text) != "" {
			m.AddAssistantMessage(msg.Text)
		}
		return m, nil

	case ErrorMsg:
		m.StopStreaming()
		if msg.Err != nil {
			m.AddErrorMessage(msg.Err.Error())
		}
		return m, nil

	// 以下由 agentCallback 经 tea.Program.Send 投递，
	// 在事件循环内落状态，避免与 View 并发读写。
	case StreamDeltaMsg:
		m.Streaming.AppendText(msg.Delta)
		return m, nil

	case ThinkingDeltaMsg:
		m.Streaming.AppendThinking(msg.Delta)
		return m, nil

	case ToolStartMsg:
		m.Streaming.SetToolInProgress(msg.Name)
		return m, nil

	case ToolEndMsg:
		m.Streaming.SetToolInProgress("")
		m.AddToolCall(msg.Name, msg.Args, msg.Result, msg.IsError)
		return m, nil

	case RunEventMsg:
		// 路由（RUN-003）：事件只属于其 SessionID。当前会话不匹配则
		// 整条忽略——旧会话运行的收尾事件不能影响本会话的界面状态。
		if msg.SessionID != "" && m.SessionID != "" && msg.SessionID != m.SessionID {
			return m, nil
		}
		ev := msg
		m.lastRunEvent = &ev
		// 运行终态可见性（UI-001）：文字标记区分完成/失败/取消，不靠颜色。
		if ev.Kind == "run.finished" {
			switch {
			case ev.Err == "":
				m.AddSystemNotice(fmt.Sprintf("[运行完成 %s]", ev.RunID))
			case strings.Contains(ev.Err, "context canceled"):
				m.AddSystemNotice(fmt.Sprintf("[运行已取消 %s]", ev.RunID))
			default:
				m.AddErrorMessage(fmt.Sprintf("[运行失败 %s] %s", ev.RunID, ev.Err))
			}
		}
		return m, nil

	case JobStatusMsg:
		// 后台任务快照：全量替换卡片数据（UI-001）。会话隔离（UI-003）：
		// 旧会话的快照不进入新会话的任务卡片。
		if msg.SessionID != "" && m.SessionID != "" && msg.SessionID != m.SessionID {
			return m, nil
		}
		m.Jobs.Update(msg)
		return m, nil

	case dialog.CloseDialogMsg:
		// 对话框自我关闭（审批决定完成等）：移除栈顶。
		m.DialogMgr.Close()
		return m, nil

	case ApprovalRequestMsg:
		// 同一时刻只允许一个审批窗口：后来的请求排队等当前窗口结束
		//（运行层已串行化，这里只是防御）。
		if m.DialogMgr.HasDialog() {
			if _, isApproval := m.DialogMgr.Top().(*dialog.ApprovalDialog); isApproval {
				// 已有审批窗口：直接拒绝新请求（宁可保守）。
				if msg.Respond != nil {
					msg.Respond(false)
				}
				return m, nil
			}
		}
		req := msg
		m.DialogMgr.Open(dialog.NewApproval(dialog.ApprovalRequest{
			ID:         req.ID,
			ToolName:   req.ToolName,
			Purpose:    req.Purpose,
			Paths:      req.Paths,
			Diff:       req.Diff,
			RiskReason: req.RiskReason,
		}, req.Respond))
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		return m, tea.Tick(timeout, func(time.Time) tea.Msg {
			return approvalExpiredMsg{requestID: req.ID}
		})

	case approvalExpiredMsg:
		// 超时：若对应审批窗口还开着，关闭并按拒绝应答（幂等：
		// broker 侧多半已先超时，Respond 返回 false 被忽略）。
		if top, isApproval := m.DialogMgr.Top().(*dialog.ApprovalDialog); isApproval && top.Request().ID == msg.requestID && !top.Decided() {
			top.Respond(false)
			m.DialogMgr.Close()
		}
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()

		// 恢复面板：任意按键收起（UI-003，用户已「继续工作」）。
		if m.Resume.Visible() {
			m.Resume.Hide()
		}

		// 0. 后台任务卡片可见时优先消费按键（UI-001）。
		if m.Jobs.Visible() && m.Jobs.HandleKey(key) {
			return m, nil
		}

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

			case keymap.ActionToggleJobs:
				m.Jobs.Toggle()
				return m, nil

			case keymap.ActionSubmit:
				if m.IsStreaming {
					return m, nil
				}
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

	// 后台任务状态卡片叠加（UI-001）。
	if jobsView := m.Jobs.Render(m.Width, m.Theme); jobsView != "" {
		content += "\n" + jobsView
	}

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

// SetJobsRuntime 注入后台任务的输出读取与取消能力（UI-001，cmd 侧绑定
// 归属后调用）。nil 参数表示对应能力不可用。
func (m *App) SetJobsRuntime(reader func(jobID string, offset int64) JobOutputMsg, cancel func(jobID string) error) {
	m.jobsReader = reader
	m.jobsCancel = cancel
	m.Jobs.SetRuntime(reader, cancel)
}

// SwitchSession 重新绑定会话（UI-003）：清空旧会话的运行观察点与流式
// 状态，重建任务卡片（复用运行时钩子）。事件路由按 SessionID 隔离——
// 旧会话迟到的 RunEventMsg 不会进入新会话的界面（RUN-003 语义）。
func (m *App) SwitchSession(sessionID string) {
	if m.SessionID == sessionID {
		return
	}
	m.SessionID = sessionID
	m.lastRunEvent = nil
	m.Streaming = NewStreamingView()
	m.Jobs = NewJobsView(m.jobsReader, m.jobsCancel)
	m.Resume.Hide()
	m.AddSystemNotice(fmt.Sprintf("[已切换到会话 %s]", shortIDText(sessionID)))
}

// shortIDText 是会话 ID 的短展示（UI-003；避免直接暴露完整 UUID）。
func shortIDText(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// SetResumeItems 装入恢复进度事实（UI-003）。空列表不显示面板。
func (m *App) SetResumeItems(items []ResumeItem) {
	m.Resume.SetItems(items)
	if m.Resume.Visible() {
		m.AddSystemNotice(fmt.Sprintf("[恢复检查] %d 项遗留，其中 %d 项需要重试/复核",
			len(items), m.Resume.RetryCount()))
	}
}

// AddSystemNotice 添加中性系统通知（状态卡片类信息）。
func (m *App) AddSystemNotice(text string) {
	m.Messages = append(m.Messages, Message{
		Role:    "system",
		Content: text,
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
