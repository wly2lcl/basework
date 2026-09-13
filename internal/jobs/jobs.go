// Package jobs 跟踪后台执行单元的状态与归属。
//
// 这个包只回答三个问题，其余（输出分页、进程树清理、持久化）留给后续任务：
//
//  1. 这个 job 现在是什么状态？
//  2. 谁有权看它、取消它？（owner 边界）
//  3. 它的终态发布了没有？（恰好一次）
//
// 之所以把 owner 做成必填而不是可选，是因为「会话 A 能不能看到会话 B 的 job」这个
// 问题的默认答案必须是"不能"。把 owner 设为可选，就等于让忘记传 owner 的调用方
// 拿到一个全局可见的 job，而且不会有任何报错。
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// State 是 job 的状态。终态之外的取值都可能继续变化。
type State string

const (
	// StateQueued 表示已登记、尚未开始执行。
	StateQueued State = "queued"
	// StateRunning 表示正在执行。
	StateRunning State = "running"
	// StateSucceeded 表示正常结束且退出码为 0。
	StateSucceeded State = "succeeded"
	// StateFailed 表示正常结束但退出码非 0，或启动/执行报错。
	StateFailed State = "failed"
	// StateCanceled 表示被 owner 取消。
	StateCanceled State = "canceled"
	// StateTimedOut 表示因自身 deadline 到期而结束。
	//
	// 它与 StateCanceled 分开，是因为「用户主动取消」和「跑太久被掐掉」需要采取
	// 不同动作：前者通常要问用户改口径，后者通常要调超时或把命令放后台。
	// 合并成一个"未成功"会让这两种情况在界面上无法区分。
	StateTimedOut State = "timed_out"
	// StateInterrupted 表示进程消失或重启后无法接管，状态由外部断定。
	// 它存在的意义是：不允许把"不知道还在不在跑"写成"还在跑"。
	StateInterrupted State = "interrupted"
)

// Terminal 报告该状态是否终态。终态只发布一次，之后不再改变。
func (s State) Terminal() bool {
	switch s {
	case StateSucceeded, StateFailed, StateCanceled, StateTimedOut, StateInterrupted:
		return true
	default:
		return false
	}
}

// 明确的可判定错误。调用方需要靠它们区分"没找到"、"越权"、"到上限"，
// 而不是去匹配错误文本。
var (
	// ErrNotFound 表示该 owner 名下没有这个 job。
	// 越权访问也返回这个错误——不区分「不存在」与「不属于你」，
	// 否则错误信息本身就成了探测其他会话 job ID 的通道。
	ErrNotFound = errors.New("jobs: 未找到该 job")
	// ErrMissingOwner 表示未指定 owner。归属是必填项。
	ErrMissingOwner = errors.New("jobs: 必须指定 owner")
	// ErrEmptyCommand 表示命令为空。
	ErrEmptyCommand = errors.New("jobs: 命令不能为空")
	// ErrNilRunner 表示没有提供执行函数。
	ErrNilRunner = errors.New("jobs: 未提供执行函数")
	// ErrConcurrencyLimit 表示已达到并发上限，job 未被登记。
	ErrConcurrencyLimit = errors.New("jobs: 并发数量已达上限")
	// ErrAlreadyTerminal 表示该 job 已经处于终态，不能再取消。
	ErrAlreadyTerminal = errors.New("jobs: 已处于终态")
)

// Runner 是 job 的实际执行体。
//
// 返回值约定：exitCode 为进程退出码；err 非 nil 表示启动/执行失败。
// ctx 被取消时 Runner 应尽快返回，并把 ctx.Err() 一并返回。
type Runner func(ctx context.Context) (exitCode int, err error)

// SinkRunner 是带输出承接端的执行体。
//
// 每个流一个 io.Writer，由 Manager 提供；写进去的字节进入内存配额与溢出文件，
// 之后可用 ReadOutput 按偏移分页取回。写入端永不因为配额或磁盘问题返回错误
// （见 output.go 的包注释），所以执行体可以照常把它当普通 Writer 用。
type SinkRunner func(ctx context.Context, stdout, stderr io.Writer) (exitCode int, err error)

