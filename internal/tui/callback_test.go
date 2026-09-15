// Package tui 测试
package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wly2lcl/basework/internal/tui/dialog"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// collectCallback 创建一个回调并把投递的消息收集到切片中。
func collectCallback() (*[]tea.Msg, agent.Callback) {
	got := &[]tea.Msg{}
	cb := NewAgentCallback(func(msg tea.Msg) {
		*got = append(*got, msg)
	})
	return got, cb
}

// TestAgentCallback_ImplementsCallback 保证返回类型满足 agent.Callback。
func TestAgentCallback_ImplementsCallback(t *testing.T) {
	_, cb := collectCallback()
	if cb == nil {
		t.Fatal("NewAgentCallback 返回 nil")
	}
}

func TestAgentCallback_ForRunAddsRoutingMetadata(t *testing.T) {
	got, cb := collectCallback()
	scoped, ok := cb.(interface {
		ForRun(string, string) agent.Callback
	})
	if !ok {
		t.Fatal("callback should expose per-run scope")
	}
	scoped.ForRun("run-7", "session-9").OnTextDelta("chunk")
	msg, ok := (*got)[0].(StreamDeltaMsg)
	if !ok || msg.RunID != "run-7" || msg.SessionID != "session-9" {
		t.Fatalf("routing metadata missing: %#v", (*got)[0])
	}
}

// TestAgentCallback_OnTextDeltaEmitsStreamDeltaMsg 验证文本增量被翻译为 StreamDeltaMsg。
func TestAgentCallback_OnTextDeltaEmitsStreamDeltaMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnTextDelta("Hello ")
	cb.OnTextDelta("World")

	if len(*got) != 2 {
		t.Fatalf("期望 2 条消息，得到 %d", len(*got))
	}
	for i, want := range []string{"Hello ", "World"} {
		msg, ok := (*got)[i].(StreamDeltaMsg)
		if !ok {
			t.Fatalf("第 %d 条消息类型应为 StreamDeltaMsg，得到 %T", i, (*got)[i])
		}
		if msg.Delta != want {
			t.Fatalf("第 %d 条增量应为 %q，得到 %q", i, want, msg.Delta)
		}
	}
}

// TestAgentCallback_OnThinkingDeltaEmitsMsg 验证思考增量。
func TestAgentCallback_OnThinkingDeltaEmitsMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnThinkingDelta("let me think")

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ThinkingDeltaMsg)
	if !ok {
		t.Fatalf("消息类型应为 ThinkingDeltaMsg，得到 %T", (*got)[0])
	}
	if msg.Delta != "let me think" {
		t.Fatalf("增量不符，得到 %q", msg.Delta)
	}
}

// TestAgentCallback_OnToolCallStartEmitsMsg 验证工具开始事件。
func TestAgentCallback_OnToolCallStartEmitsMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallStart(llm.ToolCall{ID: "1", Name: "bash", ArgsJSON: `{"cmd":"ls"}`})

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolStartMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolStartMsg，得到 %T", (*got)[0])
	}
	if msg.Name != "bash" {
		t.Fatalf("工具名应为 bash，得到 %q", msg.Name)
	}
}

// TestAgentCallback_OnToolCallEndSuccess 验证工具成功结束事件。
func TestAgentCallback_OnToolCallEndSuccess(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(
		llm.ToolCall{Name: "read", ArgsJSON: `{"path":"a.go"}`},
		&tool.Result{Content: "file body"},
		nil,
	)

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolEndMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolEndMsg，得到 %T", (*got)[0])
	}
	if msg.Name != "read" || msg.Result != "file body" || msg.IsError {
		t.Fatalf("ToolEndMsg 内容不符: %+v", msg)
	}
}

// TestAgentCallback_OnToolCallEndError 验证工具失败结束事件。
func TestAgentCallback_OnToolCallEndError(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(llm.ToolCall{Name: "bash"}, nil, errors.New("boom"))

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolEndMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolEndMsg，得到 %T", (*got)[0])
	}
	if !msg.IsError || msg.Result != "boom" {
		t.Fatalf("错误分支不符: %+v", msg)
	}
}

// TestAgentCallback_OnToolCallEndNilResult 验证 result 为 nil 且无错误时不 panic。
func TestAgentCallback_OnToolCallEndNilResult(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(llm.ToolCall{Name: "noop"}, nil, nil)

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg := (*got)[0].(ToolEndMsg)
	if msg.IsError || msg.Result != "" {
		t.Fatalf("nil result 应产出空结果且非错误: %+v", msg)
	}
}

