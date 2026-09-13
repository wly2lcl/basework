//go:build !unix

package jobs

import (
	"os/exec"
	"time"
)

// processGroupSupported：非 unix 平台（本仓库目前指 Windows）没有 POSIX 进程组。
//
// 诚实的说明：Windows 上要覆盖整棵进程树需要 Job Object，本项目尚未实现。
// 因此这里的 prepareCommand 是空操作，terminateCommand 返回
// ErrProcessGroupUnsupported，调用方退化为终止直接子进程——孙进程会残留。
// 这个限制是显式的（错误值 + 文档 + 交叉编译检查），不是"看起来能用"。
const processGroupSupported = false

// prepareCommand 在不支持的平台上不做任何事。
func prepareCommand(*exec.Cmd) {}

// terminateCommand 在不支持的平台上明确拒绝，而不是假装支持。
func terminateCommand(*exec.Cmd, time.Duration) error {
	return ErrProcessGroupUnsupported
}