// Job 是一个被跟踪的后台执行单元的快照。
type Job struct {
	// ID 在 Manager 内唯一，用于查询与取消。
	ID string
	// Owner 是归属者（产品层传会话 ID）。它是访问控制边界。
	Owner string
	// WorkspaceID 是产生任务的工作区（CTX-001）。旧日志为空。
	WorkspaceID string
	// Command 是给用户看的命令摘要，不用于执行。
	Command string
	// State 是当前状态。
	State State
	// CreatedAt 是登记时刻，StartedAt 是开始执行时刻，EndedAt 是进入终态的时刻。
	CreatedAt time.Time
	StartedAt time.Time
	EndedAt   time.Time
	// ExitCode 仅在正常结束时有意义；被取消/中断时为 -1。
	ExitCode int
	// Err 是进入 StateFailed 的原因，其他状态为空。
	Err string
	// StdoutBytes / StderrBytes 是两个流已接收的总字节数（含被丢弃的）。
	StdoutBytes int64
	StderrBytes int64
	// OutputTruncated 表示至少一个流有字节因配额或写失败被丢弃。
	OutputTruncated bool
	// OutputRefs 是发生过溢出的私有文件路径，供用户按路径检索。
	// 纯内存输出不含在内。
	OutputRefs []string
	// JournalErr 记录"写入持久化日志失败"的原因。非空表示这次运行没有可恢复记录，
	// 它是需要被看见的事实，而不是一条可以被忽略的日志。
	JournalErr string
}

// Duration 返回已执行时长；未结束时按 now 计算。
func (j Job) Duration(now time.Time) time.Duration {
	if j.StartedAt.IsZero() {
		return 0
	}
	end := j.EndedAt
	if end.IsZero() {
		end = now
	}
	return end.Sub(j.StartedAt)
}

// Options 配置 Manager。
type Options struct {
	// MaxConcurrent 是全局限额，<=0 表示不限制。
	MaxConcurrent int
	// MaxPerOwner 是单 owner 限额，<=0 表示不限制。
	MaxPerOwner int
	// Now 可注入时钟，便于测试；为空用 time.Now。
	Now func() time.Time
	// NewID 可注入 ID 生成器；为空用 IDPrefix + 单调递增编号，保证可复现。
	NewID func() string
	// IDPrefix 是默认 ID 生成器的前缀。
	//
	// 默认生成器只保证**进程内**唯一（job-001、job-002 …）。重启后编号会从头开始，
	// 于是"上一轮的 job-001"和"这一轮的 job-001"在持久化日志里会撞车。
	// 运行时因此应当给每次进程启动一个不同的前缀，让 ID 跨进程也唯一。
	IDPrefix string
	// Journal 是 job 记录的持久化去向；为 nil 表示纯内存运行（重启后无历史）。
	Journal Journal
	// WorkspaceID 是本 Manager 的工作区归属（CTX-001）。为空表示未归属——
	// 写出的记录不带 workspace_id，读取方按旧数据对待。
	WorkspaceID string
	// OnTerminal 在 job 进入终态时被调用，**恰好一次**。
	// 它在持有 Manager 锁之外调用，可以安全地做 I/O 或回调其他组件。
	OnTerminal func(Job)
	// Output 配置每个 job 的输出承接（内存配额、溢出文件、保留期）。
	Output OutputOptions
}

// Manager 管理 job 的生命周期。所有方法都可并发调用。
type Manager struct {
	opts Options

	mu      sync.Mutex
	jobs    map[string]*jobState
	order   []string
	counter int64
	wg      sync.WaitGroup
}

// jobState 是 Manager 内部的活的 job 记录。
type jobState struct {
	snapshot Job
	settled  bool
	cancel   context.CancelFunc
	out      *jobOutput
	// done 在进入终态时关闭，供 Await / CloseOwner 精确等待单个 job，
	// 而不是靠轮询或等全部 job。
	done chan struct{}
}

// jobOutput 是一个 job 的两个输出流。
//
// 两个流分开计数、分开溢出：混合计数会让「stdout 读到一半」这种偏移计算失去意义。
type jobOutput struct {
	stdout *outputSink
	stderr *outputSink
}

// sink 按流名取承接端。
func (o *jobOutput) sink(stream Stream) *outputSink {
	if stream == StreamStderr {
		return o.stderr
	}
	return o.stdout
}

