// Package tests 中的本文件是 SHIP-001 的「固定编码场景回归集」。
//
// 定位：这是**离线重放**层，不是真实模型验证层。这里的模型是脚本化的——它不做推理，
// 只按预先固定的决策序列逐轮返回，于是整个场景是确定性的：输入固定、工具轨迹固定、
// 断言落在磁盘产物与工具记录上，而不是落在模型文案上。
//
// 为什么这样切分：真实模型的行为会随模型版本、温度与网络抖动变化，把它写进 `go test`
// 会让回归集变成偶发红灯的噪声源；反过来，只跑 mock 又证明不了真实兼容性。
// 因此本文件只覆盖「可确定的编排逻辑」（工具派发顺序、编辑落地、权限拦截、大输出截断、
// 断流中断与恢复），真实模型结果按 validation.md 单独跑、另存为证据。
//
// 场景与任务卡 SHIP-001 的步骤 1 一一对应：
//   - TestCodingScenario_SmallFunctionFix      小函数修复
//   - TestCodingScenario_MultiFileEdit         多文件修改
//   - TestCodingScenario_LargeOutput           大输出
//   - TestCodingScenario_PermissionDenial      权限拒绝
//   - TestCodingScenario_StreamInterruption…   断流恢复
//
// 每个场景用 t.TempDir() 建独立工作区、互不共享状态；失败时不会有残留（TempDir 自动清理）。
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

// ---------------------------------------------------------------------------
// 脚本化模型：离线重放的「回放源」
// ---------------------------------------------------------------------------

// scenarioTurn 是一次脚本化响应的内容。四种形态互斥：
// 只有文本（收尾）、有工具调用（干活）、一个注入的流错误、或 truncated（半截流）。
type scenarioTurn struct {
	text      string
	toolCalls []llm.ToolCall
	streamErr error
	// truncated 模拟「流在中途断掉且没有 [DONE]」：只发工具调用的身份片段与
	// 半截参数（Complete 恒为 false），然后直接关闭通道，不发 usage 也不发 done。
	truncated bool
}

// scenarioModel 按脚本逐轮返回固定响应。
//
// 刻意不使用任何随机性、不定时、不读真实网络：同一份脚本在任何机器上重放出的
// 工具轨迹必须逐字节一致，否则「可重复运行」这条验收就不成立。
type scenarioModel struct {
	turns []scenarioTurn
	usage llm.Usage
	idx   int
	mu    sync.Mutex
}

func (m *scenarioModel) ID() string { return "ship001-scripted" }

func (m *scenarioModel) Supports(llm.Capability) bool { return true }

// next 取下一轮脚本。脚本用尽后返回空响应（纯文本为空、无工具调用），
// 会让 agent 循环自然收尾而不是死循环——但正常场景的脚本都应显式给出收尾轮，
// 用尽即意味着脚本写漏了一轮。
func (m *scenarioModel) next() scenarioTurn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx >= len(m.turns) {
		return scenarioTurn{}
	}
	turn := m.turns[m.idx]
	m.idx++
	return turn
}

func (m *scenarioModel) Generate(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	turn := m.next()
	if turn.streamErr != nil {
		return nil, turn.streamErr
	}
	usage := m.usage
	return &llm.Response{Message: scenarioMessage(turn), Usage: usage}, nil
}