// TestAgentCallback_OnTurnEndEmitsNothing 是本 bug 的核心护栏：
// OnTurnEnd 不得提交消息，否则会与 AgentResponseMsg 的提交重复。
func TestAgentCallback_OnTurnEndEmitsNothing(t *testing.T) {
	got, cb := collectCallback()

	cb.OnTurnEnd(&agent.Response{
		Message: llm.ChatMessage{
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "final"}},
		},
	})
	cb.OnTurnEnd(nil)

	if len(*got) != 0 {
		t.Fatalf("OnTurnEnd 不应投递任何消息，实际投递 %d 条", len(*got))
	}
}

// TestAgentCallback_OnErrorEmitsErrorMsg 验证错误事件。
func TestAgentCallback_OnErrorEmitsErrorMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnError(errors.New("network down"))

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ErrorMsg)
	if !ok {
		t.Fatalf("消息类型应为 ErrorMsg，得到 %T", (*got)[0])
	}
	if msg.Err == nil || msg.Err.Error() != "network down" {
		t.Fatalf("错误内容不符: %v", msg.Err)
	}
}

// TestAgentCallback_NilSendDoesNotPanic 验证 send 为 nil 时不 panic。
func TestAgentCallback_NilSendDoesNotPanic(t *testing.T) {
	cb := NewAgentCallback(nil)

	cb.OnTextDelta("x")
	cb.OnThinkingDelta("x")
	cb.OnToolCallStart(llm.ToolCall{Name: "bash"})
	cb.OnToolCallEnd(llm.ToolCall{Name: "bash"}, nil, nil)
	cb.OnError(errors.New("x"))
	cb.OnTurnEnd(nil)
}

// TestAppUpdate_StreamDeltaAccumulates 验证增量经事件循环累积到流式视图。
// 这是"OnTextDelta 触发后内容递增"的直接断言。
func TestAppUpdate_StreamDeltaAccumulates(t *testing.T) {
	app := NewApp("test-model", "test-provider", "")
	app.StartStreaming()

	app.Update(StreamDeltaMsg{Delta: "Hel"})
	if got := app.Streaming.FullText(); got != "Hel" {
		t.Fatalf("首次增量后期望 %q，得到 %q", "Hel", got)
	}

	app.Update(StreamDeltaMsg{Delta: "lo"})
	if got := app.Streaming.FullText(); got != "Hello" {
		t.Fatalf("二次增量后期望 %q，得到 %q", "Hello", got)
	}
}

// TestAppUpdate_ThinkingDeltaAccumulates 验证思考增量累积。
func TestAppUpdate_ThinkingDeltaAccumulates(t *testing.T) {
	app := NewApp("m", "p", "")
	app.StartStreaming()

	app.Update(ThinkingDeltaMsg{Delta: "step1 "})
	app.Update(ThinkingDeltaMsg{Delta: "step2"})

	if got := app.Streaming.thinkingText; got != "step1 step2" {
		t.Fatalf("思考文本期望 %q，得到 %q", "step1 step2", got)
	}
}

// TestAppUpdate_ToolLifecycle 验证工具开始/结束在事件循环内的状态流转。
func TestAppUpdate_ToolLifecycle(t *testing.T) {
	app := NewApp("m", "p", "")
	app.StartStreaming()

	before := len(app.Messages)

	app.Update(ToolStartMsg{Name: "bash"})
	if app.Streaming.toolInProgress != "bash" {
		t.Fatalf("工具开始时 toolInProgress 应为 bash，得到 %q", app.Streaming.toolInProgress)
	}

	app.Update(ToolEndMsg{Name: "bash", Args: `{"cmd":"ls"}`, Result: "ok"})
	if app.Streaming.toolInProgress != "" {
		t.Fatalf("工具结束后 toolInProgress 应清空，得到 %q", app.Streaming.toolInProgress)
	}
	if len(app.Messages) != before+1 {
		t.Fatalf("工具结束应追加 1 条消息，期望 %d，得到 %d", before+1, len(app.Messages))
	}
	last := app.Messages[len(app.Messages)-1]
	if last.Role != "tool" || last.ToolMsg == nil {
		t.Fatalf("最后一条消息应为 tool 角色，得到 %+v", last)
	}
	if last.ToolMsg.ToolName != "bash" || last.ToolMsg.Result != "ok" || last.ToolMsg.IsError {
		t.Fatalf("工具消息内容不符: %+v", last.ToolMsg)
	}
}

