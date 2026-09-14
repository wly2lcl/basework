//go:build windows

package jobs

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// processTreeTerminationSupported：Windows 没有 POSIX 进程组，但有等价的
// 进程树终止手段——`taskkill /T` 按父进程链枚举整棵子树。
//
// 这条路径是 CI 暴露出来的真实缺陷的修复结果：此前 Windows 退化为
// 「只杀直接子进程」，而 `sh -c "sleep 30"` 的孙进程 sleep.exe 会继续持有
// 继承来的管道写端，导致 cmd.Wait() 一直阻塞到孙进程自然退出——
// 超时/取消在 Windows 上等于没有生效。
const processTreeTerminationSupported = true

// prepareCommand 让命令脱离调用方的进程组。
//
// 与 unix 的 Setpgid 目的相同：调用方所在控制台收到 Ctrl+C 时，不该顺手把
// 后台命令一起带走——后台命令的生死只由 Manager 的取消/超时决定。
// 终止本身不依赖这个标志（taskkill 走父进程链），设它是为了让"谁会被 Ctrl+C
// 波及"这件事不随调用场景变化。
func prepareCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// terminateCommand 强制终止整棵进程树。
//
// **与 unix 的差别是没有温和阶段**，这是平台的真实能力差异，不是省事：
// Windows 对控制台进程没有可捕获的终止信号。等价物 Ctrl+Break 要求目标进程
// 安装 console control handler，且要求调用方与目标共享同一个控制台——
// 在服务进程、输出被重定向到管道的场景下两者都不成立。既然信号发不出去，
// 先等一个宽限期只是白等，所以这里直接强制结束。grace 用空标识符接收，
// 就是为了让"本平台没有宽限期的对象"这件事在签名上可见。
//
// taskkill /T 按父进程链枚举子树，/F 强制结束。退出码 128 表示进程已不存在，
// 与 unix 的 ESRCH 同义，不算失败。
func terminateCommand(cmd *exec.Cmd, _ time.Duration) error {
	pid := cmd.Process.Pid

	err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 128:
		// 进程树已经不存在，没有可终止的对象。
		return nil
	}

	// taskkill 不可用或失败：退回到只杀直接子进程。做到多少说多少，
	// 但不再返回 ErrProcessTreeUnsupported —— 那会让调用方以为自己需要
	// 再退化一次，而这里已经尽力了。
	if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return killErr
	}
	return nil
}
