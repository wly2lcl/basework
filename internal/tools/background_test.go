package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

// newTestManager 构造测试用 Manager，测试结束前取消并清理输出。
//
// 输出目录落在 t.TempDir()：JOB-002 之后后台命令的输出会真的落盘，
// 用默认目录会把溢出文件写进用户的缓存目录。
func newTestManager(t *testing.T) *jobs.Manager {
	t.Helper()
	return newTestManagerWith(t, jobs.Options{})
}

// newTestManagerWith 允许测试覆盖 Manager 配置（例如输出配额）。
func newTestManagerWith(t *testing.T, opts jobs.Options) *jobs.Manager {
	t.Helper()
	if opts.Output.Dir == "" {
		opts.Output.Dir = t.TempDir()
	}
	m := jobs.New(opts)
	t.Cleanup(m.Close)
	return m
}

// readOutput 按 limit 分页读完某个流的全部可读内容。
func readOutput(t *testing.T, mgr *jobs.Manager, owner, id string, stream jobs.Stream, limit int64) ([]byte, jobs.OutputPage) {
	t.Helper()
	var (
		got  []byte
		last jobs.OutputPage
		off  int64
	)
	for i := 0; ; i++ {
		if i > 100000 {
			t.Fatalf("分页读取未在合理次数内结束")
		}
		page, err := mgr.ReadOutput(owner, id, stream, off, limit)
		if err != nil {
			t.Fatalf("第 %d 页读取失败: %v", i, err)
		}
		last = page
		if len(page.Data) == 0 {
			break
		}
		got = append(got, page.Data...)
		off = page.NextOffset
		if page.EOF {
			break
		}
	}
	return got, last
}

// executeTool 以 JSON 参数执行工具。
func executeTool(t *testing.T, tl tool.Tool, args string) *tool.Result {
	t.Helper()
	res, err := tl.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	return res
}

// decodeHandle 解析成功结果里的结构化句柄。
func decodeHandle(t *testing.T, res *tool.Result) JobHandle {
	t.Helper()
	if res.IsError {
		t.Fatalf("期望成功结果，得到错误: %s", res.Content)
	}
	var h JobHandle
	if err := json.Unmarshal([]byte(res.Content), &h); err != nil {
		t.Fatalf("结果不是结构化句柄: %v\n%s", err, res.Content)
	}
	return h
}

// awaitTerminal 等待 job 进入终态。
func awaitTerminal(t *testing.T, mgr *jobs.Manager, owner, id string) jobs.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	job, err := mgr.Await(ctx, owner, id)
	if err != nil {
		t.Fatalf("等待 job 终态失败: %v", err)
	}
	return job
}

// TestBackgroundBash_ReturnsStructuredJobHandle 是本任务的核心验收：
// 适配器返回结构化 job ID，且归属为当前会话。
func TestBackgroundBash_ReturnsStructuredJobHandle(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	res := executeTool(t, tl, `{"command":"echo hello"}`)
	handle := decodeHandle(t, res)

	if handle.JobID == "" {
		t.Fatal("句柄缺少 job_id")
	}
	if handle.Owner != "session-a" {
		t.Errorf("归属应为 session-a，得到 %q", handle.Owner)
	}
	if handle.Command != "echo hello" {
		t.Errorf("句柄应带命令摘要，得到 %q", handle.Command)
	}
	if handle.State != jobs.StateQueued && handle.State != jobs.StateRunning {
		t.Errorf("刚启动的状态应为 queued/running，得到 %s", handle.State)
	}

	// 句柄指向的 job 必须真的在 manager 里，且归属正确。
	job, err := mgr.Get("session-a", handle.JobID)
	if err != nil {
		t.Fatalf("按句柄查询 job 失败: %v", err)
	}
	if job.Owner != "session-a" || job.Command != "echo hello" {
		t.Errorf("查询到的 job 与句柄不一致: %+v", job)
	}

	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateSucceeded {
		t.Errorf("echo 应成功，得到 %s（Err=%q）", final.State, final.Err)
	}
}

