//go:build sqlite

package main

func init() {
	// 在 sqlite build tag 启用时注册 migrate 子命令
	rootCmd.AddCommand(migrateCmd)
}
