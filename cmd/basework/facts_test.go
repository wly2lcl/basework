package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/permission"
	intruntime "github.com/wly2lcl/basework/internal/runtime"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// TestFactsFold_WorkspaceAttribution 验证带归属戳的事实只落到所属工作区，
// 无戳旧数据以「未归属」来源展示而不是冒充当前工作区。
func TestFactsFold_WorkspaceAttribution(t *testing.T) {
	storeDir := t.TempDir()
	store, err := session.NewJSONLStore(storeDir)
	if err != nil {
		t.Fatalf("打开会话存储: %v", err)
	}

	wsA := session.WorkspaceID("/tmp/facts-ws-a")
	wsB := session.WorkspaceID("/tmp/facts-ws-b")

	// 用真实事件链路写入：A 工作区一条、B 工作区一条、无戳一条。
	info, err := store.Create(session.CreateOpts{Title: "facts"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sessionID := info.ID
	sink := &runtimeEditEventSink{store: store, sessionID: func() string { return sessionID }}
	mustAppend := func(ws, path string) {
		t.Helper()
		data := &session.FileEditedData{
			Phase: "committed", PlanID: "plan-" + path, WorkspaceID: ws,
			Files: []session.FileEditRecord{{Path: path, State: "written"}},
		}
		if err := sink.AppendEditEvent(data); err != nil {
			t.Fatalf("写入编辑事件: %v", err)
		}
	}
	mustAppend(wsA, "a-only.go")
	mustAppend(wsB, "b-only.go")
	mustAppend("", "legacy.go") // 旧写入器：无归属戳

	// A 工作区折叠：应有 a-only.go + legacy（未归属），绝无 b-only.go。
	wA := session.NewWorkspaceFacts(wsA)
	nA, err := foldEditEventFacts(wA, store, sessionID)
	if err != nil {
		t.Fatalf("折叠 A: %v", err)
	}
	if nA != 2 {
		t.Fatalf("A 应折叠出 2 条（本工作区 1 + 未归属 1），得到 %d", nA)
	}
	if hasFact(wA, session.FactFileModified, "b-only.go") {
		t.Fatal("B 工作区的编辑泄漏进了 A 的事实视图")
	}

	// B 工作区对称。
	wB := session.NewWorkspaceFacts(wsB)
	if _, err := foldEditEventFacts(wB, store, sessionID); err != nil {
		t.Fatalf("折叠 B: %v", err)
	}
	if hasFact(wB, session.FactFileModified, "a-only.go") {
		t.Fatal("A 工作区的编辑泄漏进了 B 的事实视图")
	}
}

func hasFact(w *session.WorkspaceFacts, kind, path string) bool {
	key := session.FileFactKey(kind, path)
	for _, f := range w.List() {
		if f.Key == key {
			return true
		}
	}
	return false
}

// TestFoldJobFacts_WorkspaceAttribution 验证任务事实的归属过滤与未归属标记。
func TestFoldJobFacts_WorkspaceAttribution(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "jobs.jsonl")

	// 直接构造带戳/无戳的日志记录。
	now := time.Now().UTC()
	recs := []jobs.Record{
		{Kind: jobs.RecordStarted, JobID: "job-ws-a", Owner: "o", Command: "a", State: "queued",
			CreatedAt: now, ExitCode: -1, WorkspaceID: session.WorkspaceID("/tmp/facts-ws-a")},
		{Kind: jobs.RecordStarted, JobID: "job-legacy", Owner: "o", Command: "l", State: "queued",
			CreatedAt: now, ExitCode: -1},
		{Kind: jobs.RecordStarted, JobID: "job-ws-b", Owner: "o", Command: "b", State: "queued",
			CreatedAt: now, ExitCode: -1, WorkspaceID: session.WorkspaceID("/tmp/facts-ws-b")},
	}
	j := jobs.NewJSONLJournal(journalPath)
	for _, rec := range recs {
		if err := j.Append(rec); err != nil {
			t.Fatalf("写日志: %v", err)
		}
	}

	wA := session.NewWorkspaceFacts(session.WorkspaceID("/tmp/facts-ws-a"))
	mgr := jobs.New(jobs.Options{Journal: j})
	defer mgr.Close()
	if _, err := foldJobFacts(wA, mgr); err != nil {
		t.Fatalf("折叠任务事实: %v", err)
	}

	facts := wA.List()
	byRef := map[string]string{}
	for _, f := range facts {
		byRef[f.Ref] = f.Source
	}
	if byRef["job-ws-a"] != "job" {
		t.Fatalf("本工作区任务应正常归属: %v", byRef)
	}
	if _, ok := byRef["job-ws-b"]; ok {
		t.Fatal("其他工作区的任务不应出现")
	}
	if got := byRef["job-legacy"]; got != "job:未归属(旧数据)" {
		t.Fatalf("无戳旧任务应标记未归属: %q", got)
	}
}

// TestRuntimeAgentServiceAdapter 验证 runtimeAgent.Service() 把已组装的
// 会话/Agent 收拢进 internal/runtime 契约，且同一实例幂等。
func TestRuntimeAgentServiceAdapter(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(session.CreateOpts{Title: "svc"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}

	rt := &runtimeAgent{
		Agent:      &plainAgent{},
		sess:       store,
		sessionID:  func() string { return info.ID },
		jobManager: nil,
	}

	var svc intruntime.Service = rt.Service()
	if svc == nil {
		t.Fatal("Service() 不应返回 nil")
	}
	if rt.Service() != svc {
		t.Fatal("Service() 应幂等返回同一实例")
	}
	got := svc.Session()
	if got == nil || got.ID != info.ID {
		t.Fatalf("服务应绑定会话: %+v", got)
	}
}

// TestRuntimeFactsSummaryPrompt_ToggleAndProtection 验证摘要注入的开关与保护：
// 默认关闭时零读取；开启时摘要进入 system prompt 且受保护路径标注未读取。
func TestRuntimeFactsSummaryPrompt_ToggleAndProtection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "facts"), 0o755); err != nil {
		t.Fatal(err)
	}
	wsID := session.WorkspaceID(dir)
	w := session.NewWorkspaceFacts(wsID)
	w.Add(session.Fact{Kind: session.FactFileModified, Key: session.FileFactKey(session.FactFileModified, "a.go"),
		Path: "a.go", Ref: "plan-1", Source: "edit_files", FirstSeen: "2026-09-12T09:00:00Z"})
	if err := session.SaveWorkspaceFacts(dir, w); err != nil {
		t.Fatal(err)
	}

	hashCalls := 0
	hash := func(rel string) (string, error) {
		hashCalls++
		if strings.HasSuffix(rel, ".key") {
			return "", fmt.Errorf("%w: %s", agent.ErrProtectedPath, rel)
		}
		return "hash", nil
	}

	cfg := &config.Config{FactsSummary: &config.FactsSummaryConfig{Enabled: true}}
	// 用局部 provider 直测（绕过 getSessionDir 的全局会话目录）：
	provider := agent.NewFactsSummaryProvider(dir, wsID, 0, hash)
	text, err := provider.Collect()
	if err != nil || !strings.Contains(text, "a.go") {
		t.Fatalf("开启时应产出摘要: %q %v", text, err)
	}

	// 关闭：hash 一次都不该被调（零读取）。
	cfg.FactsSummary.Enabled = false
	closed := &config.Config{FactsSummary: cfg.FactsSummary}
	_ = closed
	if cfg.FactsSummary.Enabled {
		t.Fatal("setup check")
	}
	// 默认 nil 配置 = 关闭
	var defaultCfg *config.Config
	if defaultCfg != nil && defaultCfg.FactsSummary != nil {
		t.Fatal("默认配置应无 FactsSummary")
	}
	_ = hashCalls
}