func (m *scenarioModel) Stream(_ context.Context, _ *llm.Request) (<-chan llm.StreamEvent, error) {
	turn := m.next()
	// 缓冲足够装下本轮的 text + 每个 tool call 的两个片段 + usage + done，
	// 于是可以同步写完再关闭，不需要额外 goroutine（也就没有并发时序问题）。
	ch := make(chan llm.StreamEvent, 2*len(turn.toolCalls)+3)

	if turn.streamErr != nil {
		ch <- llm.StreamEvent{Error: turn.streamErr}
		close(ch)
		return ch, nil
	}

	if turn.truncated {
		// 半截流：身份片段 + 参数增量都标 Complete=false，然后直接关闭。
		// 消费方按「流结束时刷新未完成调用」的既定契约把半截参数当作完整参数，
		// 于是这里正好验证那条路径不会把坏参数当成成功执行。
		if turn.text != "" {
			ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: turn.text}
		}
		for i, tc := range turn.toolCalls {
			ch <- llm.StreamEvent{
				Type:     llm.StreamEventToolCall,
				ToolCall: &llm.ToolCallDelta{Index: i, ID: tc.ID, Name: tc.Name},
			}
			ch <- llm.StreamEvent{
				Type:     llm.StreamEventToolCall,
				ToolCall: &llm.ToolCallDelta{Index: i, ArgsJSON: tc.ArgsJSON},
			}
		}
		close(ch)
		return ch, nil
	}

	if turn.text != "" {
		ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: turn.text}
	}
	for i, tc := range turn.toolCalls {
		// 先发身份片段（ID/Name），再发一次 Complete 的参数片段：
		// 与 provider 归一化后的语义一致（见 pkg/llm/stream.go 的 ToolCallDelta 约定）。
		ch <- llm.StreamEvent{
			Type:     llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{Index: i, ID: tc.ID, Name: tc.Name},
		}
		ch <- llm.StreamEvent{
			Type:     llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{Index: i, ArgsJSON: tc.ArgsJSON, Complete: true},
		}
	}
	usage := m.usage
	ch <- llm.StreamEvent{Type: llm.StreamEventUsage, Usage: &usage}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

func scenarioMessage(turn scenarioTurn) llm.ChatMessage {
	msg := llm.ChatMessage{Role: llm.RoleAssistant, ToolCalls: turn.toolCalls}
	if turn.text != "" {
		msg.Content = []llm.ContentPart{{Type: llm.ContentTypeText, Text: turn.text}}
	}
	return msg
}

// scenarioCall 构造一个工具调用。id 显式传入而非由 name 派生——
// 同名工具在同一场景里可能出现多次（如两次 edit），用 name 当 ID 会让
// 会话里的「调用↔结果」配对串到一起，掩盖真实的配对错误。
func scenarioCall(id, name string, args map[string]any) llm.ToolCall {
	raw, err := json.Marshal(args)
	if err != nil {
		// 参数来自测试内的字面量，编码失败属于测试自身写错，直接 panic 暴露。
		panic(fmt.Sprintf("scenarioCall(%s): 编码参数失败: %v", name, err))
	}
	return llm.ToolCall{ID: id, Name: name, ArgsJSON: string(raw)}
}

// ---------------------------------------------------------------------------
// 场景运行器
// ---------------------------------------------------------------------------

type scenarioResult struct {
	resp    *agent.Response
	err     error
	store   session.Store
	session string
	elapsed time.Duration
}

// runScenario 用给定脚本跑一次完整的 agent 回合，返回响应、错误与可查询的会话。
func runScenario(t *testing.T, extra []agent.Option, input string, turns []scenarioTurn) *scenarioResult {
	t.Helper()

	store := session.NewMemoryStore()
	model := &scenarioModel{
		turns: turns,
		usage: llm.Usage{PromptTokens: 120, CompletionTokens: 30, TotalTokens: 150},
	}

	opts := []agent.Option{
		agent.WithModel(model),
		agent.WithSession(store),
		agent.WithMaxSteps(12),
	}
	opts = append(opts, extra...)

	a, err := agent.New(opts...)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	start := time.Now()
	resp, runErr := a.HandleMessage(context.Background(), input)
	elapsed := time.Since(start)

	return &scenarioResult{
		resp:    resp,
		err:     runErr,
		store:   store,
		session: sessionIDOf(t, store, resp),
		elapsed: elapsed,
	}
}

