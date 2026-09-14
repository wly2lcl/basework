package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

// JobHandle 是后台命令启动成功后返回给调用方的结构化句柄。
//
// 之所以单独定义类型而不是拼一段提示文本：后续任务（输出读取、取消、恢复）都要按
// job ID 定位同一个对象，字符串拼出来的句柄必然要被重新解析一遍，那是漂移的来源。
type JobHandle struct {
	JobID   string     `json:"job_id"`
	Owner   string     `json:"owner"`
	Command string     `json:"command"`
	State   jobs.State `json:"state"`
	// OutputDir 是输出溢出文件的根目录；输出较少时不会有文件产生。
	OutputDir string `json:"output_dir,omitempty"`
}

// BackgroundBashTool 启动后台命令并返回结构化 job ID。
//
// 与同步的 `builtin.BashTool` 保持同样的权限入口顺序：
// 黑名单检查 → 路径检查 → 启动。被拒绝时不登记任何 job，因此不会有"权限拒绝了
// 但进程已经跑起来"的窗口。
//
// 「保持 pkg/tool 接口兼容」体现在这里不新增接口方法，也不改 `tool.Result` 结构：
// 结构化句柄以 JSON 文本放在 `Result.Content` 里。
//
// 覆盖边界：本工具负责"启动 + 归属 + 状态 + 输出承接 + 进程树终止"。
//
// 输出：命令的 stdout/stderr 写进 Manager 的输出存储（内存配额 + 溢出到私有文件
// + 保留期 + 越权拒绝读取），分页读取通过 `jobs.Manager.ReadOutput` 暴露。
//
// 终止：命令跑在独立进程组/进程组等价物里，取消/超时会终止整棵进程树
// （unix 用进程组信号，Windows 用 `taskkill /T`），因此直接子进程的孙进程
// 不会残留。两个平台的差异只在「有没有温和阶段」：unix 先 SIGTERM 再 SIGKILL，
// Windows 直接强制结束（没有可捕获的信号，见 internal/jobs/process_windows.go）。
// 该能力由 `jobs.ProcessTreeTerminationSupported()` 显式声明，为 false 的平台
// 退化为只杀直接子进程。
//
// 仍未实现、也没有假装实现的部分：把输出/查询/取消注册成模型可调用工具、
// 跨重启恢复属于 JOB-004。
type BackgroundBashTool struct {
	sessionID string
	manager   *jobs.Manager

	// OwnerFunc 是归属的动态来源（例如 Agent 的会话 ID，构造工具时还拿不到）。
	// 非 nil 时优先于 sessionID；解析结果为空则拒绝启动——没有归属的 job 会变成
	// 全局可见，那是权限漏洞而不是便利。
	OwnerFunc func() string

	// PermissionMode 与 builtin.BashTool 语义一致：
	//   "yolo"        - 跳过黑名单
	//   "interactive" - 命中黑名单时返回待确认提示而非启动
	//   "default"     - 命中黑名单直接拒绝
	PermissionMode string
	// BlockedCommands 是用户自定义的额外黑名单模式。
	BlockedCommands []string
	// CheckPath 复用产品层的路径检查入口；nil 表示不做路径检查。
	// 与同步 bash 共用同一个检查器，避免两条路径的权限口径不一致。
	CheckPath func(path string) (allowed bool, reason string)
	// DefaultTimeout 是命令超时秒数，<=0 时用 30。
	DefaultTimeout int
	// NoTimeout 表示配置显式关闭了超时（`tools.timeout` 对该工具解析结果为 0）。
	//
	// 它优先于 DefaultTimeout，存在的理由是：`builtin.TimeoutConfig` 把 0 定义为
	// "不超时"，而本工具的 0 是"未配置"。若不区分，用户把超时关掉后同步 bash 不超时、
	// 后台 bash 却仍被 30 秒掐死——同一个配置在两条路径上解释不一致。
	NoTimeout bool
	// KillGrace 是温和终止与强制结束之间的宽限期；<=0 时用 jobs.DefaultKillGrace。
	KillGrace time.Duration
}

// NewBackgroundBashTool 创建后台 bash 工具。
func NewBackgroundBashTool(sessionID string, mgr *jobs.Manager) *BackgroundBashTool {
	return &BackgroundBashTool{sessionID: sessionID, manager: mgr}
}

// owner 解析本次调用应使用的归属。
func (t *BackgroundBashTool) owner() string {
	if t.OwnerFunc != nil {
		return t.OwnerFunc()
	}
	return t.sessionID
}

func (t *BackgroundBashTool) Name() string { return "bash_background" }

func (t *BackgroundBashTool) Description() string {
	return "在后台执行 shell 命令，立即返回 job ID；用 job 查询工具查看状态"
}