// refs 返回发生过溢出的文件路径。
func (o *jobOutput) refs() []string {
	var out []string
	for _, s := range []*outputSink{o.stdout, o.stderr} {
		if s == nil {
			continue
		}
		if _, _, _, path := s.stats(); path != "" {
			out = append(out, path)
		}
	}
	return out
}

// New 创建 Manager。
func New(opts Options) *Manager {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.NewID == nil {
		prefix := opts.IDPrefix
		var seq int64
		opts.NewID = func() string {
			return fmt.Sprintf("%sjob-%03d", prefix, atomic.AddInt64(&seq, 1))
		}
	}
	opts.Output = opts.Output.normalized(opts.Now)
	return &Manager{
		opts: opts,
		jobs: make(map[string]*jobState),
	}
}

// Start 登记并启动一个 job。
//
// 校验顺序刻意是「先校验、后占额度、最后登记」：任何一种拒绝都不会留下半个 job，
// 也不会把额度漏掉。达到上限时返回 ErrConcurrencyLimit，且 Manager 里查不到该 job。
func (m *Manager) Start(owner, command string, run Runner) (*Job, error) {
	if run == nil {
		return nil, ErrNilRunner
	}
	return m.start(owner, command, func(ctx context.Context, _, _ io.Writer) (int, error) {
		return run(ctx)
	})
}

// StartWithOutput 与 Start 语义相同，区别是执行体会拿到 stdout/stderr 的写入端，
// 输出进入内存配额与溢出文件，可用 ReadOutput 分页取回。
func (m *Manager) StartWithOutput(owner, command string, run SinkRunner) (*Job, error) {
	if run == nil {
		return nil, ErrNilRunner
	}
	return m.start(owner, command, run)
}

