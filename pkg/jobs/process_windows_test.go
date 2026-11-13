//go:build windows

package jobs

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestProcessTreeSupportedOnWindows 固定本平台的能力声明：
// Windows 没有 POSIX 进程组，但有 taskkill /T，因此声明支持进程树终止。
func TestProcessTreeSupportedOnWindows(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Fatal("Windows 应声明支持进程树终止（taskkill /T）")
	}
}

// TestTerminateCommand_ToleratesGoneProcess 固定"要终止的东西已经没了"不算失败。
// taskkill 对不存在的 pid 返回 128，语义与 unix 的 ESRCH 相同。
//
// 用 4242 这个不太可能存在的 pid：只验证不报错，不验证真的杀掉了什么。
func TestTerminateCommand_ToleratesGoneProcess(t *testing.T) {
	cmd := &exec.Cmd{Process: &os.Process{Pid: 4242}}
	if err := TerminateCommand(cmd, 0); err != nil {
		t.Fatalf("对已不存在的进程应返回 nil，得到 %v", err)
	}
}

// TestTerminateCommand_KillsGrandchildren 是 Windows 路径的核心验收：
// 直接子进程的**孙进程**也必须被清理。
//
// 这条测试精确复现了 CI 暴露的缺陷：Stdout 给的是 io.Writer 而非 *os.File，
// os/exec 因此自建管道并起一个复制协程，而管道的写端会被 `cmd /c ping ...`
// 的孙进程继承。此时若只杀直接子进程，复制协程读不到 EOF，cmd.Wait()
// 就阻塞到孙进程自然退出——超时/取消在 Windows 上等于没有生效。
func TestTerminateCommand_KillsGrandchildren(t *testing.T) {
	if !ProcessTreeTerminationSupported() {
		t.Skip("当前平台不支持进程树终止")
	}
	// ping -n 30 相当于睡眠 30 秒；> NUL 让孙进程不往继承的管道里写东西，
	// 只保留写端句柄——这才是"卡住"的成因。
	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 > NUL")
	PrepareCommand(cmd)
	var sink bytes.Buffer
	cmd.Stdout = &sink

	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	if err := TerminateCommand(cmd, time.Second); err != nil {
		t.Fatalf("TerminateCommand: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// 整棵树都被终止，管道写端全部关闭，Wait 及时返回。
	case <-time.After(5 * time.Second):
		t.Fatal("终止后 Wait 未在 5 秒内返回，说明孙进程仍在持有管道写端")
	}
}

// TestTerminateCommand_NilAndUnstarted 覆盖空输入：终止未启动的命令不该 panic。
func TestTerminateCommand_NilAndUnstarted(t *testing.T) {
	if err := TerminateCommand(nil, time.Second); err != nil {
		t.Fatalf("nil 命令应返回 nil，得到 %v", err)
	}
	if err := TerminateCommand(&exec.Cmd{}, time.Second); err != nil {
		t.Fatalf("未启动的命令应返回 nil，得到 %v", err)
	}
}

// TestPrepareCommand_SetsNewProcessGroup 验证命令确实脱离了调用方的进程组，
// 否则调用方所在控制台的 Ctrl+C 会连带杀死后台命令。
func TestPrepareCommand_SetsNewProcessGroup(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	PrepareCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("PrepareCommand 应填充 SysProcAttr")
	}
	const createNewProcessGroup = 0x00000200
	if cmd.SysProcAttr.CreationFlags&createNewProcessGroup == 0 {
		t.Fatalf("应设置 CREATE_NEW_PROCESS_GROUP，实际 flags=%#x", cmd.SysProcAttr.CreationFlags)
	}
}
