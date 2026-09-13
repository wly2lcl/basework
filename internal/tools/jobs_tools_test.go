package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/tool"
)

// ---------------------------------------------------------------- 辅助

// fixedOwner 返回一个恒定的归属函数。
func fixedOwner(owner string) func() string { return func() string { return owner } }

// jobToolByName 按名字取出工具，取不到就让测试失败。
func jobToolByName(t *testing.T, tools []tool.Tool, name string) tool.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Name() == name {
			return tl
		}
	}
	t.Fatalf("未找到工具 %q", name)
	return nil
}

// startBlocker 启动一个"等取消才结束"的任务，并等到它真的进入 running。
//
// 必须等：Start 返回的是登记时的快照（queued），状态由执行协程改写。
// 不等就去断言 running，测试会随调度抖动而偶发失败。
func startBlocker(t *testing.T, mgr *jobs.Manager, owner, command string) jobs.Job {
	t.Helper()
	started := make(chan struct{})
	job, err := mgr.Start(owner, command, func(ctx context.Context) (int, error) {
		close(started)
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("启动阻塞任务: %v", err)
	}
	<-started

	deadline := time.Now().Add(5 * time.Second)
	for {
		current, getErr := mgr.Get(owner, job.ID)
		if getErr != nil {
			t.Fatalf("Get: %v", getErr)
		}
		if current.State == jobs.StateRunning {
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("任务未在预期时间内进入 running，实际 %q", current.State)
		}
		time.Sleep(time.Millisecond)
	}
}

// startWriter 启动一个写完输出就结束的任务。
func startWriter(t *testing.T, mgr *jobs.Manager, owner, command, stdout, stderr string) jobs.Job {
	t.Helper()
	job, err := mgr.StartWithOutput(owner, command, func(_ context.Context, out, errOut io.Writer) (int, error) {
		if stdout != "" {
			_, _ = io.WriteString(out, stdout)
		}
		if stderr != "" {
			_, _ = io.WriteString(errOut, stderr)
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("启动写输出任务: %v", err)
	}
	job2 := awaitTerminal(t, mgr, owner, job.ID)
	return job2
}

// writeJournalLine 往日志文件追加一行原始记录。
func writeJournalLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("打开日志: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("写入日志: %v", err)
	}
}

// ---------------------------------------------------------------- 构造

// TestNewJobTools_NilManagerReturnsNothing 确认没有管理器时不注册假能力。
func TestNewJobTools_NilManagerReturnsNothing(t *testing.T) {
	if tools := NewJobTools(fixedOwner("s1"), nil); len(tools) != 0 {
		t.Fatalf("nil manager 时应返回空，得到 %d 个工具", len(tools))
	}
}

// TestJobTools_DeclaredContract 确认三个工具的名字与必填参数声明。
func TestJobTools_DeclaredContract(t *testing.T) {
	mgr := newTestManager(t)
	tools := NewJobTools(fixedOwner("s1"), mgr)
	if len(tools) != 3 {
		t.Fatalf("应注册 3 个工具，得到 %d 个", len(tools))
	}
	for _, name := range []string{"job_list", "job_output", "job_cancel"} {
		tl := jobToolByName(t, tools, name)
		if tl.Description() == "" {
			t.Fatalf("%s 缺少描述", name)
		}
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(tl.Parameters(), &schema); err != nil {
			t.Fatalf("%s 参数不是合法 JSON: %v", name, err)
		}
		if name != "job_list" {
			if len(schema.Required) != 1 || schema.Required[0] != "job_id" {
				t.Fatalf("%s 应声明 job_id 必填，得到 %v", name, schema.Required)
			}
		}
	}
}

// ---------------------------------------------------------------- job_list

func TestJobList_Empty(t *testing.T) {
	mgr := newTestManager(t)
	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")

	res := executeTool(t, list, `{}`)
	if res.IsError {
		t.Fatalf("空列表不该是错误: %s", res.Content)
	}
	if !strings.Contains(res.Content, "本会话没有后台任务记录") {
		t.Fatalf("提示不符: %s", res.Content)
	}
}

// TestJobList_MergesHistory 确认列表把"本进程的 job"和"上次进程留下的记录"合并展示。
//
// 这是"恢复会话能看到历史结果"的验收点：只看内存态的话，重启后用户什么都看不到。
func TestJobList_MergesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	// 上次进程崩在任务中途留下的记录。
	writeJournalLine(t, path,
		`{"kind":"started","job_id":"old-job-001","owner":"s1","command":"make -j8 test","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`)

	mgr := newTestManagerWith(t, jobs.Options{Journal: jobs.NewJSONLJournal(path)})
	live := startWriter(t, mgr, "s1", "echo live", "live\n", "")
	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")

	res := executeTool(t, list, `{}`)
	if res.IsError {
		t.Fatalf("列表失败: %s", res.Content)
	}
	if !strings.Contains(res.Content, live.ID) {
		t.Fatalf("应包含本进程的任务 %s: %s", live.ID, res.Content)
	}
	if !strings.Contains(res.Content, "old-job-001") {
		t.Fatalf("应包含历史记录 old-job-001: %s", res.Content)
	}
	if !strings.Contains(res.Content, "make -j8 test") {
		t.Fatalf("历史记录应带上命令摘要: %s", res.Content)
	}
	if !strings.Contains(res.Content, "不会自动重跑命令") {
		t.Fatalf("必须声明不会重跑: %s", res.Content)
	}
}

// TestJobList_StateFilter 确认状态过滤。
func TestJobList_StateFilter(t *testing.T) {
	mgr := newTestManager(t)
	done := startWriter(t, mgr, "s1", "echo done", "x", "")
	running := startBlocker(t, mgr, "s1", "sleep 999")
	t.Cleanup(func() { mgr.CloseOwner("s1") })

	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")

	res := executeTool(t, list, `{"state":"succeeded"}`)
	if strings.Contains(res.Content, running.ID) {
		t.Fatalf("按 succeeded 过滤不该出现运行中的任务: %s", res.Content)
	}
	if !strings.Contains(res.Content, done.ID) {
		t.Fatalf("应包含已完成任务: %s", res.Content)
	}

	res = executeTool(t, list, `{"state":"running"}`)
	if !strings.Contains(res.Content, running.ID) {
		t.Fatalf("按 running 过滤应包含运行中任务: %s", res.Content)
	}
	if strings.Contains(res.Content, done.ID) {
		t.Fatalf("按 running 过滤不该出现已完成任务: %s", res.Content)
	}

	// 不存在的状态要说明是"没有这个状态的任务"，而不是"没有任务"。
	res = executeTool(t, list, `{"state":"timed_out"}`)
	if !strings.Contains(res.Content, "没有状态为 timed_out 的后台任务") {
		t.Fatalf("空过滤结果应说明原因: %s", res.Content)
	}
}

// TestJobList_LimitKeepsMostRecent 确认 limit 截断保留最近的任务。
func TestJobList_LimitKeepsMostRecent(t *testing.T) {
	mgr := newTestManager(t)
	var ids []string
	for i := 0; i < 3; i++ {
		job := startWriter(t, mgr, "s1", fmt.Sprintf("echo %d", i), "x", "")
		ids = append(ids, job.ID)
	}
	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")

	res := executeTool(t, list, `{"limit":1}`)
	if !strings.Contains(res.Content, "只显示最近 1 条") {
		t.Fatalf("应提示截断: %s", res.Content)
	}
	if !strings.Contains(res.Content, ids[2]) {
		t.Fatalf("应保留最新一条 %s: %s", ids[2], res.Content)
	}
	if strings.Contains(res.Content, ids[0]) {
		t.Fatalf("应丢弃最早一条 %s: %s", ids[0], res.Content)
	}
}

// TestJobList_OwnerIsolation 确认越权与不存在同样不可见。
func TestJobList_OwnerIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	writeJournalLine(t, path,
		`{"kind":"started","job_id":"other-001","owner":"other-session","command":"secret","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`)

	mgr := newTestManagerWith(t, jobs.Options{Journal: jobs.NewJSONLJournal(path)})
	startWriter(t, mgr, "s1", "echo mine", "mine\n", "")

	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")
	res := executeTool(t, list, `{}`)
	if strings.Contains(res.Content, "other-001") || strings.Contains(res.Content, "secret") {
		t.Fatalf("不该看到别的会话的任务: %s", res.Content)
	}
}