// TestAppUpdate_ToolEndErrorFlag 验证工具失败标记透传。
func TestAppUpdate_ToolEndErrorFlag(t *testing.T) {
	app := NewApp("m", "p", "")

	app.Update(ToolEndMsg{Name: "bash", Result: "failed", IsError: true})

	if len(app.Messages) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(app.Messages))
	}
	tm := app.Messages[0].ToolMsg
	if tm == nil || !tm.IsError || tm.Result != "failed" {
		t.Fatalf("错误标记未透传: %+v", tm)
	}
}

// TestAppUpdate_StreamDeltaWhileNotStreaming 验证非流式状态下增量不破坏消息列表。
func TestAppUpdate_StreamDeltaWhileNotStreaming(t *testing.T) {
	app := NewApp("m", "p", "")

	app.Update(StreamDeltaMsg{Delta: "stray"})

	if app.IsStreaming {
		t.Fatal("未收到 UserInputMsg 时不应进入流式状态")
	}
	if len(app.Messages) != 0 {
		t.Fatalf("增量不应写入消息列表，得到 %d 条", len(app.Messages))
	}
}

// ---- RUN-003：运行事件按会话路由 ----

// TestApp_Update_RunEventRouting 事件只被其所属会话的 App 接受；
// 旧会话运行的收尾事件不能影响新会话的界面状态。
func TestApp_Update_RunEventRouting(t *testing.T) {
	app := NewApp("model", "opencode", "sess-1")

	// 本会话事件：接受。
	_, _ = app.Update(RunEventMsg{RunID: "run-1", SessionID: "sess-1", Kind: "run.started"})
	if app.lastRunEvent == nil || app.lastRunEvent.RunID != "run-1" {
		t.Fatalf("本会话事件应被接受: %+v", app.lastRunEvent)
	}

	// 他会话事件：忽略，不覆盖观察点。
	_, _ = app.Update(RunEventMsg{RunID: "run-2", SessionID: "sess-other", Kind: "run.started"})
	if app.lastRunEvent.RunID != "run-1" {
		t.Fatalf("他会话事件应被忽略: %+v", app.lastRunEvent)
	}

	// 本会话 finished：接受。
	_, _ = app.Update(RunEventMsg{RunID: "run-1", SessionID: "sess-1", Kind: "run.finished"})
	if app.lastRunEvent.Kind != "run.finished" {
		t.Fatalf("本会话 finished 应被接受: %+v", app.lastRunEvent)
	}

	// 无会话绑定（SessionID 为空）的 App：全部接受（兜底行为）。
	app2 := NewApp("model", "opencode", "")
	_, _ = app2.Update(RunEventMsg{RunID: "run-3", SessionID: "sess-9", Kind: "run.started"})
	if app2.lastRunEvent == nil || app2.lastRunEvent.RunID != "run-3" {
		t.Fatalf("未绑定会话时应接受全部事件: %+v", app2.lastRunEvent)
	}
}

// ---- UI-002：审批对话框流 ----

// TestApp_Update_ApprovalFlowIsolated 验收条件的 App 级验证：
// 批准只作用于其请求 ID；新请求需要新的确认，旧批准不泄漏。
func TestApp_Update_ApprovalFlowIsolated(t *testing.T) {
	app := NewApp("model", "opencode", "sess-1")

	var responded []bool
	respond := func(approved bool) bool {
		responded = append(responded, approved)
		return true
	}

	// 请求 1：打开审批对话框。
	_, _ = app.Update(ApprovalRequestMsg{
		ID: "apr-1", ToolName: "bash", Purpose: "执行 shell 命令",
		RiskReason: "命令包含: 删除文件", Respond: respond,
	})
	if app.DialogMgr.Depth() == 0 {
		t.Fatal("审批请求应打开对话框")
	}
	top, ok := app.DialogMgr.Top().(*dialog.ApprovalDialog)
	if !ok || top.Request().ID != "apr-1" {
		t.Fatalf("对话框应绑定请求 apr-1: %+v", app.DialogMgr.Top())
	}

	// 按 y：允许提交、对话框关闭（bubbletea 会执行返回的 Cmd 并把
	// 消息喂回 Update；单测里手动模拟这一步）。
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'y'})
	if len(responded) != 1 || !responded[0] {
		t.Fatalf("y 应提交允许: %v", responded)
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			_, _ = app.Update(msg)
		}
	}
	if app.DialogMgr.HasDialog() {
		t.Fatal("决定后对话框应关闭")
	}

	// 请求 2（相同工具相同参数的假设场景）：必须重新弹窗，不受旧批准影响。
	_, _ = app.Update(ApprovalRequestMsg{
		ID: "apr-2", ToolName: "bash", Purpose: "执行 shell 命令", Respond: respond,
	})
	if app.DialogMgr.Depth() == 0 {
		t.Fatal("新请求应重新弹窗")
	}
	top2, ok := app.DialogMgr.Top().(*dialog.ApprovalDialog)
	if !ok || top2.Request().ID != "apr-2" || top2.Decided() {
		t.Fatalf("新对话框应是未决定的新请求: %+v", top2)
	}

	// 按 esc：关闭 = 拒绝。
	_, cmd2 := app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(responded) != 2 || responded[1] {
		t.Fatalf("esc 应提交拒绝: %v", responded)
	}
	if cmd2 != nil {
		if msg := cmd2(); msg != nil {
			_, _ = app.Update(msg)
		}
	}
	if app.DialogMgr.HasDialog() {
		t.Fatal("拒绝后对话框应关闭")
	}
}

