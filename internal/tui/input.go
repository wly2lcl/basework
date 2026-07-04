// Package tui 提供基于 Bubble Tea 的终端用户界面
package tui

import (
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// 输入区样式
var (
	styleInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)

	styleInputPrefix = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	styleHistoryLine = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	stylePrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
)

// InputView 是输入区组件，支持多行输入、历史导航、Tab 补全
type InputView struct {
	// 当前输入缓冲区
	lines       []string
	currentLine int
	cursorPos   int // 在当前行内的光标位置

	// 输入历史
	history     []string
	historyPos  int // -1 表示当前输入，>=0 表示历史中的位置

	// Tab 补全
	showCompletion bool
	completions    []string
	completionIdx  int

	// 提示文本
	prompt string
}

// NewInputView 创建新的输入组件
func NewInputView() *InputView {
	return &InputView{
		lines:       []string{""},
		currentLine: 0,
		cursorPos:   0,
		history:     make([]string, 0),
		historyPos:  -1,
		prompt:      "> ",
	}
}

// Init 初始化输入组件（实现 tea.Model 子组件的 Init 风格）
func (iv *InputView) Init() tea.Cmd {
	return nil
}

// Update 处理按键消息
func (iv *InputView) Update(msg tea.Msg) (*InputView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			// 已由 App 处理，这里不做操作防止重复
			return iv, nil

		case "shift+enter":
			// 换行
			iv.insertNewline()
			return iv, nil

		case "up":
			if iv.showCompletion {
				iv.completionIdx--
				if iv.completionIdx < 0 {
					iv.completionIdx = len(iv.completions) - 1
				}
			} else {
				iv.navigateHistory(-1)
			}
			return iv, nil

		case "down":
			if iv.showCompletion {
				iv.completionIdx++
				if iv.completionIdx >= len(iv.completions) {
					iv.completionIdx = 0
				}
			} else {
				iv.navigateHistory(1)
			}
			return iv, nil

		case "tab":
			iv.handleTabCompletion()
			return iv, nil

		case "backspace":
			iv.deleteCharBefore()
			return iv, nil

		case "delete":
			iv.deleteCharAfter()
			return iv, nil

		case "left":
			if iv.cursorPos > 0 {
				iv.cursorPos--
			}
			return iv, nil

		case "right":
			line := iv.currentLineText()
			if iv.cursorPos < len(line) {
				iv.cursorPos++
			}
			return iv, nil

		case "home":
			iv.cursorPos = 0
			return iv, nil

		case "end":
			iv.cursorPos = len(iv.currentLineText())
			return iv, nil

		default:
			// 处理可打印字符
			if msg.String() != "" && len(msg.String()) == 1 {
				r := rune(msg.String()[0])
				// 只处理可打印字符
				if r >= 32 && r <= 126 {
					iv.insertRune(r)
				}
			}
			return iv, nil
		}
	}
	return iv, nil
}

// Render 渲染输入区
func (iv *InputView) Render(width int) string {
	// 计算输入框宽度（减去 prompt 和边框）
	inputWidth := width - 4 // 边框占用 2 边，prompt 占 2
	if inputWidth < 10 {
		inputWidth = 10
	}

	// 渲染当前行
	line := iv.currentLineText()
	display := iv.prompt + line

	// 如果正在补全，显示补全列表
	if iv.showCompletion && len(iv.completions) > 0 {
		completionText := make([]string, 0, len(iv.completions))
		for i, c := range iv.completions {
			if i == iv.completionIdx {
				completionText = append(completionText, "→ "+c)
			} else {
				completionText = append(completionText, "  "+c)
			}
		}
		display += "\n" + strings.Join(completionText, "  ")
	}

	return styleInputBox.Width(inputWidth).Render(display)
}

// Text 返回当前输入框的完整文本
func (iv *InputView) Text() string {
	return strings.Join(iv.lines, "\n")
}

// SetText 设置输入框文本
func (iv *InputView) SetText(text string) {
	iv.lines = strings.Split(text, "\n")
	if len(iv.lines) == 0 {
		iv.lines = []string{""}
	}
	iv.currentLine = len(iv.lines) - 1
	iv.cursorPos = len(iv.lines[iv.currentLine])
	iv.showCompletion = false
}

// Reset 清空输入框
func (iv *InputView) Reset() {
	// 将当前输入加入历史
	current := strings.TrimSpace(iv.Text())
	if current != "" {
		iv.history = append(iv.history, current)
	}
	iv.lines = []string{""}
	iv.currentLine = 0
	iv.cursorPos = 0
	iv.historyPos = -1
	iv.showCompletion = false
}

// currentLineText 返回当前行的文本
func (iv *InputView) currentLineText() string {
	if iv.currentLine >= 0 && iv.currentLine < len(iv.lines) {
		return iv.lines[iv.currentLine]
	}
	return ""
}