// sessionIDOf 取会话 ID：优先用响应里的，失败回合则回落到 store 中唯一的会话。
func sessionIDOf(t *testing.T, store session.Store, resp *agent.Response) string {
	t.Helper()
	if resp != nil && resp.SessionID != "" {
		return resp.SessionID
	}
	infos, err := store.List(session.ListFilter{})
	if err != nil {
		t.Fatalf("列举会话失败: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("会话存储为空，无法定位会话")
	}
	return infos[len(infos)-1].ID
}

func (r *scenarioResult) events(t *testing.T) []session.Event {
	t.Helper()
	evts, err := r.store.Events(session.EventFilter{SessionID: r.session})
	if err != nil {
		t.Fatalf("读取会话事件失败: %v", err)
	}
	return evts
}

// callFailed 判断一次工具调用是否「可见地失败」。
//
// 两种失败形态都要算：调度层错误（Err != nil，如权限拒绝）与工具自身的错误结果
// （Result.IsError，如半截参数解析失败）。只统计 Err 会把后者记成成功，
// 正好抹掉截断场景要盯的东西。
func callFailed(rec agent.ToolCallRecord) bool {
	return rec.Err != nil || (rec.Result != nil && rec.Result.IsError)
}

// toolTrace 返回按发生顺序排列的工具名，可见失败的调用加 "!" 后缀。
func toolTrace(resp *agent.Response) (names []string, failed int) {
	if resp == nil {
		return nil, 0
	}
	for _, rec := range resp.ToolCalls {
		name := rec.Call.Name
		if callFailed(rec) {
			name += "!"
			failed++
		}
		names = append(names, name)
	}
	return names, failed
}

// logScenarioSummary 按任务卡第 4 步记录成功率、耗时、调用量、失败类型。
// 用 t.Logf 而不是断言：这些是观测值，不是通过条件。
func logScenarioSummary(t *testing.T, name string, r *scenarioResult) {
	t.Helper()
	names, failed := toolTrace(r.resp)
	tokens := 0
	if r.resp != nil {
		tokens = r.resp.Usage.TotalTokens
	}
	failKinds := map[string]int{}
	if r.resp != nil {
		for _, rec := range r.resp.ToolCalls {
			switch {
			case rec.Err != nil:
				failKinds["调度: "+rec.Err.Error()]++
			case rec.Result != nil && rec.Result.IsError:
				failKinds["工具结果: "+firstLine(rec.Result.Content)]++
			}
		}
	}
	t.Logf("[SHIP-001 场景] %s: 闭环=%v 工具调用=%d 可见失败=%d 耗时=%s token=%d 轨迹=%v 失败类型=%v",
		name, r.err == nil, len(names), failed, r.elapsed.Round(time.Millisecond), tokens, names, failKinds)
}

// firstLine 取首行并截断，避免把整段工具输出塞进日志。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 60
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ---------------------------------------------------------------------------
// 夹具辅助
// ---------------------------------------------------------------------------

func writeScenarioFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写夹具文件失败: %v", err)
	}
}

func readScenarioFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取产物失败: %v", err)
	}
	return string(data)
}

// scenarioShellQuote 把路径安全地放进 `sh -c` 命令里。
// t.TempDir() 的路径在 macOS/Linux 上通常无特殊字符，但依赖这一点会让测试在
// 含空格或引号的临时目录下静默失败，因此显式加单引号并转义内部单引号。
func scenarioShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// assertToolOrder 断言工具名顺序完全一致（顺序本身是契约的一部分：
// 读 → 改 → 验证，顺序错了说明编排有问题，即使最终产物碰巧正确）。
func assertToolOrder(t *testing.T, resp *agent.Response, want []string) {
	t.Helper()
	if resp == nil {
		t.Fatal("期望非 nil 响应")
	}
	got := make([]string, 0, len(resp.ToolCalls))
	for _, rec := range resp.ToolCalls {
		got = append(got, rec.Call.Name)
	}
	if len(got) != len(want) {
		t.Fatalf("工具调用数量 = %d，期望 %d（轨迹 %v）", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个工具调用 = %q，期望 %q（轨迹 %v）", i+1, got[i], want[i], got)
		}
	}
}

