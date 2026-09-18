// Package load contains the project-level load and lifecycle gate.
//
// The test deliberately uses a deterministic in-process model. It exercises the
// real Agent, LocalService and JSONLStore, while keeping model quality,
// credentials and external provider capacity out of the result.
package load_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	intruntime "github.com/wly2lcl/basework/internal/runtime"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

const (
	loadEvidenceEnv = "BASEWORK_LOAD_EVIDENCE"
	loadHelperEnv   = "BASEWORK_LOAD_HELPER"
	loadSeed        = "basework-load-2026-09-18"
	loadFixture     = "deterministic-model-v2|reply:<input>|agent-local-service-jsonl|matrix=1,4,8,16,32|soak=10000"
)

// TestProjectLoad is the short, repeatable project-level gate. Set
// BASEWORK_LOAD_SOAK_OPS=10000 for the long soak required by LOAD-002.
func TestProjectLoad(t *testing.T) {
	if os.Getenv(loadHelperEnv) == "jsonl" {
		return
	}

	evidence := loadEvidence{
		Schema:      "basework.project-load.v2",
		Date:        time.Now().UTC().Format(time.RFC3339),
		Go:          runtime.Version(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		Seed:        loadSeed,
		Endpoint:    "none (in-process deterministic model)",
		FixtureHash: fixtureHash(),
	}

	for _, concurrency := range []int{1, 4, 8, 16, 32} {
		name := fmt.Sprintf("same-process-concurrency-%d", concurrency)
		t.Run(name, func(t *testing.T) {
			result := runMatrixLevel(t, concurrency)
			evidence.Matrix = append(evidence.Matrix, result)
		})
	}

	t.Run("cancel-and-restart", func(t *testing.T) {
		evidence.Cancellation = runCancelAndRestart(t)
	})
	t.Run("bounded-subscription", func(t *testing.T) {
		evidence.Subscription = runBoundedSubscription(t)
	})
	t.Run("jsonl-multiprocess-and-tail-recovery", func(t *testing.T) {
		evidence.JSONL = runJSONLMultiprocess(t)
	})
	t.Run("soak", func(t *testing.T) {
		evidence.Soak = runSoak(t, loadSoakOps())
	})

	if path := os.Getenv(loadEvidenceEnv); path != "" {
		data, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatalf("编码负载证据: %v", err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatalf("写入负载证据 %s: %v", path, err)
		}
		t.Logf("负载证据已写入 %s", path)
	}
}

func loadSoakOps() int {
	if raw := strings.TrimSpace(os.Getenv("BASEWORK_LOAD_SOAK_OPS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			return n
		}
	}
	return 256
}

func fixtureHash() string {
	sum := sha256.Sum256([]byte(loadFixture))
	return hex.EncodeToString(sum[:])
}

type loadEvidence struct {
	Schema       string               `json:"schema"`
	Date         string               `json:"date"`
	Go           string               `json:"go"`
	GOOS         string               `json:"goos"`
	GOARCH       string               `json:"goarch"`
	Seed         string               `json:"seed"`
	Endpoint     string               `json:"endpoint"`
	FixtureHash  string               `json:"fixture_hash"`
	Matrix       []matrixEvidence     `json:"same_process_matrix"`
	Cancellation checkEvidence        `json:"cancellation_and_restart"`
	Subscription subscriptionEvidence `json:"bounded_subscription"`
	JSONL        jsonlEvidence        `json:"jsonl_multiprocess_and_recovery"`
	Soak         soakEvidence         `json:"soak"`
}

type checkEvidence struct {
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type matrixEvidence struct {
	Concurrency   int              `json:"concurrency"`
	Requests      int              `json:"requests"`
	Passed        int              `json:"passed"`
	Failed        int              `json:"failed"`
	Events        int              `json:"events"`
	DroppedEvents uint64           `json:"dropped_events"`
	LatencyMS     latencyEvidence  `json:"latency_ms"`
	Resources     resourceEvidence `json:"resources"`
}

type latencyEvidence struct {
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

type resourceEvidence struct {
	GoroutinesBefore int    `json:"goroutines_before"`
	GoroutinesAfter  int    `json:"goroutines_after"`
	HeapBefore       uint64 `json:"heap_bytes_before"`
	HeapAfter        uint64 `json:"heap_bytes_after"`
	FDsBefore        int    `json:"fds_before"`
	FDsAfter         int    `json:"fds_after"`
	JSONLBytesBefore int64  `json:"jsonl_bytes_before"`
	JSONLBytesAfter  int64  `json:"jsonl_bytes_after"`
	WithinBounds     bool   `json:"within_bounds"`
}

type subscriptionEvidence struct {
	Passed       bool   `json:"passed"`
	Buffer       int    `json:"buffer"`
	Dropped      uint64 `json:"dropped"`
	RereadAdvice string `json:"reread_advice"`
}

type jsonlEvidence struct {
	Passed          bool `json:"passed"`
	Workers         int  `json:"workers"`
	EventsPerWorker int  `json:"events_per_worker"`
	Events          int  `json:"events"`
	SeqContiguous   bool `json:"seq_contiguous"`
	TailRecovered   bool `json:"truncated_tail_recovered"`
	TailAppended    bool `json:"append_after_recovery"`
}

type soakEvidence struct {
	Operations  int              `json:"operations"`
	Passed      int              `json:"passed"`
	Failed      int              `json:"failed"`
	ElapsedMS   float64          `json:"elapsed_ms"`
	Throughput  float64          `json:"requests_per_second"`
	LatencyMS   latencyEvidence  `json:"latency_ms"`
	Events      int              `json:"events"`
	Resources   resourceEvidence `json:"resources"`
	FailureType map[string]int   `json:"failure_types"`
}

type deterministicModel struct {
	mu       sync.Mutex
	requests []string
}

func (m *deterministicModel) ID() string { return "basework-load-deterministic" }

func (m *deterministicModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	input := lastUserText(req.Messages)
	m.record(input)
	return &llm.Response{
		Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{
			Type: llm.ContentTypeText,
			Text: "reply:" + input,
		}}},
		Usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}, nil
}

func (m *deterministicModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	input := lastUserText(req.Messages)
	m.record(input)
	out := make(chan llm.StreamEvent, 3)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			out <- llm.StreamEvent{Type: llm.StreamEventText, Error: ctx.Err()}
		case out <- llm.StreamEvent{Type: llm.StreamEventText, Delta: "reply:" + input}:
		}
		select {
		case <-ctx.Done():
			return
		case out <- llm.StreamEvent{Type: llm.StreamEventUsage, Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}:
		}
		select {
		case <-ctx.Done():
		case out <- llm.StreamEvent{Type: llm.StreamEventDone}:
		}
	}()
	return out, nil
}