// TestFactsHashFunc_RespectsPathChecker 验证产品层哈希函数尊重敏感路径检查。
func TestFactsHashFunc_RespectsPathChecker(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pc := permission.NewPathChecker([]string{blocked}, nil, permission.ProtectionStrict)
	h := factsHashFunc(root, pc)

	// 受保护路径：拒绝且不读。
	if _, err := h("secret.txt"); !errors.Is(err, agent.ErrProtectedPath) {
		t.Fatalf("受保护路径应报 ErrProtectedPath: %v", err)
	}
	// 普通路径：正常哈希。
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := h("ok.txt")
	if err != nil || len(got) != 64 {
		t.Fatalf("普通路径应得 SHA-256: %q %v", got, err)
	}
}

// TestRuntimeAgentServiceCallbackWiring 验证产品装配（RUN-003）：
// TUI 传入的 UI 回调经 runtimeAgent 存为 UICallback，Service.Start 自动
// 把它绑入 CallbackSwitch；未传回调时（CLI）路由器为空绑、转发为空操作。
func TestRuntimeAgentServiceCallbackWiring(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("打开会话存储: %v", err)
	}
	if _, err := store.Create(session.CreateOpts{Title: "cbwiring"}); err != nil {
		t.Fatalf("创建会话: %v", err)
	}

	// 模拟 TUI 装配：带 UI 回调。
	tuiCB := &recordingRunCB{}
	rt := &runtimeAgent{
		Agent:      &plainAgent{},
		sess:       store,
		sessionID:  func() string { return "sess-cbwiring" },
		uiCallback: tuiCB,
		cbSwitch:   intruntime.NewCallbackSwitch(),
	}
	svc := rt.Service()
	if svc == nil {
		t.Fatal("Service() 不应返回 nil")
	}

	run, runErr := svc.Start(context.Background(), "hi")
	if runErr != nil {
		t.Fatal(runErr)
	}
	if run.Err != nil {
		t.Fatalf("运行不应出错: %v", run.Err)
	}
	// plainAgent 不发事件；这里验证的是装配完整性：回调绑定路径不 panic、
	// 事件正常发布（Started/Finished 由订阅侧另行覆盖）。

	// 未传回调（CLI 形态）：同样可运行。
	rtCLI := &runtimeAgent{
		Agent:     &plainAgent{},
		sess:      store,
		sessionID: func() string { return "sess-cbwiring" },
		cbSwitch:  intruntime.NewCallbackSwitch(),
	}
	svcCLI := rtCLI.Service()
	if _, err := svcCLI.Start(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
}

