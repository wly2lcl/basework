package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// 流式消息类型定义。
//
// 这些消息由 agentCallback 发出，经 tea.Program.Send 进入 Bubble Tea 事件循环，
// 再由 App.Update 在事件循环内修改状态。
//
// 为什么必须绕这一圈：agent 的回调是在 tea.Cmd 所在的 goroutine 中被调用的
// （App.Update 的 UserInputMsg 分支返回的闭包 → InputHandler → HandleMessage
// → pipeline → Callback），而 Bubble Tea 事件循环会并发调用 App.Update 与
// App.View。若回调直接修改 App，就是明确的数据竞态。因此回调只负责投递消息，
// 所有状态变更集中在事件循环内完成。

// StreamDeltaMsg 表示一段流式文本增量。
type StreamDeltaMsg struct {
	Delta     string
	RunID     string
	SessionID string
}

// ThinkingDeltaMsg 表示一段思考过程增量。
type ThinkingDeltaMsg struct {
	Delta     string
	RunID     string
	SessionID string
}

// ToolStartMsg 表示工具开始执行。
type ToolStartMsg struct {
	Name      string
	RunID     string
	SessionID string
}

// ToolEndMsg 表示工具执行结束。
type ToolEndMsg struct {
	Name      string
	Args      string
	Result    string
	IsError   bool
	RunID     string
	SessionID string
}

// agentCallback 实现 agent.Callback，把 agent 生命周期事件翻译为 tea.Msg。
type agentCallback struct {
	send      func(tea.Msg)
	runID     string
	sessionID string
}

// NewAgentCallback 创建一个把 agent 事件转发给 TUI 的回调。
//
// send 通常传入 tea.Program.Send。由于 tea.Program 需要先有 App 才能创建，
// 而 runtimeAgent 又需要回调才能构造，调用方可先传入一个转发闭包，
// 待 Program 创建后再把真正的 Send 绑定进去。
func NewAgentCallback(send func(tea.Msg)) agent.Callback {
	return &agentCallback{send: send}
}

// ForRun 返回带有不可变运行/会话标识的回调副本。runtime.Service 在每次
// StartWithCallback 绑定前调用它，确保异步消息能在 TUI 切换会话时被过滤。
func (cb *agentCallback) ForRun(runID, sessionID string) agent.Callback {
	if cb == nil {
		return nil
	}
	return &agentCallback{send: cb.send, runID: runID, sessionID: sessionID}
}

func (cb *agentCallback) emit(msg tea.Msg) {
	if cb.send != nil {
		switch m := msg.(type) {
		case StreamDeltaMsg:
			m.RunID, m.SessionID = cb.runID, cb.sessionID
			msg = m
		case ThinkingDeltaMsg:
			m.RunID, m.SessionID = cb.runID, cb.sessionID
			msg = m
		case ToolStartMsg:
			m.RunID, m.SessionID = cb.runID, cb.sessionID
			msg = m
		case ToolEndMsg:
			m.RunID, m.SessionID = cb.runID, cb.sessionID
			msg = m
		case ErrorMsg:
			m.RunID, m.SessionID = cb.runID, cb.sessionID
			msg = m
		}
		cb.send(msg)
	}
}

func (cb *agentCallback) OnTextDelta(delta string) {
	cb.emit(StreamDeltaMsg{Delta: delta})
}

func (cb *agentCallback) OnThinkingDelta(delta string) {
	cb.emit(ThinkingDeltaMsg{Delta: delta})
}

func (cb *agentCallback) OnToolCallStart(call llm.ToolCall) {
	cb.emit(ToolStartMsg{Name: call.Name})
}

func (cb *agentCallback) OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error) {
	out := ToolEndMsg{Name: call.Name, Args: call.ArgsJSON}
	switch {
	case err != nil:
		out.Result = err.Error()
		out.IsError = true
	case result != nil:
		out.Result = result.Content
		out.IsError = result.IsError
	}
	cb.emit(out)
}

// OnTurnEnd 故意留空。
//
// 本轮最终文本由 App.Update 处理 AgentResponseMsg 时统一提交
// （StopStreaming + AddAssistantMessage）。若在此处再次提交，
// 会和流式累积的文本重复入消息列表。
func (cb *agentCallback) OnTurnEnd(resp *agent.Response) {}

func (cb *agentCallback) OnError(err error) {
	cb.emit(ErrorMsg{Err: err})
}

// 编译期断言：agentCallback 满足 agent.Callback。
var _ agent.Callback = (*agentCallback)(nil)
