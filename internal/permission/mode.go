// Package permission 提供权限系统，支持三种模式（interactive/yolo/deny-all）
// 和基于工具名称与参数的规则匹配。
package permission

import "fmt"

// Mode 是权限模式类型。
type Mode string

const (
	// ModeInteractive 交互式模式：执行工具前提示用户确认。
	ModeInteractive Mode = "interactive"
	// ModeYolo YOLO 模式：允许所有工具执行，不提示。
	ModeYolo Mode = "yolo"
	// ModeDenyAll 拒绝所有模式：拒绝所有工具执行。
	ModeDenyAll Mode = "deny-all"
)

// ParseMode 解析模式字符串。
func ParseMode(s string) (Mode, error) {
	switch s {
	case "interactive":
		return ModeInteractive, nil
	case "yolo":
		return ModeYolo, nil
	case "deny-all", "deny_all":
		return ModeDenyAll, nil
	default:
		return "", fmt.Errorf("unknown permission mode: %q (valid: interactive, yolo, deny-all)", s)
	}
}

// Valid 检查模式是否有效。
func (m Mode) Valid() bool {
	switch m {
	case ModeInteractive, ModeYolo, ModeDenyAll:
		return true
	default:
		return false
	}
}