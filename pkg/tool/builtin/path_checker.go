package builtin

import "github.com/wly2lcl/basework/internal/permission"

// 包级全局路径检查器
var globalPathChecker *permission.PathChecker

// SetPathChecker 设置全局路径检查器。
func SetPathChecker(pc *permission.PathChecker) {
	globalPathChecker = pc
}

// getPathChecker 获取当前路径检查器。
func getPathChecker() *permission.PathChecker {
	return globalPathChecker
}