// TestJobList_EmptyOwnerIsRejected 确认归属缺失时拒绝服务。
func TestJobList_EmptyOwnerIsRejected(t *testing.T) {
	mgr := newTestManager(t)
	list := jobToolByName(t, NewJobTools(fixedOwner(""), mgr), "job_list")
	res := executeTool(t, list, `{}`)
	if !res.IsError || !strings.Contains(res.Content, "当前会话为空") {
		t.Fatalf("归属缺失应报错: %s (isError=%v)", res.Content, res.IsError)
	}
}

// TestJobList_BadArgs 确认参数解析失败有明确返回。
func TestJobList_BadArgs(t *testing.T) {
	mgr := newTestManager(t)
	list := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_list")
	res := executeTool(t, list, `{"limit":"not-a-number"}`)
	if !res.IsError || !strings.Contains(res.Content, "参数解析失败") {
		t.Fatalf("参数错误应明确报出: %s", res.Content)
	}
}

// ---------------------------------------------------------------- job_output

// TestJobOutput_PaginatesAcrossStreams 确认按偏移分页读取两个流。
func TestJobOutput_PaginatesAcrossStreams(t *testing.T) {
	mgr := newTestManager(t)
	stdoutText := strings.Repeat("a", 50)
	stderrText := "boom\n"
	job := startWriter(t, mgr, "s1", "noisy", stdoutText, stderrText)
	out := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_output")

	// 第一页
	res := executeTool(t, out, fmt.Sprintf(`{"job_id":%q,"limit":20}`, job.ID))
	if res.IsError {
		t.Fatalf("读 stdout 失败: %s", res.Content)
	}
	if !strings.Contains(res.Content, "stream=stdout offset=0") {
		t.Fatalf("应说明读取位置: %s", res.Content)
	}
	if !strings.Contains(res.Content, strings.Repeat("a", 20)) {
		t.Fatalf("第一页内容不对: %s", res.Content)
	}
	if !strings.Contains(res.Content, "还有未读内容") {
		t.Fatalf("未读完时应提示继续: %s", res.Content)
	}

	// 直接读偏移 50（末尾）
	res = executeTool(t, out, fmt.Sprintf(`{"job_id":%q,"limit":20,"offset":50}`, job.ID))
	if !strings.Contains(res.Content, "offset=50") || !strings.Contains(res.Content, "eof=true") {
		t.Fatalf("末尾读取应 eof: %s", res.Content)
	}
	if !strings.Contains(res.Content, fmt.Sprintf("total=%d", len(stdoutText))) {
		t.Fatalf("应报告总字节数: %s", res.Content)
	}

	// stdderr 流
	res = executeTool(t, out, fmt.Sprintf(`{"job_id":%q,"stream":"stderr"}`, job.ID))
	if !strings.Contains(res.Content, "stream=stderr") || !strings.Contains(res.Content, stderrText) {
		t.Fatalf("stderr 读取不对: %s", res.Content)
	}
}

