//go:build unix

package jobs

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// processGroupSupported：unix 上通过 Setpgid + 负 pid 发信号，可以覆盖整棵进程树。
const processGroupSupported = true

// prepareCommand 把命令放进以自身 pid 为组长的独立进程组。
//
// 不设 Foreground/Setctty：命令不占用终端，避免和调用方的控制终端互相干扰。
func prepareCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminateCommand 先 SIGTERM 整个进程组，宽限期后对仍存活的成员 SIGKILL。
//
// 这里**不阻塞等待**：调用方是 os/exec 的取消回调，阻塞在那里等于把
// 「命令有没有死」和「Wait 什么时候返回」绑死。宽限期的到时由定时器负责，
// 并且定时器在动手前会重新确认进程组是否还有活着的成员——否则可能对
// 一个已经被系统回收、pgid 又被新进程复用的组发 SIGKILL，误杀无关进程。
func terminateCommand(cmd *exec.Cmd, grace time.Duration) error {
	pgid := cmd.Process.Pid // Setpgid 之后 pgid == 组长 pid

	// 负 pid 表示"整个进程组"。
	err := syscall.Kill(-pgid, syscall.SIGTERM)
	switch {
	case err == nil:
		// 已发出温和终止信号。
	case errors.Is(err, syscall.ESRCH):
		// 组已经不存在，没有可终止的对象。
		return nil
	default:
		return err
	}

	time.AfterFunc(grace, func() {
		if !processGroupAlive(pgid) {
			return
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	})
	return nil
}

// processGroupAlive 报告进程组里是否还有成员存在。
//
// kill(pgid, 0) 只做存在性与权限检查，不真的发信号。
// EPERM 表示"存在但没权限动它"，仍然算存活——把它当成不存在会漏杀。
func processGroupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
