package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------- 测试替身

// memJournal 是内存日志，用于断言"写了什么"以及注入写入失败。
type memJournal struct {
	mu sync.Mutex
	// failAt 里的调用序号（从 1 起）会返回错误。
	failAt  map[int]bool
	calls   int
	records []Record
}

func newMemJournal(failAt ...int) *memJournal {
	j := &memJournal{failAt: map[int]bool{}}
	for _, n := range failAt {
		j.failAt[n] = true
	}
	return j
}

func (j *memJournal) Append(rec Record) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.calls++
	if j.failAt[j.calls] {
		return fmt.Errorf("%w: 注入失败（第 %d 次）", ErrJournal, j.calls)
	}
	if rec.RecordedAt.IsZero() {
		rec.RecordedAt = time.Now()
	}
	j.records = append(j.records, rec)
	return nil
}

func (j *memJournal) Records() ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Record, len(j.records))
	copy(out, j.records)
	return out, nil
}

func (j *memJournal) kinds() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]string, 0, len(j.records))
	for _, r := range j.records {
		out = append(out, r.Kind)
	}
	return out
}

func (j *memJournal) last() Record {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.records) == 0 {
		return Record{}
	}
	return j.records[len(j.records)-1]
}

// ---------------------------------------------------------------- JSONL 往返

func TestJSONLJournal_AppendThenRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "jobs.jsonl")
	j := NewJSONLJournal(path)

	if recs, err := j.Records(); err != nil || len(recs) != 0 {
		t.Fatalf("文件不存在时应返回空且无错误，得到 %v / %v", recs, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("只读不应创建文件，err=%v", err)
	}

	want := []Record{
		{Kind: RecordStarted, JobID: "j1", Owner: "o1", Command: "echo hi", State: StateQueued, CreatedAt: time.Unix(100, 0).UTC(), ExitCode: -1},
		{Kind: RecordTerminal, JobID: "j1", Owner: "o1", Command: "echo hi", State: StateSucceeded, CreatedAt: time.Unix(100, 0).UTC(), EndedAt: time.Unix(101, 0).UTC(), ExitCode: 0},
	}
	for _, rec := range want {
		if err := j.Append(rec); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := j.Records()
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 条记录，得到 %d 条", len(got))
	}
	if got[0].Kind != RecordStarted || got[1].Kind != RecordTerminal {
		t.Fatalf("记录顺序应保持写入顺序: %+v", got)
	}
	if got[1].State != StateSucceeded || got[1].ExitCode != 0 {
		t.Fatalf("终态记录字段不对: %+v", got[1])
	}
	if got[0].RecordedAt.IsZero() {
		t.Fatal("Append 应补上 RecordedAt")
	}

	// 文件权限：日志可能含命令原文，不该是全局可读。
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("日志权限应为 0600，得到 %o", perm)
	}
}

// TestJSONLJournal_ToleratesTruncatedLastLine 确认崩溃只损失最后一行。
func TestJSONLJournal_ToleratesTruncatedLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	good := `{"kind":"started","job_id":"j1","owner":"o1","command":"echo hi","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`
	// 第二行写到一半被杀：没有换行、JSON 不完整。
	truncated := `{"kind":"terminal","job_id":"j1","owner":"o1","com`
	if err := os.WriteFile(path, []byte(good+"\n"+truncated), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	j := NewJSONLJournal(path)
	recs, err := j.Records()
	if err != nil {
		t.Fatalf("残缺最后一行应被容忍，得到错误: %v", err)
	}
	if len(recs) != 1 || recs[0].JobID != "j1" {
		t.Fatalf("应保留完好的第一行，得到 %+v", recs)
	}
	if j.Skipped() != 1 {
		t.Fatalf("应记录跳过 1 行，得到 %d", j.Skipped())
	}
}

