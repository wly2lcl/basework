package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/tool"
)

// BashTool 执行 shell 命令
type BashTool struct {
	// PermissionMode 控制黑名单的权限行为：
	//   "yolo"       - 跳过黑名单检查
	//   "interactive" - 黑名单命中时允许（由调用方负责确认交互）
	//   "default"    - 直接拒绝
	PermissionMode string
	// BlockedCommands 是用户自定义的额外黑名单正则模式
	BlockedCommands []string
	// Runtime 是实例级运行时注入（路径检查/超时/事件）。nil 时回落包级全局，
	// 行为与旧版本一致。
	Runtime *Runtime
}

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

	// 黑名单检查（YOLO 模式跳过）
	mode := b.PermissionMode
	if mode == "" {
		mode = "default"
	}
	if mode != "yolo" {
		matched, pattern, err := CheckBlacklist(params.Command, b.BlockedCommands)
		if err != nil {
			return &tool.Result{Content: fmt.Sprintf("黑名单检查失败: %v", err), IsError: true}, nil
		}
		if matched {
			if mode == "interactive" {
				// 交互模式：返回带有确认信息的错误，由调用方处理确认逻辑
				return &tool.Result{
					Content: fmt.Sprintf("命令被黑名单拦截: %s\n匹配模式: %s\n输入 CONFIRM 确认执行", params.Command, pattern),
					IsError: true,
				}, nil
			}
			// default 模式：直接拒绝
			return &tool.Result{
				Content: fmt.Sprintf("命令被黑名单拦截: %s 属于危险操作", params.Command),
				IsError: true,
			}, nil
		}
	}

	// 敏感路径检查（实例注入优先，回落全局）
	if pc := b.Runtime.resolvePathChecker(); pc != nil {
		paths := extractPathsFromCommand(params.Command)
		for _, p := range paths {
			if allowed, reason := pc.CheckPath(p); !allowed {
				return &tool.Result{Content: fmt.Sprintf("访问被拒绝: %s (%s)", p, reason), IsError: true}, nil
			}
		}
	}

	// 工具级超时控制（实例注入优先，回落全局）
	toolTimeout := b.Runtime.resolveTimeout().GetTimeout("bash")
	ctx, toolCancel := WithTimeoutBus(ctx, "bash", toolTimeout, b.Runtime.resolveEventBus())
	defer toolCancel()

	// 默认超时 30s
	timeout := params.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	execCtx, execCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer execCancel()

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

// extractPathsFromCommand 从 shell 命令中提取可能的文件路径。
// 覆盖常见场景，不做完美解析。
func extractPathsFromCommand(cmd string) []string {
	var paths []string
	seen := make(map[string]bool)

	// 按空格分割命令
	parts := strings.Fields(cmd)
	for _, part := range parts {
		// 跳过选项（以 - 开头）
		if strings.HasPrefix(part, "-") {
			continue
		}

		// 跳过重定向符号
		if part == ">" || part == ">>" || part == "<" || part == "2>" || part == "|" {
			continue
		}

		// 检查是否看起来像路径
		if looksLikePath(part) && !seen[part] {
			seen[part] = true
			paths = append(paths, part)
		}
	}

	return paths
}

// looksLikePath 检查字符串是否看起来像文件路径。
func looksLikePath(s string) bool {
	// 以 /、./、../、~/ 开头
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") || strings.HasPrefix(s, "~/") {
		return true
	}

	// 包含路径分隔符
	if strings.Contains(s, "/") {
		return true
	}

	// 包含文件扩展名（如 .txt, .go, .json 等）
	if strings.Contains(s, ".") && !strings.Contains(s, "=") {
		return true
	}

	return false
}
