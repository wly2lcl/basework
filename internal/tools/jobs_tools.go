package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/tool"
)

// jobToolset 是三个 job 工具共用的归属与数据来源。
//
// 归属用函数而不是字符串：产品组装时先有工具、后有 Agent，而 Agent 的会话 ID 要等
// Agent 构造完成才能拿到。用函数晚绑定，能在不改变构造顺序的前提下保证
// "每次调用都用当前会话的归属"，而不是启动时猜一个。
//
// 越权保护与 `BackgroundBashTool` 完全同源：全部读写都带 owner 参数走 `jobs.Manager`，
// 因此"会话 A 看不到会话 B 的 job"不是这里重新实现的一套规则，而是同一个边界。
type jobToolset struct {
	manager *jobs.Manager
	owner   func() string
}

// currentOwner 解析当前归属。解析不到时返回空串，调用方必须拒绝服务。
func (t *jobToolset) currentOwner() string {
	if t.owner == nil {
		return ""
	}
	return t.owner()
}

// NewJobTools 创建 job 查询/输出/取消三个工具。
//
// manager 为 nil 时返回空切片：宁可不注册工具，也不要注册一个"调用即报错"的假能力。
func NewJobTools(owner func() string, manager *jobs.Manager) []tool.Tool {
	if manager == nil {
		return nil
	}
	ts := &jobToolset{manager: manager, owner: owner}
	return []tool.Tool{
		&jobListTool{ts: ts},
		&jobOutputTool{ts: ts},
		&jobCancelTool{ts: ts},
	}
}

// mergedJobs 把"本进程内的 job"和"持久化历史"合并，同一个 ID 以内存态为准。
//
// 内存态优先的理由：它是唯一知道"进程还活着、命令还在跑"的来源；历史记录里的
// running 只说明"上次写记录时它在跑"。
func mergedJobs(ts *jobToolset, owner string) ([]jobs.Job, error) {
	live := ts.manager.List(owner)
	seen := make(map[string]bool, len(live))
	out := make([]jobs.Job, 0, len(live))
	for _, j := range live {
		seen[j.ID] = true
		out = append(out, j)
	}
	history, err := ts.manager.History(owner)
	if err != nil {
		return nil, err
	}
	for _, j := range history {
		if seen[j.ID] {
			continue
		}
		out = append(out, j)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// findJob 在合并视图里按 ID 找 job。
func findJob(ts *jobToolset, owner, id string) (jobs.Job, bool) {
	if job, err := ts.manager.Get(owner, id); err == nil {
		return job, true
	}
	history, err := ts.manager.History(owner)
	if err != nil {
		return jobs.Job{}, false
	}
	for _, j := range history {
		if j.ID == id {
			return j, true
		}
	}
	return jobs.Job{}, false
}

// jobListTool 列出当前会话的后台任务。
type jobListTool struct{ ts *jobToolset }

func (t *jobListTool) Name() string { return "job_list" }

func (t *jobListTool) Description() string {
	return "列出本会话的后台任务（含上次运行的持久化历史）；不重跑任何命令"
}

func (t *jobListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"state": {"type": "string", "description": "只显示该状态（queued/running/succeeded/failed/canceled/timed_out/interrupted）"},
			"limit": {"type": "integer", "description": "最多返回条数（默认 20）"}
		}
	}`)
}

func (t *jobListTool) Execute(_ context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		State string `json:"state"`
		Limit int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return errResult(fmt.Sprintf("参数解析失败: %v", err)), nil
		}
	}
	owner := t.ts.currentOwner()
	if owner == "" {
		return errResult("当前会话为空，无法列出后台任务"), nil
	}

	all, err := mergedJobs(t.ts, owner)
	if err != nil {
		return errResult(fmt.Sprintf("读取后台任务失败: %v", err)), nil
	}
	want := jobs.State(strings.TrimSpace(params.State))
	filtered := make([]jobs.Job, 0, len(all))
	for _, j := range all {
		if want != "" && j.State != want {
			continue
		}
		filtered = append(filtered, j)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	shown := filtered
	truncated := false
	if len(shown) > limit {
		shown = shown[len(shown)-limit:]
		truncated = true
	}

	if len(shown) == 0 {
		if len(all) == 0 {
			return &tool.Result{Content: "本会话没有后台任务记录。"}, nil
		}
		return &tool.Result{Content: fmt.Sprintf("没有状态为 %s 的后台任务（共 %d 条记录）。", want, len(all))}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "后台任务 %d 条", len(filtered))
	if truncated {
		fmt.Fprintf(&b, "（只显示最近 %d 条）", limit)
	}
	b.WriteString("\n")
	for _, j := range shown {
		fmt.Fprintf(&b, "- %s [%s] %s", j.ID, j.State, j.Command)
		if j.Err != "" {
			fmt.Fprintf(&b, " — %s", j.Err)
		}
		if j.ExitCode >= 0 {
			fmt.Fprintf(&b, " (exit=%d)", j.ExitCode)
		}
		if j.StdoutBytes > 0 || j.StderrBytes > 0 {
			fmt.Fprintf(&b, " out=%dB err=%dB", j.StdoutBytes, j.StderrBytes)
		}
		if j.OutputTruncated {
			b.WriteString(" [输出被截断]")
		}
		if j.JournalErr != "" {
			fmt.Fprintf(&b, " [记录写入失败: %s]", j.JournalErr)
		}
		b.WriteString("\n")
	}
	b.WriteString("用 job_output 读取输出；用 job_cancel 取消仍在运行的任务。历史记录不会自动重跑命令。")
	return &tool.Result{Content: b.String()}, nil
}

// jobOutputTool 分页读取某个 job 的输出。
type jobOutputTool struct{ ts *jobToolset }

func (t *jobOutputTool) Name() string { return "job_output" }

func (t *jobOutputTool) Description() string {
	return "按偏移分页读取后台任务的 stdout/stderr；不会重新执行命令"
}

func (t *jobOutputTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"job_id": {"type": "string", "description": "job_list 返回的任务 ID"},
			"stream": {"type": "string", "description": "stdout（默认）或 stderr"},
			"offset": {"type": "integer", "description": "起始字节偏移，默认 0"},
			"limit": {"type": "integer", "description": "本次最多返回的字节数"}
		},
		"required": ["job_id"]
	}`)
}