// start 是 Start / StartWithOutput 的共同实现。
func (m *Manager) start(owner, command string, run SinkRunner) (*Job, error) {
	if owner == "" {
		return nil, ErrMissingOwner
	}
	if command == "" {
		return nil, ErrEmptyCommand
	}
	if run == nil {
		return nil, ErrNilRunner
	}

	m.mu.Lock()
	if err := m.checkLimitsLocked(owner); err != nil {
		m.mu.Unlock()
		return nil, err
	}

	now := m.opts.Now()
	id := m.opts.NewID()
	seed := Job{
		ID:        id,
		Owner:     owner,
		Command:   command,
		State:     StateQueued,
		CreatedAt: now,
		ExitCode:  -1,
	}
	// 先落记录、再登记 job，且两件事在同一个锁区间内完成。
	//
	// 顺序理由：如果记录写不进去就必须拒绝启动——一条"没有可恢复记录"的后台命令，
	// 正是重启后最危险的形态（用户无法知道它跑没跑过）。
	// 同区间理由：把校验、落记录、登记拆成三段会让"额度被占用但 job 不存在"或
	// "记录写了但 job 没登记"这类中间态变得可能。代价是文件 I/O 落在锁内，
	// 而 `Start` 不是热路径，这个代价可以接受。
	if err := m.journalStart(seed); err != nil {
		m.mu.Unlock()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	dir := filepath.Join(m.opts.Output.Dir, ownerDirName(owner))
	out := &jobOutput{
		stdout: newOutputSink(StreamStdout, m.opts.Output, filepath.Join(dir, id+".stdout.out")),
		stderr: newOutputSink(StreamStderr, m.opts.Output, filepath.Join(dir, id+".stderr.out")),
	}
	st := &jobState{
		snapshot: seed,
		cancel:   cancel,
		out:      out,
		done:     make(chan struct{}),
	}
	m.jobs[st.snapshot.ID] = st
	m.order = append(m.order, st.snapshot.ID)
	snapshot := st.snapshot
	m.mu.Unlock()

	m.wg.Add(1)
	go m.run(ctx, st, run)

	return &snapshot, nil
}

// checkLimitsLocked 在持锁状态下校验并发额度。
func (m *Manager) checkLimitsLocked(owner string) error {
	if m.opts.MaxConcurrent <= 0 && m.opts.MaxPerOwner <= 0 {
		return nil
	}
	global, perOwner := 0, 0
	for _, st := range m.jobs {
		if st.snapshot.State.Terminal() {
			continue
		}
		global++
		if st.snapshot.Owner == owner {
			perOwner++
		}
	}
	if m.opts.MaxConcurrent > 0 && global >= m.opts.MaxConcurrent {
		return fmt.Errorf("%w: 全局上限 %d", ErrConcurrencyLimit, m.opts.MaxConcurrent)
	}
	if m.opts.MaxPerOwner > 0 && perOwner >= m.opts.MaxPerOwner {
		return fmt.Errorf("%w: 单 owner 上限 %d", ErrConcurrencyLimit, m.opts.MaxPerOwner)
	}
	return nil
}

// run 是唯一的终态发布点。
//
// 只有这里调用 settle：取消、正常结束、执行失败都收敛到这一处，
// 「重复结束不会重复发终态」因此是结构性保证，而不是靠调用方自觉。
func (m *Manager) run(ctx context.Context, st *jobState, run SinkRunner) {
	defer m.wg.Done()

	m.mu.Lock()
	if st.settled {
		m.mu.Unlock()
		return
	}
	st.snapshot.State = StateRunning
	st.snapshot.StartedAt = m.opts.Now()
	stdout, stderr := st.out.stdout, st.out.stderr
	m.mu.Unlock()

	exitCode, err := run(ctx, stdout, stderr)

	// 原因判定顺序即优先级，注释写清每一档为什么这样排：
	switch {
	case err != nil && ctx.Err() != nil:
		// owner 取消导致的返回：原因记为取消，而不是失败——否则用户看到的
		// "失败"会掩盖"是你自己取消的"。
		// 取消与 deadline 同时发生时也走这里：人有意识的动作优先于计时器。
		m.settle(st, StateCanceled, -1, "")
	case errors.Is(err, context.DeadlineExceeded):
		// 执行体自己的 deadline 到期。它与"用户取消"都不算 failed，
		// 但必须分开记录，否则界面上分不出"我掐的"和"跑太久了"。
		m.settle(st, StateTimedOut, exitCode, "执行超时")
	case err != nil:
		m.settle(st, StateFailed, exitCode, err.Error())
	case exitCode != 0:
		m.settle(st, StateFailed, exitCode, fmt.Sprintf("退出码 %d", exitCode))
	default:
		m.settle(st, StateSucceeded, 0, "")
	}
}

// settle 进入终态。重复调用是安全的空操作，OnTerminal 只触发一次。
func (m *Manager) settle(st *jobState, state State, exitCode int, errMsg string) {
	// 输出统计在 Manager 锁之外取：sink 有自己的锁，两把锁不能嵌套持有。
	var (
		stdoutBytes, stderrBytes int64
		truncated                bool
		refs                     []string
	)
	if st.out != nil {
		outTotal, _, outDropped, _ := st.out.stdout.stats()
		errTotal, _, errDropped, _ := st.out.stderr.stats()
		stdoutBytes, stderrBytes = outTotal, errTotal
		truncated = outDropped > 0 || errDropped > 0
		refs = st.out.refs()
	}

	m.mu.Lock()
	if st.settled {
		m.mu.Unlock()
		return
	}
	st.settled = true
	st.snapshot.State = state
	st.snapshot.ExitCode = exitCode
	st.snapshot.Err = errMsg
	st.snapshot.EndedAt = m.opts.Now()
	st.snapshot.StdoutBytes = stdoutBytes
	st.snapshot.StderrBytes = stderrBytes
	st.snapshot.OutputTruncated = truncated
	st.snapshot.OutputRefs = refs
	snapshot := st.snapshot
	cancel := st.cancel
	done := st.done
	m.mu.Unlock()

	// 终态记录写失败不能让终态本身失败（状态已经确定，掩盖它才是错的），
	// 但也不能假装写成功了——把原因记在快照上，由调用方决定怎么告诉用户。
	if err := m.journalTerminal(snapshot); err != nil {
		snapshot.JournalErr = err.Error()
		m.mu.Lock()
		st.snapshot.JournalErr = snapshot.JournalErr
		m.mu.Unlock()
	}

	// 释放 context 资源；settle 之后 cancel 不再有意义。
	if cancel != nil {
		cancel()
	}
	// 关闭等待通道放在锁外：等待方被唤醒后会再去 Get，若此时还持锁就会互等。
	// 关闭恰好一次由上面的 settled 判定保证。
	if done != nil {
		close(done)
	}
	if m.opts.OnTerminal != nil {
		m.opts.OnTerminal(snapshot)
	}
}

// Get 返回 owner 名下某个 job 的快照。
//
// 归属不匹配时返回 ErrNotFound，与"不存在"完全一致，避免通过错误信息探测。
func (m *Manager) Get(owner, id string) (Job, error) {
	if owner == "" {
		return Job{}, ErrMissingOwner
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.jobs[id]
	if !ok || st.snapshot.Owner != owner {
		return Job{}, ErrNotFound
	}
	return st.snapshot, nil
}

// List 返回 owner 名下的全部 job 快照，按登记顺序排列。
func (m *Manager) List(owner string) []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.order))
	for _, id := range m.order {
		st, ok := m.jobs[id]
		if !ok || st.snapshot.Owner != owner {
			continue
		}
		out = append(out, st.snapshot)
	}
	return out
}