func (m *deterministicModel) Supports(cap llm.Capability) bool {
	return cap == llm.CapStreaming || cap == llm.CapTools
}

func (m *deterministicModel) record(input string) {
	m.mu.Lock()
	m.requests = append(m.requests, input)
	m.mu.Unlock()
}

func (m *deterministicModel) requestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func lastUserText(messages []llm.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != llm.RoleUser {
			continue
		}
		var b strings.Builder
		for _, p := range messages[i].Content {
			if p.Type == llm.ContentTypeText {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

type callbackFactory struct {
	mu      sync.Mutex
	records map[string]*callbackRecord
}

type callbackRecord struct {
	mu      sync.Mutex
	text    string
	turns   int
	errors  int
	session string
}

func newCallbackFactory() *callbackFactory {
	return &callbackFactory{records: make(map[string]*callbackRecord)}
}

func (f *callbackFactory) ForRun(runID, _ string) agent.Callback {
	r := &callbackRecord{}
	f.mu.Lock()
	f.records[runID] = r
	f.mu.Unlock()
	return r
}

func (f *callbackFactory) OnTextDelta(string)                              {}
func (f *callbackFactory) OnToolCallStart(llm.ToolCall)                    {}
func (f *callbackFactory) OnToolCallEnd(llm.ToolCall, *tool.Result, error) {}
func (f *callbackFactory) OnThinkingDelta(string)                          {}
func (f *callbackFactory) OnTurnEnd(*agent.Response)                       {}
func (f *callbackFactory) OnError(error)                                   {}

func (r *callbackRecord) OnTextDelta(delta string) {
	r.mu.Lock()
	r.text += delta
	r.mu.Unlock()
}
func (r *callbackRecord) OnToolCallStart(llm.ToolCall)                    {}
func (r *callbackRecord) OnToolCallEnd(llm.ToolCall, *tool.Result, error) {}
func (r *callbackRecord) OnThinkingDelta(string)                          {}
func (r *callbackRecord) OnTurnEnd(resp *agent.Response) {
	r.mu.Lock()
	r.turns++
	if resp != nil {
		r.session = resp.SessionID
	}
	r.mu.Unlock()
}
func (r *callbackRecord) OnError(error) {
	r.mu.Lock()
	r.errors++
	r.mu.Unlock()
}

func runMatrixLevel(t *testing.T, concurrency int) matrixEvidence {
	t.Helper()
	before := snapshotResources()
	root := t.TempDir()
	store, err := session.NewJSONLStore(root)
	if err != nil {
		t.Fatalf("创建 JSONL store: %v", err)
	}
	model := &deterministicModel{}
	factory := newCallbackFactory()
	switcher := intruntime.NewCallbackSwitch()
	agt, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
		agent.WithCallback(switcher),
		agent.WithMaxSteps(2),
	)
	if err != nil {
		t.Fatalf("创建 Agent: %v", err)
	}
	provider, ok := agt.(agent.SessionIDProvider)
	if !ok {
		t.Fatal("Agent 未提供稳定 SessionID")
	}
	svc, err := intruntime.NewLocal(intruntime.LocalDeps{
		Agent:          agt,
		Session:        store,
		SessionID:      provider.SessionID(),
		EventBuffer:    concurrency*4 + 16,
		CallbackSwitch: switcher,
	})
	if err != nil {
		_ = agt.Close()
		t.Fatalf("创建 runtime service: %v", err)
	}

	sub := svc.Subscribe()
	times := make([]float64, concurrency)
	inputs := make([]string, concurrency)
	results := make(chan matrixRun, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		i := i
		inputs[i] = fmt.Sprintf("matrix-%d-%d", concurrency, i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			run, startErr := svc.StartWithCallback(context.Background(), inputs[i], factory)
			results <- matrixRun{index: i, run: run, err: startErr, elapsed: time.Since(started)}
		}()
	}
	wg.Wait()
	close(results)

	allRuns := make([]matrixRun, 0, concurrency)
	passed := 0
	runIDs := make(map[string]bool)
	for result := range results {
		allRuns = append(allRuns, result)
		times[result.index] = float64(result.elapsed.Microseconds()) / 1000
		if result.err == nil && result.run != nil && result.run.Err == nil && result.run.Response != nil &&
			result.run.ID != "" && !runIDs[result.run.ID] &&
			textOf(result.run.Response.Message) == "reply:"+inputs[result.index] {
			runIDs[result.run.ID] = true
			passed++
		}
	}

	events, err := store.Events(session.EventFilter{SessionID: provider.SessionID()})
	if err != nil {
		t.Fatalf("读取矩阵会话事件: %v", err)
	}
	counts := map[session.EventType]int{}
	seqOK := true
	for i, event := range events {
		counts[event.Type]++
		if event.Seq != int64(i+1) {
			seqOK = false
		}
	}
	for i := 0; i < 2*concurrency; i++ {
		select {
		case _, ok := <-sub.Events():
			if !ok {
				t.Fatalf("矩阵订阅提前关闭")
			}
		default:
			if i < 2*concurrency {
				// The service is synchronous; all events must already be queued.
				t.Fatalf("矩阵事件不足：第 %d 个事件尚未到达", i+1)
			}
		}
	}
	dropped := sub.Dropped()
	sub.Unsubscribe()
	if err := svc.Close(); err != nil {
		t.Fatalf("关闭矩阵 service: %v", err)
	}
	after := snapshotResources()

	resources := resourceDiff(before, after)
	resources.JSONLBytesBefore = 0
	resources.JSONLBytesAfter = directorySize(root)
	if passed != concurrency || model.requestCount() != concurrency || len(runIDs) != concurrency || counts[session.EventPrompted] != concurrency ||
		counts[session.EventTurnEnded] != concurrency || !seqOK || dropped != 0 {
		t.Fatalf("矩阵不变量失败 concurrency=%d requests=%d passed=%d runs=%d events=%d counts=%v seq=%v dropped=%d",
			concurrency, model.requestCount(), passed, len(runIDs), len(events), counts, seqOK, dropped)
	}
	if !resources.WithinBounds {
		t.Fatalf("矩阵资源超出边界 concurrency=%d resources=%+v", concurrency, resources)
	}
	factory.mu.Lock()
	callbackCount := len(factory.records)
	factory.mu.Unlock()
	if callbackCount != concurrency {
		t.Fatalf("回调记录数=%d，期望=%d", callbackCount, concurrency)
	}
	factory.mu.Lock()
	for runID, record := range factory.records {
		record.mu.Lock()
		valid := record.turns == 1 && record.errors == 0 && strings.HasPrefix(record.text, "reply:") && record.session != ""
		record.mu.Unlock()
		if !valid {
			factory.mu.Unlock()
			t.Fatalf("运行 %s 回调未隔离或未完成", runID)
		}
	}
	factory.mu.Unlock()

	return matrixEvidence{
		Concurrency:   concurrency,
		Requests:      concurrency,
		Passed:        passed,
		Failed:        concurrency - passed,
		Events:        len(events),
		DroppedEvents: dropped,
		LatencyMS:     percentile(times),
		Resources:     resources,
	}
}

type matrixRun struct {
	index   int
	run     *intruntime.Run
	err     error
	elapsed time.Duration
}

func textOf(message llm.ChatMessage) string {
	var b strings.Builder
	for _, part := range message.Content {
		if part.Type == llm.ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func runSoak(t *testing.T, operations int) soakEvidence {
	t.Helper()
	before := snapshotResources()
	root := t.TempDir()
	store, err := session.NewJSONLStore(root)
	if err != nil {
		t.Fatalf("创建 soak store: %v", err)
	}
	model := &deterministicModel{}
	agt, err := agent.New(agent.WithModel(model), agent.WithSession(store), agent.WithMaxSteps(2))
	if err != nil {
		t.Fatalf("创建 soak Agent: %v", err)
	}
	provider := agt.(agent.SessionIDProvider)
	svc, err := intruntime.NewLocal(intruntime.LocalDeps{Agent: agt, Session: store, SessionID: provider.SessionID(), EventBuffer: operations*2 + 16})
	if err != nil {
		_ = agt.Close()
		t.Fatalf("创建 soak service: %v", err)
	}
	defer func() { _ = svc.Close() }()

	times := make([]float64, 0, operations)
	failures := make(map[string]int)
	start := time.Now()
	passed := 0
	for i := 0; i < operations; i++ {
		started := time.Now()
		run, startErr := svc.Start(context.Background(), fmt.Sprintf("soak-%d", i))
		times = append(times, float64(time.Since(started).Microseconds())/1000)
		if startErr != nil {
			failures[errorClass(startErr)]++
			continue
		}
		if run == nil || run.Err != nil {
			if run != nil && run.Err != nil {
				failures[errorClass(run.Err)]++
			} else {
				failures["nil_run"]++
			}
			continue
		}
		passed++
	}
	elapsed := time.Since(start)
	events, err := store.Events(session.EventFilter{SessionID: provider.SessionID()})
	if err != nil {
		t.Fatalf("读取 soak 事件: %v", err)
	}
	if passed != operations || len(events) == 0 {
		t.Fatalf("soak 失败：passed=%d/%d events=%d failures=%v", passed, operations, len(events), failures)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("关闭 soak service: %v", err)
	}
	after := snapshotResources()
	resources := resourceDiff(before, after)
	resources.JSONLBytesBefore = 0
	resources.JSONLBytesAfter = directorySize(root)
	if model.requestCount() != operations {
		t.Fatalf("soak 模型请求数=%d，期望=%d", model.requestCount(), operations)
	}
	if resources.JSONLBytesAfter <= resources.JSONLBytesBefore {
		t.Fatalf("soak JSONL 体积未增长：%+v", resources)
	}
	if !resources.WithinBounds {
		t.Fatalf("soak 资源超出边界：%+v", resources)
	}
	return soakEvidence{
		Operations:  operations,
		Passed:      passed,
		Failed:      operations - passed,
		ElapsedMS:   float64(elapsed.Microseconds()) / 1000,
		Throughput:  float64(passed) / elapsed.Seconds(),
		LatencyMS:   percentile(times),
		Events:      len(events),
		Resources:   resources,
		FailureType: failures,
	}
}

func errorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	return "error"
}

type blockingAgent struct {
	started chan struct{}
	once    sync.Once
}

func (a *blockingAgent) HandleMessage(ctx context.Context, input string) (*agent.Response, error) {
	if input != "cancel-me" {
		return &agent.Response{SessionID: "restart-ok"}, nil
	}
	a.once.Do(func() { close(a.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}
func (a *blockingAgent) HandleMessages(context.Context, []llm.ChatMessage) (*agent.Response, error) {
	return nil, errors.New("not implemented")
}
func (a *blockingAgent) Tools() []tool.Tool { return nil }
func (a *blockingAgent) Close() error       { return nil }

func runCancelAndRestart(t *testing.T) checkEvidence {
	t.Helper()
	agt := &blockingAgent{started: make(chan struct{})}
	svc, err := intruntime.NewLocal(intruntime.LocalDeps{Agent: agt, EventBuffer: 8})
	if err != nil {
		t.Fatalf("创建取消 service: %v", err)
	}
	sub := svc.Subscribe()
	done := make(chan *intruntime.Run, 1)
	go func() {
		run, _ := svc.Start(context.Background(), "cancel-me")
		done <- run
	}()
	var runID string
	select {
	case ev := <-sub.Events():
		runID = ev.RunID
	case <-time.After(5 * time.Second):
		t.Fatal("取消运行未开始")
	}
	if runID == "" || !svc.Cancel(runID) {
		t.Fatal("取消进行中的运行失败")
	}
	var run *intruntime.Run
	select {
	case run = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("取消运行未结束")
	}
	if run == nil || !errors.Is(run.Err, context.Canceled) {
		t.Fatalf("取消结果错误: %+v", run)
	}
	if svc.Cancel(runID) {
		t.Fatal("结束后的 Cancel 不应成功")
	}
	restarted, err := svc.Start(context.Background(), "restart-me")
	if err != nil || restarted == nil || restarted.Err != nil || restarted.Response == nil || restarted.Response.SessionID != "restart-ok" {
		t.Fatalf("取消后重启失败: run=%+v err=%v", restarted, err)
	}
	sub.Unsubscribe()
	_ = svc.Close()
	return checkEvidence{Passed: true, Detail: "cancel returns context.Canceled; subsequent Cancel is false; a new run succeeds; Close completes"}
}

func runBoundedSubscription(t *testing.T) subscriptionEvidence {
	t.Helper()
	svc, err := intruntime.NewLocal(intruntime.LocalDeps{Agent: &fastAgent{}, EventBuffer: 1})
	if err != nil {
		t.Fatalf("创建有界订阅 service: %v", err)
	}
	sub := svc.Subscribe()
	for i := 0; i < 8; i++ {
		if _, err := svc.Start(context.Background(), fmt.Sprintf("drop-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	dropped := sub.Dropped()
	if dropped == 0 {
		t.Fatal("慢订阅者没有记录丢弃事件")
	}
	sub.Unsubscribe()
	_ = svc.Close()
	return subscriptionEvidence{Passed: true, Buffer: 1, Dropped: dropped, RereadAdvice: "Dropped>0 requires rereading durable session events"}
}

type fastAgent struct{}

func (a *fastAgent) HandleMessage(context.Context, string) (*agent.Response, error) {
	return &agent.Response{SessionID: "load-fast"}, nil
}
func (a *fastAgent) HandleMessages(context.Context, []llm.ChatMessage) (*agent.Response, error) {
	return &agent.Response{SessionID: "load-fast"}, nil
}
func (a *fastAgent) Tools() []tool.Tool { return nil }
func (a *fastAgent) Close() error       { return nil }

func runJSONLMultiprocess(t *testing.T) jsonlEvidence {
	t.Helper()
	const workers = 8
	const eventsPerWorker = 128
	root := t.TempDir()
	store, err := session.NewJSONLStore(root)
	if err != nil {
		t.Fatalf("创建 JSONL 并发 store: %v", err)
	}
	info, err := store.Create(session.CreateOpts{ID: "load-multiprocess", Title: "load"})
	if err != nil {
		t.Fatalf("创建 JSONL 会话: %v", err)
	}

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestJSONLAppendHelper$", "-test.v")
			cmd.Env = append(os.Environ(),
				loadHelperEnv+"=jsonl",
				"BASEWORK_LOAD_JSONL_DIR="+root,
				"BASEWORK_LOAD_JSONL_SESSION="+info.ID,
				"BASEWORK_LOAD_JSONL_WORKER="+strconv.Itoa(worker),
				"BASEWORK_LOAD_JSONL_COUNT="+strconv.Itoa(eventsPerWorker),
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("子进程 %d 追加失败: %v\n%s", worker, err, output)
			}
			if ctx.Err() != nil {
				t.Errorf("子进程 %d 未在超时内退出: %v", worker, ctx.Err())
			}
		}()
	}
	wg.Wait()

	fresh, err := session.NewJSONLStore(root)
	if err != nil {
		t.Fatalf("重开 JSONL store: %v", err)
	}
	events, err := fresh.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("读取多进程事件: %v", err)
	}
	seqOK := len(events) == workers*eventsPerWorker
	for i, event := range events {
		if event.Seq != int64(i+1) {
			seqOK = false
			break
		}
	}
	if !seqOK {
		t.Fatalf("多进程 JSONL 序列不连续：len=%d", len(events))
	}

	path := filepath.Join(root, info.ID+".jsonl")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 JSONL 文件: %v", err)
	}
	recoveryRoot := t.TempDir()
	recoveryPath := filepath.Join(recoveryRoot, info.ID+".jsonl")
	if err := os.WriteFile(recoveryPath, bytes[:len(bytes)-1-10], 0o600); err != nil {
		t.Fatalf("写入截断 JSONL: %v", err)
	}
	recoveredStore, err := session.NewJSONLStore(recoveryRoot)
	if err != nil {
		t.Fatalf("创建恢复 store: %v", err)
	}
	recovered, err := recoveredStore.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("读取截断尾: %v", err)
	}
	tailRecovered := len(recovered) == len(events)-1
	if !tailRecovered {
		t.Fatalf("截断尾恢复事件数=%d，期望=%d", len(recovered), len(events)-1)
	}
	if err := recoveredStore.AppendEvent(session.Event{SessionID: info.ID, Type: session.EventTextDelta, Data: json.RawMessage(`{"delta":"after-recovery"}`)}); err != nil {
		t.Fatalf("恢复后追加: %v", err)
	}
	finalEvents, err := recoveredStore.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("读取恢复后事件: %v", err)
	}
	tailAppended := len(finalEvents) == len(events)
	if !tailAppended {
		t.Fatalf("恢复后追加事件数=%d，期望=%d", len(finalEvents), len(events))
	}
	return jsonlEvidence{Passed: true, Workers: workers, EventsPerWorker: eventsPerWorker, Events: len(events), SeqContiguous: seqOK, TailRecovered: tailRecovered, TailAppended: tailAppended}
}

// TestJSONLAppendHelper is executed in child test processes by
// runJSONLMultiprocess. It is intentionally inert during normal test runs.
func TestJSONLAppendHelper(t *testing.T) {
	if os.Getenv(loadHelperEnv) != "jsonl" {
		return
	}
	root := os.Getenv("BASEWORK_LOAD_JSONL_DIR")
	sessionID := os.Getenv("BASEWORK_LOAD_JSONL_SESSION")
	worker, _ := strconv.Atoi(os.Getenv("BASEWORK_LOAD_JSONL_WORKER"))
	count, _ := strconv.Atoi(os.Getenv("BASEWORK_LOAD_JSONL_COUNT"))
	store, err := session.NewJSONLStore(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		data, _ := json.Marshal(session.TextDeltaData{Delta: fmt.Sprintf("worker-%d-event-%d", worker, i)})
		if err := store.AppendEvent(session.Event{SessionID: sessionID, Type: session.EventTextDelta, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
}

type resourceSnapshot struct {
	goroutines int
	heap       uint64
	fds        int
}

func snapshotResources() resourceSnapshot {
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return resourceSnapshot{goroutines: runtime.NumGoroutine(), heap: mem.HeapAlloc, fds: fdCount()}
}

func resourceDiff(before, after resourceSnapshot) resourceEvidence {
	heapGrowth := uint64(0)
	if after.heap > before.heap {
		heapGrowth = after.heap - before.heap
	}
	goroutineGrowth := 0
	if after.goroutines > before.goroutines {
		goroutineGrowth = after.goroutines - before.goroutines
	}
	within := goroutineGrowth <= 4 && heapGrowth <= 64*1024*1024
	if runtime.GOOS != "windows" {
		fdGrowth := 0
		if after.fds > before.fds {
			fdGrowth = after.fds - before.fds
		}
		within = within && before.fds >= 0 && after.fds >= 0 && fdGrowth <= 4
	}
	return resourceEvidence{
		GoroutinesBefore: before.goroutines,
		GoroutinesAfter:  after.goroutines,
		HeapBefore:       before.heap,
		HeapAfter:        after.heap,
		FDsBefore:        before.fds,
		FDsAfter:         after.fds,
		WithinBounds:     within,
	}
}

func fdCount() int {
	for _, root := range []string{"/dev/fd", "/proc/self/fd"} {
		entries, err := os.ReadDir(root)
		if err == nil {
			return len(entries)
		}
		file, err := os.Open(root)
		if err != nil {
			continue
		}
		names, readErr := file.Readdirnames(-1)
		_ = file.Close()
		if readErr == nil {
			return len(names)
		}
	}
	return -1
}

func directorySize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func percentile(values []float64) latencyEvidence {
	if len(values) == 0 {
		return latencyEvidence{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	quantile := func(p float64) float64 {
		if len(sorted) == 1 {
			return sorted[0]
		}
		pos := p * float64(len(sorted)-1)
		lo := int(pos)
		hi := lo + 1
		if hi >= len(sorted) {
			return sorted[lo]
		}
		return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
	}
	return latencyEvidence{Min: sorted[0], P50: quantile(.5), P95: quantile(.95), P99: quantile(.99), Max: sorted[len(sorted)-1]}
}
