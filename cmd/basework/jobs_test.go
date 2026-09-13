package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// ---------------------------------------------------------------- 测试替身

// stubSessionAgent 是"能报出会话 ID"的最小 Agent 实现。
type stubSessionAgent struct{ sessionID string }

func (s *stubSessionAgent) HandleMessage(context.Context, string) (*agent.Response, error) {
	return &agent.Response{SessionID: s.sessionID}, nil
}

func (s *stubSessionAgent) HandleMessages(context.Context, []llm.ChatMessage) (*agent.Response, error) {
	return &agent.Response{SessionID: s.sessionID}, nil
}

func (s *stubSessionAgent) Tools() []tool.Tool { return nil }

func (s *stubSessionAgent) Close() error { return nil }

func (s *stubSessionAgent) SessionID() string { return s.sessionID }

// plainAgent 不实现 SessionIDProvider，用于确认绑定过程不会因此 panic。
type plainAgent struct{}

func (p *plainAgent) HandleMessage(context.Context, string) (*agent.Response, error) {
	return &agent.Response{}, nil
}

func (p *plainAgent) HandleMessages(context.Context, []llm.ChatMessage) (*agent.Response, error) {
	return &agent.Response{}, nil
}

func (p *plainAgent) Tools() []tool.Tool { return nil }

func (p *plainAgent) Close() error { return nil }

var (
	_ agent.SessionIDProvider = (*stubSessionAgent)(nil)
	_ agent.Agent             = (*plainAgent)(nil)
)

// ---------------------------------------------------------------- 归属持有者

func TestJobOwnerHolder_SetGet(t *testing.T) {
	h := &jobOwnerHolder{}
	if got := h.Get(); got != "" {
		t.Fatalf("初始应为空，得到 %q", got)
	}
	h.Set("session-1")
	if got := h.Get(); got != "session-1" {
		t.Fatalf("Get() = %q, want session-1", got)
	}
}

// TestJobOwnerHolder_ConcurrentAccess 确认回填与读取之间没有数据竞争。
// 产品上 Set 发生在构造线程、Get 发生在工具执行线程，这正是 -race 会抓的形态。
func TestJobOwnerHolder_ConcurrentAccess(t *testing.T) {
	h := &jobOwnerHolder{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = h.Get()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.Set("session-race")
	}()
	wg.Wait()
}

// ---------------------------------------------------------------- 管理器装配

// TestNewRuntimeJobManager_IDsDoNotCollideAcrossRestart 是"重启不破坏历史"的关键。
//
// 两个 Manager 代表同一会话的两次进程启动，共用一个日志文件。若 ID 在重启后重来，
// foldRecords 会把两轮运行折叠成一条，历史就失真了。
func TestNewRuntimeJobManager_IDsDoNotCollideAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "jobs.jsonl")
	outputDir := filepath.Join(dir, "out")

	owner := &jobOwnerHolder{}
	owner.Set("abcdefghijklmnop")

	first := newRuntimeJobManagerAt(owner, journalPath, outputDir)
	t.Cleanup(first.Close)
	job1, err := first.StartWithOutput("abcdefghijklmnop", "echo first", func(_ context.Context, out, _ io.Writer) (int, error) {
		_, _ = io.WriteString(out, "first\n")
		return 0, nil
	})
	if err != nil {
		t.Fatalf("第一轮启动失败: %v", err)
	}
	if _, err := first.Await(context.Background(), "abcdefghijklmnop", job1.ID); err != nil {
		t.Fatalf("等待第一轮结束失败: %v", err)
	}

	second := newRuntimeJobManagerAt(owner, journalPath, outputDir)
	t.Cleanup(second.Close)
	job2, err := second.StartWithOutput("abcdefghijklmnop", "echo second", func(_ context.Context, out, _ io.Writer) (int, error) {
		_, _ = io.WriteString(out, "second\n")
		return 0, nil
	})
	if err != nil {
		t.Fatalf("第二轮启动失败: %v", err)
	}
	if _, err := second.Await(context.Background(), "abcdefghijklmnop", job2.ID); err != nil {
		t.Fatalf("等待第二轮结束失败: %v", err)
	}

	if job1.ID == job2.ID {
		t.Fatalf("重启后 job ID 撞车: %q", job1.ID)
	}
	if !strings.HasPrefix(job1.ID, "abcdefghijkl") {
		t.Fatalf("ID 应带会话前缀以便人读: %q", job1.ID)
	}

	history, err := second.AllHistory()
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("两轮运行应留下 2 条独立记录，得到 %d 条（%+v）", len(history), history)
	}
	states := map[string]jobs.State{}
	for _, job := range history {
		states[job.ID] = job.State
	}
	if states[job1.ID] != jobs.StateSucceeded || states[job2.ID] != jobs.StateSucceeded {
		t.Fatalf("两条记录都应为 succeeded，得到 %+v", states)
	}
}