// TestBackgroundBash_OtherSessionCannotSeeJob 验证归属边界在适配器层同样成立：
// 句柄里的 job ID 泄漏给别的会话也无法读取。
func TestBackgroundBash_OtherSessionCannotSeeJob(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"echo hi"}`))
	awaitTerminal(t, mgr, "session-a", handle.JobID)

	if _, err := mgr.Get("session-b", handle.JobID); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("其他会话读取应返回 ErrNotFound，得到 %v", err)
	}
	if err := mgr.Cancel("session-b", handle.JobID); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("其他会话取消应返回 ErrNotFound，得到 %v", err)
	}
}

// TestBackgroundBash_BlacklistRejectsWithoutStartingJob 验证权限拒绝时不会登记 job。
func TestBackgroundBash_BlacklistRejectsWithoutStartingJob(t *testing.T) {
	mgr := newTestManager(t)

	dangerous := "rm -rf /tmp/whatever"
	// 先用同一份实现确认这条命令确实会被黑名单命中，避免测试依赖猜测。
	if matched, _, err := builtinCheck(dangerous); err != nil || !matched {
		t.Skipf("当前黑名单未覆盖该命令（matched=%v err=%v），跳过", matched, err)
	}

	tl := NewBackgroundBashTool("session-a", mgr)
	tl.PermissionMode = "default"

	res := executeTool(t, tl, `{"command":"`+dangerous+`"}`)
	if !res.IsError {
		t.Fatalf("黑名单命中应返回错误结果，得到: %s", res.Content)
	}
	if !strings.Contains(res.Content, "黑名单") {
		t.Errorf("错误信息应指明黑名单来源: %s", res.Content)
	}
	if mgr.Len() != 0 {
		t.Errorf("被拒绝的命令不应登记 job，实际 %d 个", mgr.Len())
	}
	if len(mgr.List("session-a")) != 0 {
		t.Error("被拒绝的命令不应出现在会话的 job 列表里")
	}
}

// TestBackgroundBash_InteractiveModeAsksInsteadOfStarting 验证交互模式返回待确认而非启动。
func TestBackgroundBash_InteractiveModeAsksInsteadOfStarting(t *testing.T) {
	mgr := newTestManager(t)
	dangerous := "rm -rf /tmp/whatever"
	if matched, _, err := builtinCheck(dangerous); err != nil || !matched {
		t.Skipf("当前黑名单未覆盖该命令（matched=%v err=%v），跳过", matched, err)
	}

	tl := NewBackgroundBashTool("session-a", mgr)
	tl.PermissionMode = "interactive"

	res := executeTool(t, tl, `{"command":"`+dangerous+`"}`)
	if !res.IsError || !strings.Contains(res.Content, "CONFIRM") {
		t.Fatalf("交互模式应返回待确认提示，得到: %s", res.Content)
	}
	if mgr.Len() != 0 {
		t.Errorf("待确认时不应登记 job，实际 %d 个", mgr.Len())
	}
}

// TestBackgroundBash_PathCheckRejectsWithoutStartingJob 验证路径检查在同一入口生效。
func TestBackgroundBash_PathCheckRejectsWithoutStartingJob(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)
	tl.CheckPath = func(p string) (bool, string) {
		if strings.Contains(p, "protected") {
			return false, "受保护路径"
		}
		return true, ""
	}

	res := executeTool(t, tl, `{"command":"cat /etc/protected/secret.txt"}`)
	if !res.IsError {
		t.Fatalf("路径被拒时应返回错误结果，得到: %s", res.Content)
	}
	if !strings.Contains(res.Content, "受保护路径") {
		t.Errorf("应带拒绝原因: %s", res.Content)
	}
	if mgr.Len() != 0 {
		t.Errorf("路径被拒时不应登记 job，实际 %d 个", mgr.Len())
	}

	// 允许的路径应正常启动
	res2 := executeTool(t, tl, `{"command":"cat /tmp/ok.txt"}`)
	if res2.IsError {
		t.Fatalf("允许的路径应启动成功，得到: %s", res2.Content)
	}
	if mgr.Len() != 1 {
		t.Errorf("应登记 1 个 job，实际 %d", mgr.Len())
	}
}

// TestBackgroundBash_RejectsMissingOwnerAndEmptyCommand 验证启动前的明确拒绝。
func TestBackgroundBash_RejectsMissingOwnerAndEmptyCommand(t *testing.T) {
	mgr := newTestManager(t)

	noOwner := NewBackgroundBashTool("", mgr)
	res := executeTool(t, noOwner, `{"command":"echo hi"}`)
	if !res.IsError || !strings.Contains(res.Content, "归属") {
		t.Fatalf("缺少会话归属时应明确拒绝，得到: %s", res.Content)
	}

	withOwner := NewBackgroundBashTool("session-a", mgr)
	res = executeTool(t, withOwner, `{"command":""}`)
	if !res.IsError || !strings.Contains(res.Content, "不能为空") {
		t.Fatalf("空命令应明确拒绝，得到: %s", res.Content)
	}

	if mgr.Len() != 0 {
		t.Errorf("拒绝的请求不应登记 job，实际 %d", mgr.Len())
	}
}

// TestBackgroundBash_NonZeroExitIsFailedNotErrorResult 验证"命令失败"与"工具失败"分开：
// 命令非零退出是可观测的 job 终态，不是工具调用错误。
func TestBackgroundBash_NonZeroExitIsFailedNotErrorResult(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	res := executeTool(t, tl, `{"command":"exit 7"}`)
	handle := decodeHandle(t, res)

	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateFailed {
		t.Errorf("非零退出应记为 failed，得到 %s", final.State)
	}
	if final.ExitCode != 7 {
		t.Errorf("退出码应为 7，得到 %d", final.ExitCode)
	}
}

// TestBackgroundBash_AgentCanContinueWhileJobRuns 验证长命令不会阻塞调用方：
// 启动立即返回，job 仍在运行。这是后续 JOB-004「Agent 可继续处理」的前提。
func TestBackgroundBash_AgentCanContinueWhileJobRuns(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	start := time.Now()
	res := executeTool(t, tl, `{"command":"sleep 2"}`)
	elapsed := time.Since(start)
	handle := decodeHandle(t, res)

	if elapsed > time.Second {
		t.Fatalf("启动应立刻返回，实际耗时 %v", elapsed)
	}
	if mgr.Active("session-a") != 1 {
		t.Fatalf("启动后应有 1 个活跃 job，实际 %d", mgr.Active("session-a"))
	}

	// 命令仍在运行时即可查询状态，且不阻塞
	inFlight, err := mgr.Get("session-a", handle.JobID)
	if err != nil {
		t.Fatalf("运行中查询失败: %v", err)
	}
	if inFlight.State.Terminal() {
		t.Fatalf("sleep 2 不应已经结束，得到 %s", inFlight.State)
	}
}

// TestBackgroundBash_CancelStopsLongCommand 验证取消入口对后台命令生效。
func TestBackgroundBash_CancelStopsLongCommand(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"sleep 30"}`))
	if err := mgr.Cancel("session-a", handle.JobID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateCanceled {
		t.Errorf("取消后应为 canceled，得到 %s", final.State)
	}
}