func (t *BackgroundBashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The shell command to execute in background"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default 30)"}
		},
		"required": ["command"]
	}`)
}

func (t *BackgroundBashTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}
	if params.Command == "" {
		return errResult("命令不能为空"), nil
	}
	if t.manager == nil {
		return errResult("后台任务管理器未初始化"), nil
	}

	// 归属缺失时不能启动：没有 owner 的 job 会变成全局可见，这是权限漏洞而不是便利。
	owner := t.owner()
	if owner == "" {
		return errResult("当前会话为空，无法建立后台任务归属"), nil
	}

	// 1. 黑名单检查：与同步 bash 同一份实现
	mode := t.PermissionMode
	if mode == "" {
		mode = "default"
	}
	if mode != "yolo" {
		matched, pattern, err := builtin.CheckBlacklist(params.Command, t.BlockedCommands)
		if err != nil {
			return errResult(fmt.Sprintf("黑名单检查失败: %v", err)), nil
		}
		if matched {
			if mode == "interactive" {
				return errResult(fmt.Sprintf(
					"命令被黑名单拦截: %s\n匹配模式: %s\n输入 CONFIRM 确认执行", params.Command, pattern)), nil
			}
			return errResult(fmt.Sprintf("命令被黑名单拦截: %s 属于危险操作", params.Command)), nil
		}
	}

	// 2. 路径检查：复用产品层检查器
	if t.CheckPath != nil {
		for _, p := range extractCommandPaths(params.Command) {
			if allowed, reason := t.CheckPath(p); !allowed {
				return errResult(fmt.Sprintf("访问被拒绝: %s (%s)", p, reason)), nil
			}
		}
	}

	// 3. 启动。注意顺序：以上检查全部通过之后才登记 job。
	timeout := params.Timeout
	if timeout <= 0 && !t.NoTimeout {
		// 本次调用没给 timeout、且配置没关闭超时：用工具默认值。
		timeout = t.DefaultTimeout
		if timeout <= 0 {
			timeout = 30
		}
	}
	// timeout == 0 只可能来自 NoTimeout：不带自身 deadline，只受取消控制。
	command := params.Command

	job, err := t.manager.StartWithOutput(owner, command, func(jobCtx context.Context, stdout, stderr io.Writer) (int, error) {
		execCtx := jobCtx
		if timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(jobCtx, time.Duration(timeout)*time.Second)
			defer cancel()
		}

		cmd := exec.CommandContext(execCtx, "sh", "-c", command)
		// 让命令脱离调用方的进程组，并接管取消动作：默认的取消只杀直接子进程，
		// `sh -c "make -j8 test"` 这类命令的孙进程会被留下继续跑。
		jobs.PrepareCommand(cmd)
		cmd.Cancel = func() error {
			err := jobs.TerminateCommand(cmd, t.KillGrace)
			if err == nil {
				return nil
			}
			if errors.Is(err, jobs.ErrProcessTreeUnsupported) {
				// 既没有 POSIX 进程组、也没有 taskkill 这类等价手段的平台：
				// 退化为终止直接子进程。孙进程会残留，这个事实不掩饰——
				// 错误值本身就是这句话的载体。
				if cmd.Process != nil {
					return cmd.Process.Kill()
				}
			}
			return err
		}
		// 输出接进 Manager 的输出存储：先驻留内存，超过配额后溢出到 0600 私有文件。
		// 写端永不因配额或磁盘问题报错，所以这里的复制协程不会被输出量"打死"。
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		if runErr := cmd.Run(); runErr != nil {
			// 先看 context 状态：os/exec 在 ctx 结束时可能返回 ctx 的错误，
			// 也可能返回"被信号杀死"的 ExitError。两者必须收敛到同一个可判定错误，
			// 否则「取消」会被记成「退出码 -1 的失败」。
			if execCtx.Err() != nil {
				return -1, execCtx.Err()
			}
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return -1, runErr
		}
		return 0, nil
	})
	if err != nil {
		return errResult(fmt.Sprintf("启动后台任务失败: %v", err)), nil
	}

	handle := JobHandle{
		JobID:     job.ID,
		Owner:     job.Owner,
		Command:   job.Command,
		State:     job.State,
		OutputDir: t.manager.OutputDir(),
	}
	payload, marshalErr := json.Marshal(handle)
	if marshalErr != nil {
		// 句柄字段全是字符串，序列化实际不可达；这里不掩盖失败。
		return errResult(fmt.Sprintf("序列化 job 句柄失败: %v", marshalErr)), nil
	}
	return &tool.Result{Content: string(payload)}, nil
}

// extractCommandPaths 从命令中挑出需要做路径检查的片段。
//
// 与同步 bash 使用同一套启发式规则（见 builtin.extractPathsFromCommand），
// 这里是它的可导出等价实现，避免两处口径漂移。
func extractCommandPaths(command string) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, field := range splitCommandFields(command) {
		if field == "" || field[0] == '-' {
			continue
		}
		if !looksLikePath(field) || seen[field] {
			continue
		}
		seen[field] = true
		paths = append(paths, field)
	}
	return paths
}

// splitCommandFields 按空白切分命令。
func splitCommandFields(command string) []string {
	var fields []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			fields = append(fields, string(current))
			current = current[:0]
		}
	}
	for _, r := range command {
		switch r {
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			current = append(current, r)
		}
	}
	flush()
	return fields
}

// looksLikePath 判断片段是否像文件路径，规则与同步 bash 保持一致。
func looksLikePath(s string) bool {
	if s == ">" || s == ">>" || s == "<" || s == "|" || s == "2>" {
		return false
	}
	if len(s) >= 1 && (s[0] == '/' || s[0] == '~') {
		return true
	}
	if len(s) >= 2 && s[0] == '.' && (s[1] == '/' || s[1] == '.') {
		return true
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
		if s[i] == '=' {
			return false
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
}

// errResult 构造错误结果，统一 IsError 标记。
func errResult(msg string) *tool.Result {
	return &tool.Result{Content: msg, IsError: true}
}
