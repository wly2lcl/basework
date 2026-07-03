package loopdetect

import "fmt"

// FusionConfig 是多信号融合的配置。
type FusionConfig struct {
	// RepeatedThreshold 重复内容检测阈值（默认 3）。
	RepeatedThreshold int `json:"repeated_threshold"`
	// ToolLoopThreshold 工具循环检测阈值（默认 5）。
	ToolLoopThreshold int `json:"tool_loop_threshold"`
	// HighConfidenceThreshold 高置信度所需的连续重复次数（默认 6）。
	HighConfidenceThreshold int `json:"high_confidence_threshold"`
}

// FusedResult 是多信号融合的检测结果。
type FusedResult struct {
	// IsLoop 是否检测到循环。
	IsLoop bool `json:"is_loop"`
	// Confidence 置信度，范围 0.0~1.0。
	Confidence float64 `json:"confidence"`
	// Signal 触发检测的主要信号描述。
	Signal string `json:"signal"`
}

// Fuse 融合多个检测信号，输出最终检测结果。
//
// 融合逻辑：
//   - 多信号确认：多个信号同时触发 → 高置信度（0.9），无论阈值如何
//   - 单信号但重复次数超过高置信度阈值 → 高置信度（0.85）
//   - 单信号达到阈值 → 中置信度（0.7）
//   - 无信号 → 不触发
func Fuse(repeated bool, toolLoop bool, patternMatch bool, config FusionConfig) (*FusedResult, error) {
	signalCount := 0
	if repeated {
		signalCount++
	}
	if toolLoop {
		signalCount++
	}
	if patternMatch {
		signalCount++
	}

	// 无信号
	if signalCount == 0 {
		return &FusedResult{
			IsLoop:     false,
			Confidence: 0,
			Signal:     "",
		}, nil
	}

	// 多信号确认
	if signalCount >= 2 {
		return &FusedResult{
			IsLoop:     true,
			Confidence: 0.9,
			Signal:     fmt.Sprintf("多信号确认（%d 个信号触发", signalCount),
		}, nil
	}

	// 单信号
	// 需要知道具体重复次数来确定置信度
	if repeated {
		// 默认 3 为阈值，6 为高置信度阈值
		threshold := config.RepeatedThreshold
		if threshold <= 0 {
			threshold = 3
		}
		highThreshold := config.HighConfidenceThreshold
		if highThreshold <= 0 {
			highThreshold = 6
		}
		if threshold >= highThreshold {
			return &FusedResult{
				IsLoop:     true,
				Confidence: 0.85,
				Signal:     "重复内容检测（单信号）",
			}, nil
		}
		return &FusedResult{
			IsLoop:     true,
			Confidence: 0.7,
			Signal:     "重复内容检测（单信号）",
		}, nil
	}

	if toolLoop {
		threshold := config.ToolLoopThreshold
		if threshold <= 0 {
			threshold = 5
		}
		highThreshold := config.HighConfidenceThreshold
		if highThreshold <= 0 {
			highThreshold = 6
		}
		if threshold >= highThreshold {
			return &FusedResult{
				IsLoop:     true,
				Confidence: 0.85,
				Signal:     "工具循环检测（单信号）",
			}, nil
		}
		return &FusedResult{
			IsLoop:     true,
			Confidence: 0.7,
			Signal:     "工具循环检测（单信号）",
		}, nil
	}

	if patternMatch {
		return &FusedResult{
			IsLoop:     true,
			Confidence: 0.7,
			Signal:     "模型循环模式检测（单信号）",
		}, nil
	}

	return &FusedResult{
		IsLoop:     false,
		Confidence: 0,
		Signal:     "",
	}, nil
}