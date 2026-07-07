package agent

import (
	"bytes"
	"os/exec"
	"strings"
)

// gitProvider 提供当前 Git 仓库的状态信息。
// 如果不在 Git 仓库内，则静默返回空字符串。
type gitProvider struct{}

func (p *gitProvider) Name() string {
	return "git"
}

func (p *gitProvider) Collect() (string, error) {
	// 检查是否在 Git 仓库中
	branch, err := p.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", nil // 不在 git 仓库中，静默跳过
	}

	commit, err := p.run("log", "--oneline", "-1")
	if err != nil {
		commit = ""
	}

	status, err := p.run("status", "--porcelain")
	if err != nil {
		status = ""
	}

	dirty := ""
	if strings.TrimSpace(status) != "" {
		dirty = " (dirty)"
	}

	return "Git branch: " + strings.TrimSpace(branch) + dirty +
		"\nLatest commit: " + strings.TrimSpace(commit), nil
}

func (p *gitProvider) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}