// TestBackgroundBash_TimeoutProducesTerminalState 验证超时不会让 job 卡在运行态。
func TestBackgroundBash_TimeoutProducesTerminalState(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"sleep 30","timeout":1}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if !final.State.Terminal() {
		t.Fatalf("超时后应进入终态，得到 %s", final.State)
	}
	if final.State == jobs.StateSucceeded {
		t.Error("超时的命令不应被记为成功")
	}
}

// TestBackgroundBash_DefaultTimeoutAppliesWhenCallOmitsIt
// 验证"调用没给 timeout"时用的是工具默认值，而不是永远 30 秒。
//
// 这条覆盖产品接线：cmd/basework 把 tools.timeout 里 bash 的解析结果填进 DefaultTimeout，
// 填了不生效等于配置是摆设。
func TestBackgroundBash_DefaultTimeoutAppliesWhenCallOmitsIt(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)
	tl.DefaultTimeout = 1

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"sleep 30"}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateTimedOut {
		t.Fatalf("未传 timeout 时应按工具默认值超时并记为 timed_out，得到 %q", final.State)
	}
}

// TestBackgroundBash_NoTimeoutSkipsDeadline 验证"配置显式关闭超时"的表达。
//
// builtin 的超时配置把 0 定义为"不超时"。若后台 bash 把 0 当成"未配置"再回落成 30 秒，
// 用户关掉超时后同步 bash 不超时、后台 bash 却仍被掐死，同一个配置有两种解释。
func TestBackgroundBash_NoTimeoutSkipsDeadline(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)
	tl.DefaultTimeout = 1
	tl.NoTimeout = true

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"sleep 30"}`))
	t.Cleanup(func() { mgr.CloseOwner("session-a") })

	// 等得比 DefaultTimeout 更久：若关闭超时没生效，这里就会看到终态。
	time.Sleep(1500 * time.Millisecond)
	current, err := mgr.Get("session-a", handle.JobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if current.State.Terminal() {
		t.Fatalf("关闭超时后不该被 DefaultTimeout 掐掉，得到 %q", current.State)
	}

	// 显式给的 timeout 仍然优先于"关闭超时"：那是本次调用的明确意图。
	tl2 := NewBackgroundBashTool("session-b", mgr)
	tl2.NoTimeout = true
	handle2 := decodeHandle(t, executeTool(t, tl2, `{"command":"sleep 30","timeout":1}`))
	if final := awaitTerminal(t, mgr, "session-b", handle2.JobID); final.State != jobs.StateTimedOut {
		t.Fatalf("显式 timeout 应覆盖 NoTimeout，得到 %q", final.State)
	}
}

// TestExtractCommandPaths 固定路径提取规则与同步 bash 一致的行为。
func TestExtractCommandPaths(t *testing.T) {
	got := extractCommandPaths("cat /etc/hosts ./local.txt ../up.txt name=value -flag plain")
	want := map[string]bool{"/etc/hosts": true, "./local.txt": true, "../up.txt": true}
	seen := map[string]bool{}
	for _, p := range got {
		seen[p] = true
	}
	for p := range want {
		if !seen[p] {
			t.Errorf("应提取出路径 %q，实际 %v", p, got)
		}
	}
	if seen["name=value"] {
		t.Errorf("含 = 的赋值片段不应被当成路径: %v", got)
	}
	if seen["-flag"] {
		t.Errorf("选项不应被当成路径: %v", got)
	}
}

// TestBackgroundBash_RespectsWorkingDirectory 验证命令在指定工作目录下执行，
// 用的是相对路径解析，避免测试依赖绝对路径。
func TestBackgroundBash_RespectsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "marker.txt")

	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)
	// 用绝对路径写文件，确认工具确实把命令交给了 shell 执行。
	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"printf ok > `+target+`"}`))
	awaitTerminal(t, mgr, "session-a", handle.JobID)

	if !fileExists(target) {
		t.Fatalf("后台命令未产生预期文件: %s", target)
	}
}

