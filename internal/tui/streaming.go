// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// 流式渲染样式

// spinner 动画帧
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// StreamingView 是流式输出组件，支持逐字输出和工具调用进度指示
type StreamingView struct {
	// 当前累积的文本
	currentText string

	// 思考过程文本
	thinkingText string

	// 工具调用进度
	toolInProgress string // 当前正在执行的工具名

	// 状态
	isRunning bool
	startTime time.Time

	// 宽度
	width int

	// spinner 状态
	spinnerIdx int
}

// NewStreamingView 创建新的流式渲染组件
func NewStreamingView() *StreamingView {
	return &StreamingView{
		currentText:    "",
		thinkingText:   "",
		toolInProgress: "",
		isRunning:      false,
		startTime:      time.Now(),
		width:          80,
		spinnerIdx:     0,
	}
}

// Start 开始流式输出
func (sv *StreamingView) Start() {
	sv.isRunning = true
	sv.currentText = ""
	sv.thinkingText = ""
	sv.toolInProgress = ""
	sv.startTime = time.Now()
	sv.spinnerIdx = 0
}

// Stop 停止流式输出
func (sv *StreamingView) Stop() {
	sv.isRunning = false
	sv.toolInProgress = ""
}

// UpdateText 更新流式输出文本
func (sv *StreamingView) UpdateText(text string) {
	sv.currentText = text
}

// AppendText 追加流式输出文本
func (sv *StreamingView) AppendText(delta string) {
	sv.currentText += delta
}

// AppendThinking 追加思考过程文本
func (sv *StreamingView) AppendThinking(delta string) {
	sv.thinkingText += delta
}

// SetToolInProgress 设置当前正在执行的工具
func (sv *StreamingView) SetToolInProgress(toolName string) {
	sv.toolInProgress = toolName
}

// Render 渲染流式输出区域
func (sv *StreamingView) Render(width int, t *theme.Theme) string {
	if !sv.isRunning {
		return ""
	}

	// 创建主题感知样式
	styleStreamText := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorAssistMsg)))

	styleThinkingStream := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorThinking))).
		Italic(true)

	styleToolIndicator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Get(theme.ColorToolCall))).
		Bold(true)

	var buf strings.Builder

	// 渲染思考过程（如果有）
	if sv.thinkingText != "" {
		lines := strings.Split(sv.thinkingText, "\n")
		for _, line := range lines {
			if line != "" {
				buf.WriteString(styleThinkingStream.Render("… " + line))
				buf.WriteString("\n")
			}
		}
	}

	// 渲染累积的文本
	if sv.currentText != "" {
		buf.WriteString(styleStreamText.Render(sv.currentText))
		buf.WriteString("\n")
	}

	// 渲染工具调用进度
	if sv.toolInProgress != "" {
		spinner := spinnerFrames[sv.spinnerIdx%len(spinnerFrames)]
		elapsed := time.Since(sv.startTime).Truncate(time.Second)
		indicator := fmt.Sprintf("%s %s %s [%s]",
			spinner,
			styleToolIndicator.Render(sv.toolInProgress),
			"进行中…",
			elapsed.String(),
		)
		buf.WriteString(indicator)
		buf.WriteString("\n")
		sv.spinnerIdx++
	}

	return strings.TrimRight(buf.String(), "\n")
}

// FullText 返回完整的流式输出文本
func (sv *StreamingView) FullText() string {
	return sv.currentText
}

// IsRunning 返回是否正在流式输出
func (sv *StreamingView) IsRunning() bool {
	return sv.isRunning
}

// Tick 心跳更新（用于 spinner 动画）
func (sv *StreamingView) Tick() {
	sv.spinnerIdx++
}

// TickerInterval 返回 spinner 更新的间隔
var TickerInterval = 100 * time.Millisecond

// 确保 time 包被使用
var _ = fmt.Sprintf
