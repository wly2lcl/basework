package agent

import (
	"os"
	"runtime"
	"strings"
)

// platformProvider 提供操作系统、架构和默认 Shell 信息。
type platformProvider struct{}

func (p *platformProvider) Name() string {
	return "platform"
}

func (p *platformProvider) Collect() (string, error) {
	shell := p.detectShell()
	return "Platform: " + runtime.GOOS + "/" + runtime.GOARCH +
		"\nShell: " + shell, nil
}

// detectShell 检测当前用户的默认 Shell。
func (p *platformProvider) detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = os.Getenv("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
		} else {
			shell = "/bin/sh"
		}
	}
	// 提取可执行文件名
	parts := strings.Split(shell, "/")
	return parts[len(parts)-1]
}
