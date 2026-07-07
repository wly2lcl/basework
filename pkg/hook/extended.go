package hook

import (
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// EventType 表示扩展事件的类型
type EventType int

const (
	EventPreStep      EventType = iota // 步骤执行前
	EventPostStep                      // 步骤执行后
	EventOnToolError                   // 工具执行出错
	EventOnCompaction                  // 上下文压缩时
)

// StepContext 步骤执行前的上下文
type StepContext struct {
	Messages   []llm.ChatMessage
	ToolNames  []string
	StepNumber int
}

// PostStepContext 步骤执行后的上下文
type PostStepContext struct {
	Response    *llm.ChatMessage
	ToolResults []*tool.Result
	StepNumber  int
}

// ToolErrorContext 工具错误的上下文
type ToolErrorContext struct {
	ToolName string
	Args     string
	Err      error
}

// CompactionContext 上下文压缩的上下文
type CompactionContext struct {
	BeforeCount int
	AfterCount  int
	Summary     string
}

// ExtendedHook 扩展 Hook 接口（可选实现）
type ExtendedHook interface {
	Hook // 嵌入基础接口
	OnPreStep(ctx *StepContext) error
	OnPostStep(ctx *PostStepContext)
	OnToolError(ctx *ToolErrorContext)
	OnCompaction(ctx *CompactionContext)
}

// NopExtendedHook 空实现，可嵌入
type NopExtendedHook struct {
	NopHook
}

func (NopExtendedHook) OnPreStep(ctx *StepContext) error    { return nil }
func (NopExtendedHook) OnPostStep(ctx *PostStepContext)     {}
func (NopExtendedHook) OnToolError(ctx *ToolErrorContext)   {}
func (NopExtendedHook) OnCompaction(ctx *CompactionContext) {}

// FuncExtendedHook 函数式扩展 hook
type FuncExtendedHook struct {
	FuncHook
	OnPreStepFn    func(*StepContext) error
	OnPostStepFn   func(*PostStepContext)
	OnToolErrorFn  func(*ToolErrorContext)
	OnCompactionFn func(*CompactionContext)
}

func (h *FuncExtendedHook) OnPreStep(ctx *StepContext) error {
	if h.OnPreStepFn != nil {
		return h.OnPreStepFn(ctx)
	}
	return nil
}

func (h *FuncExtendedHook) OnPostStep(ctx *PostStepContext) {
	if h.OnPostStepFn != nil {
		h.OnPostStepFn(ctx)
	}
}

func (h *FuncExtendedHook) OnToolError(ctx *ToolErrorContext) {
	if h.OnToolErrorFn != nil {
		h.OnToolErrorFn(ctx)
	}
}

func (h *FuncExtendedHook) OnCompaction(ctx *CompactionContext) {
	if h.OnCompactionFn != nil {
		h.OnCompactionFn(ctx)
	}
}

// 确保类型实现了接口
var _ ExtendedHook = (*FuncExtendedHook)(nil)
var _ ExtendedHook = (*NopExtendedHook)(nil)

// RunOnPreStep 按注册顺序执行，前一个返回 error 则中止
func (c *Chain) RunOnPreStep(ctx *StepContext) error {
	for _, h := range c.hooks {
		if eh, ok := h.(ExtendedHook); ok {
			if err := eh.OnPreStep(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunOnPostStep 按注册逆序执行
func (c *Chain) RunOnPostStep(ctx *PostStepContext) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		if eh, ok := c.hooks[i].(ExtendedHook); ok {
			eh.OnPostStep(ctx)
		}
	}
}

// RunOnToolError 按注册顺序执行
func (c *Chain) RunOnToolError(ctx *ToolErrorContext) {
	for _, h := range c.hooks {
		if eh, ok := h.(ExtendedHook); ok {
			eh.OnToolError(ctx)
		}
	}
}

// RunOnCompaction 按注册顺序执行
func (c *Chain) RunOnCompaction(ctx *CompactionContext) {
	for _, h := range c.hooks {
		if eh, ok := h.(ExtendedHook); ok {
			eh.OnCompaction(ctx)
		}
	}
}
