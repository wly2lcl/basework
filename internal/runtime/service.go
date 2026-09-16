// Package runtime 定义并实现 basework 的运行服务（RUN-001）。
//
// 目标是把「一次运行的驱动能力」收拢到一个与 UI 无关的契约后面：CLI 与 TUI
// 都通过 Service 启动运行、订阅事件、取消任务，而不是各自直接操作 Agent、
// 会话存储与后台任务管理器。所有权因此清晰：
//
//   - Service 拥有 Agent 的生命周期（Close 时释放）；
//   - Service 拥有后台任务管理器的关闭职责（Close 时结束遗留任务）；
//   - 会话存储的目录由产品层创建并传入，Service 只在运行期使用它；
//   - 订阅（Subscription）归调用方持有，退订后不再收到事件。
//
// 本包刻意不依赖任何具体 UI（包括 Bubble Tea），也不监听任何网络端口——
// 它是进程内的服务边界，不是 server。
//
// 两类事件的边界（RUN-002 明确）：
//   - **持久化事件**：file.edited / turn.* 等，由 pkg/agent 与工具层写进
//     会话存储，跨进程可重放，是审计与恢复的事实来源；
//   - **瞬时 UI 事件**：本包的 RunEvent（run.started/run.finished），只服务
//     于在场的观察者，不落盘、不重放——订阅者错过就是错过，这与「事实」
//     的可靠性要求刻意不同。
//
// 并发策略（RUN-002 明确）：**同一会话的运行串行排队，不拒绝也不并发**。
// 并发写同一会话会让事件交错、投影混乱；排队让交互式调用方像用户预期
// 那样逐轮执行。取消（Cancel）作用于运行级 context，不影响排队中的运行。
// 关闭（Close）会取消所有进行中的运行并唤醒全部订阅者。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/session"
)

// ErrClosed 表示服务已关闭，不能再启动运行。
var ErrClosed = errors.New("runtime: 服务已关闭")

// 事件类别。事件排序与补齐的完整规则由 RUN-002 定义；这里只保证
// 单个订阅者收到的事件按发布顺序排列。
const (
	EventRunStarted  = "run.started"
	EventRunFinished = "run.finished"
)

// RunEvent 是运行生命周期事件。
type RunEvent struct {
	RunID string
	Kind  string
	// Err 是运行结束时的错误文本（成功为空）。
	Err string
	// SessionID 是运行所属会话（RUN-003）。订阅者据此路由/过滤：
	// 会话切换后，旧会话的事件对 新会话的观察者不可见。空表示无会话模式。
	SessionID string
}

// Run 是一次运行的记录。Start 返回时运行已结束（当前实现是同步的）。
type Run struct {
	ID       string
	Input    string
	Response *agent.Response
	// Err 是运行错误；context 取消在这里体现为该字段而非 panic。
	Err error
}

// Subscription 是一个事件订阅。Events 通道随事件发布持续送达；
// Unsubscribe 之后通道会被关闭，调用方不应再读取。
type Subscription interface {
	Events() <-chan RunEvent
	Unsubscribe()
	// Dropped 返回因消费过慢而被丢弃的事件数。非零意味着观察窗口有缺口，
	// 调用方据此决定是否重查会话存储补齐事实。
	Dropped() uint64
}

// Service 是运行服务的最小契约（start / cancel / session / subscribe）。
type Service interface {
	// Start 处理一条输入直到回合并结束。ctx 取消会中断运行。
	Start(ctx context.Context, input string) (*Run, error)
	// Cancel 取消一个进行中的运行；运行已结束或不存在返回 false。
	Cancel(runID string) bool
	// Session 返回当前会话信息；服务未绑定会话时返回 nil。
	Session() *session.Info
	// Subscribe 订阅此后全部运行事件（全量流，不按 runID 过滤；
	// 按 runID 的有序子流属 RUN-002）。订阅不回放历史事件。
	Subscribe() Subscription
	// Close 释放服务持有的资源。重复 Close 是幂等的。
	Close() error
}

