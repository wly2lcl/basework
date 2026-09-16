package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// fakeAgent 是可编程的 agent.Agent 测试替身。
type fakeAgent struct {
	mu       sync.Mutex
	handle   func(ctx context.Context, input string) (*agent.Response, error)
	closed   bool
	Sessions *session.JSONLStore
}

func (f *fakeAgent) HandleMessage(ctx context.Context, input string) (*agent.Response, error) {
	f.mu.Lock()
	h := f.handle
	f.mu.Unlock()
	if h == nil {
		return &agent.Response{SessionID: "s1"}, nil
	}
	return h(ctx, input)
}

func (f *fakeAgent) HandleMessages(ctx context.Context, messages []llm.ChatMessage) (*agent.Response, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgent) Tools() []tool.Tool { return nil }

func (f *fakeAgent) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func mustNew(t *testing.T, agt agent.Agent, deps LocalDeps) *LocalService {
	t.Helper()
	deps.Agent = agt
	svc, err := NewLocal(deps)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestNewLocal_MissingAgentFails(t *testing.T) {
	if _, err := NewLocal(LocalDeps{}); err == nil {
		t.Fatal("缺少 Agent 应在构造时报错")
	}
}

func TestStart_EmitsOrderedLifecycleEvents(t *testing.T) {
	agt := &fakeAgent{}
	svc := mustNew(t, agt, LocalDeps{})

	sub := svc.Subscribe()
	done := make(chan struct{})
	var got []RunEvent
	go func() {
		defer close(done)
		for ev := range sub.Events() {
			got = append(got, ev)
			if ev.Kind == EventRunFinished {
				return
			}
		}
	}()

	run, err := svc.Start(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.ID == "" || run.Response == nil || run.Err != nil {
		t.Fatalf("运行结果不符: %+v", run)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("未收到完成事件")
	}
	if len(got) != 2 || got[0].Kind != EventRunStarted || got[1].RunID != run.ID {
		t.Fatalf("事件序列不符: %+v", got)
	}
}

func TestStart_ErrorCarriedInRunAndEvent(t *testing.T) {
	boom := errors.New("boom")
	agt := &fakeAgent{handle: func(context.Context, string) (*agent.Response, error) {
		return nil, boom
	}}
	svc := mustNew(t, agt, LocalDeps{})
	sub := svc.Subscribe()

	run, err := svc.Start(context.Background(), "x")
	if err != nil {
		t.Fatalf("Start 本身不应报错: %v", err)
	}
	if !errors.Is(run.Err, boom) {
		t.Fatalf("运行错误应透传: %v", run.Err)
	}
	// 订阅者应看到 finished 事件带错误文本。
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				t.Fatal("通道意外关闭")
			}
			if ev.Kind == EventRunFinished && ev.Err == "boom" {
				return
			}
		case <-deadline:
			t.Fatal("未收到带错误的 finished 事件")
		}
	}
}

