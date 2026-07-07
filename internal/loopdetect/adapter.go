package loopdetect

import (
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
)

// LoopDetectorAdapter 将 *Detector 适配为 agent.LoopDetector 接口。
type LoopDetectorAdapter struct {
	inner *Detector
}

// NewLoopDetectorAdapter 创建适配器。
func NewLoopDetectorAdapter(d *Detector) *LoopDetectorAdapter {
	return &LoopDetectorAdapter{inner: d}
}

// Check 实现 agent.LoopDetector。
func (a *LoopDetectorAdapter) Check(messages []llm.ChatMessage, toolCalls []agent.LoopToolCall) (*agent.LoopDetectResult, error) {
	// 将 agent.LoopToolCall 转为 loopdetect.ToolCall
	ldCalls := make([]ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		ldCalls[i] = ToolCall{
			ToolName: tc.ToolName,
			Args:     tc.Args,
		}
	}

	result, err := a.inner.Check(messages, ldCalls)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return &agent.LoopDetectResult{
		IsLoop:     result.IsLoop,
		Signal:     result.Signal,
		Confidence: result.Confidence,
		Details:    result.Details,
	}, nil
}

// Ensure LoopDetectorAdapter implements agent.LoopDetector.
var _ agent.LoopDetector = (*LoopDetectorAdapter)(nil)