// Cancel 请求取消 owner 名下的某个 job。
//
// 它只发出取消信号并立即返回；终态由执行协程在观察到取消后发布。这样终态仍然只有
// 一个发布点，不会出现"Cancel 先写成 canceled，执行体随后又写成 failed"的双写。
// 需要等待终态时用 Await。
func (m *Manager) Cancel(owner, id string) error {
	if owner == "" {
		return ErrMissingOwner
	}
	m.mu.Lock()
	st, ok := m.jobs[id]
	if !ok || st.snapshot.Owner != owner {
		m.mu.Unlock()
		return ErrNotFound
	}
	if st.settled {
		m.mu.Unlock()
		return ErrAlreadyTerminal
	}
	cancel := st.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

// CancelOwner 取消该 owner 名下所有未结束的 job，返回被取消的个数。
func (m *Manager) CancelOwner(owner string) int {
	if owner == "" {
		return 0
	}
	m.mu.Lock()
	var cancels []context.CancelFunc
	for _, id := range m.order {
		st, ok := m.jobs[id]
		if !ok || st.snapshot.Owner != owner || st.settled {
			continue
		}
		if st.cancel != nil {
			cancels = append(cancels, st.cancel)
		}
	}
	m.mu.Unlock()

	for _, c := range cancels {
		c()
	}
	return len(cancels)
}

// Await 阻塞等待某个 job 进入终态并返回其最终快照。
// ctx 结束时返回 ctx 的错误与该 job 当前快照。
//
// 它等待的是终态通道而不是轮询状态，因此不会出现"job 已结束但等待方还要等一个
// 轮询周期"的延迟，也不会因为轮询间隔而漏掉状态。
func (m *Manager) Await(ctx context.Context, owner, id string) (Job, error) {
	if owner == "" {
		return Job{}, ErrMissingOwner
	}
	m.mu.Lock()
	st, ok := m.jobs[id]
	if !ok || st.snapshot.Owner != owner {
		m.mu.Unlock()
		return Job{}, ErrNotFound
	}
	done := st.done
	m.mu.Unlock()

	if done == nil {
		return m.Get(owner, id)
	}
	select {
	case <-done:
		return m.Get(owner, id)
	case <-ctx.Done():
		job, err := m.Get(owner, id)
		if err != nil {
			return Job{}, err
		}
		return job, ctx.Err()
	}
}

// CloseOwner 结束该 owner 的全部 job 并清理它的输出文件，返回被取消的 job 数。
//
// 用途是"会话/owner 结束"这一事件：不做这件事，会话结束后它的后台命令还在跑、
// 输出文件还占着磁盘。等待是无界的——`Runner` 的契约就是必须响应 ctx 取消，
// 一个不响应的执行体不该靠超时来掩盖。
func (m *Manager) CloseOwner(owner string) int {
	if owner == "" {
		return 0
	}
	m.mu.Lock()
	var (
		cancels []context.CancelFunc
		dones   []chan struct{}
		sinks   []*outputSink
	)
	for _, id := range m.order {
		st, ok := m.jobs[id]
		if !ok || st.snapshot.Owner != owner {
			continue
		}
		if st.out != nil {
			sinks = append(sinks, st.out.stdout, st.out.stderr)
		}
		if !st.settled {
			if st.cancel != nil {
				cancels = append(cancels, st.cancel)
			}
			if st.done != nil {
				dones = append(dones, st.done)
			}
		}
	}
	m.mu.Unlock()

	for _, c := range cancels {
		c()
	}
	for _, d := range dones {
		<-d
	}
	for _, s := range sinks {
		if s != nil {
			s.closeAndMaybeRemove(true)
		}
	}
	return len(cancels)
}

// Wait 等待所有已登记的 job 结束。用于测试与优雅关闭。
func (m *Manager) Wait() { m.wg.Wait() }

// Active 返回 owner 名下未结束的 job 数量。
func (m *Manager) Active(owner string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, id := range m.order {
		st, ok := m.jobs[id]
		if !ok || st.snapshot.Owner != owner || st.settled {
			continue
		}
		n++
	}
	return n
}

// Len 返回登记的 job 总数（含终态）。
func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

// OutputDir 返回溢出文件的根目录，供产品层生成可检索引用。
func (m *Manager) OutputDir() string { return m.opts.Output.Dir }

// ReadOutput 按偏移分页读取 owner 名下某个 job 的某个流。
//
// 越权与不存在返回同一个 ErrNotFound，理由与 Get 相同：错误信息不能成为探测通道。
// 因此调用方无法通过本方法区分「这个 job 不存在」和「这个 job 不属于你」。
func (m *Manager) ReadOutput(owner, id string, stream Stream, offset, limit int64) (OutputPage, error) {
	if owner == "" {
		return OutputPage{}, ErrMissingOwner
	}
	if !stream.Valid() {
		return OutputPage{}, ErrBadStream
	}
	m.mu.Lock()
	st, ok := m.jobs[id]
	if !ok || st.snapshot.Owner != owner {
		m.mu.Unlock()
		return OutputPage{}, ErrNotFound
	}
	out := st.out
	now := m.opts.Now()
	m.mu.Unlock()

	if out == nil {
		return OutputPage{}, ErrOutputUnavailable
	}
	return out.sink(stream).read(offset, limit, now)
}

// CleanupOutputs 删除已过保留期的输出文件，返回删除的文件数与释放的字节数。
//
// 它不清理未过期的文件：正在被读取的输出被删掉，会让「分页不漏字节」失效。
func (m *Manager) CleanupOutputs() (removed int, freed int64) {
	now := m.opts.Now()
	m.mu.Lock()
	var targets []*outputSink
	for _, st := range m.jobs {
		if st.out == nil {
			continue
		}
		for _, s := range []*outputSink{st.out.stdout, st.out.stderr} {
			if s != nil && s.expired(now) {
				targets = append(targets, s)
			}
		}
	}
	m.mu.Unlock()

	for _, s := range targets {
		_, _, _, path := s.stats()
		var size int64
		if path != "" {
			if info, err := os.Stat(path); err == nil {
				size = info.Size()
			}
		}
		if s.closeAndMaybeRemove(true) {
			removed++
			freed += size
		}
	}
	return removed, freed
}

// Close 取消所有未结束的 job，等待执行体返回，然后关闭并删除全部输出文件。
//
// 顺序不能反：先删文件再等执行体，正在写的 sink 会往已删除的 inode 里写，
// 结果是"命令成功了但输出丢了"这种最难查的现象。
func (m *Manager) Close() {
	m.mu.Lock()
	var cancels []context.CancelFunc
	for _, st := range m.jobs {
		if !st.settled && st.cancel != nil {
			cancels = append(cancels, st.cancel)
		}
	}
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	m.wg.Wait()

	m.mu.Lock()
	sinks := make([]*outputSink, 0, len(m.jobs)*2)
	for _, st := range m.jobs {
		if st.out == nil {
			continue
		}
		sinks = append(sinks, st.out.stdout, st.out.stderr)
	}
	m.mu.Unlock()

	for _, s := range sinks {
		if s != nil {
			s.closeAndMaybeRemove(true)
		}
	}
}