func TestApp_ApprovalModalHasPriorityOverJobs(t *testing.T) {
	app := NewApp("model", "opencode", "sess-1")
	app.Jobs.Toggle()
	if !app.Jobs.Visible() {
		t.Fatal("测试前应打开任务卡片")
	}
	responded := false
	_, _ = app.Update(ApprovalRequestMsg{
		ID: "apr-modal", ToolName: "edit_files", Purpose: "提交编辑",
		Respond: func(approved bool) bool { responded = approved; return true },
	})
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'y'})
	if !responded {
		t.Fatal("任务卡片可见时 y 仍应由审批弹窗消费")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			_, _ = app.Update(msg)
		}
	}
	if app.DialogMgr.HasDialog() {
		t.Fatal("审批决定后弹窗应关闭")
	}
}

// ---- UI-003：会话切换与可恢复进度 ----

// TestApp_SwitchSession_Isolation 切换后不串消息/任务状态：
// 旧会话迟到的 RunEventMsg 与 JobStatusMsg 一律忽略。
func TestApp_SwitchSession_Isolation(t *testing.T) {
	app := NewApp("model", "opencode", "sess-old")
	app.SetJobsRuntime(nil, nil)

	app.SwitchSession("sess-new")
	if app.SessionID != "sess-new" {
		t.Fatalf("应绑定新会话: %s", app.SessionID)
	}

	// 旧会话运行事件：忽略。
	_, _ = app.Update(RunEventMsg{RunID: "run-old", SessionID: "sess-old", Kind: "run.finished", Err: "boom"})
	if app.lastRunEvent != nil {
		t.Fatal("旧会话事件不应进入新会话")
	}
	// 旧会话任务快照：忽略。
	_, _ = app.Update(JobStatusMsg{SessionID: "sess-old", Jobs: []JobStatus{{ID: "j1", State: "running"}}})
	if len(app.Jobs.jobs) != 0 {
		t.Fatalf("旧会话快照不应进入新卡片: %+v", app.Jobs.jobs)
	}

	// 新会话事件：接受。
	_, _ = app.Update(RunEventMsg{RunID: "run-new", SessionID: "sess-new", Kind: "run.started"})
	if app.lastRunEvent == nil || app.lastRunEvent.RunID != "run-new" {
		t.Fatalf("新会话事件应被接受: %+v", app.lastRunEvent)
	}
}

// TestResumePanel_RetryMarking 状态区分：interrupted/failed 标需重试，
// succeeded 不标；任意键收起。
func TestResumePanel_RetryMarking(t *testing.T) {
	p := NewResumePanel()
	p.SetItems([]ResumeItem{
		{Kind: "job", Label: "go test ./...", State: "interrupted", NeedsRetry: true},
		{Kind: "job", Label: "make build", State: "succeeded", NeedsRetry: false},
		{Kind: "file", Label: "a.go", State: "已修改"},
		{Kind: "verify", Label: "go test ./... -count=1", State: "待确认"},
	})

	if p.RetryCount() != 1 {
		t.Fatalf("应恰 1 项需重试: %d", p.RetryCount())
	}
	rendered := p.Render(100)
	for _, want := range []string{"[需重试]", "interrupted", "succeeded", "a.go", "go test ./... -count=1"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("渲染缺 %q:\n%s", want, rendered)
		}
	}

	p.Hide()
	if p.Render(100) != "" {
		t.Fatal("隐藏后不应渲染")
	}
}