// TestJSONLJournal_MiddleCorruptionIsError 确认中间行损坏必须报错。
//
// 静默跳过中间损坏行会让"日志被截断或被外部改写"看起来像"一切正常"，
// 那比直接报错危险得多。
func TestJSONLJournal_MiddleCorruptionIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	good := `{"kind":"started","job_id":"j1","owner":"o1","command":"echo hi","state":"queued","created_at":"2026-01-01T00:00:00Z","exit_code":-1}`
	content := good + "\n" + "这不是 JSON\n" + good + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	j := NewJSONLJournal(path)
	if _, err := j.Records(); err == nil {
		t.Fatal("中间行损坏必须返回错误")
	} else if !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("错误应指出损坏行号: %v", err)
	}
}

// ---------------------------------------------------------------- 启动即落记录

// TestStart_WritesStartedRecordBeforeRunner 确认 started 记录在命令跑起来之前就已落盘。
//
// 顺序很重要：如果先跑命令再落记录，进程在两者之间被杀，磁盘上就完全没有这条任务，
// 用户会以为命令没执行过。
func TestStart_WritesStartedRecordBeforeRunner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	m, _ := newTestManager(t, Options{Journal: NewJSONLJournal(path)})

	observed := make(chan int, 1)
	job, err := m.Start("owner-1", "echo hi", func(context.Context) (int, error) {
		recs, readErr := NewJSONLJournal(path).Records()
		if readErr != nil {
			observed <- -1
			return 1, readErr
		}
		observed <- len(recs)
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Await(context.Background(), "owner-1", job.ID); err != nil {
		t.Fatalf("Await: %v", err)
	}
	if n := <-observed; n != 1 {
		t.Fatalf("命令执行时磁盘上应已有 1 条 started 记录，实际 %d 条", n)
	}

	recs, err := NewJSONLJournal(path).Records()
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("结束后应有 started + terminal 两条记录，实际 %d 条", len(recs))
	}
	if recs[0].Owner != "owner-1" || recs[0].Command != "echo hi" {
		t.Fatalf("started 记录应带上归属与命令摘要: %+v", recs[0])
	}
}

// TestStart_RejectsWhenJournalUnwritable 确认记录写不进去就拒绝启动。
//
// 一条"没有可恢复记录"的后台命令，正是重启后最危险的形态：用户无法知道它跑没跑过。
func TestStart_RejectsWhenJournalUnwritable(t *testing.T) {
	j := newMemJournal(1) // 第一次 Append（started）就失败
	m, _ := newTestManager(t, Options{Journal: j})

	ran := false
	_, err := m.Start("owner-1", "echo hi", func(context.Context) (int, error) {
		ran = true
		return 0, nil
	})
	if err == nil {
		t.Fatal("落记录失败时必须拒绝启动")
	}
	if !errors.Is(err, ErrJournal) {
		t.Fatalf("错误应可判定为 ErrJournal，得到 %v", err)
	}
	if ran {
		t.Fatal("被拒绝的任务不该被执行")
	}
	if m.Len() != 0 {
		t.Fatalf("被拒绝后不该留下半个 job，实际登记 %d 个", m.Len())
	}
}

// TestSettle_WritesTerminalRecord 确认终态被持久化，含退出码与输出字节数。
func TestSettle_WritesTerminalRecord(t *testing.T) {
	j := newMemJournal()
	m, _ := newTestManager(t, Options{Journal: j})

	job, err := m.StartWithOutput("owner-1", "echo hi", func(_ context.Context, stdout, _ io.Writer) (int, error) {
		_, _ = io.WriteString(stdout, "hello\n")
		return 3, nil
	})
	if err != nil {
		t.Fatalf("StartWithOutput: %v", err)
	}
	final, err := m.Await(context.Background(), "owner-1", job.ID)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if final.State != StateFailed || final.ExitCode != 3 {
		t.Fatalf("预期 failed/3，得到 %s/%d", final.State, final.ExitCode)
	}
	if final.JournalErr != "" {
		t.Fatalf("日志写入不该失败: %s", final.JournalErr)
	}

	if kinds := j.kinds(); len(kinds) != 2 || kinds[0] != RecordStarted || kinds[1] != RecordTerminal {
		t.Fatalf("记录序列应为 started→terminal，得到 %v", kinds)
	}
	last := j.last()
	if last.State != StateFailed || last.ExitCode != 3 {
		t.Fatalf("终态记录字段不对: %+v", last)
	}
	if last.StdoutBytes != int64(len("hello\n")) {
		t.Fatalf("终态记录应带上输出字节数，得到 %d", last.StdoutBytes)
	}
	if last.EndedAt.IsZero() {
		t.Fatal("终态记录应带上结束时间")
	}
}