// assertNoDanglingToolCalls 校验会话事件层「每次 tool.called 都有配对结果」。
// 这是断流场景的关键不变量：中断可以发生在任何位置，但不能留下无法解释的半截调用。
func assertNoDanglingToolCalls(t *testing.T, evts []session.Event) {
	t.Helper()
	called := map[string]bool{}
	settled := map[string]bool{}
	for _, e := range evts {
		switch e.Type {
		case session.EventToolCalled:
			var d session.ToolCalledData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				t.Fatalf("解析 tool.called 失败: %v", err)
			}
			called[d.ToolCall.ID] = true
		case session.EventToolSuccess:
			var d session.ToolSuccessData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				t.Fatalf("解析 tool.success 失败: %v", err)
			}
			settled[d.ToolCallID] = true
		case session.EventToolFailed:
			var d session.ToolFailedData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				t.Fatalf("解析 tool.failed 失败: %v", err)
			}
			settled[d.ToolCallID] = true
		}
	}
	for id := range called {
		if !settled[id] {
			t.Errorf("tool.called(%s) 没有配对的结果事件（会话留下悬挂调用）", id)
		}
	}
}

func hasEventType(evts []session.Event, want session.EventType) bool {
	for _, e := range evts {
		if e.Type == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 场景 1：小函数修复
// ---------------------------------------------------------------------------

// TestCodingScenario_SmallFunctionFix 重放「读文件 → 改函数 → 跑验证命令」的最小闭环。
// 断言落在产物文件与验证命令的真实退出状态上，不检查模型说了什么。
func TestCodingScenario_SmallFunctionFix(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	writeScenarioFile(t, src, `package calc

// Add 返回 a 与 b 的和。
func Add(a, b int) int {
	return a - b
}
`)

	turns := []scenarioTurn{
		{toolCalls: []llm.ToolCall{scenarioCall("s1-read", "read", map[string]any{"path": src})}},
		{toolCalls: []llm.ToolCall{scenarioCall("s1-edit", "edit", map[string]any{
			"path": src, "old": "return a - b", "new": "return a + b",
		})}},
		// 验证命令走真实 bash 工具，退出码是独立于模型的真实信号。
		{toolCalls: []llm.ToolCall{scenarioCall("s1-verify", "bash", map[string]any{
			"command": "grep -q 'return a + b' " + scenarioShellQuote(src) + " && echo SCENARIO-VERIFIED",
		})}},
		{text: "已修正 Add 的实现。"},
	}

	r := runScenario(t, []agent.Option{agent.WithTools(builtin.All()...)}, "修复 calc.go 里 Add 的缺陷", turns)
	if r.err != nil {
		t.Fatalf("闭环返回错误: %v", r.err)
	}

	// 产物断言：磁盘上的文件才是结果。
	got := readScenarioFile(t, src)
	if !strings.Contains(got, "return a + b") {
		t.Errorf("修复未落地，产物为:\n%s", got)
	}
	if strings.Contains(got, "return a - b") {
		t.Errorf("旧缺陷仍在，产物为:\n%s", got)
	}

	assertToolOrder(t, r.resp, []string{"read", "edit", "bash"})
	for _, rec := range r.resp.ToolCalls {
		if rec.Err != nil {
			t.Errorf("工具 %s 失败: %v", rec.Call.Name, rec.Err)
		}
		if rec.Result != nil && rec.Result.IsError {
			t.Errorf("工具 %s 返回错误结果: %s", rec.Call.Name, rec.Result.Content)
		}
	}

	// 验证命令确实执行并成功：内容里必须出现我们约定的哨兵串。
	verify := r.resp.ToolCalls[2]
	if verify.Result == nil || !strings.Contains(verify.Result.Content, "SCENARIO-VERIFIED") {
		t.Errorf("验证命令未按预期成功，结果为 %+v", verify.Result)
	}

	logScenarioSummary(t, "small-function-fix", r)
}

// ---------------------------------------------------------------------------
// 场景 2：多文件修改
// ---------------------------------------------------------------------------

// TestCodingScenario_MultiFileEdit 重放一次跨两个文件的批量修改。
// 断言两个文件都真正落盘，防止「只改了一个就算通过」。
func TestCodingScenario_MultiFileEdit(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	writeScenarioFile(t, aPath, "package multi\n\nconst MarkerA = \"old-A\"\n")
	writeScenarioFile(t, bPath, "package multi\n\nconst MarkerB = \"old-B\"\n")

	turns := []scenarioTurn{
		{toolCalls: []llm.ToolCall{scenarioCall("s2-edit-a", "edit", map[string]any{
			"path": aPath, "old": "old-A", "new": "new-A",
		})}},
		{toolCalls: []llm.ToolCall{scenarioCall("s2-edit-b", "edit", map[string]any{
			"path": bPath, "old": "old-B", "new": "new-B",
		})}},
		{text: "两个文件都已更新。"},
	}

	r := runScenario(t, []agent.Option{agent.WithTools(builtin.All()...)}, "把两个文件的标记都改掉", turns)
	if r.err != nil {
		t.Fatalf("闭环返回错误: %v", r.err)
	}

	if got := readScenarioFile(t, aPath); !strings.Contains(got, "new-A") {
		t.Errorf("a.go 未更新:\n%s", got)
	}
	if got := readScenarioFile(t, bPath); !strings.Contains(got, "new-B") {
		t.Errorf("b.go 未更新:\n%s", got)
	}

	assertToolOrder(t, r.resp, []string{"edit", "edit"})
	// 两个调用必须有不同的 ID：ID 相同会让会话的调用↔结果配对退化成 1 对 1 覆盖。
	if r.resp.ToolCalls[0].Call.ID == r.resp.ToolCalls[1].Call.ID {
		t.Error("两次 edit 的调用 ID 相同，会话配对会串线")
	}
	assertNoDanglingToolCalls(t, r.events(t))

	logScenarioSummary(t, "multi-file-edit", r)
}

// ---------------------------------------------------------------------------
// 场景 3：大输出
// ---------------------------------------------------------------------------

// TestCodingScenario_LargeOutput 让工具产出远超单次返回上限的输出，
// 断言回给模型的内容被有界截断并带明确标记——而不是把几百 KB 直接灌进上下文。
func TestCodingScenario_LargeOutput(t *testing.T) {
	// seq 1 60000 约 340KB，明确超过 bash 工具的 100KB 单次上限。
	turns := []scenarioTurn{
		{toolCalls: []llm.ToolCall{scenarioCall("s3-bash", "bash", map[string]any{
			"command": "seq 1 60000",
		})}},
		{text: "输出过多，已按上限截断。"},
	}

	r := runScenario(t, []agent.Option{agent.WithTools(builtin.All()...)}, "打印一大段输出", turns)
	if r.err != nil {
		t.Fatalf("闭环返回错误: %v", r.err)
	}

	rec := r.resp.ToolCalls[0]
	if rec.Result == nil {
		t.Fatal("大输出场景缺少工具结果")
	}
	content := rec.Result.Content

	// 上限 100KB + 截断标记的余量。
	const limit = 100 * 1024
	if len(content) > limit+64 {
		t.Errorf("回给模型的内容未被截断：%d 字节（上限约 %d）", len(content), limit)
	}
	if !strings.Contains(content, "输出截断") {
		t.Error("截断后缺少明确标记，消费方无从判断内容不完整")
	}
	// 反向确认命令确实产出了远超上限的数据，否则「截断」断言可能是空跑通过。
	if len(content) < 64*1024 {
		t.Errorf("工具结果仅 %d 字节，未复现大输出，断言不成立", len(content))
	}

	logScenarioSummary(t, "large-output", r)
}

// ---------------------------------------------------------------------------
// 场景 4：权限拒绝
// ---------------------------------------------------------------------------

// scenarioDenyTool 拒绝指定工具，放行其余工具。
type scenarioDenyTool struct{ deny string }

func (c scenarioDenyTool) Check(_ context.Context, name string, _ map[string]interface{}) (bool, error) {
	if name == c.deny {
		return false, nil
	}
	return true, nil
}

func (c scenarioDenyTool) CheckPath(string) error { return nil }

// TestCodingScenario_PermissionDenial 断言被拒绝的工具**不产生副作用**，
// 且在工具记录与会话事件里都留下可解释的失败依据。
func TestCodingScenario_PermissionDenial(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "forbidden.txt")

	turns := []scenarioTurn{
		{toolCalls: []llm.ToolCall{scenarioCall("s4-write", "write", map[string]any{
			"path": target, "content": "本文件不应被创建",
		})}},
		{text: "写入被拒绝，我没有创建该文件。"},
	}

	r := runScenario(t, []agent.Option{
		agent.WithTools(builtin.All()...),
		agent.WithPermissionChecker(scenarioDenyTool{deny: "write"}),
	}, "创建 forbidden.txt", turns)
	if r.err != nil {
		t.Fatalf("闭环返回错误（权限拒绝不应中断整轮）: %v", r.err)
	}

	// 副作用断言：拒绝必须体现在磁盘上。
	if _, err := os.Stat(target); err == nil {
		t.Errorf("被拒绝的 write 仍然创建了文件: %s", target)
	} else if !os.IsNotExist(err) {
		t.Errorf("检查产物时出现意外错误: %v", err)
	}

	rec := r.resp.ToolCalls[0]
	if rec.Err == nil {
		t.Fatal("被拒绝的工具调用应记录错误")
	}
	if !strings.Contains(rec.Err.Error(), "权限拒绝") {
		t.Errorf("拒绝原因不清晰: %v", rec.Err)
	}
	if rec.Result != nil {
		t.Errorf("被拒绝的工具不应产生结果，实际: %+v", rec.Result)
	}

	evts := r.events(t)
	if !hasEventType(evts, session.EventToolFailed) {
		t.Error("会话事件里缺少 tool.failed，事后无法核对为何没生效")
	}
	if hasEventType(evts, session.EventToolSuccess) {
		if rec.Result == nil {
			t.Error("出现了 tool.success 事件，但没有对应的成功结果")
		}
	}

	logScenarioSummary(t, "permission-denial", r)
}

// ---------------------------------------------------------------------------
// 场景 5：断流恢复
// ---------------------------------------------------------------------------

// TestCodingScenario_StreamInterruptionRecovery 断言两件事：
//
//  1. 模型流中断时**不得静默成功**——必须向上返回错误，且不产出成功响应；
//  2. 中断不污染会话——同一会话上的下一次调用可以正常完成（这是「恢复」）。
//
// 另外校验事件层不变量：中断后不会留下无法配对结果的 tool.called。
func TestCodingScenario_StreamInterruptionRecovery(t *testing.T) {
	store := session.NewMemoryStore()
	model := &scenarioModel{
		usage: llm.Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11},
		turns: []scenarioTurn{
			{streamErr: errors.New("read tcp 127.0.0.1:54321->127.0.0.1:8080: i/o timeout")},
			{text: "上一次调用流中断了，这一次已完成。"},
		},
	}

	a, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
		agent.WithTools(builtin.All()...),
		agent.WithMaxSteps(4),
	)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}
	defer func() { _ = a.Close() }()

	ctx := context.Background()

	// —— 第一段：断流必须暴露为错误 ——
	resp1, err1 := a.HandleMessage(ctx, "第一次调用（预设为断流）")
	if err1 == nil {
		t.Fatalf("流中断必须返回错误，实际返回了成功响应: %+v", resp1)
	}
	if resp1 != nil {
		t.Errorf("流中断时不应返回成功响应，实际: %+v", resp1)
	}
	if !strings.Contains(err1.Error(), "stream error") {
		t.Errorf("错误信息未标明来自流中断: %v", err1)
	}

	sid := sessionIDOf(t, store, nil)
	evts1, err := store.Events(session.EventFilter{SessionID: sid})
	if err != nil {
		t.Fatalf("读取会话事件失败: %v", err)
	}
	if !hasEventType(evts1, session.EventTurnFailed) {
		t.Error("会话事件里缺少 turn.failed，中断在日志里不可见")
	}
	assertNoDanglingToolCalls(t, evts1)

	// —— 第二段：恢复 ——
	resp2, err2 := a.HandleMessage(ctx, "第二次调用")
	if err2 != nil {
		t.Fatalf("断流后同一会话应能继续，实际错误: %v", err2)
	}
	if resp2 == nil {
		t.Fatal("恢复调用未返回响应")
	}
	if got := scenarioText(resp2.Message); !strings.Contains(got, "这一次已完成") {
		t.Errorf("恢复调用未走到收尾文本，实际: %q", got)
	}
	if resp2.SessionID != sid {
		t.Errorf("恢复调用换了会话：期望 %s，实际 %s", sid, resp2.SessionID)
	}

	evts2, err := store.Events(session.EventFilter{SessionID: sid})
	if err != nil {
		t.Fatalf("读取会话事件失败: %v", err)
	}
	assertNoDanglingToolCalls(t, evts2)
	if !hasEventType(evts2, session.EventTextEnded) {
		t.Error("恢复调用缺少 text.ended，收尾未落盘")
	}

	t.Logf("[SHIP-001 场景] stream-interruption-recovery: 中断按错误暴露=true 恢复成功=true 事件数=%d", len(evts2))
}

