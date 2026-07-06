// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// MessageView 负责渲染各种类型的消息
type MessageView struct {
	renderer *glamour.TermRenderer
}

// NewMessageView 创建消息渲染器
func NewMessageView() *MessageView {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		// 回退：不使用 glamour
		return &MessageView{renderer: nil}
	}
	return &MessageView{renderer: r}
}

// RenderUserMessage 渲染用户消息
func (mv *MessageView) RenderUserMessage(text string, t *theme.Theme) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorUserMsg))).
		Bold(true)
	prefix := style.Render("> ")
	return prefix + text
}

// RenderAssistantMessage 渲染助手消息（使用 Glamour 渲染 Markdown）
func (mv *MessageView) RenderAssistantMessage(text string) string {
	if mv.renderer == nil || text == "" {
		return text
	}
	rendered, err := mv.renderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(rendered, "\n")
}

// RenderToolCall 渲染工具调用
func (mv *MessageView) RenderToolCall(toolName, args, result string, isError bool, t *theme.Theme) string {
	styleToolName := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorToolCall))).
		Bold(true)

	styleToolResult := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorSuccess)))

	styleToolError := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorError)))

	var buf strings.Builder

	// 工具名 + 参数
	header := "🔧 " + styleToolName.Render(toolName)
	if args != "" {
		header += "(" + args + ")"
	}
	buf.WriteString(header)
	buf.WriteString("\n")

	// 结果
	if isError {
		buf.WriteString(styleToolError.Render("  ⚠ " + result))
	} else {
		buf.WriteString(styleToolResult.Render("  ✓ " + result))
	}

	return buf.String()
}

// RenderError 渲染错误消息
func (mv *MessageView) RenderError(text string, t *theme.Theme) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorError))).
		Bold(true)
	return style.Render("✗ " + text)
}

// RenderThinking 渲染思考过程
func (mv *MessageView) RenderThinking(text string, t *theme.Theme) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorThinking))).
		Italic(true)
	return style.Render("… " + text)
}

// SetWidth 设置渲染宽度
func (mv *MessageView) SetWidth(width int) {
	if mv.renderer == nil {
		return
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return
	}
	mv.renderer = r
}