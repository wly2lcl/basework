// Package command 提供命令面板系统
package command

import (
	"fmt"
	"strings"
)

// App 接口——builtin 命令通过它操作应用
// 采用接口避免循环依赖
type App interface {
	SetTheme(name string) error
	ListThemes() []string
	ClearMessages()
	Quit()
}

// SessionSwitcher 是可选的会话切换能力。命令包只依赖这个窄接口，
// 避免把 runtime 生命周期和 TUI 组件耦合在一起。
type SessionSwitcher interface {
	RequestSessionSwitch(sessionID string) error
}

// RegisterBuiltinCommands 注册内置命令
func RegisterBuiltinCommands(r *Registry, app App) {
	// /help 命令
	r.Register(&Command{
		Name:        "help",
		Description: "显示帮助信息",
		Handler: func(args string) error {
			return nil // help 命令由面板自身处理
		},
	})

	// /clear 命令
	r.Register(&Command{
		Name:        "clear",
		Description: "清空消息历史",
		Handler: func(args string) error {
			app.ClearMessages()
			return nil
		},
	})

	// /theme 命令
	r.Register(&Command{
		Name:        "theme",
		Description: "切换主题（查看可用主题或设置主题）",
		Args:        "[theme_name]",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)
			if args == "" {
				themes := app.ListThemes()
				return fmt.Errorf("可用主题: %s", strings.Join(themes, ", "))
			}
			return app.SetTheme(args)
		},
	})

	// /quit 命令
	r.Register(&Command{
		Name:        "quit",
		Description: "退出程序",
		Handler: func(args string) error {
			app.Quit()
			return nil
		},
	})

	// /config 命令
	r.Register(&Command{
		Name:        "config",
		Description: "查看当前配置信息",
		Handler: func(args string) error {
			// 配置信息由 UI 层展示
			return nil
		},
	})

	// /session <id> 命令：切换到已存在的历史会话。具体的 runtime 重建
	// 由产品层注入，TUI 只负责校验入口和显示错误。
	r.Register(&Command{
		Name:        "session",
		Description: "切换到已有会话（/session <id>）",
		Args:        "<session_id>",
		Handler: func(args string) error {
			id := strings.TrimSpace(args)
			if id == "" {
				return fmt.Errorf("用法: /session <session_id>；可用 `basework session list` 查看会话")
			}
			switcher, ok := app.(SessionSwitcher)
			if !ok {
				return fmt.Errorf("当前运行模式不支持会话切换")
			}
			return switcher.RequestSessionSwitch(id)
		},
	})
}
