package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// appendFault 描述要注入的写入故障。nil 表示不注入。
type appendFault struct {
	all bool              // 所有事件都写失败
	on  session.EventType // 只让该类型写失败
}

// failingAppendStore 是一个 AppendEvent 会失败的 Store，用来复现「审计写不进去」。
// 其余方法透传给内嵌的 MemoryStore，保证请求重建仍能读到既有事件。
type failingAppendStore struct {
	session.Store
	fault appendFault
}

func (s *failingAppendStore) AppendEvent(e session.Event) error {
	if s.fault.all || (s.fault.on != "" && e.Type == s.fault.on) {
		return errors.New("磁盘写入被注入的故障拒绝")
	}
	return s.Store.AppendEvent(e)
}

// countingModel 记录 Stream 被调用的次数，用于断言「严格模式下模型调用次数为 0」。
type countingModel struct {
	streamCalls int
	answer      string
	streamErr   error
}

func (m *countingModel) ID() string { return "counting" }
func (m *countingModel) Generate(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	return &llm.Response{Message: llm.ChatMessage{Role: llm.RoleAssistant}}, nil
}
func (m *countingModel) Stream(ctx context.Context, _ *llm.Request) (<-chan llm.StreamEvent, error) {
	m.streamCalls++
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: m.answer}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}
func (m *countingModel) Supports(llm.Capability) bool { return true }

// auditTurnD 在 testTurnD 之上加一个审计策略，模拟实现了 AuditPolicyProvider 的实现。
type auditTurnD struct {
	*testTurnD
	mode AuditMode
}

func (d *auditTurnD) AuditMode() AuditMode { return d.mode }

// newAuditTurnD 构造带审计策略的 TurnD；fault 为 nil 时存储不会注入故障。
func newAuditTurnD(t *testing.T, model llm.Model, mode AuditMode, fault *appendFault) (*auditTurnD, *failingAppendStore) {
	t.Helper()
	base := setupTestTurnD(t, model)
	var store *failingAppendStore
	if fault == nil {
		store = &failingAppendStore{Store: base.session}
	} else {
		store = &failingAppendStore{Store: base.session, fault: *fault}
	}
	base.session = store
	return &auditTurnD{testTurnD: base, mode: mode}, store
}

// TestStrictAudit_WriteFailureBlocksProviderCall 是本任务的核心验收：
// 严格模式下审计写失败时，模型调用次数必须为 0。
func TestStrictAudit_WriteFailureBlocksProviderCall(t *testing.T) {
	model := &countingModel{answer: "不该被调用"}
	td, _ := newAuditTurnD(t, model, AuditModeStrict, &appendFault{on: session.EventRequestBuilt})

	p := NewPipeline(td)
	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("严格模式下审计写失败应当返回错误")
	}
	if model.streamCalls != 0 {
		t.Fatalf("严格模式下模型调用次数应为 0，实际 %d", model.streamCalls)
	}
	if !IsAuditError(err) {
		t.Fatalf("期望 AuditError，得到 %T: %v", err, err)
	}
	var ae *AuditError
	if !errors.As(err, &ae) {
		t.Fatal("errors.As 未取到 AuditError")
	}
	if ae.Stage != AuditStagePersist {
		t.Errorf("阶段应为 %s，实际 %s", AuditStagePersist, ae.Stage)
	}
	if ae.Source != string(session.EventRequestBuilt) {
		t.Errorf("来源应为 %s，实际 %s", session.EventRequestBuilt, ae.Source)
	}
	if !strings.Contains(err.Error(), "request.built") {
		t.Errorf("错误信息应指明失败的审计来源: %v", err)
	}
}

// TestCompatibleAudit_WriteFailureStillCallsProvider 验证默认模式保持旧行为，
// 既有嵌入调用不会因为审计写失败而中断对话。
func TestCompatibleAudit_WriteFailureStillCallsProvider(t *testing.T) {
	model := &countingModel{answer: "继续对话"}
	// failOn 为空表示所有 AppendEvent 都失败。
	td, _ := newAuditTurnD(t, model, AuditModeCompatible, nil)

	p := NewPipeline(td)
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("兼容模式下不应因审计失败而中断: %v", err)
	}
	if model.streamCalls != 1 {
		t.Fatalf("兼容模式下模型应被调用 1 次，实际 %d", model.streamCalls)
	}
}

// TestAuditMode_Normalize 确认零值等价于兼容模式，未知取值不被静默接受。
func TestAuditMode_Normalize(t *testing.T) {
	var zero AuditMode
	if got := zero.Normalize(); got != AuditModeCompatible {
		t.Errorf("零值应归一化为 compatible，得到 %s", got)
	}
	if !AuditModeCompatible.Valid() || !AuditModeStrict.Valid() {
		t.Error("已定义取值应判定为合法")
	}
	if AuditMode("yolo").Valid() {
		t.Error("未知取值不应判定为合法")
	}
}

