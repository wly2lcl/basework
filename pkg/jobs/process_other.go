//go:build !unix && !windows

package jobs

import (
	"os/exec"
	"time"
)

// processTreeTerminationSupported：既非 unix 也非 windows 的平台（如 plan9、
// js/wasm）既没有 POSIX 进程组，也没有等价的进程树枚举手段。
//
// 这里的 prepareCommand 是空操作，terminateCommand 返回
// ErrProcessTreeUnsupported，调用方退化为终止直接子进程——孙进程会残留。
// 这个限制是显式的（错误值 + 文档 + 交叉编译检查），不是"看起来能用"。
const processTreeTerminationSupported = false

// prepareCommand 在不支持的平台上不做任何事。
func prepareCommand(*exec.Cmd) {}

// terminateCommand 在不支持的平台上明确拒绝，而不是假装支持。
//
// 这里直接忽略 grace：连温和信号都发不出去，等一个宽限期没有意义。
func terminateCommand(*exec.Cmd, time.Duration) error {
	return ErrProcessTreeUnsupported
}
