package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRun_TerminationReasonMapping 逐档固定"取消 / 超时 / 正常非零退出 / 执行报错"的
// 终态与原因，防止它们被合并成一个模糊的"未成功"。
func TestRun_TerminationReasonMapping(t *testing.T) {
	clock := newMemClock()
	cases := []struct {
		name     string
		run      SinkRunner
		want     State
		wantExit int
		wantErr  string // 期望 Err 字段包含的片段；空表示必须为空
	}{
		{
			name:     "正常结束",
			run:      func(context.Context, io.Writer, io.Writer) (int, error) { return 0, nil },
			want:     StateSucceeded,
			wantExit: 0,
		},
		{
			name:     "正常非零退出",
			run:      func(context.Context, io.Writer, io.Writer) (int, error) { return 7, nil },
			want:     StateFailed,
			wantExit: 7,
			wantErr:  "退出码 7",
		},
		{
			name: "执行报错",
			run: func(context.Context, io.Writer, io.Writer) (int, error) {
				return -1, errors.New("启动失败：可执行文件不存在")
			},
			want:     StateFailed,
			wantExit: -1,
			wantErr:  "可执行文件不存在",
		},
		{
			name: "执行体自己的 deadline 到期",
			run: func(context.Context, io.Writer, io.Writer) (int, error) {
				return -1, fmt.Errorf("sh -c sleep 30: %w", context.DeadlineExceeded)
			},
			want:     StateTimedOut,
			wantExit: -1,
			wantErr:  "超时",
		},
		{
			name: "owner 取消",
			run: func(ctx context.Context, _ io.Writer, _ io.Writer) (int, error) {
				<-ctx.Done()
				return -1, ctx.Err()
			},
			want:     StateCanceled,
			wantExit: -1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr, _ := newTestManager(t, Options{Now: clock.Now})
			const owner = "sess-A"
			job, err := mgr.StartWithOutput(owner, tc.name, tc.run)
			if err != nil {
				t.Fatalf("启动失败: %v", err)
			}
			if tc.want == StateCanceled {
				if err := mgr.Cancel(owner, job.ID); err != nil {
					t.Fatalf("取消失败: %v", err)
				}
			}
			final, err := mgr.Await(context.Background(), owner, job.ID)
			if err != nil {
				t.Fatalf("等待终态失败: %v", err)
			}
			if final.State != tc.want {
				t.Fatalf("终态应为 %s，得到 %s（Err=%q）", tc.want, final.State, final.Err)
			}
			if !final.State.Terminal() {
				t.Fatalf("%s 应是终态", final.State)
			}
			if final.ExitCode != tc.wantExit {
				t.Fatalf("退出码应为 %d，得到 %d", tc.wantExit, final.ExitCode)
			}
			if tc.wantErr == "" {
				if final.Err != "" {
					t.Fatalf("该终态不应带原因，得到 %q", final.Err)
				}
			} else if !contains(final.Err, tc.wantErr) {
				t.Fatalf("原因应包含 %q，得到 %q", tc.wantErr, final.Err)
			}
		})
	}
}

