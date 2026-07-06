// Package main 是 basework CLI 的入口点
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd 是所有子命令的根命令
var rootCmd = &cobra.Command{
	Use:   "basework",
	Short: "basework - AI Agent 框架的 CLI 工具",
	Long: `basework 是一个 AI Agent 框架的 CLI 工具。
支持交互式 Agent 会话、模型管理、会话管理等。`,
	Run: func(cmd *cobra.Command, args []string) {
		// 无子命令时默认运行 agent
		runAgent(cmd, args)
	},
}

// Execute 执行根命令
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径（默认由 Discover 自动查找）")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "启用详细输出")

	// 注册子命令
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(profileCmd)
}

func main() {
	Execute()
}
