package jobs

import (
	"errors"
	"os/exec"
	"time"
)

// DefaultKillGrace 是先发温和终止信号、再强制结束之间等待的默认宽限期。
//
// 它不能设成 0：直接 SIGKILL 会让命令行进程失去清理临时文件、写回状态的机会，
// 而这正是"取消一个正在跑的命令"最常见的破坏来源。
//
// 只在提供温和终止信号的平台上有意义（unix）；Windows 没有可捕获的进程终止
// 信号，见 process_windows.go 中的说明。
const DefaultKillGrace = 2 * time.Second

// ErrProcessTreeUnsupported 表示当前平台既不支持 POSIX 进程组、也没有等价的
// 进程树终止手段。
//
// 它单独成一个错误值，是为了让调用方能显式退化为"只终止直接子进程"，
// 而不是静默地以为整棵树都清理干净了。
var ErrProcessTreeUnsupported = errors.New("jobs: 当前平台不支持进程树终止")

// PrepareCommand 让命令脱离调用方的进程组（在不支持的平台上为空操作）。
//
// 为什么需要它：`exec.CommandContext` 在 ctx 结束时只终止**直接子进程**，
// 孙进程会被留下来继续跑。`sh -c "a | b"` 或 `sh -c "make -j8 test"` 这类命令
// 的孙进程恰恰是最需要被清理的部分。
func PrepareCommand(cmd *exec.Cmd) { prepareCommand(cmd) }

// TerminateCommand 终止命令所属的整棵进程树。
//
// unix：顺序是「先温和、后强制」——先向进程组发可捕获的终止信号，等待 grace；
// 若仍有存活成员，再强制结束。grace <= 0 时用 DefaultKillGrace。
//
// Windows：直接强制终止整棵树（`taskkill /T /F`）。平台没有可捕获的信号，
// 宽限期在那边没有对象可等，见 process_windows.go。
//
// 返回 ErrProcessTreeUnsupported 表示平台没有进程树终止手段（此时调用方只能
// 退化为 os.Process.Kill，孙进程会残留）；返回其他错误表示终止动作失败。
func TerminateCommand(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if grace <= 0 {
		grace = DefaultKillGrace
	}
	return terminateCommand(cmd, grace)
}

// ProcessTreeTerminationSupported 报告当前平台是否支持终止整棵进程树。
//
// unix 靠进程组（Setpgid + 负 pid 信号），Windows 靠 `taskkill /T`。
// 返回 false 的平台调用方会退化为只杀直接子进程。
func ProcessTreeTerminationSupported() bool { return processTreeTerminationSupported }
