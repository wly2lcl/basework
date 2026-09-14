//go:build !unix && !windows

package jobs

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestProcessTreeUnsupported_Explicit 固定既无进程组也无进程树终止手段的平台行为：
// 不是"静默地只杀一个进程"，而是显式返回一个可判定的错误。
//
// 本地无法运行这些平台，这个文件的价值在于：交叉编译会编译它，
// 从而保证该平台的代码路径不是一句空话。
func TestProcessTreeUnsupported_Explicit(t *testing.T) {
	if ProcessTreeTerminationSupported() {
		t.Fatal("该平台不应声明支持进程树终止")
	}
	// 造一个"已经有 pid 的命令"：不需要真的启动进程，只要走到终止那一步。
	cmd := &exec.Cmd{Process: &os.Process{Pid: 4242}}
	if err := TerminateCommand(cmd, time.Second); !errors.Is(err, ErrProcessTreeUnsupported) {
		t.Fatalf("应返回 ErrProcessTreeUnsupported，得到 %v", err)
	}
	if err := TerminateCommand(nil, time.Second); err != nil {
		t.Fatalf("nil 命令应返回 nil，得到 %v", err)
	}
	if err := TerminateCommand(&exec.Cmd{}, time.Second); err != nil {
		t.Fatalf("未启动的命令应返回 nil，得到 %v", err)
	}
}
