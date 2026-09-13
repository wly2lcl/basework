package runtime

import (
	"sync"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// CallbackSwitch 是按运行绑定的回调路由器（RUN-003）。
//
// 背景：agent.Callback 在 Agent 构造时注册，一次注册终身生效；而 RUN-002
// 之后同一会话的运行是串行排队执行的。若把 UI 回调直接注册给 Agent，
// 一次运行的收尾增量（取消后仍在 flush 的流式片段）会落进下一次运行的
// 界面——这就是「旧任务回调串到新会话」。
//
// 解法：Agent 注册的永远是本路由器；每次 Start 只把当次运行的回调绑进来，
// 运行结束（无论成功、取消、报错）立即解绑。串行化保证同一时刻至多一个
// 绑定，解绑后到达的事件一律丢弃——它们属于已经结束的运行。
//
// 线程安全：Bind/Unbind 与回调转发可在不同 goroutine。
type CallbackSwitch struct {
	mu      sync.Mutex
	current agent.Callback
}

// NewCallbackSwitch 构造空路由器。未绑定任何回调时转发是空操作。
func NewCallbackSwitch() *CallbackSwitch {
	return &CallbackSwitch{}
}

// Bind 绑定当次运行的回调。若已有绑定（不应发生：运行串行），后绑者胜出。
func (s *CallbackSwitch) Bind(cb agent.Callback) {
	if cb == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = cb
}

// Unbind 解绑指定回调。只有当前绑定与传入者一致才清除——防御性地避免
// 误清新一次运行刚绑上的回调。
func (s *CallbackSwitch) Unbind(cb agent.Callback) {
	if cb == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == cb {
		s.current = nil
	}
}

func (s *CallbackSwitch) active() agent.Callback {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *CallbackSwitch) OnTextDelta(delta string) {
	if cb := s.active(); cb != nil {
		cb.OnTextDelta(delta)
	}
}

func (s *CallbackSwitch) OnThinkingDelta(delta string) {
	if cb := s.active(); cb != nil {
		cb.OnThinkingDelta(delta)
	}
}

func (s *CallbackSwitch) OnToolCallStart(call llm.ToolCall) {
	if cb := s.active(); cb != nil {
		cb.OnToolCallStart(call)
	}
}

func (s *CallbackSwitch) OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error) {
	if cb := s.active(); cb != nil {
		cb.OnToolCallEnd(call, result, err)
	}
}

func (s *CallbackSwitch) OnTurnEnd(resp *agent.Response) {
	if cb := s.active(); cb != nil {
		cb.OnTurnEnd(resp)
	}
}

func (s *CallbackSwitch) OnError(err error) {
	if cb := s.active(); cb != nil {
		cb.OnError(err)
	}
}

// 编译期断言：CallbackSwitch 满足 agent.Callback。
var _ agent.Callback = (*CallbackSwitch)(nil)