// LocalDeps 是进程内服务的组装依赖。全部必填项在 NewLocal 里校验，
// 让「少传了谁」在构造时失败，而不是第一次 Start 时。
type LocalDeps struct {
	// Agent 是对话执行者。Service 拥有它并在 Close 时释放。
	Agent agent.Agent
	// Session 是会话存储；可为 nil（无会话模式，如一次性子进程）。
	Session *session.JSONLStore
	// SessionID 是当前会话；Session 非 nil 时应同时给出。
	SessionID string
	// Jobs 是后台任务管理器；可为 nil（无后台任务模式）。
	// 非 nil 时 Service 在 Close 时结束遗留任务并释放输出。
	Jobs *jobs.Manager
	// EventBuffer 是每个订阅者的事件通道容量；<=0 用默认 256。
	// 有界是硬要求：无界队列在慢消费者场景下等于内存泄漏。
	EventBuffer int
	// CallbackSwitch 是按运行绑定的回调路由器（RUN-003）。nil 时
	// StartWithCallback 的回调参数不生效。产品层把 Agent 的回调注册成
	// 这个路由器，每次运行绑入当次的 UI 回调、结束即解绑。
	CallbackSwitch *CallbackSwitch
	// UICallback 是默认的每次运行 UI 回调（RUN-003）。Start 时自动绑入
	// CallbackSwitch；StartWithCallback 可按运行覆盖。可为 nil。
	UICallback agent.Callback
}

// LocalService 是进程内 Service 实现。零网络、零 UI 依赖。
type LocalService struct {
	agent     agent.Agent
	sess      *session.JSONLStore
	sessionID string
	jobs      *jobs.Manager

	eventBuffer int
	cbSwitch    *CallbackSwitch
	uiCallback  agent.Callback

	closed atomic.Bool
	runSeq atomic.Int64

	// runGate 串行化同一会话的运行（RUN-002 并发策略）。使用 channel
	// 而不是不可取消的 Mutex，使排队中的调用能响应 ctx 取消；Close
	// 持有令牌等待在途运行结束后再释放 Agent。
	runGate  chan struct{}
	closedCh chan struct{}

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	subs    []*localSub
}

// compile-time 契约检查：LocalService 必须满足 Service。
var _ Service = (*LocalService)(nil)

// NewLocal 构造进程内运行服务。
func NewLocal(deps LocalDeps) (*LocalService, error) {
	if deps.Agent == nil {
		return nil, errors.New("runtime: NewLocal 缺少 Agent")
	}
	buf := deps.EventBuffer
	if buf <= 0 {
		buf = 256
	}
	return &LocalService{
		agent:       deps.Agent,
		sess:        deps.Session,
		sessionID:   deps.SessionID,
		jobs:        deps.Jobs,
		eventBuffer: buf,
		cbSwitch:    deps.CallbackSwitch,
		uiCallback:  deps.UICallback,
		cancels:     make(map[string]context.CancelFunc),
		runGate:     func() chan struct{} { ch := make(chan struct{}, 1); ch <- struct{}{}; return ch }(),
		closedCh:    make(chan struct{}),
	}, nil
}

// Start 处理一条输入。运行事件（started/finished）发给全部订阅者；
// finished 事件在返回前发布，保证同步调用方 Start 返回后再读订阅通道
// 也能看到完整生命周期。deps.UICallback（若有）自动绑入回调路由器。
func (s *LocalService) Start(ctx context.Context, input string) (*Run, error) {
	return s.StartWithCallback(ctx, input, nil)
}

// StartWithCallback 以「按运行覆盖」的方式处理一条输入（RUN-003）。
// cb 非 nil 时优先于 deps.UICallback 绑入回调路由器；运行结束（成功、
// 取消、报错均含）立即解绑，此后到达的回调事件属于已结束的运行，
// 一律丢弃——这是「旧任务回调不串到新会话」的机制保证。
// deps.CallbackSwitch 为 nil 时 cb 不生效。
func (s *LocalService) StartWithCallback(ctx context.Context, input string, cb agent.Callback) (*Run, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}
	if cb == nil {
		cb = s.uiCallback
	}
	// 串行化：排队等前一个运行结束再开始；排队期间响应调用方取消。
	select {
	case <-s.runGate:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closedCh:
		return nil, ErrClosed
	}
	defer func() { s.runGate <- struct{}{} }()
	if s.closed.Load() {
		return nil, ErrClosed
	}
	runID := fmt.Sprintf("run-%06d", s.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(ctx)
	// Admission and cancellation registration are one synchronized operation.
	// Close holds the same mutex while draining cancels, so it cannot finish its
	// cancellation pass and then have this run enter Agent with an untracked
	// context.
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		cancel()
		return nil, ErrClosed
	}
	s.cancels[runID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, runID)
		s.mu.Unlock()
		cancel()
	}()

	// 可选的按运行回调工厂让 UI 给每条异步消息附上不可变的 run/session
	// 标识。旧回调即使在解绑边界附近迟到，也不会被新会话误收。
	if scoped, ok := cb.(interface {
		ForRun(string, string) agent.Callback
	}); ok {
		cb = scoped.ForRun(runID, s.sessionID)
	}
	// ForRun belongs to the UI boundary and may take time. Close can cancel the
	// registered context while it runs; do not enter Agent after that boundary.
	if err := runCtx.Err(); err != nil || s.closed.Load() {
		return &Run{ID: runID, Input: input, Err: context.Canceled}, nil
	}

	if s.cbSwitch != nil && cb != nil {
		s.cbSwitch.Bind(cb)
		defer s.cbSwitch.Unbind(cb)
	}

	s.publish(RunEvent{RunID: runID, SessionID: s.sessionID, Kind: EventRunStarted})

	resp, err := s.agent.HandleMessage(runCtx, input)

	errText := ""
	if err != nil {
		errText = err.Error()
	}
	s.publish(RunEvent{RunID: runID, SessionID: s.sessionID, Kind: EventRunFinished, Err: errText})

	return &Run{ID: runID, Input: input, Response: resp, Err: err}, nil
}

