package loopdetect

import (
	"fmt"
	"log"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Config 是循环检测的配置。
type Config struct {
	Enabled           bool     `json:"enabled"`
	RepeatedThreshold int      `json:"repeated_threshold"`  // 重复内容阈值，默认 3
	ToolLoopThreshold int      `json:"tool_loop_threshold"` // 工具循环阈值，默认 5
	ResponseStrategy  string   `json:"response_strategy"`   // "warn", "interrupt", "prompt"
	CustomPatterns    []string `json:"custom_patterns"`     // 自定义循环模式
}

// DefaultConfig 返回默认的循环检测配置。
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		RepeatedThreshold: 3,
		ToolLoopThreshold: 5,
		ResponseStrategy:  "warn",
		CustomPatterns:    nil,
	}
}

// DetectionResult 是单次循环检测的结果。
type DetectionResult struct {
	// IsLoop 是否检测到循环。
	IsLoop bool `json:"is_loop"`
	// Signal 触发检测的信号描述。
	Signal string `json:"signal"`
	// Confidence 置信度，范围 0.0~1.0。
	Confidence float64 `json:"confidence"`
	// Details 详细信息，用于日志和提示。
	Details string `json:"details"`
}

// Detector 是循环检测器，集成多个检测信号。
type Detector struct {
	Config Config
	// PromptFunc 供 prompt 策略使用的回调函数。
	PromptFunc PromptFunc
}

// NewDetector 创建一个新的循环检测器。
func NewDetector(cfg Config) *Detector {
	return &Detector{
		Config: cfg,
	}
}

// NewDetectorWithPrompt 创建一个带提示回调的循环检测器。
func NewDetectorWithPrompt(cfg Config, promptFn PromptFunc) *Detector {
	return &Detector{
		Config:     cfg,
		PromptFunc: promptFn,
	}
}

// Check 对消息列表和工具调用执行完整的循环检测流程。
//
// 流程：
//  1. 如果检测器未启用，返回空结果
//  2. 分别执行三种检测：重复内容、工具循环、模式匹配
//  3. 融合多个信号得出最终判断
//  4. 如果检测到循环，根据配置的响应策略处理
//
// 返回 DetectionResult 和可能的错误（中断策略时返回 LoopInterruptError）。
func (d *Detector) Check(messages []llm.ChatMessage, toolCalls []ToolCall) (*DetectionResult, error) {
	if !d.Config.Enabled {
		return &DetectionResult{
			IsLoop:     false,
			Signal:     "",
			Confidence: 0,
			Details:    "循环检测未启用",
		}, nil
	}

	// 1. 重复内容检测
	repeated, repeatCount := CheckRepeatedContent(messages, d.Config.RepeatedThreshold)
	repeatedDetail := ""
	if repeated {
		repeatedDetail = fmt.Sprintf("连续 %d 次重复内容", repeatCount)
	}

	// 2. 工具循环检测
	toolLoop, toolRepeatCount := CheckToolLoop(toolCalls, d.Config.ToolLoopThreshold)
	toolDetail := ""
	if toolLoop {
		toolDetail = fmt.Sprintf("连续 %d 次重复工具调用", toolRepeatCount)
	}

	// 3. 模式检测（取最后一条 assistant 消息）
	patternMatch := false
	patternDetail := ""
	patterns := d.Config.CustomPatterns
	lastAssistantText := getLastAssistantText(messages)
	if lastAssistantText != "" {
		matched, matchedPattern := CheckLoopPattern(lastAssistantText, patterns)
		if matched {
			patternMatch = true
			patternDetail = fmt.Sprintf("匹配模式: %q", matchedPattern)
		}
	}

	// 4. 融合信号
	fusionCfg := FusionConfig{
		RepeatedThreshold: d.Config.RepeatedThreshold,
		ToolLoopThreshold: d.Config.ToolLoopThreshold,
	}
	fused, err := Fuse(repeated, toolLoop, patternMatch, fusionCfg)
	if err != nil {
		return nil, fmt.Errorf("信号融合失败: %w", err)
	}

	// 构建详情
	details := buildDetails(repeatedDetail, toolDetail, patternDetail)

	result := &DetectionResult{
		IsLoop:     fused.IsLoop,
		Signal:     fused.Signal,
		Confidence: fused.Confidence,
		Details:    details,
	}

	// 5. 如果检测到循环，根据策略处理
	if result.IsLoop {
		strategy, err := ParseResponseStrategy(d.Config.ResponseStrategy)
		if err != nil {
			log.Printf("[循环检测] 解析响应策略失败，使用默认 warn 策略: %v", err)
			strategy = StrategyWarn
		}

		if strategy == StrategyPrompt && d.PromptFunc != nil {
			if err := HandleLoopWithPrompt(result, d.PromptFunc); err != nil {
				return result, err
			}
		} else {
			if err := HandleLoop(result, strategy); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

// getLastAssistantText 从消息列表中提取最后一条 assistant 消息的文本。
func getLastAssistantText(messages []llm.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant {
			return extractText(messages[i].Content)
		}
	}
	return ""
}

// extractText 从 ContentPart 列表中提取文本。
func extractText(parts []llm.ContentPart) string {
	for _, part := range parts {
		if part.Type == llm.ContentTypeText {
			return part.Text
		}
	}
	return ""
}

// buildDetails 构建检测详情字符串。
func buildDetails(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "未检测到循环"
	}
	detail := ""
	for i, p := range nonEmpty {
		if i > 0 {
			detail += "; "
		}
		detail += p
	}
	return detail
}