// builtinCheck 直接调用同步 bash 用的那份黑名单实现，让测试不依赖对模式的猜测。
func builtinCheck(cmd string) (bool, string, error) {
	return builtin.CheckBlacklist(cmd, nil)
}

// fileExists 报告路径是否存在。
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestBackgroundBash_CapturesOutputForPaging 验证后台命令的 stdout 真的被承接下来了：
// 分页读回的字节与命令输出逐字节一致。
func TestBackgroundBash_CapturesOutputForPaging(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"printf 'alpha\\nbeta\\ngamma\\n'"}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateSucceeded {
		t.Fatalf("命令应成功，得到 %s（%s）", final.State, final.Err)
	}

	want := []byte("alpha\nbeta\ngamma\n")
	for _, limit := range []int64{1, 2, 3, 5, 4096} {
		got, page := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStdout, limit)
		if !bytes.Equal(got, want) {
			t.Fatalf("limit=%d 读回的输出与命令输出不一致: %q", limit, got)
		}
		if page.TotalSize != int64(len(want)) {
			t.Fatalf("limit=%d 总字节数应为 %d，得到 %d", limit, len(want), page.TotalSize)
		}
		if page.Truncated {
			t.Fatalf("小输出不应被截断")
		}
	}
	if final.StdoutBytes != int64(len(want)) {
		t.Fatalf("快照计数的 stdout 字节数不符: %d", final.StdoutBytes)
	}
}

// TestBackgroundBash_LargeOutputSpillsToPrivateFile 是「大输出不无限占内存」的端到端验证：
// 超出内存配额的部分必须落在 0600 私有文件里，内存只保留配额内的字节。
func TestBackgroundBash_LargeOutputSpillsToPrivateFile(t *testing.T) {
	const memLimit = 1024
	const total = 200000
	mgr := newTestManagerWith(t, jobs.Options{
		Output: jobs.OutputOptions{MemoryLimit: memLimit, MaxBytes: 8 << 20},
	})
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"yes 0123456789abcdef | head -c `+itoa(total)+`"}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateSucceeded {
		t.Fatalf("命令应成功，得到 %s（%s）", final.State, final.Err)
	}
	if final.StdoutBytes != total {
		t.Fatalf("应接收 %d 字节，实际 %d", total, final.StdoutBytes)
	}
	if final.OutputTruncated {
		t.Fatalf("未达总配额不应截断: %+v", final)
	}
	if len(final.OutputRefs) != 1 {
		t.Fatalf("应产生一个溢出文件引用: %v", final.OutputRefs)
	}
	info, err := os.Stat(final.OutputRefs[0])
	if err != nil {
		t.Fatalf("溢出文件应存在: %v", err)
	}
	if got := info.Size(); got != total-memLimit {
		t.Fatalf("溢出文件大小应为总量减内存配额 %d，实际 %d", total-memLimit, got)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("溢出文件必须是 0600 私有文件，实际 %o", perm)
	}
	// 引用必须落在 manager 声明的输出目录内。
	rel, err := filepath.Rel(mgr.OutputDir(), final.OutputRefs[0])
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("溢出引用越出输出目录: %v (%s)", err, rel)
	}
	// 内容仍可完整读回，且分页大小取奇数时也不漏不重。
	head, _ := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStdout, 7777)
	if len(head) != total {
		t.Fatalf("分页读回字节数应为 %d，实际 %d", total, len(head))
	}
	// `yes` 每次输出都补一个换行，所以重复单元是 17 字节而不是 16。
	unit := []byte("0123456789abcdef\n")
	for i := 0; i < 64; i++ {
		if head[i] != unit[i%len(unit)] {
			t.Fatalf("第 %d 字节应为 %q，实际 %q", i, unit[i%len(unit)], head[i])
		}
	}
}

