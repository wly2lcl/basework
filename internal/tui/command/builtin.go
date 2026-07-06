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
}