// TestNewRuntimeJobManager_EmptyOwnerUsesPendingPrefix 确认归属未回填时的 ID 可辨认。
//
// 这个窗口只可能出现在"工具已构造、Agent 尚未构造完成"之间；用 pending 而不是
// 空串前缀，是为了让日志里留下的任何记录都能一眼看出归属缺失。
func TestNewRuntimeJobManager_EmptyOwnerUsesPendingPrefix(t *testing.T) {
	dir := t.TempDir()
	mgr := newRuntimeJobManagerAt(&jobOwnerHolder{}, filepath.Join(dir, "jobs.jsonl"), filepath.Join(dir, "out"))
	t.Cleanup(mgr.Close)

	job, err := mgr.Start("whatever-owner", "echo hi", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.HasPrefix(job.ID, "pending-") {
		t.Fatalf("归属缺失时 ID 应以 pending- 开头，得到 %q", job.ID)
	}
}

// ---------------------------------------------------------------- 重启归并

// TestBindRuntimeJobOwner_ReconcilesLeftoverRunning 模拟"上一进程崩在任务中途"。
//
// 磁盘上留下的就是一条 started 记录（没有任何 terminal）。重新启动时：
//  1. 归属被回填成会话 ID；
//  2. 那条记录被改成 interrupted，而不是继续显示 queued/running。
func TestBindRuntimeJobOwner_ReconcilesLeftoverRunning(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "jobs.jsonl")
	const owner = "session-leftover"

	// 这是崩溃后磁盘上真实的样子：只有 started，没有 terminal。
	line := `{"kind":"started","job_id":"session-leftover-job-001","owner":"session-leftover",` +
		`"command":"sleep 999","state":"queued","created_at":"2026-01-02T03:04:05Z","exit_code":-1}` + "\n"
	if err := os.WriteFile(journalPath, []byte(line), 0o600); err != nil {
		t.Fatalf("写入遗留日志: %v", err)
	}

	holder := &jobOwnerHolder{}
	mgr := newRuntimeJobManagerAt(holder, journalPath, filepath.Join(dir, "out"))
	t.Cleanup(mgr.Close)

	before, err := mgr.AllHistory()
	if err != nil {
		t.Fatalf("读历史: %v", err)
	}
	if len(before) != 1 || before[0].State != jobs.StateQueued {
		t.Fatalf("归并前应仍是非终态记录，得到 %+v", before)
	}

	bindRuntimeJobOwner(&stubSessionAgent{sessionID: owner}, holder, mgr)

	if got := holder.Get(); got != owner {
		t.Fatalf("归属回填失败: %q", got)
	}
	after, err := mgr.AllHistory()
	if err != nil {
		t.Fatalf("归并后读历史: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("归并不应新增记录，得到 %d 条", len(after))
	}
	if after[0].State != jobs.StateInterrupted {
		t.Fatalf("遗留任务应被标为 interrupted，得到 %q", after[0].State)
	}
	if !after[0].State.Terminal() {
		t.Fatal("interrupted 必须是终态")
	}
	if after[0].EndedAt.IsZero() {
		t.Fatal("归并应写上结束时间，否则用户看不出它是何时被断定的")
	}

	// 再跑一次归并应无事发生：已经是终态，不该被重复改写。
	n, err := mgr.ReconcileOwner(owner)
	if err != nil {
		t.Fatalf("重复归并报错: %v", err)
	}
	if n != 0 {
		t.Fatalf("重复归并应改写 0 条，实际 %d 条", n)
	}
}

// TestBindRuntimeJobOwner_IgnoresAgentWithoutSessionID 确认可选接口缺失时不 panic。
func TestBindRuntimeJobOwner_IgnoresAgentWithoutSessionID(t *testing.T) {
	dir := t.TempDir()
	holder := &jobOwnerHolder{}
	mgr := newRuntimeJobManagerAt(holder, filepath.Join(dir, "jobs.jsonl"), filepath.Join(dir, "out"))
	t.Cleanup(mgr.Close)

	bindRuntimeJobOwner(&plainAgent{}, holder, mgr)
	if got := holder.Get(); got != "" {
		t.Fatalf("没有会话 ID 时不该回填，得到 %q", got)
	}
}

// TestBindRuntimeJobOwner_OnlyTouchesOwnSession 确认归并不会波及别的会话的记录。
//
// 同机可能有另一个进程正在跑别的会话，它名下的非终态记录是活的，不能被动到。
func TestBindRuntimeJobOwner_OnlyTouchesOwnSession(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "jobs.jsonl")
	mk := func(id, owner, state string) string {
		return `{"kind":"started","job_id":"` + id + `","owner":"` + owner +
			`","command":"c","state":"` + state + `","created_at":"2026-01-02T03:04:05Z","exit_code":-1}` + "\n"
	}
	if err := os.WriteFile(journalPath, []byte(mk("a-1", "owner-a", "queued")+mk("b-1", "owner-b", "queued")), 0o600); err != nil {
		t.Fatalf("写入日志: %v", err)
	}

	holder := &jobOwnerHolder{}
	mgr := newRuntimeJobManagerAt(holder, journalPath, filepath.Join(dir, "out"))
	t.Cleanup(mgr.Close)

	bindRuntimeJobOwner(&stubSessionAgent{sessionID: "owner-a"}, holder, mgr)

	all, err := mgr.AllHistory()
	if err != nil {
		t.Fatalf("读历史: %v", err)
	}
	got := map[string]jobs.State{}
	for _, job := range all {
		got[job.ID] = job.State
	}
	if got["a-1"] != jobs.StateInterrupted {
		t.Fatalf("本会话记录应归并为 interrupted，得到 %q", got["a-1"])
	}
	if got["b-1"] != jobs.StateQueued {
		t.Fatalf("别的会话的记录不该被动，得到 %q", got["b-1"])
	}
}