// TestAuditPolicyProvider_AbsentMeansCompatible 确认没实现可选接口的实现方
// 仍然按旧行为工作——这是「旧嵌入调用保持兼容」的落点。
func TestAuditPolicyProvider_AbsentMeansCompatible(t *testing.T) {
	model := &countingModel{answer: "ok"}
	base := setupTestTurnD(t, model)
	base.session = &failingAppendStore{Store: base.session, fault: appendFault{all: true}}

	p := NewPipeline(base) // base 是 *testTurnD，未实现 AuditMode()
	if got := p.auditMode(); got != AuditModeCompatible {
		t.Fatalf("未实现 AuditPolicyProvider 时应回落到 compatible，得到 %s", got)
	}
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("回落到兼容模式后不应中断: %v", err)
	}
	if model.streamCalls != 1 {
		t.Fatalf("模型应被调用 1 次，实际 %d", model.streamCalls)
	}
}

// TestVerifyRequestBuilt_PassesAtEachRequestsOwnSeq 验证可以按每次请求发生时的
// Seq 重建并核对，且不会拿最终日志冒充当时快照。
func TestVerifyRequestBuilt_PassesAtEachRequestsOwnSeq(t *testing.T) {
	model := &countingModel{answer: "第一轮"}
	td, _ := newAuditTurnD(t, model, AuditModeCompatible, nil)

	p := NewPipeline(td)
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	firstSeq := singleRequestBuiltSeq(t, td.session, td.sessionID)
	tools := td.registry.Materialize()

	// 再跑一轮，日志尾部会多出 assistant 回复与新的 request.built。
	model.answer = "第二轮"
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}

	events := allEvents(t, td.session, td.sessionID)
	if len(RequestBuiltSeqs(events)) != 2 {
		t.Fatalf("期望 2 条 request.built，实际 %d", len(RequestBuiltSeqs(events)))
	}

	// 按 Seq 核对第一条请求：必须通过。
	if err := VerifyRequestBuilt(events, firstSeq, tools); err != nil {
		t.Errorf("按第一条请求自身的 Seq 核对应当通过: %v", err)
	}

	// 同一份最终日志直接重建，得到的已经不是第一条请求的内容——
	// 这正是「不能用最终日志代替当时快照」的证据。
	finalMsgs := BuildRequestMessages(events)
	finalHash := RequestFingerprint(finalMsgs, tools)
	if finalHash == hashAt(t, events, firstSeq) {
		t.Error("最终日志重建出的请求与第一条请求指纹相同，测试前提不成立")
	}
}

// TestVerifyRequestBuilt_DetectsHookRewrite 固定 hook 改写属于边界外：
// 指纹记录的是改写后的形态，但改写本身无法从日志重建。
func TestVerifyRequestBuilt_DetectsHookRewrite(t *testing.T) {
	model := &countingModel{answer: "ok"}
	td, _ := newAuditTurnD(t, model, AuditModeCompatible, nil)

	td.hooks = hook.NewChain()
	td.hooks.Add(&hook.FuncHook{
		BeforeLLMFn: func(msgs []llm.ChatMessage) ([]llm.ChatMessage, error) {
			return append(msgs, llm.ChatMessage{
				Role:    llm.RoleSystem,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "hook 注入"}},
			}), nil
		},
	})

	p := NewPipeline(td)
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	events := allEvents(t, td.session, td.sessionID)
	seqs := RequestBuiltSeqs(events)
	if len(seqs) != 1 {
		t.Fatalf("期望 1 条 request.built，实际 %d", len(seqs))
	}

	err := VerifyRequestBuilt(events, seqs[0], td.registry.Materialize())
	if err == nil {
		t.Fatal("hook 改写后按日志重建必然不一致，应当报错")
	}
	if !strings.Contains(err.Error(), "消息条数不一致") {
		t.Errorf("应指出条数差异来自 hook 注入: %v", err)
	}
}

// TestVerifyRequestBuilt_DetectsToolCountDrift 验证工具定义不传就查不出工具漂移，
// 传了则能查出——说明工具定义快照在日志之外，是核对时必须补上的输入。
func TestVerifyRequestBuilt_DetectsToolCountDrift(t *testing.T) {
	model := &countingModel{answer: "ok"}
	td, _ := newAuditTurnD(t, model, AuditModeCompatible, nil)

	td.registry = tool.NewRegistry()
	if err := td.registry.Register(&mockTool{name: "alpha", description: "a"}); err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}

	p := NewPipeline(td)
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	seqs := RequestBuiltSeqs(allEvents(t, td.session, td.sessionID))
	if len(seqs) != 1 {
		t.Fatalf("期望 1 条 request.built，实际 %d", len(seqs))
	}

	// 传入不同数量的工具定义 → 必须报错。
	drifted := []llm.ToolDefinition{{Name: "alpha"}, {Name: "beta"}}
	if err := VerifyRequestBuilt(allEvents(t, td.session, td.sessionID), seqs[0], drifted); err == nil {
		t.Error("工具定义数量漂移应被检出")
	} else if !strings.Contains(err.Error(), "工具定义条数不一致") {
		t.Errorf("应指明工具定义条数差异: %v", err)
	}
}