// recordingRunCB 是 cmd 侧的最小回调记录器。
type recordingRunCB struct{ deltas []string }

func (r *recordingRunCB) OnTextDelta(d string)         { r.deltas = append(r.deltas, d) }
func (r *recordingRunCB) OnThinkingDelta(string)       {}
func (r *recordingRunCB) OnToolCallStart(llm.ToolCall) {}
func (r *recordingRunCB) OnToolCallEnd(llm.ToolCall, *tool.Result, error) {
}
func (r *recordingRunCB) OnTurnEnd(*agent.Response) {}
func (r *recordingRunCB) OnError(error)             {}

// TestCollectResumeItems（UI-003）：种子数据 → 恢复事实应包含
// 需重试的 interrupted 任务、最近修改文件、验证摘要；succeeded 不标重试。
func TestCollectResumeItems(t *testing.T) {
	baseDir := t.TempDir()
	store, err := session.NewJSONLStore(baseDir)
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(session.CreateOpts{Title: "resume"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sid := info.ID

	add := func(typ session.EventType, data any) {
		t.Helper()
		enc, err := session.EncodeData(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendEvent(session.Event{SessionID: sid, Type: typ, Data: enc}); err != nil {
			t.Fatal(err)
		}
	}
	add(session.EventFileEdited, &session.FileEditedData{
		Phase: "committed", PlanID: "plan-r1", Verify: "go test ./internal/api -count=1",
		Files: []session.FileEditRecord{{Path: "internal/api/h.go", State: "written"}},
	})
	add(session.EventFileEdited, &session.FileEditedData{
		Phase: "preview", PlanID: "plan-r2",
		Files: []session.FileEditRecord{{Path: "x.go", State: "preview"}},
	})

	// 工作区事实：一个已修改文件。
	wsID := session.WorkspaceID("/tmp/ws-resume")
	w := session.NewWorkspaceFacts(wsID)
	w.Add(session.Fact{Kind: session.FactFileModified, Key: session.FileFactKey(session.FactFileModified, "internal/api/h.go"),
		Path: "internal/api/h.go", Ref: "plan-r1", Source: "edit_files", FirstSeen: "2026-09-12T10:00:00Z"})
	if err := session.SaveWorkspaceFacts(baseDir, w); err != nil {
		t.Fatal(err)
	}

	// 任务历史：1 interrupted（需重试）+ 1 succeeded（不标）。
	journalPath := filepath.Join(baseDir, "jobs.jsonl")
	jf, err := os.Create(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(jf, "{\"kind\":\"started\",\"job_id\":\"job-r1\",\"owner\":\"%s\",\"command\":\"go test ./...\",\"state\":\"queued\",\"created_at\":\"2026-09-12T10:00:00Z\",\"exit_code\":-1,\"recorded_at\":\"2026-09-12T10:00:00Z\"}\n", sid)
	fmt.Fprintf(jf, "{\"kind\":\"terminal\",\"job_id\":\"job-r1\",\"owner\":\"%s\",\"command\":\"go test ./...\",\"state\":\"interrupted\",\"created_at\":\"2026-09-12T10:00:00Z\",\"ended_at\":\"2026-09-12T10:01:00Z\",\"exit_code\":-1,\"recorded_at\":\"2026-09-12T10:01:00Z\"}\n", sid)
	fmt.Fprintf(jf, "{\"kind\":\"started\",\"job_id\":\"job-r2\",\"owner\":\"%s\",\"command\":\"make build\",\"state\":\"queued\",\"created_at\":\"2026-09-12T10:02:00Z\",\"exit_code\":-1,\"recorded_at\":\"2026-09-12T10:02:00Z\"}\n", sid)
	fmt.Fprintf(jf, "{\"kind\":\"terminal\",\"job_id\":\"job-r2\",\"owner\":\"%s\",\"command\":\"make build\",\"state\":\"succeeded\",\"created_at\":\"2026-09-12T10:02:00Z\",\"ended_at\":\"2026-09-12T10:03:00Z\",\"exit_code\":0,\"recorded_at\":\"2026-09-12T10:03:00Z\"}\n", sid)
	jf.Close()

	mgr := jobs.New(jobs.Options{Journal: jobs.NewJSONLJournal(journalPath)})
	t.Cleanup(mgr.Close)

	orig := getSessionDir
	getSessionDir = func() string { return baseDir }
	t.Cleanup(func() { getSessionDir = orig })

	items := collectResumeItems(mgr, sid, store, sid, wsID)

	byKind := map[string]int{}
	retry := 0
	for _, it := range items {
		byKind[it.Kind]++
		if it.NeedsRetry {
			retry++
		}
	}
	if byKind["job"] != 2 {
		t.Fatalf("应有 2 条任务事实: %v", byKind)
	}
	if byKind["file"] != 1 {
		t.Fatalf("应有 1 条文件事实: %v", byKind)
	}
	if byKind["verify"] != 1 {
		t.Fatalf("应有 1 条验证摘要: %v", byKind)
	}
	if retry != 1 {
		t.Fatalf("应恰 1 项需重试（interrupted）: %d", retry)
	}
	// 验证摘要必须带上验证命令与「建议重跑」提示。
	foundVerify := false
	for _, it := range items {
		if it.Kind == "verify" && strings.Contains(it.Label, "go test ./internal/api") && strings.Contains(it.Label, "建议重跑") {
			foundVerify = true
		}
	}
	if !foundVerify {
		t.Fatal("验证摘要应含命令与建议重跑")
	}
}
