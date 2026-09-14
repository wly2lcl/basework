//go:build unix

package jobs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// pidAlive 报告进程是否还存在。EPERM 表示存在但无权限操作，仍算存活。
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// waitForFile 等待文件出现（子进程写出的 pid 文件）。
func waitForFile(t *testing.T, path string) {
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

// readPID 读取 pid 文件。
func readPID(t *testing.T, path string) int {
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

// waitForNoPID 在给定时间内等待进程消失，超时即失败。
func waitForNoPID(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("进程 %d 在 %v 内仍存活（残留）", pid, within)
}

// TestPrepareCommand_ChildBecomesProcessGroupLeader 验证命令真的被放进独立进程组，
// 否则后面的"杀整组"只是杀单个进程的另一种说法。
//
// 用 syscall.Getpgid 而不是 `ps`：一是少一次进程启动，二是沙箱里 `ps` 会被拒绝执行，
// 那不是被测代码的问题。
func TestPrepareCommand_ChildBecomesProcessGroupLeader(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 5")
	PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer func() {
		_ = TerminateCommand(cmd, 200*time.Millisecond)
		_ = cmd.Wait()
	}()

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("读取进程组失败: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Fatalf("子进程应是自己进程组的组长: pgid=%d pid=%d", pgid, cmd.Process.Pid)
	}
}

// TestTerminateCommand_KillsGrandchildren 是本任务的核心验收：
// 直接子进程的**孙进程**也必须被清理，不能只杀 sh。
func TestTerminateCommand_KillsGrandchildren(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Skip("当前平台不支持进程组终止")
	}
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	// `&` 起的 sleep 是 sh 的孙进程（sh 自身是直接子进程）。
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitForFile(t, pidFile)
	grandchild := readPID(t, pidFile)
	if !pidAlive(grandchild) {
		t.Fatalf("孙进程 %d 应已启动", grandchild)
	}

	if err := TerminateCommand(cmd, 500*time.Millisecond); err != nil {
		t.Fatalf("终止失败: %v", err)
	}
	waitForNoPID(t, grandchild, 3*time.Second)
	_ = cmd.Wait()
}

// TestTerminateCommand_ForceKillsAfterGrace 验证忽略温和信号的进程会在宽限期后被强制结束。
func TestTerminateCommand_ForceKillsAfterGrace(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Skip("当前平台不支持进程组终止")
	}
	const grace = 300 * time.Millisecond
	// 忽略 TERM 的 shell：温和终止信号对它无效，只能靠强制结束。
	cmd := exec.Command("sh", "-c", `trap "" TERM; while :; do sleep 0.05; done`)
	PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // 等 trap 生效

	start := time.Now()
	if err := TerminateCommand(cmd, grace); err != nil {
		t.Fatalf("终止失败: %v", err)
	}
	_ = cmd.Wait() // 阻塞到进程真的结束
	elapsed := time.Since(start)

	if elapsed < grace {
		t.Fatalf("应在宽限期之后才强制结束，实际 %v 就返回了", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("强制结束过慢: %v", elapsed)
	}
	waitForNoPID(t, cmd.Process.Pid, 2*time.Second)
}

// TestTerminateCommand_AlreadyExitedIsNoop 验证对已退出的进程不发信号、不报错。
func TestTerminateCommand_AlreadyExitedIsNoop(t *testing.T) {
	cmd := exec.Command("true")
	PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("true 应成功: %v", err)
	}
	if err := TerminateCommand(cmd, 100*time.Millisecond); err != nil {
		t.Fatalf("对已退出的命令应返回 nil，得到 %v", err)
	}
}

// TestTerminateCommand_NilInputsAreNoop 验证空输入不 panic。
func TestTerminateCommand_NilInputsAreNoop(t *testing.T) {
	if err := TerminateCommand(nil, time.Second); err != nil {
		t.Fatalf("nil 命令应返回 nil，得到 %v", err)
	}
	bare := &exec.Cmd{}
	if err := TerminateCommand(bare, time.Second); err != nil {
		t.Fatalf("未启动的命令应返回 nil，得到 %v", err)
	}
	if err := TerminateCommand(nil, 0); err != nil {
		t.Fatalf("grace<=0 应回落到默认宽限期而不是报错，得到 %v", err)
	}
}

// TestTerminateCommand_ZeroGraceUsesDefault 验证 grace<=0 时按默认宽限期处理
// （配置漏填不能退化成"直接强杀"）。
func TestTerminateCommand_ZeroGraceUsesDefault(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Skip("当前平台不支持进程组终止")
	}
	cmd := exec.Command("sh", "-c", `trap "" TERM; while :; do sleep 0.05; done`)
	PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	if err := TerminateCommand(cmd, 0); err != nil {
		t.Fatalf("终止失败: %v", err)
	}
	_ = cmd.Wait()
	// 默认宽限期是 2s，进程忽略 TERM，所以必须等到接近 2s 才被强杀。
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("grace<=0 不应退化成立即强杀，实际 %v", elapsed)
	}
	waitForNoPID(t, cmd.Process.Pid, 3*time.Second)
}

// TestProcessTreeTerminationSupportedOnUnix 固定 unix 平台的能力声明。
func TestProcessTreeTerminationSupportedOnUnix(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Fatal("unix 平台应支持进程树终止")
	}
}
