package builtin

// PathChecker 是路径检查接口。
//
// 在 builtin 内部定义该接口（而非直接引用 internal/permission），是为了保持
// pkg 层不依赖 internal 层的分层约束：pkg 是可嵌入核心，internal 是终端产品
// 专用实现。
//
// 任何拥有 CheckPath(path string) (bool, string) 方法的类型都满足此接口，
// 包括 *permission.PathChecker，无需适配器。
type PathChecker interface {
	// CheckPath 检查路径是否被允许访问。
	// 返回 (是否允许, 拒绝原因)。
	CheckPath(path string) (bool, string)
}

// 包级全局路径检查器
var globalPathChecker PathChecker

// SetPathChecker 设置全局路径检查器。
func SetPathChecker(pc PathChecker) {
	globalPathChecker = pc
}

// getPathChecker 获取当前路径检查器。
func getPathChecker() PathChecker {
	return globalPathChecker
}