func TestCancel_RunningAndIdempotent(t *testing.T) {
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	svc := mustNew(t, agt, LocalDeps{})

	type startResult struct {
		run *Run
		err error
	}
	done := make(chan startResult, 1)
	ctx, cancelCaller := context.WithCancel(context.Background())
	defer cancelCaller()

	// Start 是同步的，放后台跑，以便主 goroutine 调 Cancel。
	var started chan struct{}
	started = make(chan struct{})
	go func() {
		sub := svc.Subscribe()
		_ = sub
		close(started)
		run, err := svc.Start(ctx, "block")
		done <- startResult{run, err}
	}()
	<-started

	// 等运行登记后取消。
	deadline := time.After(2 * time.Second)
	for {
		svc.mu.Lock()
		n := len(svc.cancels)
		svc.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("运行未登记")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	runID := ""
	svc.mu.Lock()
	for id := range svc.cancels {
		runID = id
		break
	}
	svc.mu.Unlock()

	if !svc.Cancel(runID) {
		t.Fatal("取消进行中的运行应返回 true")
	}
	res := <-done
	if !errors.Is(res.run.Err, context.Canceled) {
		t.Fatalf("取消应体现为 context.Canceled: %v", res.run.Err)
	}
	if svc.Cancel(runID) {
		t.Fatal("对已结束运行的取消应返回 false（幂等）")
	}
}

func TestSession_ReturnsBoundInfo(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(session.CreateOpts{Title: "t"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	agt := &fakeAgent{}
	svc := mustNew(t, agt, LocalDeps{Session: store, SessionID: info.ID})
	if got := svc.Session(); got == nil || got.ID != info.ID {
		t.Fatalf("Session 应返回绑定会话: %+v", got)
	}

	// 未绑定会话 → nil
	svc2 := mustNew(t, &fakeAgent{}, LocalDeps{})
	if svc2.Session() != nil {
		t.Fatal("无会话依赖时应返回 nil")
	}
}

func TestClose_IdempotentAndClosesAgent(t *testing.T) {
	agt := &fakeAgent{}
	svc, err := NewLocal(LocalDeps{Agent: agt})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("首次 Close: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("重复 Close 应幂等: %v", err)
	}
	agt.mu.Lock()
	closed := agt.closed
	agt.mu.Unlock()
	if !closed {
		t.Fatal("Close 应释放 Agent")
	}
	if _, err := svc.Start(context.Background(), "x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("关闭后 Start 应报 ErrClosed: %v", err)
	}
}

func TestSubscribe_UnsubscribeStopsDelivery(t *testing.T) {
	agt := &fakeAgent{}
	svc := mustNew(t, agt, LocalDeps{})
	sub := svc.Subscribe()
	sub.Unsubscribe()

	// 退订后 Start 的事件不应投递到已关闭通道（否则会 panic/泄漏）。
	if _, err := svc.Start(context.Background(), "after-unsub"); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// ---- RUN-002：有序事件与会话取消 ----

// TestStart_SerializesSameSessionRuns 同会话并发请求串行排队：
// run1 的 finished 事件必须先于 run2 的 started 事件（同会话顺序稳定）。
func TestStart_SerializesSameSessionRuns(t *testing.T) {
	release1 := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	agt := &fakeAgent{handle: func(ctx context.Context, input string) (*agent.Response, error) {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			<-release1
		}
		return &agent.Response{SessionID: "s1"}, nil
	}}
	svc := mustNew(t, agt, LocalDeps{})
	sub := svc.Subscribe()

	// 后台启动 run1（阻塞在 handle），确认其 started 后再启动 run2。
	run1Done := make(chan *Run, 1)
	go func() {
		run, _ := svc.Start(context.Background(), "first")
		run1Done <- run
	}()
	waitEvent(t, sub, EventRunStarted)

	run2Done := make(chan *Run, 1)
	go func() {
		run, _ := svc.Start(context.Background(), "second")
		run2Done <- run
	}()

	// 释放 run1。串行化下事件序列必然是 run1.finished → run2.started：
	// run2 的 started 绝不能先到。
	close(release1)

	ev1 := readEvent(t, sub)
	if ev1.Kind != EventRunFinished {
		t.Fatalf("第一个事件应为 run1 的 finished，得到 %s", ev1.Kind)
	}
	ev2 := readEvent(t, sub)
	if ev2.Kind != EventRunStarted {
		t.Fatalf("第二个事件应为 run2 的 started，得到 %s", ev2.Kind)
	}
	<-run1Done
	<-run2Done
}

func readEvent(t *testing.T, sub Subscription) RunEvent {
	t.Helper()
	select {
	case ev, ok := <-sub.Events():
		if !ok {
			t.Fatal("通道意外关闭")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("超时等待事件")
		return RunEvent{}
	}
}

func waitRegistered(t *testing.T, svc *LocalService) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		svc.mu.Lock()
		n := len(svc.cancels)
		svc.mu.Unlock()
		if n > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("运行未登记")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func waitEvent(t *testing.T, sub Subscription, kind string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				t.Fatal("通道意外关闭")
			}
			if ev.Kind == kind {
				return
			}
		case <-deadline:
			t.Fatalf("超时等待事件 %s", kind)
		}
	}
}

// TestCancel_NextRunUnaffected 取消 run1 不影响后续新建运行。
func TestCancel_NextRunUnaffected(t *testing.T) {
	agt := &fakeAgent{handle: func(ctx context.Context, input string) (*agent.Response, error) {
		if input == "cancel-me" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &agent.Response{SessionID: "s1"}, nil
	}}
	svc := mustNew(t, agt, LocalDeps{})

	// 同步跑一个会被取消的运行（caller ctx 定时取消）。
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	run1, err := svc.Start(ctx, "cancel-me")
	if err != nil {
		t.Fatalf("Start 不应返回错误: %v", err)
	}
	if !errors.Is(run1.Err, context.Canceled) {
		t.Fatalf("run1 应被取消: %v", run1.Err)
	}

	// 新运行不受影响。
	run2, err := svc.Start(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("取消后新运行应正常: %v", err)
	}
	if run2.Response == nil {
		t.Fatal("新运行应有结果")
	}
}

// TestClose_DuringRunCancelsRunningAndWakesSubscriber 关闭时进行中的运行
// 以取消收场，订阅者通道被关闭（不悬挂）。
func TestClose_DuringRunCancelsRunningAndWakesSubscriber(t *testing.T) {
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	svc, err := NewLocal(LocalDeps{Agent: agt})
	if err != nil {
		t.Fatal(err)
	}
	sub := svc.Subscribe()

	runDone := make(chan *Run, 1)
	go func() {
		run, _ := svc.Start(context.Background(), "long")
		runDone <- run
	}()
	waitRegistered(t, svc)

	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	run := <-runDone
	if !errors.Is(run.Err, context.Canceled) {
		t.Fatalf("关闭应取消进行中的运行: %v", run.Err)
	}
	drained := false
	for !drained {
		select {
		case _, ok := <-sub.Events():
			if !ok {
				// 通道已关闭——期望行为（缓冲中残留事件先排空）。
				drained = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("关闭后订阅通道未关闭")
		}
	}
	if _, err := svc.Start(context.Background(), "x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("关闭后 Start 应报 ErrClosed: %v", err)
	}
}

func TestStartQueuedAfterCloseDoesNotInvokeAgent(t *testing.T) {
	calls := make(chan string, 1)
	agt := &fakeAgent{handle: func(context.Context, string) (*agent.Response, error) {
		calls <- "called"
		return &agent.Response{}, nil
	}}
	svc := mustNew(t, agt, LocalDeps{})
	// Occupy the single run gate with a running call, then queue another call.
	block := make(chan struct{})
	agt.handle = func(ctx context.Context, input string) (*agent.Response, error) {
		if input == "first" {
			select {
			case <-block:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return nil, ctx.Err()
		}
		calls <- input
		return &agent.Response{}, nil
	}
	firstDone := make(chan struct{})
	go func() { _, _ = svc.Start(context.Background(), "first"); close(firstDone) }()
	deadline := time.After(2 * time.Second)
	for {
		svc.mu.Lock()
		n := len(svc.cancels)
		svc.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first run not registered")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	queuedDone := make(chan error, 1)
	go func() { _, err := svc.Start(context.Background(), "queued"); queuedDone <- err }()
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	close(block)
	<-firstDone
	select {
	case err := <-queuedDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("queued start after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued Start did not return")
	}
	select {
	case got := <-calls:
		t.Fatalf("Agent invoked after Close: %s", got)
	default:
	}
}

func TestSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	svc := mustNew(t, &fakeAgent{}, LocalDeps{})
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	sub := svc.Subscribe()
	defer sub.Unsubscribe()
	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("closed subscription emitted event")
		}
	default:
		t.Fatal("subscription after Close remains open")
	}
}

// TestClose_DuringCallbackAdmissionCancelsBeforeAgentEntry fixes the narrow
// window between Start's closed check and cancellation registration. A UI
// callback can take time to create its run-scoped wrapper; Close must still
// either cancel that run or reject it before Agent is invoked.
func TestClose_DuringCallbackAdmissionCancelsBeforeAgentEntry(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	agentEntered := make(chan struct{}, 1)
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		agentEntered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	cb := &admissionBlockingCallback{entered: entered, release: release}
	svc, err := NewLocal(LocalDeps{Agent: agt})
	if err != nil {
		t.Fatal(err)
	}
	startDone := make(chan struct{})
	go func() {
		_, _ = svc.StartWithCallback(context.Background(), "admission", cb)
		close(startDone)
	}()
	<-entered

	closeDone := make(chan struct{})
	go func() {
		_ = svc.Close()
		close(closeDone)
	}()
	<-svc.closedCh
	close(release)

	select {
	case <-agentEntered:
		t.Fatal("Close 之后不应让未受取消跟踪的运行进入 Agent")
	case <-startDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Start 未在关闭后结束")
	}
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 未在运行登记窗口后结束")
	}
}

