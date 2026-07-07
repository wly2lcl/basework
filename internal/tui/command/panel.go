// Package command 提供命令面板系统
package command

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// 命令面板样式
var (
	stylePanelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(0, 1)

	stylePanelQuery = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	stylePanelMatch = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	stylePanelSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("63")).
				Padding(0, 1)

	stylePanelHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)
)

// PanelModel 命令面板 Bubble Tea Model
type PanelModel struct {
	registry *Registry
	query    string
	matches  []*Command
	selected int
	visible  bool
	width    int
	height   int
}

// NewPanelModel 创建命令面板
func NewPanelModel(registry *Registry) *PanelModel {
	return &PanelModel{
		registry: registry,
		query:    "",
		selected: 0,
		visible:  false,
		width:    60,
		height:   20,
	}
}

// Init 实现 tea.Model
func (pm *PanelModel) Init() tea.Cmd {
	return nil
}

// Update 处理消息
func (pm *PanelModel) Update(msg tea.Msg) (*PanelModel, tea.Cmd) {
	if !pm.visible {
		return pm, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			pm.Close()
			return pm, nil

		case "enter":
			if len(pm.matches) > 0 && pm.selected >= 0 && pm.selected < len(pm.matches) {
				cmd := pm.matches[pm.selected]
				pm.Close()
				return pm, func() tea.Msg {
					return ExecuteCommandMsg{Command: cmd.Name, Args: ""}
				}
			}
			return pm, nil

		case "up":
			if pm.selected > 0 {
				pm.selected--
			}
			return pm, nil

		case "down":
			if pm.selected < len(pm.matches)-1 {
				pm.selected++
			}
			return pm, nil

		case "tab":
			// Tab 补全：用当前选中的命令名填充查询
			if len(pm.matches) > 0 && pm.selected >= 0 && pm.selected < len(pm.matches) {
				pm.query = pm.matches[pm.selected].Name + " "
				pm.updateMatches()
			}
			return pm, nil

		case "backspace":
			if len(pm.query) > 0 {
				pm.query = pm.query[:len(pm.query)-1]
				pm.updateMatches()
			}
			return pm, nil

		default:
			// 处理可打印字符
			if msg.String() != "" && len(msg.String()) == 1 {
				r := rune(msg.String()[0])
				if r >= 32 && r <= 126 {
					pm.query += string(r)
					pm.updateMatches()
				}
			}
			return pm, nil
		}
	}

	return pm, nil
}

// View 渲染命令面板
func (pm *PanelModel) View(width int) string {
	if !pm.visible {
		return ""
	}

	pm.width = width

	// 限制面板高度
	panelHeight := len(pm.matches) + 3
	if panelHeight > pm.height {
		panelHeight = pm.height
	}
	if panelHeight < 3 {
		panelHeight = 3
	}

	var buf strings.Builder

	// 查询行
	queryLine := "/" + pm.query
	buf.WriteString(stylePanelQuery.Render(queryLine))
	buf.WriteString("\n")

	// 分隔提示
	buf.WriteString(stylePanelHint.Render("Tab 补全 ↑↓ 导航 Enter 执行 Esc 关闭"))
	buf.WriteString("\n")

	// 匹配列表
	maxDisplay := panelHeight - 2
	displayMatches := pm.matches
	if len(displayMatches) > maxDisplay {
		// 保持选中项在可视区域
		start := pm.selected - maxDisplay/2
		if start < 0 {
			start = 0
		}
		if start+maxDisplay > len(displayMatches) {
			start = len(displayMatches) - maxDisplay
		}
		displayMatches = displayMatches[start : start+maxDisplay]
	}

	for i, cmd := range displayMatches {
		line := cmd.Name
		if cmd.Args != "" {
			line += " " + cmd.Args
		}
		desc := cmd.Description
		if desc != "" {
			line += " — " + desc
		}

		if i == pm.selected {
			buf.WriteString(stylePanelSelected.Render("→ " + line))
		} else {
			buf.WriteString(stylePanelMatch.Render("  " + line))
		}
		buf.WriteString("\n")
	}

	return stylePanelBorder.Width(width - 4).Render(strings.TrimRight(buf.String(), "\n"))
}

// Open 打开命令面板
func (pm *PanelModel) Open() {
	pm.visible = true
	pm.query = ""
	pm.selected = 0
	pm.updateMatches()
}

// Close 关闭命令面板
func (pm *PanelModel) Close() {
	pm.visible = false
	pm.query = ""
	pm.selected = 0
	pm.matches = nil
}

// IsVisible 返回面板是否可见
func (pm *PanelModel) IsVisible() bool {
	return pm.visible
}

// Query 返回当前查询字符串
func (pm *PanelModel) Query() string {
	return pm.query
}

// SetQuery 设置查询并更新匹配
func (pm *PanelModel) SetQuery(q string) {
	pm.query = q
	pm.selected = 0
	pm.updateMatches()
}

// updateMatches 更新匹配列表
func (pm *PanelModel) updateMatches() {
	pm.matches = pm.registry.Search(pm.query)
	if pm.selected >= len(pm.matches) {
		pm.selected = len(pm.matches) - 1
	}
	if pm.selected < 0 && len(pm.matches) > 0 {
		pm.selected = 0
	}
}

// ExecuteCommandMsg 表示执行命令的消息
type ExecuteCommandMsg struct {
	Command string
	Args    string
}