// TestSettle_RecordsJournalErrOnTerminalFailure 确认终态记录写失败不会掩盖状态，
// 但会作为可见事实留在快照上。
func TestSettle_RecordsJournalErrOnTerminalFailure(t *testing.T) {
	j := newMemJournal(2) // started 成功，terminal 失败
	m, _ := newTestManager(t, Options{Journal: j})

	job, err := m.Start("owner-1", "echo hi", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	final, err := m.Await(context.Background(), "owner-1", job.ID)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if final.State != StateSucceeded {
		t.Fatalf("终态记录写失败不该改变状态，得到 %s", final.State)
	}
	if final.JournalErr == "" {
		t.Fatal("终态记录写失败必须留在快照上")
	}
	if !strings.Contains(final.JournalErr, "注入失败") {
		t.Fatalf("应保留原始原因: %s", final.JournalErr)
	}
}

// TestNoJournalKeepsWorking 确认不配日志时行为不变（纯内存运行）。
func TestNoJournalKeepsWorking(t *testing.T) {
	m, _ := newTestManager(t, Options{})
	job, err := m.Start("owner-1", "echo hi", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Await(context.Background(), "owner-1", job.ID); err != nil {
		t.Fatalf("Await: %v", err)
	}
	all, err := m.AllHistory()
	if err != nil {
		t.Fatalf("AllHistory: %v", err)
	}
	if all != nil {
		t.Fatalf("无日志时应返回空历史，得到 %+v", all)
	}
	if n, err := m.ReconcileOwner("owner-1"); err != nil || n != 0 {
		t.Fatalf("无日志时归并应为 (0, nil)，得到 (%d, %v)", n, err)
	}
}

// ---------------------------------------------------------------- 折叠与查询

// TestFoldRecords_LastRecordWins 确认同一 job 的多条记录以后者为准。
func TestFoldRecords_LastRecordWins(t *testing.T) {
	created := time.Unix(1000, 0).UTC()
	recs := []Record{
		{Kind: RecordStarted, JobID: "j1", Owner: "o1", Command: "c1", State: StateQueued, CreatedAt: created, ExitCode: -1},
		{Kind: RecordTerminal, JobID: "j1", Owner: "o1", Command: "c1", State: StateSucceeded, CreatedAt: created, EndedAt: created.Add(time.Second), ExitCode: 0},
		// 后续进程对同一 ID 的归并记录必须覆盖前面的终态。
		{Kind: RecordTerminal, JobID: "j1", Owner: "o1", Command: "c1", State: StateInterrupted, CreatedAt: created, EndedAt: created.Add(2 * time.Second), ExitCode: -1, Err: "重启"},
	}
	got := foldRecords(recs)
	if len(got) != 1 {
		t.Fatalf("同一 ID 应折叠成一条，得到 %d 条", len(got))
	}
	if got[0].State != StateInterrupted || got[0].Err != "重启" {
		t.Fatalf("应以后出现的记录为准: %+v", got[0])
	}
}

// TestFoldRecords_StartedDoesNotOverwriteTerminal 确认迟到的 started 不会把终态改回 queued。
func TestFoldRecords_StartedDoesNotOverwriteTerminal(t *testing.T) {
	created := time.Unix(1000, 0).UTC()
	recs := []Record{
		{Kind: RecordTerminal, JobID: "j1", Owner: "o1", State: StateSucceeded, CreatedAt: created, ExitCode: 0},
		{Kind: RecordStarted, JobID: "j1", Owner: "o1", State: StateQueued, CreatedAt: created, ExitCode: -1},
	}
	got := foldRecords(recs)
	if len(got) != 1 || got[0].State != StateSucceeded {
		t.Fatalf("终态不该被 started 覆盖: %+v", got)
	}
}

// TestHistory_FiltersByOwner 确认历史查询按归属隔离。
func TestHistory_FiltersByOwner(t *testing.T) {
	j := newMemJournal()
	m, _ := newTestManager(t, Options{Journal: j})

	for _, owner := range []string{"o1", "o2", "o1"} {
		job, err := m.Start(owner, "echo "+owner, func(context.Context) (int, error) { return 0, nil })
		if err != nil {
			t.Fatalf("Start(%s): %v", owner, err)
		}
		if _, err := m.Await(context.Background(), owner, job.ID); err != nil {
			t.Fatalf("Await(%s): %v", owner, err)
		}
	}

	o1, err := m.History("o1")
	if err != nil {
		t.Fatalf("History(o1): %v", err)
	}
	if len(o1) != 2 {
		t.Fatalf("o1 应有 2 条，得到 %d 条", len(o1))
	}
	for _, job := range o1 {
		if job.Owner != "o1" {
			t.Fatalf("越权返回了别的 owner: %+v", job)
		}
	}

	if _, err := m.History(""); !errors.Is(err, ErrMissingOwner) {
		t.Fatalf("空 owner 应返回 ErrMissingOwner，得到 %v", err)
	}
}

// TestReconcileOwner_FlipsNonTerminalOnly 确认归并的精确边界。
func TestReconcileOwner_FlipsNonTerminalOnly(t *testing.T) {
	j := newMemJournal()
	m, _ := newTestManager(t, Options{Journal: j})

	// 已终态的任务：归并不该动它。
	done, err := m.Start("o1", "done", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Await(context.Background(), "o1", done.ID); err != nil {
		t.Fatalf("Await: %v", err)
	}
	// 未终态的任务：模拟上次进程留下的。
	blocked, err := m.Start("o1", "blocked", func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 别的 owner 的未终态任务：归并不该动它。
	other, err := m.Start("o2", "other", func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		m.CancelOwner("o1")
		m.CancelOwner("o2")
		m.Wait()
	})
	_ = blocked
	_ = other

	// 归并会往日志里追加终态记录，但内存态不由它改写（它管的是"下次读到什么"）。
	n, err := m.ReconcileOwner("o1")
	if err != nil {
		t.Fatalf("ReconcileOwner: %v", err)
	}
	if n != 1 {
		t.Fatalf("只该归并 o1 的那 1 条未终态记录，实际 %d 条", n)
	}
	last := j.last()
	if last.JobID != blocked.ID || last.State != StateInterrupted {
		t.Fatalf("归并记录不对: %+v", last)
	}
	if last.Err == "" {
		t.Fatal("归并应写明原因，而不是留一个空的 interrupted")
	}

	// 折叠后的视图：o1 的未终态记录变成 interrupted，其余不受影响。
	folded := foldRecords(j.mustRecords(t))
	states := map[string]State{}
	for _, job := range folded {
		states[job.ID] = job.State
	}
	if states[done.ID] != StateSucceeded {
		t.Fatalf("已终态记录不该被改写: %q", states[done.ID])
	}
	if states[blocked.ID] != StateInterrupted {
		t.Fatalf("未终态记录应变成 interrupted: %q", states[blocked.ID])
	}
	if states[other.ID] != StateQueued {
		t.Fatalf("别的 owner 的记录不该被动: %q", states[other.ID])
	}
}

// TestReconcileOwner_EmptyOwnerRejected 确认归并同样要求归属。
func TestReconcileOwner_EmptyOwnerRejected(t *testing.T) {
	m, _ := newTestManager(t, Options{Journal: newMemJournal()})
	if _, err := m.ReconcileOwner(""); !errors.Is(err, ErrMissingOwner) {
		t.Fatalf("应返回 ErrMissingOwner，得到 %v", err)
	}
}

// mustRecords 读日志，读不到就让测试失败。
func (j *memJournal) mustRecords(t *testing.T) []Record {
	t.Helper()
	recs, err := j.Records()
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	return recs
}
