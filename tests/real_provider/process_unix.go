//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func configureTestProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Kill the whole verification process group so a test that spawned a
		// child cannot survive the runner timeout.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
