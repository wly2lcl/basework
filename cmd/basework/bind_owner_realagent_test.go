package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// realAgentOwnerModel 只需满足 llm.Model；本测试不发起任何 LLM 请求。
type realAgentOwnerModel struct{}

func (realAgentOwnerModel) ID() string { return "real-agent-owner-model" }

func (realAgentOwnerModel) Supports(llm.Capability) bool { return false }

func (realAgentOwnerModel) Generate(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}

func (realAgentOwnerModel) Stream(context.Context, *llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

// newBoundAgentAndManager 复刻 newRuntimeAgent 的关键顺序：agent.New →
// bindRuntimeJobOwner，返回绑定后的 (agent, holder, manager)。
func newBoundAgentAndManager(t *testing.T) (agent.Agent, *jobOwnerHolder, *jobs.Manager) {
	t.Helper()
	agt, err := agent.New(agent.WithModel(realAgentOwnerModel{}), agent.WithSession(session.NewMemoryStore()))
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	t.Cleanup(func() { _ = agt.Close() })

	dir := t.TempDir()
	holder := &jobOwnerHolder{}
	mgr := newRuntimeJobManagerAt(holder, filepath.Join(dir, "jobs.jsonl"), filepath.Join(dir, "out"))
	t.Cleanup(mgr.Close)
	bindRuntimeJobOwner(agt, holder, mgr)
	return agt, holder, mgr
}

// TestBindRuntimeJobOwner_WithRealAgentType 用 agent.New 的**真实返回类型**走一遍
// bindRuntimeJobOwner——SHIP-003 发现的缺陷回归测试。
//
// 既有测试都用自行实现 SessionIDProvider 的替身（stubSessionAgent），替身当然
// 通过；而 agent.New 实际返回的 *AgentLoop 当时并没有实现该接口，类型断言在
// 生产路径上永远静默失败，导致 bash_background 一直报「当前会话为空，无法建立
// 后台任务归属」。这条测试锁住「真实类型必须能被绑定」这一事实。
func TestBindRuntimeJobOwner_WithRealAgentType(t *testing.T) {
	agt, holder, _ := newBoundAgentAndManager(t)

	provider, ok := agt.(agent.SessionIDProvider)
	if !ok {
		t.Fatal("agent.New 的返回值未实现 SessionIDProvider：bindRuntimeJobOwner 会静默早退")
	}
	if provider.SessionID() == "" {
		t.Fatal("真实 agent 的 SessionID 为空")
	}
	if got := holder.Get(); got != provider.SessionID() {
		t.Fatalf("真实 agent 类型下归属回填失败：holder=%q，会话=%q", got, provider.SessionID())
	}
}

// TestBackgroundBashTool_StartSucceedsAfterBind 是端到端行为回归：按生产顺序
// 绑定完成后，真实的 bash_background 工具必须真的把 job 登记进管理器并写日志，
// 而不是报「当前会话为空，无法建立后台任务归属」。
func TestBackgroundBashTool_StartSucceedsAfterBind(t *testing.T) {
	_, holder, mgr := newBoundAgentAndManager(t)

	if holder.Get() == "" {
		t.Fatal("前置失败：归属未绑定")
	}

	bg := tools.NewBackgroundBashTool("", mgr) // sessionID 留空：归属只能来自 OwnerFunc
	bg.OwnerFunc = holder.Get
	bg.PermissionMode = "yolo"
	bg.NoTimeout = true

	res, err := bg.Execute(context.Background(), []byte(`{"command":"true"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("bash_background 不应报错，得到: %s", strings.SplitN(res.Content, "\n", 2)[0])
	}

	// 等 runner 收尾，然后核对登记记录。
	deadline := time.Now().Add(5 * time.Second)
	var recs []jobs.Job
	for time.Now().Before(deadline) {
		recs, err = mgr.AllHistory()
		if err != nil {
			t.Fatalf("读历史: %v", err)
		}
		if len(recs) == 1 && recs[0].State.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(recs) != 1 {
		t.Fatalf("应恰好登记 1 条任务，得到 %d 条", len(recs))
	}
	if recs[0].Owner != holder.Get() {
		t.Fatalf("任务归属应是会话 ID %q，得到 %q", holder.Get(), recs[0].Owner)
	}
	if !recs[0].State.Terminal() {
		t.Fatalf("`true` 应立即到达终态，得到 %q", recs[0].State)
	}
}
