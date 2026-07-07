// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// StatusBarView 是状态栏组件，显示模型名、Provider、token 用量、会话 ID 等
type StatusBarView struct {
	// 静态信息
	ModelName string
	Provider  string
	SessionID string

	// token 用量
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int

	// MCP 连接状态
	MCPConnected bool

	// 忙/闲状态
	IsBusy bool

	// spinner 状态
	spinnerIdx int
	startTime  time.Time
}

// NewStatusBarView 创建新的状态栏组件
func NewStatusBarView(modelName, provider, sessionID string) *StatusBarView {
	return &StatusBarView{
		ModelName:    modelName,
		Provider:     provider,
		SessionID:    sessionID,
		MCPConnected: false,
		IsBusy:       false,
		spinnerIdx:   0,
		startTime:    time.Now(),
	}
}

// Render 渲染状态栏
func (sb *StatusBarView) Render(width int, t *theme.Theme) string {
	// 缩短会话 ID
	shortSessionID := sb.SessionID
	if len(shortSessionID) > 8 {
		shortSessionID = shortSessionID[:8]
	}

	// 创建主题感知样式
	styleLeft := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorStatusFg))).
		Padding(0, 1)

	styleRight := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorStatusFg))).
		Padding(0, 1)

	styleMCPConnected := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorSuccess))).
		Padding(0, 1)

	styleMCPDisconnected := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorError))).
		Padding(0, 1)

	styleBusy := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorWarning))).
		Bold(true).
		Padding(0, 1)

	styleIdle := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Get(theme.ColorStatusBar))).
		Foreground(lipgloss.Color(t.Get(theme.ColorSuccess))).
		Padding(0, 1)

	// 左侧信息：模型名 + Provider + token
	leftParts := []string{}
	if sb.ModelName != "" {
		leftParts = append(leftParts, styleLeft.Render(sb.ModelName))
	}
	if sb.Provider != "" {
		leftParts = append(leftParts, styleLeft.Render(sb.Provider))
	}
	if sb.TotalTokens > 0 {
		tokenStr := formatTokens(sb.TotalTokens)
		leftParts = append(leftParts, styleLeft.Render(tokenStr))
	}
	leftContent := strings.Join(leftParts, " ")

	// 右侧信息：会话 ID + MCP 状态 + 忙闲状态
	rightParts := []string{}

	if shortSessionID != "" {
		rightParts = append(rightParts, styleRight.Render("ID:"+shortSessionID))
	}

	// MCP 连接状态
	if sb.MCPConnected {
		rightParts = append(rightParts, styleMCPConnected.Render("MCP ✓"))
	} else {
		rightParts = append(rightParts, styleMCPDisconnected.Render("MCP ✗"))
	}

	// 忙闲状态
	if sb.IsBusy {
		sb.spinnerIdx++
		spinner := spinnerFrames[sb.spinnerIdx%len(spinnerFrames)]
		rightParts = append(rightParts, styleBusy.Render(spinner+" 忙"))
	} else {
		rightParts = append(rightParts, styleIdle.Render("● 空闲"))
	}

	rightContent := strings.Join(rightParts, " ")

	// 用空格填充中间
	totalLen := len(leftContent) + len(rightContent)
	if totalLen < width {
		padding := width - totalLen
		if padding > 0 {
			leftContent += strings.Repeat(" ", padding)
		}
	}

	return leftContent + rightContent
}

// UpdateTokens 更新 token 用量
func (sb *StatusBarView) UpdateTokens(prompt, completion, total int) {
	sb.PromptTokens = prompt
	sb.CompletionTokens = completion
	sb.TotalTokens = total
}

// SetBusy 设置忙/闲状态
func (sb *StatusBarView) SetBusy(busy bool) {
	sb.IsBusy = busy
	sb.spinnerIdx = 0
}

// SetMCPConnected 设置 MCP 连接状态
func (sb *StatusBarView) SetMCPConnected(connected bool) {
	sb.MCPConnected = connected
}

// SetModelInfo 更新模型信息
func (sb *StatusBarView) SetModelInfo(modelName, provider string) {
	sb.ModelName = modelName
	sb.Provider = provider
}

// Tick 心跳更新（用于 spinner 动画）
func (sb *StatusBarView) Tick() {
	sb.spinnerIdx++
}

// formatTokens 格式化 token 数量
func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d tok", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fk tok", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM tok", float64(n)/1000000)
}