// TestJobOutput_RejectsBadArgs 覆盖参数校验分支。
func TestJobOutput_RejectsBadArgs(t *testing.T) {
	mgr := newTestManager(t)
	out := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_output")

	cases := []struct {
		name string
		args string
		want string
	}{
		{"缺 job_id", `{}`, "job_id 不能为空"},
		{"job_id 为空白", `{"job_id":"   "}`, "job_id 不能为空"},
		{"未知流", `{"job_id":"j","stream":"stdin"}`, "未知输出流"},
		{"参数不是对象", `[]`, "参数解析失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := executeTool(t, out, tc.args)
			if !res.IsError || !strings.Contains(res.Content, tc.want) {
				t.Fatalf("期望包含 %q，得到 %s (isError=%v)", tc.want, res.Content, res.IsError)
			}
		})
	}
}

// TestJobOutput_OwnerIsolationLooksLikeNotFound 确认越权与"不存在"返回同一句话。
//
// 两者一旦可区分，job ID 就变成了探测其他会话的通道。
func TestJobOutput_OwnerIsolationLooksLikeNotFound(t *testing.T) {
	mgr := newTestManager(t)
	job := startWriter(t, mgr, "owner-a", "echo secret", "secret\n", "")

	out := jobToolByName(t, NewJobTools(fixedOwner("owner-b"), mgr), "job_output")
	res := executeTool(t, out, fmt.Sprintf(`{"job_id":%q}`, job.ID))
	if !res.IsError {
		t.Fatalf("越权读取必须失败: %s", res.Content)
	}
	if !strings.Contains(res.Content, "未找到后台任务") {
		t.Fatalf("越权应与不存在返回同一提示: %s", res.Content)
	}
	if strings.Contains(res.Content, "secret") {
		t.Fatalf("不该泄漏任何输出内容: %s", res.Content)
	}
}

