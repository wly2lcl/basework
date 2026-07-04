// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// 状态栏样式
var (
	styleStatusLeft = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleStatusRight = lipgloss.NewStyle().
				Background(lipgloss.Color("63")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	styleStatusMCPConnected = lipgloss.NewStyle().
				Background(lipgloss.Color("63")).
				Foreground(lipgloss.Color("120")). // 绿色
				Padding(0, 1)

	styleStatusMCPDisconnected = lipgloss.NewStyle().
					Background(lipgloss.Color("63")).
					Foreground(lipgloss.Color("196")). // 红色
					Padding(0, 1)

	styleStatusBusy = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("228")). // 黄色
			Bold(true).
			Padding(0, 1)

	styleStatusIdle = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("120")). // 绿色
			Padding(0, 1)
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
func (sb *StatusBarView) Render(width int) string {
	// 缩短会话 ID
	shortSessionID := sb.SessionID
	if len(shortSessionID) > 8 {
		shortSessionID = shortSessionID[:8]
	}

	// 左侧信息：模型名 + Provider + token
	leftParts := []string{}
	if sb.ModelName != "" {
		leftParts = append(leftParts, styleStatusLeft.Render(sb.ModelName))
	}
	if sb.Provider != "" {
		leftParts = append(leftParts, styleStatusLeft.Render(sb.Provider))
	}
	if sb.TotalTokens > 0 {
		tokenStr := formatTokens(sb.TotalTokens)
		leftParts = append(leftParts, styleStatusLeft.Render(tokenStr))
	}
	leftContent := strings.Join(leftParts, " ")

	// 右侧信息：会话 ID + MCP 状态 + 忙闲状态
	rightParts := []string{}

	if shortSessionID != "" {
		rightParts = append(rightParts, styleStatusRight.Render("ID:"+shortSessionID))
	}

	// MCP 连接状态
	if sb.MCPConnected {
		rightParts = append(rightParts, styleStatusMCPConnected.Render("MCP ✓"))
	} else {
		rightParts = append(rightParts, styleStatusMCPDisconnected.Render("MCP ✗"))
	}

	// 忙闲状态
	if sb.IsBusy {
		sb.spinnerIdx++
		spinner := spinnerFrames[sb.spinnerIdx%len(spinnerFrames)]
		rightParts = append(rightParts, styleStatusBusy.Render(spinner+" 忙"))
	} else {
		rightParts = append(rightParts, styleStatusIdle.Render("● 空闲"))
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