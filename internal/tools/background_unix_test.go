//go:build unix

package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
)

// grandchildPIDAlive 报告进程是否还存在（EPERM 仍算存活）。
func grandchildPIDAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// grandchildPIDFromFile 读取命令写出的孙进程 pid。
func grandchildPIDFromFile(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 pid 文件失败: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("pid 文件内容不是数字: %q", string(b))
	}
	return pid
}

// waitForFileToAppear 等待文件出现。
func waitForFileToAppear(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待文件出现超时: %s", path)
}

// waitForProcessGone 等待进程消失，超时即失败。
func waitForProcessGone(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !grandchildPIDAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("进程 %d 在 %v 内仍存活（残留）", pid, within)
}

// TestBackgroundBash_CancelLeavesNoGrandchildren 是本任务的核心验收：
// 取消后子进程无残留——包括直接子进程（sh）和它的孙进程（后台 sleep）。
func TestBackgroundBash_CancelLeavesNoGrandchildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"sleep 30 & echo $! > `+pidFile+`; wait"}`))
	waitForFileToAppear(t, pidFile)
	grandchild := grandchildPIDFromFile(t, pidFile)
	if !grandchildPIDAlive(grandchild) {
		t.Fatalf("孙进程 %d 应已启动", grandchild)
	}

	if err := mgr.Cancel("session-a", handle.JobID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateCanceled {
		t.Fatalf("取消后应记为 canceled，得到 %s（%s）", final.State, final.Err)
	}
	if final.Err != "" {
		t.Fatalf("取消不应带失败原因，得到 %q", final.Err)
	}
	waitForProcessGone(t, grandchild, 3*time.Second)
}

// TestBackgroundBash_TimeoutIsTimedOutNotFailed 验证超时被记为 timed_out，
// 而不是一个看不出原因的 failed。
func TestBackgroundBash_TimeoutIsTimedOutNotFailed(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"sleep 30","timeout":1}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateTimedOut {
		t.Fatalf("超时应记为 timed_out，得到 %s（%s）", final.State, final.Err)
	}
	if !strings.Contains(final.Err, "超时") {
		t.Fatalf("原因应说明超时: %q", final.Err)
	}
}

// TestBackgroundBash_TimeoutKillsGrandchildren 验证超时同样清理整棵进程树。
func TestBackgroundBash_TimeoutKillsGrandchildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"sleep 30 & echo $! > `+pidFile+`; wait","timeout":1}`))
	waitForFileToAppear(t, pidFile)
	grandchild := grandchildPIDFromFile(t, pidFile)

	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateTimedOut {
		t.Fatalf("超时应记为 timed_out，得到 %s（%s）", final.State, final.Err)
	}
	waitForProcessGone(t, grandchild, 3*time.Second)
}

// TestBackgroundBash_NonZeroExitReasonIsExitCode 验证"正常非零退出"与"超时/取消"
// 在终态上可区分：退出码与原因都指向命令自己退出。
func TestBackgroundBash_NonZeroExitReasonIsExitCode(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"exit 3"}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateFailed {
		t.Fatalf("非零退出应记为 failed，得到 %s", final.State)
	}
	if final.ExitCode != 3 {
		t.Fatalf("退出码应为 3，得到 %d", final.ExitCode)
	}
	if !strings.Contains(final.Err, "退出码 3") {
		t.Fatalf("原因应指向退出码: %q", final.Err)
	}
	if final.ExitCode == -1 {
		t.Fatal("非零退出的退出码不应是取消/中断用的 -1")
	}
}

// TestBackgroundBash_KillGraceIsConfigurable 验证宽限期可配置且被真正使用：
// 忽略温和信号的命令必须在宽限期之后才被强制结束。
func TestBackgroundBash_KillGraceIsConfigurable(t *testing.T) {
	const grace = 300 * time.Millisecond
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)
	tl.KillGrace = grace

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"trap \"\" TERM; while :; do sleep 0.05; done"}`))
	time.Sleep(200 * time.Millisecond) // 等 trap 生效

	start := time.Now()
	if err := mgr.Cancel("session-a", handle.JobID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	elapsed := time.Since(start)

	if final.State != jobs.StateCanceled {
		t.Fatalf("应记为 canceled，得到 %s", final.State)
	}
	if elapsed < grace {
		t.Fatalf("应在宽限期后才强制结束，实际 %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("强制结束过慢: %v", elapsed)
	}
}

// TestBackgroundBash_ProcessGroupSupportedOnUnix 固定本平台的能力声明，
// 让"支持/不支持"这件事在测试里有明确期望值而不是隐含假设。
func TestBackgroundBash_ProcessGroupSupportedOnUnix(t *testing.T) {
	if !jobs.ProcessGroupSupported() {
		t.Fatal("unix 平台应支持进程组终止")
	}
}