// TestRestartReconcileEndToEnd 走真实路径解析，把"重启后看历史"整条链路串起来。
//
// 与前面的归并测试的区别：这里不注入任何路径，用 getSessionDir() 解析出的真实位置，
// 因此顺带验证了 jobJournalPath / jobOutputDir 的拼法，以及 CLI 渲染的是归并后的结果。
func TestRestartReconcileEndToEnd(t *testing.T) {
	// HOME 一改，getSessionDir() 就指向临时目录，不会碰到本机真实会话数据。
	t.Setenv("HOME", t.TempDir())

	const owner = "e2e-session-0001"
	journalPath := jobJournalPath()
	if err := os.MkdirAll(filepath.Dir(journalPath), 0o700); err != nil {
		t.Fatalf("创建会话目录: %v", err)
	}
	leftover := `{"kind":"started","job_id":"e2e-session-0001-abcd-job-001","owner":"e2e-session-0001",` +
		`"command":"make -j8 test","state":"queued","created_at":"2026-09-12T08:00:00Z","exit_code":-1}` + "\n"
	if err := os.WriteFile(journalPath, []byte(leftover), 0o600); err != nil {
		t.Fatalf("写入遗留日志: %v", err)
	}

	// 重启前：CLI 看到的是"非终态"，并明确说明无法确认是否仍在运行。
	var before bytes.Buffer
	if err := runJobsList(&before); err != nil {
		t.Fatalf("重启前 jobs list: %v", err)
	}
	if !containsAll(before.String(), "queued", "无法确认是否仍在运行") {
		t.Fatalf("重启前应看到非终态记录: %s", before.String())
	}

	// 重启：构造管理器 -> 绑定会话归属（内部触发归并）。
	holder := &jobOwnerHolder{}
	mgr := newRuntimeJobManager(holder)
	t.Cleanup(mgr.Close)
	bindRuntimeJobOwner(&stubSessionAgent{sessionID: owner}, holder, mgr)

	// 重启后：CLI 看到 interrupted，且没有新增任何 job（没重跑）。
	var after bytes.Buffer
	if err := runJobsList(&after); err != nil {
		t.Fatalf("重启后 jobs list: %v", err)
	}
	if !containsAll(after.String(), "interrupted", "make -j8 test") {
		t.Fatalf("重启后应显示 interrupted: %s", after.String())
	}
	if strings.Contains(after.String(), "处于非终态") {
		t.Fatalf("归并后不该再有非终态记录: %s", after.String())
	}
	if !strings.Contains(after.String(), "共 1 条记录") {
		t.Fatalf("归并只应改写状态，不该新增记录: %s", after.String())
	}

	// jobs show 的说明也要跟着变：从"无法确认"变成"无法接管"。
	var detail bytes.Buffer
	if err := runJobsShow(&detail, "e2e-session-0001-abcd-job-001"); err != nil {
		t.Fatalf("jobs show: %v", err)
	}
	if !containsAll(detail.String(), "状态: interrupted", "无法接管") {
		t.Fatalf("详情应说明是重启遗留: %s", detail.String())
	}
}