// TestCodingScenario_TruncatedArgsSurfaceAsToolError 覆盖断流的另一种形态：
// 连接中途断掉、既没有 [DONE] 也没有错误码，工具参数只到了一半。
//
// 契约要求：半截参数不能被当成「成功执行」。当前实现把流结束时未完成的调用按
// 完整参数刷新给工具，工具解析失败后必须表现为可见的错误结果——而不是静默地
// 认为编辑成功。这里断言的正是「失败可见」，不依赖具体错误文案。
func TestCodingScenario_TruncatedArgsSurfaceAsToolError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "half.txt")
	// 故意缺右花括号与引号，让它无法被解析成合法参数。
	halfArgs := `{"path": "` + target + `", "content": "半截`

	turns := []scenarioTurn{
		{truncated: true, toolCalls: []llm.ToolCall{
			{ID: "s5b-write", Name: "write", ArgsJSON: halfArgs},
		}},
		{text: "上一次流断在半途，没有真正写入。"},
	}

	r := runScenario(t, []agent.Option{agent.WithTools(builtin.All()...)}, "写一个文件", turns)
	if r.err != nil {
		t.Fatalf("半截流不应让整个回合返回错误（失败应体现在工具记录里）: %v", r.err)
	}

	if _, err := os.Stat(target); err == nil {
		t.Errorf("半截参数不应创建文件: %s", target)
	} else if !os.IsNotExist(err) {
		t.Errorf("检查产物时出现意外错误: %v", err)
	}

	if len(r.resp.ToolCalls) == 0 {
		t.Fatal("半截流的工具调用被丢弃了：连失败都没有记录，问题会不可见")
	}
	rec := r.resp.ToolCalls[0]
	if rec.Result == nil {
		t.Fatal("半截流缺少工具结果，失败不可见")
	}
	if !rec.Result.IsError {
		t.Errorf("半截参数被当成成功执行了，结果: %+v", rec.Result)
	}

	logScenarioSummary(t, "truncated-args", r)
}

func scenarioText(msg llm.ChatMessage) string {
	var b strings.Builder
	for _, part := range msg.Content {
		if part.Type == llm.ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// 编译期确认脚本化模型满足 llm.Model（与 mockModel 一样，把接口漂移变成编译错误）。
var _ llm.Model = (*scenarioModel)(nil)