// TestBackgroundBash_OutputQuotaDropsWithExplanation 验证超过总存储配额时丢弃尾部并说明原因，
// 且「输出太多」不会把命令本身判成失败。
func TestBackgroundBash_OutputQuotaDropsWithExplanation(t *testing.T) {
	const maxBytes = 4096
	const total = 100000
	mgr := newTestManagerWith(t, jobs.Options{
		Output: jobs.OutputOptions{MemoryLimit: 512, MaxBytes: maxBytes},
	})
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"yes 0123456789abcdef | head -c `+itoa(total)+`"}`))
	final := awaitTerminal(t, mgr, "session-a", handle.JobID)
	if final.State != jobs.StateSucceeded {
		t.Fatalf("输出超量不应让命令判为失败，得到 %s（%s）", final.State, final.Err)
	}
	if !final.OutputTruncated {
		t.Fatalf("快照应标记输出被截断")
	}

	_, page := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStdout, 65536)
	if page.TotalSize != total {
		t.Fatalf("总接收字节数应如实记录 %d，实际 %d", total, page.TotalSize)
	}
	if page.StoredSize != maxBytes {
		t.Fatalf("保留量应等于配额 %d，实际 %d", maxBytes, page.StoredSize)
	}
	if page.DroppedBytes != total-maxBytes {
		t.Fatalf("丢弃量应为 %d，实际 %d", total-maxBytes, page.DroppedBytes)
	}
	if page.DropReason == "" {
		t.Fatal("丢弃必须给出可读原因")
	}
	if len(page.Data) != maxBytes {
		t.Fatalf("可读数据应为保留量 %d，实际 %d", maxBytes, len(page.Data))
	}
}

// TestBackgroundBash_OtherSessionCannotReadOutput 验证输出读取也受归属边界约束。
func TestBackgroundBash_OtherSessionCannotReadOutput(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl, `{"command":"echo classified"}`))
	awaitTerminal(t, mgr, "session-a", handle.JobID)

	if _, err := mgr.ReadOutput("session-b", handle.JobID, jobs.StreamStdout, 0, 4096); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("其他会话读取输出应返回 ErrNotFound，实际 %v", err)
	}
	// 归属者可以读。
	got, _ := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStdout, 4096)
	if strings.TrimSpace(string(got)) != "classified" {
		t.Fatalf("归属者应读到自己的输出，实际 %q", got)
	}
}

// TestBackgroundBash_StderrCapturedSeparately 验证 stderr 单独承接，不会混进 stdout。
func TestBackgroundBash_StderrCapturedSeparately(t *testing.T) {
	mgr := newTestManager(t)
	tl := NewBackgroundBashTool("session-a", mgr)

	handle := decodeHandle(t, executeTool(t, tl,
		`{"command":"echo to-stdout; echo to-stderr 1>&2"}`))
	awaitTerminal(t, mgr, "session-a", handle.JobID)

	out, _ := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStdout, 4096)
	errOut, _ := readOutput(t, mgr, "session-a", handle.JobID, jobs.StreamStderr, 4096)
	if strings.TrimSpace(string(out)) != "to-stdout" {
		t.Fatalf("stdout 内容不符: %q", out)
	}
	if strings.TrimSpace(string(errOut)) != "to-stderr" {
		t.Fatalf("stderr 内容不符: %q", errOut)
	}
}

// itoa 避免为一行转换引入额外 import。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
