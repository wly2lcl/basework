package agent

import (
	"context"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// fakeOwnerModel 只需满足 llm.Model，测试里不会真正发起请求。
type fakeOwnerModel struct{}

func (fakeOwnerModel) ID() string { return "fake-owner-model" }

func (fakeOwnerModel) Supports(llm.Capability) bool { return false }

func (fakeOwnerModel) Generate(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}

func (fakeOwnerModel) Stream(context.Context, *llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

// TestNewResultSatisfiesSessionIDProvider 是 bindRuntimeJobOwner 缺陷的回归测试。
//
// New 返回的具体类型必须自己实现 SessionIDProvider：产品层（cmd/basework）在
// agent.New 之后立即做类型断言来回填后台任务归属。曾因只有包装类型
// agentInstance 实现了该接口、New 返回的 *AgentLoop 没有，导致断言静默失败，
// 所有 bash_background 调用都报「当前会话为空，无法建立后台任务归属」。
func TestNewResultSatisfiesSessionIDProvider(t *testing.T) {
	agt, err := New(WithModel(fakeOwnerModel{}), WithSession(session.NewMemoryStore()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = agt.Close() })

	provider, ok := agt.(SessionIDProvider)
	if !ok {
		t.Fatal("agent.New 的返回值未实现 SessionIDProvider：产品层无法回填后台任务归属")
	}
	if provider.SessionID() == "" {
		t.Fatal("SessionID() 为空：后台任务归属会缺省")
	}
	// 稳定性：同一实例多次调用结果一致（接口契约要求）。
	if again := provider.SessionID(); again != provider.SessionID() {
		t.Fatalf("SessionID() 不稳定: %q != %q", again, provider.SessionID())
	}
}