// insertRune 在当前光标位置插入字符
func (iv *InputView) insertRune(r rune) {
	line := iv.currentLineText()
	if iv.cursorPos >= len(line) {
		iv.lines[iv.currentLine] = line + string(r)
	} else {
		iv.lines[iv.currentLine] = line[:iv.cursorPos] + string(r) + line[iv.cursorPos:]
	}
	iv.cursorPos++
	iv.showCompletion = false
}

// insertNewline 插入换行
func (iv *InputView) insertNewline() {
	line := iv.currentLineText()
	before := line[:iv.cursorPos]
	after := line[iv.cursorPos:]

	iv.lines[iv.currentLine] = before
	// 插入新行
	newLines := make([]string, 0, len(iv.lines)+1)
	newLines = append(newLines, iv.lines[:iv.currentLine+1]...)
	newLines = append(newLines, after)
	newLines = append(newLines, iv.lines[iv.currentLine+1:]...)
	iv.lines = newLines

	iv.currentLine++
	iv.cursorPos = 0
	iv.showCompletion = false
}

// deleteCharBefore 删除光标前的字符
func (iv *InputView) deleteCharBefore() {
	if iv.cursorPos > 0 {
		line := iv.currentLineText()
		iv.lines[iv.currentLine] = line[:iv.cursorPos-1] + line[iv.cursorPos:]
		iv.cursorPos--
	} else if iv.currentLine > 0 {
		// 合并到上一行
		prevLine := iv.lines[iv.currentLine-1]
		currentLine := iv.lines[iv.currentLine]
		iv.cursorPos = len(prevLine)
		iv.lines[iv.currentLine-1] = prevLine + currentLine
		iv.lines = append(iv.lines[:iv.currentLine], iv.lines[iv.currentLine+1:]...)
		iv.currentLine--
	}
	iv.showCompletion = false
}

// deleteCharAfter 删除光标后的字符
func (iv *InputView) deleteCharAfter() {
	line := iv.currentLineText()
	if iv.cursorPos < len(line) {
		iv.lines[iv.currentLine] = line[:iv.cursorPos] + line[iv.cursorPos+1:]
	}
	iv.showCompletion = false
}

// navigateHistory 导航输入历史
func (iv *InputView) navigateHistory(direction int) {
	if len(iv.history) == 0 {
		return
	}

	if direction < 0 { // 上箭头：回到更早的历史
		if iv.historyPos == -1 {
			// 保存当前输入
			iv.historyPos = len(iv.history) - 1
		} else if iv.historyPos > 0 {
			iv.historyPos--
		}
	} else { // 下箭头：回到更新的历史
		if iv.historyPos == len(iv.history)-1 {
			// 回到当前输入
			iv.historyPos = -1
		} else if iv.historyPos >= 0 {
			iv.historyPos++
		}
	}

	if iv.historyPos >= 0 && iv.historyPos < len(iv.history) {
		iv.SetText(iv.history[iv.historyPos])
	} else {
		iv.Reset()
	}
}

// handleTabCompletion 处理 Tab 补全
func (iv *InputView) handleTabCompletion() {
	line := iv.currentLineText()

	if !iv.showCompletion {
		// 查找光标前的最后一个词（文件路径）
		beforeCursor := line[:iv.cursorPos]
		words := strings.Fields(beforeCursor)
		if len(words) == 0 {
			return
		}
		lastWord := words[len(words)-1]

		// 尝试文件路径补全
		completions := findPathCompletions(lastWord)
		if len(completions) == 0 {
			return
		}

		iv.completions = completions
		iv.completionIdx = 0
		iv.showCompletion = true
	} else {
		// 选择当前补全项
		if iv.completionIdx >= 0 && iv.completionIdx < len(iv.completions) {
			selected := iv.completions[iv.completionIdx]

			// 替换行中最后一个词
			beforeCursor := line[:iv.cursorPos]
			spaceIdx := strings.LastIndex(beforeCursor, " ")
			if spaceIdx >= 0 {
				iv.lines[iv.currentLine] = line[:spaceIdx+1] + selected + line[iv.cursorPos:]
				iv.cursorPos = spaceIdx + 1 + len(selected)
			} else {
				iv.lines[iv.currentLine] = selected + line[iv.cursorPos:]
				iv.cursorPos = len(selected)
			}
		}
		iv.showCompletion = false
		iv.completions = nil
	}
}

// findPathCompletions 查找文件路径补全
func findPathCompletions(prefix string) []string {
	dir := "."
	base := prefix

	if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
		dir = prefix[:idx]
		base = prefix[idx+1:]
		if dir == "" {
			dir = "/"
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var completions []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base) {
			fullPath := prefix
			if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
				fullPath = prefix[:idx+1] + name
			} else {
				fullPath = name
			}
			if entry.IsDir() {
				fullPath += "/"
			}
			completions = append(completions, fullPath)
		}
	}

	sort.Strings(completions)
	return completions
}