// ---------------------------------------------------------------- 清理

// TestRuntimeJobCleanup_StopsOwnerJobs 确认退出钩子真的结束了本会话的后台任务。
func TestRuntimeJobCleanup_StopsOwnerJobs(t *testing.T) {
	dir := t.TempDir()
	holder := &jobOwnerHolder{}
	holder.Set("session-x")
	mgr := newRuntimeJobManagerAt(holder, filepath.Join(dir, "jobs.jsonl"), filepath.Join(dir, "out"))

	started := make(chan struct{})
	_, err := mgr.Start("session-x", "blocker", func(ctx context.Context) (int, error) {
		close(started)
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	if err := runtimeJobCleanup(mgr, holder)(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if n := mgr.Active("session-x"); n != 0 {
		t.Fatalf("cleanup 后不该还有活动任务，得到 %d", n)
	}
}

// TestRuntimeJobCleanup_NilManagerIsSafe 确认没有管理器时钩子不炸。
func TestRuntimeJobCleanup_NilManagerIsSafe(t *testing.T) {
	if err := runtimeJobCleanup(nil, &jobOwnerHolder{})(); err != nil {
		t.Fatalf("nil manager: %v", err)
	}
}

// ---------------------------------------------------------------- CLI 只读性

// TestOpenJobsHistoryDoesNotCreateFile 确认"看一眼历史"不会在磁盘上留下空文件。
func TestOpenJobsHistoryDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	mgr, err := openJobsHistoryAt(path)
	if err != nil {
		t.Fatalf("openJobsHistoryAt: %v", err)
	}
	all, err := mgr.AllHistory()
	if err != nil {
		t.Fatalf("AllHistory: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("空日志不该有记录，得到 %d 条", len(all))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("只读查看不应创建日志文件，err=%v", err)
	}
}

// TestRenderJobsList_Empty 确认空记录的提示。
func TestRenderJobsList_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJobsList(&buf, nil, ""); err != nil {
		t.Fatalf("renderJobsList: %v", err)
	}
	if !strings.Contains(buf.String(), "没有后台任务记录") {
		t.Fatalf("提示不符: %s", buf.String())
	}
}

// TestRenderJobsList_MarksNonTerminalAsUnconfirmed 确认列表不会替已消失的进程背书。
func TestRenderJobsList_MarksNonTerminalAsUnconfirmed(t *testing.T) {
	all := []jobs.Job{
		{ID: "s1-job-001", Owner: "session-1", Command: "sleep 999", State: jobs.StateQueued, ExitCode: -1},
		{ID: "s1-job-002", Owner: "session-1", Command: "echo hi", State: jobs.StateSucceeded, ExitCode: 0},
	}
	var buf bytes.Buffer
	if err := renderJobsList(&buf, all, ""); err != nil {
		t.Fatalf("renderJobsList: %v", err)
	}
	out := buf.String()
	if !containsAll(out, "s1-job-001", "s1-job-002", "共 2 条记录") {
		t.Fatalf("列表内容缺项: %s", out)
	}
	if !containsAll(out, "1 条记录处于非终态", "无法确认是否仍在运行") {
		t.Fatalf("非终态记录必须带说明: %s", out)
	}
	if !strings.Contains(out, "不会重新执行任何命令") {
		t.Fatalf("必须声明不会重跑: %s", out)
	}
}

// TestRenderJobsList_OwnerFilter 确认 --owner 过滤与"该会话无记录"的措辞。
func TestRenderJobsList_OwnerFilter(t *testing.T) {
	all := []jobs.Job{
		{ID: "a-1", Owner: "owner-a", State: jobs.StateSucceeded, ExitCode: 0},
		{ID: "b-1", Owner: "owner-b", State: jobs.StateSucceeded, ExitCode: 0},
	}
	var buf bytes.Buffer
	if err := renderJobsList(&buf, all, "owner-a"); err != nil {
		t.Fatalf("renderJobsList: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "b-1") {
		t.Fatalf("过滤后不该出现别的会话: %s", out)
	}
	if !strings.Contains(out, "a-1") {
		t.Fatalf("过滤后应保留本会话: %s", out)
	}

	buf.Reset()
	if err := renderJobsList(&buf, all, "owner-c"); err != nil {
		t.Fatalf("renderJobsList: %v", err)
	}
	if !containsAll(buf.String(), "owner-c 没有后台任务记录", "共 2 条记录") {
		t.Fatalf("应说明是过滤结果而非完全无记录: %s", buf.String())
	}
}

// TestRenderJobDetail_InterruptedExplainsNoReplay 确认单条详情把 interrupted 说清楚。
func TestRenderJobDetail_InterruptedExplainsNoReplay(t *testing.T) {
	job := jobs.Job{
		ID:        "s-job-001",
		Owner:     "session-z",
		Command:   "make -j8 test",
		State:     jobs.StateInterrupted,
		ExitCode:  -1,
		Err:       "进程重启：无法接管上次运行中的任务",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		EndedAt:   time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC),
	}
	var buf bytes.Buffer
	if err := renderJobDetail(&buf, job); err != nil {
		t.Fatalf("renderJobDetail: %v", err)
	}
	out := buf.String()
	if !containsAll(out, "状态: interrupted", "无法接管", "不会重跑它") {
		t.Fatalf("interrupted 说明不完整: %s", out)
	}
	if strings.Contains(out, "退出码: -1") {
		t.Fatalf("退出码 -1 表示无意义，不该展示: %s", out)
	}
}

// TestRenderJobDetail_StaleNonTerminal 确认非终态记录不会被当成"正在运行"展示。
func TestRenderJobDetail_StaleNonTerminal(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJobDetail(&buf, jobs.Job{
		ID: "s-job-002", Owner: "session-z", Command: "sleep 999",
		State: jobs.StateQueued, ExitCode: -1,
	}); err != nil {
		t.Fatalf("renderJobDetail: %v", err)
	}
	if !containsAll(buf.String(), "无法确认它是否仍在运行", "也不会替它重跑命令") {
		t.Fatalf("非终态说明不完整: %s", buf.String())
	}
}

// TestRenderJobDetail_JournalErrIsVisible 确认"没有可恢复记录"是要被看见的事实。
func TestRenderJobDetail_JournalErrIsVisible(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJobDetail(&buf, jobs.Job{
		ID: "s-job-003", Owner: "session-z", Command: "echo hi",
		State: jobs.StateSucceeded, ExitCode: 0, JournalErr: "磁盘只读",
	}); err != nil {
		t.Fatalf("renderJobDetail: %v", err)
	}
	if !strings.Contains(buf.String(), "记录写入失败: 磁盘只读") {
		t.Fatalf("日志写入失败必须展示: %s", buf.String())
	}
}

// TestCommandSummary 确认命令摘要按字符截断而不是按字节。
func TestCommandSummary(t *testing.T) {
	if got := commandSummary("  a\tb\nc  "); got != "  a b c  " {
		t.Fatalf("应把空白压平: %q", got)
	}
	short := "echo hi"
	if got := commandSummary(short); got != short {
		t.Fatalf("短命令不该被改: %q", got)
	}
	// 50 个汉字：按字节截断会把最后一个字切成乱码，按字符截断不会。
	long := strings.Repeat("命", 50)
	got := commandSummary(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("超长命令应带省略号: %q", got)
	}
	if len([]rune(got)) != 49 {
		t.Fatalf("应保留 48 个字符再加省略号，得到 %d 个字符", len([]rune(got)))
	}
}

// TestJobsCmdIsRegistered 确认 CLI 入口挂上了 list/show，且默认只读。
func TestJobsCmdIsRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "jobs" {
			found = true
		}
	}
	if !found {
		t.Fatal("根命令应注册 jobs 子命令")
	}

	names := map[string]bool{}
	for _, c := range jobsCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"list", "show"} {
		if !names[want] {
			t.Fatalf("jobs 缺少子命令 %q", want)
		}
	}
	if jobsShowCmd.Args == nil {
		t.Fatal("jobs show 必须要求 job-id 参数")
	}
}
