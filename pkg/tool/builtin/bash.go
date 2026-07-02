package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/wly2lcl/basework/pkg/tool"
)

// BashTool 执行 shell 命令
type BashTool struct{}

func (b *BashTool) Name() string { return "bash" }

func (b *BashTool) Description() string { return "执行 shell 命令并返回输出" }

func (b *BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The shell command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default 30)"}
		},
		"required": ["command"]
	}`)
}

func (b *BashTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	if params.Command == "" {
		return &tool.Result{Content: "命令不能为空", IsError: true}, nil
	}

	// 默认超时 30s
	timeout := params.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(execCtx, "sh", "-c", params.Command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// 合并输出
	var output bytes.Buffer
	if stdout.Len() > 0 {
		output.Write(stdout.Bytes())
	}
	if stderr.Len() > 0 {
		if stdout.Len() > 0 {
			output.WriteByte('\n')
		}
		output.Write(stderr.Bytes())
	}

	content := output.String()

	// 限制输出长度 max 100KB
	const maxOutputSize = 100 * 1024
	if len(content) > maxOutputSize {
		content = content[:maxOutputSize] + "\n... (输出截断，超出 100KB)"
	}

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &tool.Result{
			Content: fmt.Sprintf("命令执行失败 (退出码 %d):\n%s", exitCode, content),
			IsError: true,
		}, nil
	}

	return &tool.Result{Content: content}, nil
}