func (t *jobOutputTool) Execute(_ context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		JobID  string `json:"job_id"`
		Stream string `json:"stream"`
		Offset int64  `json:"offset"`
		Limit  int64  `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}
	if strings.TrimSpace(params.JobID) == "" {
		return errResult("job_id 不能为空"), nil
	}
	owner := t.ts.currentOwner()
	if owner == "" {
		return errResult("当前会话为空，无法读取后台任务输出"), nil
	}

	stream := jobs.Stream(params.Stream)
	if stream == "" {
		stream = jobs.StreamStdout
	}
	if !stream.Valid() {
		return errResult(fmt.Sprintf("未知输出流 %q（可选 stdout / stderr）", params.Stream)), nil
	}

	page, err := t.ts.manager.ReadOutput(owner, params.JobID, stream, params.Offset, params.Limit)
	if err == nil {
		return &tool.Result{Content: formatOutputPage(page)}, nil
	}
	// 归属不匹配与不存在返回同一个错误值（见 jobs.Manager.ReadOutput），
	// 所以这里也不区分，避免把"别人的 job ID"变成一个可探测的信号。
	if errors.Is(err, jobs.ErrNotFound) {
		if _, ok := findJob(t.ts, owner, params.JobID); !ok {
			return errResult(fmt.Sprintf("未找到后台任务 %s", params.JobID)), nil
		}
		return errResult(fmt.Sprintf("后台任务 %s 不在当前进程中，其输出不可读（进程已退出）", params.JobID)), nil
	}
	if errors.Is(err, jobs.ErrOutputExpired) {
		return errResult(fmt.Sprintf("后台任务 %s 的输出已过期（截止 %s），文件已不可读",
			params.JobID, page.ExpiresAt.Format("2006-01-02 15:04:05"))), nil
	}
	return errResult(fmt.Sprintf("读取后台任务输出失败: %v", err)), nil
}

// formatOutputPage 把分页结果渲染成"模型能接着翻页"的文本。
func formatOutputPage(page jobs.OutputPage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "stream=%s offset=%d next_offset=%d eof=%t total=%d stored=%d",
		page.Stream, page.Offset, page.NextOffset, page.EOF, page.TotalSize, page.StoredSize)
	if page.Truncated {
		fmt.Fprintf(&b, " truncated=true dropped=%d reason=%s", page.DroppedBytes, page.DropReason)
	}
	b.WriteString("\n")
	b.WriteString(page.Text)
	if !page.EOF {
		fmt.Fprintf(&b, "\n[还有未读内容，用 offset=%d 继续读取]", page.NextOffset)
	}
	return b.String()
}

// jobCancelTool 取消仍在运行的后台任务。
type jobCancelTool struct{ ts *jobToolset }

func (t *jobCancelTool) Name() string { return "job_cancel" }

func (t *jobCancelTool) Description() string {
	return "取消本会话仍在运行的后台任务；已结束的任务不会被重启"
}

func (t *jobCancelTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"job_id": {"type": "string", "description": "job_list 返回的任务 ID"}
		},
		"required": ["job_id"]
	}`)
}

func (t *jobCancelTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}
	if strings.TrimSpace(params.JobID) == "" {
		return errResult("job_id 不能为空"), nil
	}
	owner := t.ts.currentOwner()
	if owner == "" {
		return errResult("当前会话为空，无法取消后台任务"), nil
	}

	err := t.ts.manager.Cancel(owner, params.JobID)
	switch {
	case err == nil:
		// 取消只是发出信号；终态由执行体发布。这里等到终态再回复，
		// 否则模型会看到"取消成功"但状态还是 running，从而重复取消。
		final, awaitErr := t.ts.manager.Await(ctx, owner, params.JobID)
		if awaitErr != nil {
			return &tool.Result{Content: fmt.Sprintf(
				"已发出取消信号：%s；等待终态未完成（%v），可用 job_list 查看当前状态", params.JobID, awaitErr)}, nil
		}
		return &tool.Result{Content: fmt.Sprintf("已取消 %s，终态 %s", params.JobID, final.State)}, nil
	case errors.Is(err, jobs.ErrAlreadyTerminal):
		job, _ := t.ts.manager.Get(owner, params.JobID)
		return &tool.Result{Content: fmt.Sprintf(
			"任务 %s 已是终态（%s），未做任何操作；历史任务不会被重新执行", params.JobID, job.State)}, nil
	case errors.Is(err, jobs.ErrNotFound):
		return errResult(fmt.Sprintf("未找到可取消的后台任务 %s", params.JobID)), nil
	default:
		return errResult(fmt.Sprintf("取消后台任务失败: %v", err)), nil
	}
}
