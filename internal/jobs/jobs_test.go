package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// recorder 收集终态回调，用于断言"恰好一次"。
type recorder struct {
	mu     sync.Mutex
	events []Job
}

func (r *recorder) add(j Job) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, j)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *recorder) countOf(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.ID == id {
			n++
		}
	}
	return n
}

// newTestManager 返回一个可注入时钟与回调的 Manager。
//
// 输出目录默认落在 t.TempDir()：测试不该往用户的缓存目录里写溢出文件。
func newTestManager(t *testing.T, opts Options) (*Manager, *recorder) {
	t.Helper()
	rec := &recorder{}
	if opts.OnTerminal == nil {
		opts.OnTerminal = rec.add
	}
	if opts.Output.Dir == "" {
		opts.Output.Dir = t.TempDir()
	}
	m := New(opts)
	t.Cleanup(m.Close)
	return m, rec
}

// TestStart_ValidationRejectsBeforeRegistering 确认启动失败有明确返回，
// 且被拒绝时不会留下半个 job。
func TestStart_ValidationRejectsBeforeRegistering(t *testing.T) {
	m, _ := newTestManager(t, Options{})

	cases := []struct {
		name    string
		owner   string
		command string
		run     Runner
		want    error
	}{
		{"缺少 owner", "", "echo hi", func(context.Context) (int, error) { return 0, nil }, ErrMissingOwner},
		{"空命令", "s1", "", func(context.Context) (int, error) { return 0, nil }, ErrEmptyCommand},
		{"缺少执行体", "s1", "echo hi", nil, ErrNilRunner},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job, err := m.Start(tc.owner, tc.command, tc.run)
			if !errors.Is(err, tc.want) {
				t.Fatalf("期望 %v，得到 %v", tc.want, err)
			}
			if job != nil {
				t.Errorf("被拒绝时不应返回 job，得到 %+v", job)
			}
		})
	}
	if m.Len() != 0 {
		t.Fatalf("被拒绝的启动不应登记 job，实际登记 %d 个", m.Len())
	}
}