// TestRun_CancelWinsOverDeadline 固定"取消与 deadline 同时发生"的时序：
// 人有意识的动作优先于计时器，原因必须记为取消。
func TestRun_CancelWinsOverDeadline(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{Now: clock.Now})
	const owner = "sess-A"

	// 执行体等到 ctx 结束（用户取消）后，返回 deadline 错误——
	// 模拟"取消信号到达时命令恰好因为自己的 deadline 结束"。
	job, err := mgr.StartWithOutput(owner, "race", func(ctx context.Context, _ io.Writer, _ io.Writer) (int, error) {
		<-ctx.Done()
		return -1, fmt.Errorf("wrap: %w", context.DeadlineExceeded)
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := mgr.Cancel(owner, job.ID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	final, err := mgr.Await(context.Background(), owner, job.ID)
	if err != nil {
		t.Fatalf("等待终态失败: %v", err)
	}
	if final.State != StateCanceled {
		t.Fatalf("同时发生时应记为取消，得到 %s（Err=%q）", final.State, final.Err)
	}
	if final.Err != "" {
		t.Fatalf("取消不应带失败原因，得到 %q", final.Err)
	}
}

// TestRun_NaturalExitAtCancelMomentKeepsObservedResult 固定另一半时序：
// 命令已经正常结束、随后取消才到达时，终态是"已发生的事实"，不是"取消"。
func TestRun_NaturalExitAtCancelMomentKeepsObservedResult(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{Now: clock.Now})
	const owner = "sess-A"

	job, err := mgr.StartWithOutput(owner, "fast", func(ctx context.Context, _ io.Writer, _ io.Writer) (int, error) {
		// 命令正常结束，不理会取消。
		return 0, nil
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	final, err := mgr.Await(context.Background(), owner, job.ID)
	if err != nil {
		t.Fatalf("等待终态失败: %v", err)
	}
	// 此时再取消，必须返回 ErrAlreadyTerminal，且状态不被改写。
	if err := mgr.Cancel(owner, job.ID); !errors.Is(err, ErrAlreadyTerminal) {
		t.Fatalf("终态后取消应返回 ErrAlreadyTerminal，得到 %v", err)
	}
	again, err := mgr.Get(owner, job.ID)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if again.State != StateSucceeded || final.State != StateSucceeded {
		t.Fatalf("已结束的命令不应被取消改写: %s / %s", final.State, again.State)
	}
}

// TestState_TimedOutIsTerminalAndDistinct 固定新增终态的语义。
func TestState_TimedOutIsTerminalAndDistinct(t *testing.T) {
	if !StateTimedOut.Terminal() {
		t.Fatal("timed_out 必须是终态")
	}
	if StateTimedOut == StateCanceled || StateTimedOut == StateFailed {
		t.Fatal("timed_out 必须与 canceled/failed 区分开")
	}
	if StateQueued.Terminal() || StateRunning.Terminal() {
		t.Fatal("queued/running 不是终态")
	}
}

// TestAwait_ReturnsImmediatelyOnTerminal 验证等待是基于终态通道而不是轮询：
// 已结束的 job 再 Await 必须立刻返回，且指向终态。
func TestAwait_ReturnsImmediatelyOnTerminal(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{Now: clock.Now})
	const owner = "sess-A"
	job, err := mgr.Start(owner, "x", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if _, err := mgr.Await(context.Background(), owner, job.ID); err != nil {
		t.Fatalf("首次等待失败: %v", err)
	}
	done := make(chan State, 1)
	go func() {
		final, _ := mgr.Await(context.Background(), owner, job.ID)
		done <- final.State
	}()
	select {
	case st := <-done:
		if st != StateSucceeded {
			t.Fatalf("终态应为 succeeded，得到 %s", st)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("对已结束的 job 重复 Await 应立即返回")
	}
}

// TestAwait_ContextTimeoutReturnsSnapshot 验证 ctx 超时时返回当前快照与 ctx 错误。
func TestAwait_ContextTimeoutReturnsSnapshot(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{Now: clock.Now})
	const owner = "sess-A"
	// 执行体不响应取消，用于制造"仍未终态"的快照。
	release := make(chan struct{})
	job, err := mgr.Start(owner, "stuck", func(context.Context) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	snapshot, err := mgr.Await(ctx, owner, job.ID)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应返回 ctx 错误，得到 %v", err)
	}
	if snapshot.ID != job.ID || snapshot.State.Terminal() {
		t.Fatalf("超时时应返回运行中的快照: %+v", snapshot)
	}
	close(release)
	if _, err := mgr.Await(context.Background(), owner, job.ID); err != nil {
		t.Fatalf("放行后等待失败: %v", err)
	}
}

// TestCloseOwner_CancelsAndCleansOnlyThatOwner 验证 owner 关闭清理：
// 只影响该 owner 的 job，取消后等待终态，并删除它的输出文件。
func TestCloseOwner_CancelsAndCleansOnlyThatOwner(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{
		Now:    clock.Now,
		Output: OutputOptions{MemoryLimit: 8, MaxBytes: 1 << 20},
	})

	// A：长命令 + 大输出，产生溢出文件。
	blockA := make(chan struct{})
	jobA, err := mgr.StartWithOutput("sess-A", "A 的长任务", func(ctx context.Context, out, _ io.Writer) (int, error) {
		out.Write([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-blockA:
			return 0, nil
		}
	})
	if err != nil {
		t.Fatalf("启动 A 失败: %v", err)
	}
	// B：同类任务，不应被波及。
	blockB := make(chan struct{})
	jobB, err := mgr.StartWithOutput("sess-B", "B 的长任务", func(ctx context.Context, out, _ io.Writer) (int, error) {
		out.Write([]byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-blockB:
			return 0, nil
		}
	})
	if err != nil {
		t.Fatalf("启动 B 失败: %v", err)
	}

	// 直接关掉 A 的命令（不通过 cancel、不经 Await），确保 CloseOwner 面对的是仍在运行的 job。
	closed := make(chan int, 1)
	go func() {
		closed <- mgr.CloseOwner("sess-A")
	}()

	var canceled int
	select {
	case canceled = <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseOwner 未在合理时间内返回")
	}
	if canceled != 1 {
		t.Fatalf("应取消 1 个 job，实际 %d", canceled)
	}

	finalA, err := mgr.Get("sess-A", jobA.ID)
	if err != nil {
		t.Fatalf("查询 A 失败: %v", err)
	}
	if finalA.State != StateCanceled {
		t.Fatalf("A 应被取消，得到 %s", finalA.State)
	}
	// A 的输出文件应已删除。快照字段在终态才填充，所以这里直接按约定路径检查磁盘。
	if got := len(finalA.OutputRefs); got != 1 {
		t.Fatalf("A 应有一个溢出文件引用，实际 %d", got)
	}
	for _, p := range finalA.OutputRefs {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("A 的输出文件应被清理: %s (%v)", p, err)
		}
	}
	// B 不受影响：仍在运行，输出仍可读。先读一次——读满 40 字节必须跨越
	// 内存/文件边界，因此它同时证明 B 的溢出文件已经落地。
	finalB, err := mgr.Get("sess-B", jobB.ID)
	if err != nil {
		t.Fatalf("查询 B 失败: %v", err)
	}
	if finalB.State.Terminal() {
		t.Fatalf("B 不应被 A 的关闭清理波及，得到 %s", finalB.State)
	}
	pageB, err := mgr.ReadOutput("sess-B", jobB.ID, StreamStdout, 0, 4096)
	if err != nil {
		t.Fatalf("B 的输出应仍可读: %v", err)
	}
	if pageB.TotalSize != 40 {
		t.Fatalf("B 的输出不应丢失: %d", pageB.TotalSize)
	}
	// B 的溢出文件仍然存在（同根目录、不同 owner 目录）。
	refB := filepath.Join(mgr.OutputDir(), ownerDirName("sess-B"), jobB.ID+".stdout.out")
	if _, err := os.Stat(refB); err != nil {
		t.Fatalf("B 的输出文件不应被删除: %v", err)
	}
	close(blockB)
	if _, err := mgr.Await(context.Background(), "sess-B", jobB.ID); err != nil {
		t.Fatalf("等待 B 失败: %v", err)
	}
	close(blockA)
}

// TestCloseOwner_EmptyOwnerIsNoop 验证空 owner 不做任何事（否则会变成"关闭所有人"）。
func TestCloseOwner_EmptyOwnerIsNoop(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{Now: clock.Now})
	job, err := mgr.Start("sess-A", "x", func(context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if n := mgr.CloseOwner(""); n != 0 {
		t.Fatalf("空 owner 应返回 0，得到 %d", n)
	}
	if mgr.Len() != 1 {
		t.Fatalf("空 owner 不应动任何 job，实际 %d", mgr.Len())
	}
	if _, err := mgr.Await(context.Background(), "sess-A", job.ID); err != nil {
		t.Fatalf("等待失败: %v", err)
	}
}

// TestCloseOwner_WaitsForOutputToBeFlushed 验证 CloseOwner 先等执行体返回再删文件：
// 反过来会让"命令成功了但输出丢了"。
func TestCloseOwner_WaitsForOutputToBeFlushed(t *testing.T) {
	clock := newMemClock()
	mgr, _ := newTestManager(t, Options{
		Now:    clock.Now,
		Output: OutputOptions{MemoryLimit: 4, MaxBytes: 1 << 20},
	})
	const owner = "sess-A"

	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = 'w'
	}
	job, err := mgr.StartWithOutput(owner, "慢写", func(ctx context.Context, out, _ io.Writer) (int, error) {
		// 分两次写；中间睡一下，制造"CloseOwner 已经发出取消、但执行体还有一次写没做"的窗口。
		out.Write(payload[:2048])
		time.Sleep(150 * time.Millisecond)
		out.Write(payload[2048:])
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	spillPath := filepath.Join(mgr.OutputDir(), ownerDirName(owner), job.ID+".stdout.out")
	waitForPathExists(t, spillPath)

	closed := make(chan int, 1)
	go func() { closed <- mgr.CloseOwner(owner) }()

	// 执行体还在睡（至少 150ms），这段窗口里文件必须仍然存在：
	// 若 CloseOwner 先删文件再等执行体，这里就会看到文件消失。
	window := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(window) {
		if _, err := os.Stat(spillPath); err != nil {
			t.Fatalf("执行体尚未返回，输出文件就被删除了: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	var canceled int
	select {
	case canceled = <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseOwner 未在合理时间内返回")
	}
	if canceled != 1 {
		t.Fatalf("应取消 1 个 job，实际 %d", canceled)
	}

	final, err := mgr.Await(context.Background(), owner, job.ID)
	if err != nil {
		t.Fatalf("等待失败: %v", err)
	}
	if final.State != StateCanceled {
		t.Fatalf("应记为取消，得到 %s", final.State)
	}
	if final.StdoutBytes != int64(len(payload)) {
		t.Fatalf("两段写入都必须落账: got %d want %d", final.StdoutBytes, len(payload))
	}
	if final.OutputTruncated {
		t.Fatalf("不应截断: %+v", final)
	}
	if _, err := os.Stat(spillPath); !os.IsNotExist(err) {
		t.Fatalf("关闭后输出文件应被删除: %v", err)
	}
	if _, err := mgr.ReadOutput(owner, job.ID, StreamStdout, 0, 16); !errors.Is(err, ErrOutputClosed) {
		t.Fatalf("关闭后输出应不可读，得到 %v", err)
	}
	// 目录里不应残留任何文件。
	dir := filepath.Join(mgr.OutputDir(), ownerDirName(owner))
	if entries, err := os.ReadDir(dir); err == nil && len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("owner 关闭后不应残留输出文件: %v", names)
	}
}

// waitForPathExists 等待路径出现，超时即失败。
func waitForPathExists(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待文件出现超时: %s", path)
}

// contains 是 strings.Contains 的本地替身，避免为一个断言引入 import。
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
