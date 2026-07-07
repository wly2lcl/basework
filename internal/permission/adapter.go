package permission

import (
	"context"
	"fmt"

	"github.com/wly2lcl/basework/pkg/agent"
)

// PermissionAdapter 将 *Checker 适配为 agent.PermissionChecker 接口。
type PermissionAdapter struct {
	inner *Checker
}

// NewPermissionAdapter 创建适配器。
func NewPermissionAdapter(c *Checker) *PermissionAdapter {
	return &PermissionAdapter{inner: c}
}

// Check 实现 agent.PermissionChecker。
func (a *PermissionAdapter) Check(ctx context.Context, toolName string, args map[string]interface{}) (bool, error) {
	return a.inner.Check(ctx, toolName, args)
}

// CheckPath 实现 agent.PermissionChecker。
// 注意：仅当 inner 同时是 PathChecker 时有效；否则返回允许（不限制路径）。
func (a *PermissionAdapter) CheckPath(path string) error {
	// Checker 不直接支持 CheckPath，使用 PathChecker 做路径检查
	// 如果调用方需要路径检查，建议直接使用 *PathChecker
	return nil
}

// Ensure PermissionAdapter implements agent.PermissionChecker.
var _ agent.PermissionChecker = (*PermissionAdapter)(nil)

// PathPermissionAdapter 将 *PathChecker 适配为 agent.PermissionChecker 接口。
type PathPermissionAdapter struct {
	inner        *Checker
	pathChecker  *PathChecker
}

// NewPathPermissionAdapter 创建同时支持权限和路径检查的适配器。
func NewPathPermissionAdapter(c *Checker, pc *PathChecker) *PathPermissionAdapter {
	return &PathPermissionAdapter{inner: c, pathChecker: pc}
}

// Check 实现 agent.PermissionChecker。
func (a *PathPermissionAdapter) Check(ctx context.Context, toolName string, args map[string]interface{}) (bool, error) {
	return a.inner.Check(ctx, toolName, args)
}

// CheckPath 实现 agent.PermissionChecker。
func (a *PathPermissionAdapter) CheckPath(path string) error {
	if a.pathChecker == nil {
		return nil
	}
	allowed, reason := a.pathChecker.CheckPath(path)
	if !allowed {
		return fmt.Errorf("路径被拒绝: %s (%s)", path, reason)
	}
	return nil
}

// Ensure PathPermissionAdapter implements agent.PermissionChecker.
var _ agent.PermissionChecker = (*PathPermissionAdapter)(nil)