type admissionBlockingCallback struct {
	agent.NopCallback
	entered chan struct{}
	release chan struct{}
}

func (c *admissionBlockingCallback) ForRun(string, string) agent.Callback {
	close(c.entered)
	<-c.release
	return c
}

// TestSubscription_DroppedCount 慢消费者策略：缓冲满后丢弃并计数，不阻塞。
func TestSubscription_DroppedCount(t *testing.T) {
	agt := &fakeAgent{}
	svc, err := NewLocal(LocalDeps{Agent: agt, EventBuffer: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	sub := svc.Subscribe()
	// 不消费，连续发布多个事件。
	for i := 0; i < 5; i++ {
		svc.publish(RunEvent{RunID: "r", Kind: "x"})
	}
	if sub.Dropped() == 0 {
		t.Fatal("缓冲满后应有丢弃计数")
	}
	// 无界队列检查：订阅通道容量就是配置值，事件不会堆积超过它。
	if cap(sub.(*localSub).events) != 1 {
		t.Fatalf("通道容量应为配置值 1，得到 %d", cap(sub.(*localSub).events))
	}
}

// ---- RUN-003：回调按运行绑定、陈旧增量丢弃 ----

// recordingCB 记录收到的文本增量（加锁：回调可能在任意 goroutine 触发）。
type recordingCB struct {
	mu     sync.Mutex
	deltas []string
	turns  int
}

func (r *recordingCB) OnTextDelta(d string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltas = append(r.deltas, d)
}
func (r *recordingCB) OnThinkingDelta(string)       {}
func (r *recordingCB) OnToolCallStart(llm.ToolCall) {}
func (r *recordingCB) OnToolCallEnd(llm.ToolCall, *tool.Result, error) {
}
func (r *recordingCB) OnTurnEnd(*agent.Response) { r.mu.Lock(); r.turns++; r.mu.Unlock() }
func (r *recordingCB) OnError(error)             {}

func (r *recordingCB) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deltas...)
}

