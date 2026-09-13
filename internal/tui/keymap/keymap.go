// Package keymap 提供键盘绑定系统
package keymap

import (
	"fmt"
	"strings"
)

// Layer 定义按键绑定层
type Layer string

const (
	LayerGlobal Layer = "global"
	LayerApp    Layer = "app"
	LayerExit   Layer = "exit"
)

// Action 定义操作类型
type Action string

const (
	ActionQuit           Action = "quit"
	ActionClear          Action = "clear"
	ActionSubmit         Action = "submit"
	ActionNewline        Action = "newline"
	ActionHistoryUp      Action = "history_up"
	ActionHistoryDown    Action = "history_down"
	ActionTabComplete    Action = "tab_complete"
	ActionCursorLeft     Action = "cursor_left"
	ActionCursorRight    Action = "cursor_right"
	ActionCursorHome     Action = "cursor_home"
	ActionCursorEnd      Action = "cursor_end"
	ActionDeleteBefore   Action = "delete_before"
	ActionDeleteAfter    Action = "delete_after"
	ActionCommandPalette Action = "command_palette"
	ActionToggleJobs     Action = "toggle_jobs"
	ActionEscape         Action = "escape"
	ActionFocusInput     Action = "focus_input"
)

// KeyBinding 定义键绑定
type KeyBinding struct {
	Key    string `json:"key"`    // "ctrl+c", "ctrl+l", "esc", 等
	Action Action `json:"action"` // 对应的操作
	Layer  Layer  `json:"layer"`  // 所属层
}

// 默认键绑定列表
var DefaultBindings = []KeyBinding{
	// 全局绑定
	{Key: "ctrl+c", Action: ActionQuit, Layer: LayerGlobal},
	{Key: "ctrl+d", Action: ActionQuit, Layer: LayerGlobal},
	{Key: "ctrl+p", Action: ActionCommandPalette, Layer: LayerGlobal},
	{Key: "ctrl+j", Action: ActionToggleJobs, Layer: LayerGlobal},
	{Key: "esc", Action: ActionEscape, Layer: LayerGlobal},

	// 应用层绑定（输入区）
	{Key: "enter", Action: ActionSubmit, Layer: LayerApp},
	{Key: "shift+enter", Action: ActionNewline, Layer: LayerApp},
	{Key: "tab", Action: ActionTabComplete, Layer: LayerApp},
	{Key: "up", Action: ActionHistoryUp, Layer: LayerApp},
	{Key: "down", Action: ActionHistoryDown, Layer: LayerApp},
	{Key: "left", Action: ActionCursorLeft, Layer: LayerApp},
	{Key: "right", Action: ActionCursorRight, Layer: LayerApp},
	{Key: "home", Action: ActionCursorHome, Layer: LayerApp},
	{Key: "end", Action: ActionCursorEnd, Layer: LayerApp},
	{Key: "backspace", Action: ActionDeleteBefore, Layer: LayerApp},
	{Key: "delete", Action: ActionDeleteAfter, Layer: LayerApp},

	// 退出层绑定
	{Key: "ctrl+c", Action: ActionQuit, Layer: LayerExit},
	{Key: "y", Action: ActionQuit, Layer: LayerExit},
	{Key: "n", Action: ActionEscape, Layer: LayerExit},
	{Key: "enter", Action: ActionQuit, Layer: LayerExit},
}

// DescribeAction 返回操作的人类可读描述
func DescribeAction(a Action) string {
	descriptions := map[Action]string{
		ActionQuit:           "退出程序",
		ActionClear:          "清空消息",
		ActionSubmit:         "提交输入",
		ActionNewline:        "换行",
		ActionHistoryUp:      "上一条历史",
		ActionHistoryDown:    "下一条历史",
		ActionTabComplete:    "Tab 补全",
		ActionCursorLeft:     "光标左移",
		ActionCursorRight:    "光标右移",
		ActionCursorHome:     "光标到行首",
		ActionCursorEnd:      "光标到行尾",
		ActionDeleteBefore:   "删除前一个字符",
		ActionDeleteAfter:    "删除后一个字符",
		ActionCommandPalette: "打开命令面板",
		ActionToggleJobs:     "后台任务卡片",
		ActionEscape:         "取消/关闭",
		ActionFocusInput:     "聚焦输入区",
	}
	if desc, ok := descriptions[a]; ok {
		return desc
	}
	return string(a)
}

// FormatKey 格式化按键为可读形式
func FormatKey(key string) string {
	replacements := map[string]string{
		"ctrl+":       "Ctrl+",
		"shift+enter": "Shift+Enter",
		"shift+":      "Shift+",
		"enter":       "Enter",
		"tab":         "Tab",
		"esc":         "Esc",
		"up":          "↑",
		"down":        "↓",
		"left":        "←",
		"right":       "→",
		"home":        "Home",
		"end":         "End",
		"backspace":   "Backspace",
		"delete":      "Delete",
		"space":       "Space",
	}

	if result, ok := replacements[key]; ok {
		return result
	}
	if strings.HasPrefix(key, "ctrl+") {
		return "Ctrl+" + strings.ToUpper(strings.TrimPrefix(key, "ctrl+"))
	}
	return key
}

// ValidateBindings 验证绑定列表是否有冲突
func ValidateBindings(bindings []KeyBinding) error {
	// 检查同一键在同一层绑定到多个动作
	type layerKey struct {
		layer Layer
		key   string
	}
	seen := make(map[layerKey]Action)
	for _, b := range bindings {
		lk := layerKey{layer: b.Layer, key: b.Key}
		if existing, ok := seen[lk]; ok {
			if existing != b.Action {
				return fmt.Errorf("键 %q 在层 %q 上同时绑定到 %q 和 %q",
					b.Key, b.Layer, existing, b.Action)
			}
		}
		seen[lk] = b.Action
	}
	return nil
}
