package agent

import (
	"os"
)

// workdirProvider 提供当前工作目录信息。
type workdirProvider struct{}

func (p *workdirProvider) Name() string {
	return "workdir"
}

func (p *workdirProvider) Collect() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return "Working directory: " + dir, nil
}