// TestVerifyRequestBuilt_MissingSeq 验证查不到对应事件时明确失败。
func TestVerifyRequestBuilt_MissingSeq(t *testing.T) {
	err := VerifyRequestBuilt(nil, 42, nil)
	if err == nil {
		t.Fatal("缺少对应 Seq 时应报错")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("错误信息应包含缺失的 Seq: %v", err)
	}
}

// TestAuditBoundaries_DeclaresUnreproducibleParts 守住边界声明本身：
// hook 改写与工具定义被覆盖但不可重建，Provider 传输变换不在覆盖内。
// 若有人把 hook 改成“可重建”，这条测试会失败，从而阻止把边界悄悄放宽。
func TestAuditBoundaries_DeclaresUnreproducibleParts(t *testing.T) {
	byName := map[string]AuditBoundary{}
	for _, b := range AuditBoundaries() {
		byName[b.Name] = b
	}

	hookBoundary, ok := byName["hook 改写后的 messages"]
	if !ok {
		t.Fatal("边界清单缺少 hook 改写条目")
	}
	if !hookBoundary.Covered || hookBoundary.Reproducible {
		t.Errorf("hook 改写应为“被覆盖但不可重建”，实际 %+v", hookBoundary)
	}

	toolsBoundary, ok := byName["工具定义快照"]
	if !ok {
		t.Fatal("边界清单缺少工具定义条目")
	}
	if !toolsBoundary.Covered || toolsBoundary.Reproducible {
		t.Errorf("工具定义应为“被覆盖但不可重建”，实际 %+v", toolsBoundary)
	}

	transport, ok := byName["Provider 传输层变换"]
	if !ok {
		t.Fatal("边界清单缺少 Provider 传输条目")
	}
	if transport.Covered {
		t.Errorf("Provider 传输变换不应算在指纹覆盖范围内，实际 %+v", transport)
	}

	history, ok := byName["事件日志投影出的 messages"]
	if !ok || !history.Covered || !history.Reproducible {
		t.Errorf("事件日志投影应是可重建部分，实际 %+v", history)
	}
}

// TestRequestBuilt_WrittenOncePerAttemptInOrder 验证重试/多轮各自留痕，
// 且写入顺序严格递增——不会把两次请求合并成一条。
func TestRequestBuilt_WrittenOncePerAttemptInOrder(t *testing.T) {
	model := &countingModel{answer: "ok", streamErr: errors.New("首次失败")}
	td, _ := newAuditTurnD(t, model, AuditModeCompatible, nil)

	p := NewPipeline(td)
	if _, err := p.Run(context.Background()); err == nil {
		t.Fatal("第一次请求应失败")
	}
	// 审计记录写在 Stream 之前，因此失败的那次也留痕。
	if seqs := RequestBuiltSeqs(allEvents(t, td.session, td.sessionID)); len(seqs) != 1 {
		t.Fatalf("失败的那次请求也应留痕，实际 %d 条", len(seqs))
	}

	model.streamErr = nil
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("第二次请求应成功: %v", err)
	}
	seqs := RequestBuiltSeqs(allEvents(t, td.session, td.sessionID))
	if len(seqs) != 2 {
		t.Fatalf("两次请求应各留一条记录，实际 %d 条", len(seqs))
	}
	if seqs[0] >= seqs[1] {
		t.Errorf("请求记录 Seq 应严格递增，实际 %v", seqs)
	}
}

// TestStrictAudit_CanceledContextStillLeavesAuditRecord 固定取消场景的时序：
// 即便上下文已取消，审计记录也先于那次（未完成的）调用写入。
func TestStrictAudit_CanceledContextStillLeavesAuditRecord(t *testing.T) {
	model := &countingModel{answer: "ok"}
	td, _ := newAuditTurnD(t, model, AuditModeStrict, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewPipeline(td)
	if _, err := p.Run(ctx); err == nil {
		t.Fatal("已取消的上下文应导致失败")
	}
	// 审计成功写入，因此不是审计失败；失败来自被取消的调用本身。
	seqs := RequestBuiltSeqs(allEvents(t, td.session, td.sessionID))
	if len(seqs) != 1 {
		t.Fatalf("取消前应先写入审计记录，实际 %d 条", len(seqs))
	}
}

// singleRequestBuiltSeq 返回唯一一条 request.built 的 Seq。
func singleRequestBuiltSeq(t *testing.T, store session.Store, sessionID string) int64 {
	t.Helper()
	seqs := RequestBuiltSeqs(allEvents(t, store, sessionID))
	if len(seqs) != 1 {
		t.Fatalf("期望 1 条 request.built，实际 %d", len(seqs))
	}
	return seqs[0]
}

// allEvents 读取会话的全部事件。
func allEvents(t *testing.T, store session.Store, sessionID string) []session.Event {
	t.Helper()
	events, err := store.Events(session.EventFilter{SessionID: sessionID})
	if err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	return events
}

// hashAt 返回指定 Seq 那条 request.built 记录的指纹。
func hashAt(t *testing.T, events []session.Event, seq int64) string {
	t.Helper()
	built, err := findRequestBuilt(events, seq)
	if err != nil {
		t.Fatalf("读取 request.built 失败: %v", err)
	}
	return built.Hash
}
