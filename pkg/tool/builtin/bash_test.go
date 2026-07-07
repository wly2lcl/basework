package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- 黑名单拦截测试 ---

func TestBashBlacklist_RmRfRoot(t *testing.T) {
	b := &BashTool{PermissionMode: "default"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("rm -rf / 应被拦截")
	}
	if !strings.Contains(result.Content, "黑名单拦截") {
		t.Fatalf("内容应包含'黑名单拦截'，得到: %s", result.Content)
	}
}

func TestBashBlacklist_RmRfRootWildcard(t *testing.T) {
	b := &BashTool{PermissionMode: "default"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /*"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("rm -rf /* 应被拦截")
	}
}

func TestBashBlacklist_Mkfs(t *testing.T) {
	b := &BashTool{PermissionMode: "default"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"mkfs.ext4 /dev/sda1"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("mkfs 应被拦截")
	}
}

func TestBashBlacklist_SafeCommandNotBlocked(t *testing.T) {
	b := &BashTool{PermissionMode: "default"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("echo hello 不应被拦截: %s", result.Content)
	}
}

// --- 权限绕过测试 ---

func TestBashBlacklist_YoloModeBypass(t *testing.T) {
	b := &BashTool{PermissionMode: "yolo"}
	// yolo 模式下，rm -rf 在测试环境中不会真正删东西（sh -c 模拟）
	// 我们只用 ls 验证它没有被黑名单拦截
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"echo yolo-bypass"}`))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("YOLO 模式下不应拦截: %s", result.Content)
	}
	if !strings.Contains(result.Content, "yolo-bypass") {
		t.Fatalf("期望输出包含 yolo-bypass，得到: %s", result.Content)
	}
}

func TestBashBlacklist_DefaultModeDeny(t *testing.T) {
	b := &BashTool{PermissionMode: "default"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"dd if=/dev/zero of=/tmp/test bs=1 count=1"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("default 模式下黑名单应直接拒绝")
	}
}

func TestBashBlacklist_EmptyModeDefaultsToDeny(t *testing.T) {
	b := &BashTool{PermissionMode: ""}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /*"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("空权限模式应默认为 default，直接拒绝")
	}
}

func TestBashBlacklist_InteractiveMode(t *testing.T) {
	b := &BashTool{PermissionMode: "interactive"}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /*"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("interactive 模式命中黑名单应返回错误提示")
	}
	if !strings.Contains(result.Content, "CONFIRM") {
		t.Fatalf("交互模式应包含 CONFIRM 提示，得到: %s", result.Content)
	}
}

// --- 配置扩展测试 ---

func TestBashBlacklist_CustomBlockedCommands(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "default",
		BlockedCommands: []string{"docker rm -f"},
	}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"docker rm -f mycontainer"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("自定义黑名单 docker rm -f 应被拦截")
	}
}

func TestBashBlacklist_CustomAndBuiltinBothWork(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "default",
		BlockedCommands: []string{"docker rm -f"},
	}
	// 内置模式仍然生效
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("内置黑名单仍应生效")
	}

	// 自定义模式也生效
	result2, err2 := b.Execute(context.Background(), json.RawMessage(`{"command":"docker rm -f mycontainer"}`))
	if err2 != nil {
		t.Fatalf("Execute 不应返回 error: %v", err2)
	}
	if !result2.IsError {
		t.Fatal("自定义黑名单也应生效")
	}
}

func TestBashBlacklist_EmptyCustomCommands(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "default",
		BlockedCommands: []string{},
	}
	// 内置模式仍然生效
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"mkfs.ext4 /dev/sda1"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("空自定义配置时内置模式仍应生效")
	}
}

func TestBashBlacklist_NilCustomCommands(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "default",
		BlockedCommands: nil,
	}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"halt"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("nil 自定义配置时内置模式仍应生效")
	}
}

func TestBashBlacklist_YoloModeSkipsCustomBlocked(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "yolo",
		BlockedCommands: []string{"echo"},
	}
	// yolo 模式下即使是黑名单命令也正常执行
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"echo safe-in-yolo"}`))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("YOLO 模式下自定义黑名单也应跳过: %s", result.Content)
	}
}

func TestBashBlacklist_InteractiveModeWithCustomPattern(t *testing.T) {
	b := &BashTool{
		PermissionMode:  "interactive",
		BlockedCommands: []string{"git push --force"},
	}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"git push --force origin main"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error: %v", err)
	}
	if !result.IsError {
		t.Fatal("interactive 模式应返回确认提示")
	}
	if !strings.Contains(result.Content, "CONFIRM") {
		t.Fatalf("应包含 CONFIRM 提示，得到: %s", result.Content)
	}
}