// Cancel 取消一个进行中的运行。它作用于 Start 内部的运行级 context：
// 对已结束/不存在的 runID 返回 false（幂等，不报错）。
func (s *LocalService) Cancel(runID string) bool {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// Session 返回当前会话信息；未绑定会话或会话不存在时返回 nil。
// Get 的失败按「无会话」处理——服务不必为过期的会话 ID 变成不可用。
func (s *LocalService) Session() *session.Info {
	if s.sess == nil || s.sessionID == "" {
		return nil
	}
	info, err := s.sess.Get(s.sessionID)
	if err != nil {
		return nil
	}
	return info
}

// Subscribe 订阅此后发布的全部运行事件。
// 通道容量 256：事件消费慢于生产时新事件被丢弃（并递增丢弃计数），
// 绝不阻塞运行主链路——事件是观察窗口，不能反过来卡住执行。
func (s *LocalService) Subscribe() Subscription {
	ch := make(chan RunEvent, s.eventBuffer)
	sub := &localSub{svc: s, events: ch}
	s.mu.Lock()
	if s.closed.Load() {
		sub.closeOnce.Do(func() { close(ch) })
		s.mu.Unlock()
		return sub
	}
	s.subs = append(s.subs, sub)
	s.mu.Unlock()
	return sub
}

// Close 释放资源：先结束后台任务（它们可能还在写输出），再关 Agent。
// 幂等；第二次调用直接返回 nil。
func (s *LocalService) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(s.closedCh)
	// 取消所有进行中的运行：关闭语义是「全停」，让 Start 的调用方
	// 以 context.Canceled 返回，而不是悬挂到自然结束。
	s.mu.Lock()
	cancels := s.cancels
	s.cancels = make(map[string]context.CancelFunc)
	for _, cancel := range cancels {
		cancel()
	}
	// 唤醒所有订阅者：关闭其事件通道，让等待方不悬挂。
	subs := s.subs
	s.subs = nil
	for _, sub := range subs {
		sub.closeOnce.Do(func() { close(sub.events) })
	}
	s.mu.Unlock()
	// 等待当前持有 runGate 的运行退出。令牌在等待期间保持占有，
	// 因此不会有新的运行进入 Agent；排队调用会通过 closedCh 返回。
	<-s.runGate

	var firstErr error
	if s.jobs != nil {
		// jobs.Manager.Close 无返回值：内部自行告警，这里没有可合并的错误。
		s.jobs.Close()
	}
	if s.agent != nil {
		if err := s.agent.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// 允许仅为避免误用而释放令牌；closedCh 已关闭，后续 Start 仍会拒绝。
	s.runGate <- struct{}{}
	return firstErr
}

// publish 向全部订阅者非阻塞投递事件；通道满则丢弃（丢弃不回填、不阻塞）。
func (s *LocalService) publish(ev RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subs {
		select {
		case sub.events <- ev:
		default:
			sub.dropped.Add(1)
		}
	}
}

// localSub 是订阅的实现。closeOnce 保证 Unsubscribe 与 Close 竞争时
// 通道只被关闭一次。
type localSub struct {
	svc       *LocalService
	events    chan RunEvent
	closeOnce sync.Once
	dropped   atomic.Int64
}

func (s *localSub) Events() <-chan RunEvent { return s.events }

func (s *localSub) Dropped() uint64 { return uint64(s.dropped.Load()) }

func (s *localSub) Unsubscribe() {
	s.svc.mu.Lock()
	defer s.svc.mu.Unlock()
	for i, sub := range s.svc.subs {
		if sub == s {
			s.svc.subs = append(s.svc.subs[:i], s.svc.subs[i+1:]...)
			break
		}
	}
	s.closeOnce.Do(func() { close(s.events) })
}