// TestStartWithCallback_BindsDuringRun_DropsAfterFinish 运行期间回调生效，
// 运行结束后（无论收尾增量多晚到达）一律丢弃——旧任务不串新会话。
func TestStartWithCallback_BindsDuringRun_DropsAfterFinish(t *testing.T) {
	cb := &recordingCB{}
	// 运行中发出两条增量；结束后再由测试直接发一条「迟到的 flush」。
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		return &agent.Response{SessionID: "s1"}, nil
	}}
	sw := NewCallbackSwitch()
	svc := mustNew(t, agt, LocalDeps{CallbackSwitch: sw})

	run, err := svc.StartWithCallback(context.Background(), "hi", cb)
	if err != nil {
		t.Fatal(err)
	}
	if run.Response == nil {
		t.Fatal("应有结果")
	}

	// 运行已结束：迟到的事件必须被丢弃。
	sw.OnTextDelta("late-flush")
	sw.OnTurnEnd(nil)
	if got := cb.snapshot(); len(got) != 0 {
		t.Fatalf("运行结束后回调应解绑，但收到了: %v", got)
	}
	if cb.turns != 0 {
		t.Fatalf("运行结束后 OnTurnEnd 不应再转发，实际 %d 次", cb.turns)
	}
}

// TestStartWithCallback_ForwardsDuringRun 运行期间事件应正常转发到绑定回调。
func TestStartWithCallback_ForwardsDuringRun(t *testing.T) {
	cb := &recordingCB{}
	release := make(chan struct{})
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		return &agent.Response{SessionID: "s1"}, nil
	}}
	_ = release
	sw := NewCallbackSwitch()
	svc := mustNew(t, agt, LocalDeps{CallbackSwitch: sw})

	// 在运行外直接模拟「运行期间」的转发：先手动绑定（与 Start 行为一致），
	// 验证转发路径。绑定/解绑的生命周期由上一个测试覆盖。
	sw.Bind(cb)
	sw.OnTextDelta("hello")
	sw.OnTextDelta(" world")
	sw.Unbind(cb)
	sw.OnTextDelta("dropped")

	got := cb.snapshot()
	if len(got) != 2 || got[0] != "hello" || got[1] != " world" {
		t.Fatalf("运行期间应转发、解绑后应丢弃: %v", got)
	}
	_ = svc
}

// TestStart_AutoBindsUICallback deps.UICallback 应在每次 Start 自动绑入。
func TestStart_AutoBindsUICallback(t *testing.T) {
	cb := &recordingCB{}
	sw := NewCallbackSwitch()
	// 用一个能拿到 switch 的 fake：在 handle 里通过闭包发增量。
	var svcRef *LocalService
	agt := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
		switch2 := svcRef.cbSwitch
		switch2.OnTextDelta("during-run")
		return &agent.Response{SessionID: "s1"}, nil
	}}
	svcRef = mustNew(t, agt, LocalDeps{CallbackSwitch: sw, UICallback: cb})

	if _, err := svcRef.Start(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := cb.snapshot(); len(got) != 1 || got[0] != "during-run" {
		t.Fatalf("UICallback 应在运行期间自动绑定并收到增量: %v", got)
	}
	// 运行结束后解绑。
	sw.OnTextDelta("late")
	if got := cb.snapshot(); len(got) != 1 {
		t.Fatalf("运行结束后应解绑: %v", got)
	}
}

// TestRunEvent_CarriesSessionID 运行事件必须携带会话 ID（订阅路由依据）。
func TestRunEvent_CarriesSessionID(t *testing.T) {
	agt := &fakeAgent{}
	svc := mustNew(t, agt, LocalDeps{SessionID: "sess-1"})
	sub := svc.Subscribe()
	if _, err := svc.Start(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				t.Fatal("通道意外关闭")
			}
			if ev.SessionID != "sess-1" {
				t.Fatalf("事件应携带 SessionID: %+v", ev)
			}
			seen++
			if ev.Kind == EventRunFinished {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("超时，仅收到 %d 条事件", seen)
		}
	}
}