// TestJobOutput_HistoricalJobExplainsUnreadable 确认历史任务的输出不可读时说清楚原因。
func TestJobOutput_HistoricalJobExplainsUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	writeJournalLine(t, path,
		`{"kind":"started","job_id":"old-001","owner":"s1","command":"echo old","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`)

	mgr := newTestManagerWith(t, jobs.Options{Journal: jobs.NewJSONLJournal(path)})
	out := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_output")

	res := executeTool(t, out, `{"job_id":"old-001"}`)
	if !res.IsError {
		t.Fatalf("历史任务的输出不可读，必须报错而不是给出空内容: %s", res.Content)
	}
	if !strings.Contains(res.Content, "不在当前进程中") {
		t.Fatalf("应说明是进程已退出: %s", res.Content)
	}
}

// TestJobOutput_EmptyOwnerIsRejected 确认归属缺失时拒绝服务。
func TestJobOutput_EmptyOwnerIsRejected(t *testing.T) {
	mgr := newTestManager(t)
	out := jobToolByName(t, NewJobTools(fixedOwner(""), mgr), "job_output")
	res := executeTool(t, out, `{"job_id":"j"}`)
	if !res.IsError || !strings.Contains(res.Content, "当前会话为空") {
		t.Fatalf("归属缺失应报错: %s", res.Content)
	}
}

// ---------------------------------------------------------------- job_cancel

// TestJobCancel_RunningReachesTerminal 确认取消会等到终态，而不是只发信号。
//
// 只发信号的话，模型会看到"取消成功"但状态还是 running，于是重复取消。
func TestJobCancel_RunningReachesTerminal(t *testing.T) {
	mgr := newTestManager(t)
	job := startBlocker(t, mgr, "s1", "sleep 999")

	cancel := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_cancel")
	res := executeTool(t, cancel, fmt.Sprintf(`{"job_id":%q}`, job.ID))
	if res.IsError {
		t.Fatalf("取消失败: %s", res.Content)
	}
	if !strings.Contains(res.Content, "已取消") || !strings.Contains(res.Content, string(jobs.StateCanceled)) {
		t.Fatalf("应报告取消并给出终态: %s", res.Content)
	}

	final, err := mgr.Get("s1", job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if final.State != jobs.StateCanceled {
		t.Fatalf("终态应为 canceled，得到 %q", final.State)
	}
}

// TestJobCancel_AlreadyTerminalDoesNotRestart 确认已结束的任务不会被重启。
func TestJobCancel_AlreadyTerminalDoesNotRestart(t *testing.T) {
	mgr := newTestManager(t)
	job := startWriter(t, mgr, "s1", "echo done", "x", "")

	cancel := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_cancel")
	res := executeTool(t, cancel, fmt.Sprintf(`{"job_id":%q}`, job.ID))
	if res.IsError {
		t.Fatalf("终态任务不该报错: %s", res.Content)
	}
	if !strings.Contains(res.Content, "已是终态") || !strings.Contains(res.Content, "不会被重新执行") {
		t.Fatalf("应明确说明不会重跑: %s", res.Content)
	}
}

// TestJobCancel_HistoricalJobIsNotRestartable 确认历史任务无法被"取消"（更不会被重跑）。
func TestJobCancel_HistoricalJobIsNotRestartable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	writeJournalLine(t, path,
		`{"kind":"started","job_id":"old-001","owner":"s1","command":"make","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`)

	mgr := newTestManagerWith(t, jobs.Options{Journal: jobs.NewJSONLJournal(path)})
	cancel := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_cancel")

	res := executeTool(t, cancel, `{"job_id":"old-001"}`)
	if !res.IsError || !strings.Contains(res.Content, "未找到可取消的后台任务") {
		t.Fatalf("历史任务不可取消: %s", res.Content)
	}
}