// TestOwnership_OtherSessionCannotReadOrCancel 是本任务的核心验收之一。
func TestOwnership_OtherSessionCannotReadOrCancel(t *testing.T) {
	release := make(chan struct{})
	m, _ := newTestManager(t, Options{})

	job, err := m.Start("session-a", "sleep", func(ctx context.Context) (int, error) {
		select {
		case <-release:
			return 0, nil
		case <-ctx.Done():
			return -1, ctx.Err()
		}
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 本人可读
	if _, err := m.Get("session-a", job.ID); err != nil {
		t.Fatalf("owner 自己应当能读取: %v", err)
	}

	// 他人不可读：错误必须与"不存在"完全一致，避免探测
	if _, err := m.Get("session-b", job.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("其他会话读取应返回 ErrNotFound，得到 %v", err)
	}

	// 他人不可取消
	if err := m.Cancel("session-b", job.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("其他会话取消应返回 ErrNotFound，得到 %v", err)
	}
	// 确认取消真的没生效：job 仍在运行
	if got := m.Active("session-a"); got != 1 {
		t.Errorf("越权取消不应生效，Active=%d", got)
	}
	// 他人的 List / CancelOwner 也不应包含它
	if len(m.List("session-b")) != 0 {
		t.Error("其他会话的 List 不应包含该 job")
	}
	if n := m.CancelOwner("session-b"); n != 0 {
		t.Errorf("CancelOwner 不应影响其他会话，返回 %d", n)
	}

	close(release)
	final, err := m.Await(context.Background(), "session-a", job.ID)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if final.State != StateSucceeded {
		t.Errorf("期望 succeeded，得到 %s", final.State)
	}
}

// TestSettle_TerminalPublishedExactlyOnce 验证重复结束不会重复发终态。
func TestSettle_TerminalPublishedExactlyOnce(t *testing.T) {
	m, rec := newTestManager(t, Options{})

	job, err := m.Start("s1", "quick", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Await(context.Background(), "s1", job.ID); err != nil {
		t.Fatalf("Await: %v", err)
	}

	// 终态之后重复取消：明确报"已处于终态"，且不产生第二个终态。
	if err := m.Cancel("s1", job.ID); !errors.Is(err, ErrAlreadyTerminal) {
		t.Errorf("终态后取消应返回 ErrAlreadyTerminal，得到 %v", err)
	}
	if err := m.Cancel("s1", job.ID); !errors.Is(err, ErrAlreadyTerminal) {
		t.Errorf("第二次取消仍应明确报错，得到 %v", err)
	}
	if n := rec.countOf(job.ID); n != 1 {
		t.Fatalf("终态应恰好发布一次，实际 %d 次", n)
	}
}

// TestSettle_CancelRacingNaturalExit 验证"取消与正常结束同时发生"时也只发布一次终态，
// 且状态不会在两个写点之间来回。
func TestSettle_CancelRacingNaturalExit(t *testing.T) {
	const rounds = 200
	for i := 0; i < rounds; i++ {
		m, rec := newTestManager(t, Options{})
		started := make(chan struct{})

		job, err := m.Start("s1", "race", func(ctx context.Context) (int, error) {
			close(started)
			// 让执行体有机会在取消之前就结束
			select {
			case <-ctx.Done():
				return -1, ctx.Err()
			default:
				return 0, nil
			}
		})
		if err != nil {
			t.Fatalf("第 %d 轮 Start: %v", i, err)
		}
		<-started

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = m.Cancel("s1", job.ID) }()
		go func() { defer wg.Done(); m.Wait() }()
		wg.Wait()

		final, err := m.Get("s1", job.ID)
		if err != nil {
			t.Fatalf("第 %d 轮 Get: %v", i, err)
		}
		if !final.State.Terminal() {
			t.Fatalf("第 %d 轮结束状态不是终态: %s", i, final.State)
		}
		if n := rec.countOf(job.ID); n != 1 {
			t.Fatalf("第 %d 轮终态发布 %d 次，应为 1 次（状态 %s）", i, n, final.State)
		}
	}
}

// TestLimits_ExplicitRejection 验证并发数量上限有明确返回。
func TestLimits_ExplicitRejection(t *testing.T) {
	release := make(chan struct{})
	m, _ := newTestManager(t, Options{MaxConcurrent: 2, MaxPerOwner: 1})

	blocked := func(ctx context.Context) (int, error) {
		select {
		case <-release:
			return 0, nil
		case <-ctx.Done():
			return -1, ctx.Err()
		}
	}

	j1, err := m.Start("s1", "a", blocked)
	if err != nil {
		t.Fatalf("第一个 job 应被接受: %v", err)
	}

	// 同一 owner 超过单 owner 上限
	if _, err := m.Start("s1", "b", blocked); !errors.Is(err, ErrConcurrencyLimit) {
		t.Fatalf("应因单 owner 上限被拒绝，得到 %v", err)
	}

	// 另一个 owner 占满全局额度
	j2, err := m.Start("s2", "c", blocked)
	if err != nil {
		t.Fatalf("第二个 owner 的第一个 job 应被接受: %v", err)
	}
	// 全局额度已满
	if _, err := m.Start("s3", "d", blocked); !errors.Is(err, ErrConcurrencyLimit) {
		t.Fatalf("应因全局上限被拒绝，得到 %v", err)
	}

	// 被拒绝的启动不占用额度、不登记
	if m.Len() != 2 {
		t.Errorf("只应登记 2 个 job，实际 %d", m.Len())
	}

	// 释放一个额度后可以继续启动
	if err := m.Cancel("s1", j1.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := m.Await(context.Background(), "s1", j1.ID); err != nil {
		t.Fatalf("Await: %v", err)
	}
	if _, err := m.Start("s3", "e", blocked); err != nil {
		t.Fatalf("额度释放后应可启动: %v", err)
	}

	close(release)
	_ = m.CancelOwner("s2")
	m.Wait()
	_ = j2
}

// TestCancel_MarksCanceledNotFailed 验证取消的原因不会被写成失败。
func TestCancel_MarksCanceledNotFailed(t *testing.T) {
	m, _ := newTestManager(t, Options{})
	job, err := m.Start("s1", "long", func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Cancel("s1", job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	final, err := m.Await(context.Background(), "s1", job.ID)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if final.State != StateCanceled {
		t.Fatalf("期望 canceled，得到 %s（Err=%q）", final.State, final.Err)
	}
	if final.Err != "" {
		t.Errorf("取消不应带失败原因，得到 %q", final.Err)
	}
	if final.ExitCode != -1 {
		t.Errorf("取消的退出码应为 -1，得到 %d", final.ExitCode)
	}
}

// TestRun_FailureAndNonZeroExit 验证失败的两种来源都被记为 failed 且带原因。
func TestRun_FailureAndNonZeroExit(t *testing.T) {
	m, _ := newTestManager(t, Options{})

	// 执行体报错
	j1, err := m.Start("s1", "boom", func(context.Context) (int, error) {
		return -1, errors.New("启动失败：找不到可执行文件")
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	f1, _ := m.Await(context.Background(), "s1", j1.ID)
	if f1.State != StateFailed || f1.Err == "" {
		t.Errorf("执行体报错应记为 failed 并带原因，得到 %s / %q", f1.State, f1.Err)
	}

	// 非零退出码
	j2, err := m.Start("s1", "exit 3", func(context.Context) (int, error) { return 3, nil })
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	f2, _ := m.Await(context.Background(), "s1", j2.ID)
	if f2.State != StateFailed || f2.ExitCode != 3 {
		t.Errorf("非零退出码应记为 failed/3，得到 %s/%d", f2.State, f2.ExitCode)
	}
	if f2.Err == "" {
		t.Error("非零退出码应带可读原因")
	}
}

// TestState_TerminalClassification 验证终态分类，含 interrupted。
func TestState_TerminalClassification(t *testing.T) {
	terminal := []State{StateSucceeded, StateFailed, StateCanceled, StateInterrupted}
	for _, s := range terminal {
		if !s.Terminal() {
			t.Errorf("%s 应为终态", s)
		}
	}
	for _, s := range []State{StateQueued, StateRunning} {
		if s.Terminal() {
			t.Errorf("%s 不应为终态", s)
		}
	}
}

// TestList_OnlyOwnJobsInOrder 验证 List 只返回自己的 job 且顺序稳定。
func TestList_OnlyOwnJobsInOrder(t *testing.T) {
	m, _ := newTestManager(t, Options{})
	var ids []string
	for i := 0; i < 3; i++ {
		j, err := m.Start("s1", fmt.Sprintf("cmd-%d", i), func(context.Context) (int, error) { return 0, nil })
		if err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
		ids = append(ids, j.ID)
	}
	if _, err := m.Start("s2", "other", func(context.Context) (int, error) { return 0, nil }); err != nil {
		t.Fatalf("Start other: %v", err)
	}
	m.Wait()

	list := m.List("s1")
	if len(list) != 3 {
		t.Fatalf("期望 3 个 job，得到 %d", len(list))
	}
	for i, j := range list {
		if j.ID != ids[i] {
			t.Errorf("第 %d 个应为 %s，得到 %s", i, ids[i], j.ID)
		}
		if j.Owner != "s1" {
			t.Errorf("List 输出的 owner 应为 s1，得到 %s", j.Owner)
		}
	}
	if len(m.List("nobody")) != 0 {
		t.Error("无 job 的 owner 应返回空列表")
	}
}

// TestConcurrent_StartCancelAcrossOwners 用较多并发触发竞态检测（配合 -race）。
func TestConcurrent_StartCancelAcrossOwners(t *testing.T) {
	m, rec := newTestManager(t, Options{MaxConcurrent: 0})

	const owners, perOwner = 8, 25
	var wg sync.WaitGroup
	for o := 0; o < owners; o++ {
		owner := fmt.Sprintf("owner-%d", o)
		for i := 0; i < perOwner; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				job, err := m.Start(owner, "work", func(ctx context.Context) (int, error) {
					select {
					case <-time.After(time.Millisecond):
						return 0, nil
					case <-ctx.Done():
						return -1, ctx.Err()
					}
				})
				if err != nil {
					return
				}
				_ = m.Cancel(owner, job.ID)
				_, _ = m.Get(owner, job.ID)
				_ = m.List(owner)
				_ = m.Active(owner)
			}()
		}
	}
	wg.Wait()
	m.Wait()

	total := owners * perOwner
	if got := len(rec.events); got != total {
		t.Fatalf("每个 job 应恰好发布一次终态，期望 %d，实际 %d", total, got)
	}
	seen := make(map[string]int, total)
	for _, e := range rec.events {
		seen[e.ID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("job %s 发布了 %d 次终态", id, n)
		}
	}
	if len(seen) != total {
		t.Errorf("终态事件应覆盖全部 %d 个 job，实际 %d", total, len(seen))
	}
}

// TestIDsAreUnique 验证默认 ID 生成器不重复。
func TestIDsAreUnique(t *testing.T) {
	m, _ := newTestManager(t, Options{})
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		j, err := m.Start("s1", "x", func(context.Context) (int, error) { return 0, nil })
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if seen[j.ID] {
			t.Fatalf("ID 重复: %s", j.ID)
		}
		seen[j.ID] = true
	}
	m.Wait()
}
