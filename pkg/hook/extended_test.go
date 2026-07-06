package hook

import (
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestPreStepPass(t *testing.T) {
	called := false
	h := &FuncExtendedHook{
		OnPreStepFn: func(ctx *StepContext) error {
			called = true
			return nil
		},
	}
	chain := NewChain()
	chain.Add(h)

	err := chain.RunOnPreStep(&StepContext{StepNumber: 1})
	if err != nil {
		t.Fatalf("RunOnPreStep 不应返回错误: %v", err)
	}
	if !called {
		t.Error("OnPreStep 未被调用")
	}
}

func TestPreStepBlock(t *testing.T) {
	callOrder := []string{}
	h1 := &FuncExtendedHook{
		OnPreStepFn: func(ctx *StepContext) error {
			callOrder = append(callOrder, "h1")
			return errors.New("h1 阻止了执行")
		},
	}
	h2 := &FuncExtendedHook{
		OnPreStepFn: func(ctx *StepContext) error {
			callOrder = append(callOrder, "h2")
			return nil
		},
	}
	chain := NewChain()
	chain.Add(h1, h2)

	err := chain.RunOnPreStep(&StepContext{StepNumber: 1})
	if err == nil {
		t.Fatal("期望 RunOnPreStep 返回错误")
	}
	if len(callOrder) != 1 || callOrder[0] != "h1" {
		t.Errorf("h2 不应被执行, 调用顺序: %v", callOrder)
	}
}

func TestPostStepAll(t *testing.T) {
	callOrder := []string{}
	h1 := &FuncExtendedHook{
		OnPostStepFn: func(ctx *PostStepContext) {
			callOrder = append(callOrder, "h1")
		},
	}
	h2 := &FuncExtendedHook{
		OnPostStepFn: func(ctx *PostStepContext) {
			callOrder = append(callOrder, "h2")
		},
	}
	chain := NewChain()
	chain.Add(h1, h2)

	chain.RunOnPostStep(&PostStepContext{StepNumber: 1})
	if len(callOrder) != 2 {
		t.Fatalf("期望 2 个 hook 被调用, 得到 %d", len(callOrder))
	}
}

func TestOnToolError(t *testing.T) {
	called := false
	var capturedCtx *ToolErrorContext
	h := &FuncExtendedHook{
		OnToolErrorFn: func(ctx *ToolErrorContext) {
			called = true
			capturedCtx = ctx
		},
	}
	chain := NewChain()
	chain.Add(h)

	chain.RunOnToolError(&ToolErrorContext{
		ToolName: "test_tool",
		Args:     `{"key": "value"}`,
		Err:      errors.New("something went wrong"),
	})
	if !called {
		t.Error("OnToolError 未被调用")
	}
	if capturedCtx == nil {
		t.Fatal("capturedCtx 不应为 nil")
	}
	if capturedCtx.ToolName != "test_tool" {
		t.Errorf("期望 ToolName=test_tool, 得到 %s", capturedCtx.ToolName)
	}
	if capturedCtx.Err.Error() != "something went wrong" {
		t.Errorf("期望错误信息, 得到 %v", capturedCtx.Err)
	}
}

func TestOnCompaction(t *testing.T) {
	called := false
	var capturedCtx *CompactionContext
	h := &FuncExtendedHook{
		OnCompactionFn: func(ctx *CompactionContext) {
			called = true
			capturedCtx = ctx
		},
	}
	chain := NewChain()
	chain.Add(h)

	chain.RunOnCompaction(&CompactionContext{
		BeforeCount: 10,
		AfterCount:  5,
		Summary:     "压缩了 5 条消息",
	})
	if !called {
		t.Error("OnCompaction 未被调用")
	}
	if capturedCtx == nil {
		t.Fatal("capturedCtx 不应为 nil")
	}
	if capturedCtx.BeforeCount != 10 {
		t.Errorf("期望 BeforeCount=10, 得到 %d", capturedCtx.BeforeCount)
	}
	if capturedCtx.AfterCount != 5 {
		t.Errorf("期望 AfterCount=5, 得到 %d", capturedCtx.AfterCount)
	}
	if capturedCtx.Summary != "压缩了 5 条消息" {
		t.Errorf("期望 Summary 正确, 得到 %s", capturedCtx.Summary)
	}
}

func TestChainOrder(t *testing.T) {
	// PreStep: 按注册顺序执行
	callOrder := []string{}
	h1 := &FuncExtendedHook{
		OnPreStepFn: func(ctx *StepContext) error {
			callOrder = append(callOrder, "h1")
			return nil
		},
	}
	h2 := &FuncExtendedHook{
		OnPreStepFn: func(ctx *StepContext) error {
			callOrder = append(callOrder, "h2")
			return nil
		},
		OnPostStepFn: func(ctx *PostStepContext) {
			callOrder = append(callOrder, "h2-post")
		},
	}
	h3 := &FuncExtendedHook{
		OnPostStepFn: func(ctx *PostStepContext) {
			callOrder = append(callOrder, "h3-post")
		},
	}
	chain := NewChain()
	chain.Add(h1, h2, h3)

	// PreStep 顺序: h1 -> h2
	err := chain.RunOnPreStep(&StepContext{StepNumber: 1})
	if err != nil {
		t.Fatalf("RunOnPreStep 失败: %v", err)
	}
	expectedPre := []string{"h1", "h2"}
	for i, v := range expectedPre {
		if callOrder[i] != v {
			t.Errorf("PreStep[%d]: 期望 %s, 得到 %s", i, v, callOrder[i])
		}
	}

	// PostStep 逆序: h3 -> h2
	chain.RunOnPostStep(&PostStepContext{StepNumber: 1})
	expectedPost := []string{"h1", "h2", "h3-post", "h2-post"}
	for i, v := range expectedPost {
		if callOrder[i] != v {
			t.Errorf("PostStep[%d]: 期望 %s, 得到 %s", i, v, callOrder[i])
		}
	}
}

func TestNopExtendedHook(t *testing.T) {
	h := NopExtendedHook{}
	// NopExtendedHook 的所有方法应不 panic
	_ = h.OnPreStep(&StepContext{StepNumber: 1})
	h.OnPostStep(&PostStepContext{StepNumber: 1})
	h.OnToolError(&ToolErrorContext{ToolName: "test"})
	h.OnCompaction(&CompactionContext{BeforeCount: 1, AfterCount: 1, Summary: "test"})
	// 基础方法也应正常
	msgs, _ := h.BeforeLLM([]llm.ChatMessage{})
	if len(msgs) != 0 {
		t.Errorf("NopHook.BeforeLLM 应直接通过消息, 期望 len=0, 得到 %d", len(msgs))
	}

	// 非空切片也应通过
	msgs2, _ := h.BeforeLLM([]llm.ChatMessage{{Role: llm.RoleUser}})
	if len(msgs2) != 1 {
		t.Errorf("NopHook.BeforeLLM 应直接通过消息, 期望 len=1, 得到 %d", len(msgs2))
	}
	h.AfterLLM(nil, nil)
}

func TestFuncExtendedHookDefaults(t *testing.T) {
	h := &FuncExtendedHook{}
	// 未设置函数时应返回默认值
	err := h.OnPreStep(&StepContext{StepNumber: 1})
	if err != nil {
		t.Errorf("默认 OnPreStep 应返回 nil, 得到 %v", err)
	}
	// 应不 panic
	h.OnPostStep(&PostStepContext{StepNumber: 1})
	h.OnToolError(&ToolErrorContext{ToolName: "test"})
	h.OnCompaction(&CompactionContext{BeforeCount: 1, AfterCount: 1, Summary: "test"})
}

// 验证非 ExtendedHook 不会被扩展方法执行
func TestNonExtendedHookIgnored(t *testing.T) {
	called := false
	basicHook := &FuncHook{
		BeforeLLMFn: func(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
			called = true
			return messages, nil
		},
	}
	chain := NewChain()
	chain.Add(basicHook)

	// 扩展方法不应调用非 ExtendedHook
	err := chain.RunOnPreStep(&StepContext{StepNumber: 1})
	if err != nil {
		t.Fatalf("RunOnPreStep 失败: %v", err)
	}
	if called {
		t.Error("普通 Hook 的 BeforeLLM 不应在 RunOnPreStep 中被调用")
	}
}