// TestJobCancel_RejectsBadArgs 覆盖参数校验分支。
func TestJobCancel_RejectsBadArgs(t *testing.T) {
	mgr := newTestManager(t)
	cancel := jobToolByName(t, NewJobTools(fixedOwner("s1"), mgr), "job_cancel")

	res := executeTool(t, cancel, `{}`)
	if !res.IsError || !strings.Contains(res.Content, "job_id 不能为空") {
		t.Fatalf("缺 job_id 应报错: %s", res.Content)
	}

	res = executeTool(t, cancel, `{"job_id":"nope"}`)
	if !res.IsError || !strings.Contains(res.Content, "未找到可取消的后台任务") {
		t.Fatalf("不存在的 job 应报错: %s", res.Content)
	}

	empty := jobToolByName(t, NewJobTools(fixedOwner(""), mgr), "job_cancel")
	res = executeTool(t, empty, `{"job_id":"x"}`)
	if !res.IsError || !strings.Contains(res.Content, "当前会话为空") {
		t.Fatalf("归属缺失应报错: %s", res.Content)
	}
}

// TestJobCancel_OwnerIsolation 确认不能取消别的会话的任务。
func TestJobCancel_OwnerIsolation(t *testing.T) {
	mgr := newTestManager(t)
	job := startBlocker(t, mgr, "owner-a", "sleep 999")
	t.Cleanup(func() { mgr.CloseOwner("owner-a") })

	cancel := jobToolByName(t, NewJobTools(fixedOwner("owner-b"), mgr), "job_cancel")
	res := executeTool(t, cancel, fmt.Sprintf(`{"job_id":%q}`, job.ID))
	if !res.IsError || !strings.Contains(res.Content, "未找到可取消的后台任务") {
		t.Fatalf("越权取消失败: %s", res.Content)
	}
	if final, err := mgr.Get("owner-a", job.ID); err != nil || final.State == jobs.StateCanceled {
		t.Fatalf("越权取消不该生效: %+v / %v", final, err)
	}
}

// ---------------------------------------------------------------- 输出渲染

// TestFormatOutputPage_ReportsExpiryAndTruncation 确认分页文本把配额情况说清楚。
func TestFormatOutputPage_ReportsExpiryAndTruncation(t *testing.T) {
	page := jobs.OutputPage{
		Stream:       jobs.StreamStdout,
		Offset:       10,
		NextOffset:   30,
		Data:         []byte("hello"),
		Text:         "hello",
		TotalSize:    100,
		StoredSize:   40,
		Truncated:    true,
		DroppedBytes: 60,
		DropReason:   "超出内存配额",
		EOF:          true,
		ExpiresAt:    time.Unix(2000, 0).UTC(),
	}
	out := formatOutputPage(page)
	if !strings.Contains(out, "truncated=true dropped=60 reason=超出内存配额") {
		t.Fatalf("应报告丢弃量与原因: %s", out)
	}
	if !strings.Contains(out, "total=100 stored=40") {
		t.Fatalf("应报告总量与实存量: %s", out)
	}
	if strings.Contains(out, "继续读取") {
		t.Fatalf("EOF 时不该提示继续: %s", out)
	}
	if !strings.HasSuffix(out, "hello") {
		t.Fatalf("正文应放在最后: %q", out)
	}
}
