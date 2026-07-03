package loopdetect

import (
	"fmt"
	"log"
)

// ResponseStrategy 表示循环检测的响应策略。
type ResponseStrategy int

const (
	// StrategyWarn 警告策略：记录日志，继续执行。
	StrategyWarn ResponseStrategy = iota
	// StrategyInterrupt 中断策略：停止生成，返回错误。
	StrategyInterrupt
	// StrategyPrompt 提示策略：调用回调函数询问用户。
	StrategyPrompt
)

// String 返回响应策略的字符串表示。
func (s ResponseStrategy) String() string {
	switch s {
	case StrategyWarn:
		return "warn"
	case StrategyInterrupt:
		return "interrupt"
	case StrategyPrompt:
		return "prompt"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ParseResponseStrategy 将字符串解析为 ResponseStrategy。
func ParseResponseStrategy(str string) (ResponseStrategy, error) {
	switch str {
	case "warn":
		return StrategyWarn, nil
	case "interrupt":
		return StrategyInterrupt, nil
	case "prompt":
		return StrategyPrompt, nil
	default:
		return StrategyWarn, fmt.Errorf("未知响应策略: %q（有效值: warn, interrupt, prompt）", str)
	}
}

// PromptFunc 是用户提示回调函数。
// 返回用户是否希望继续。
type PromptFunc func(detail string) (continue_ bool, err error)

// HandleLoop 根据策略处理检测到的循环。
func HandleLoop(result *DetectionResult, strategy ResponseStrategy) error {
	if result == nil {
		return fmt.Errorf("HandleLoop: result 为 nil")
	}

	switch strategy {
	case StrategyWarn:
		log.Printf("[循环检测-警告] 信号: %s, 置信度: %.1f, 详情: %s",
			result.Signal, result.Confidence, result.Details)
		return nil

	case StrategyInterrupt:
		return &LoopInterruptError{
			Signal:     result.Signal,
			Confidence: result.Confidence,
			Details:    result.Details,
		}

	case StrategyPrompt:
		return fmt.Errorf("HandleLoop: prompt 策略需要 PromptFunc，请使用 HandleLoopWithPrompt")
	}
	return nil
}

// HandleLoopWithPrompt 使用 prompt 策略处理循环，调用回调函数获取用户决策。
func HandleLoopWithPrompt(result *DetectionResult, promptFn PromptFunc) error {
	if result == nil {
		return fmt.Errorf("HandleLoopWithPrompt: result 为 nil")
	}
	if promptFn == nil {
		return fmt.Errorf("HandleLoopWithPrompt: promptFn 为 nil")
	}

	detail := fmt.Sprintf("检测到可能的循环\n信号: %s\n置信度: %.1f\n详情: %s",
		result.Signal, result.Confidence, result.Details)

	continue_, err := promptFn(detail)
	if err != nil {
		return fmt.Errorf("用户提示失败: %w", err)
	}
	if !continue_ {
		return &LoopInterruptError{
			Signal:     result.Signal,
			Confidence: result.Confidence,
			Details:    result.Details,
		}
	}
	return nil
}

// LoopInterruptError 表示因循环检测而中断生成。
type LoopInterruptError struct {
	Signal     string
	Confidence float64
	Details    string
}

func (e *LoopInterruptError) Error() string {
	return fmt.Sprintf("循环检测中断: %s（置信度 %.1f）", e.Signal, e.Confidence)
}