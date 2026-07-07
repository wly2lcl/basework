package loopdetect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToolCall 表示用于循环检测的工具调用。
// 简化自 pkg/llm.ToolCall，专注于检测所需的字段。
type ToolCall struct {
	ToolName string                 `json:"tool_name"`
	Args     map[string]interface{} `json:"args"`
}

// CheckToolLoop 检测连续的工具调用是否重复（相同工具名且相同参数）。
// 返回 (是否循环, 连续重复次数)。
func CheckToolLoop(calls []ToolCall, threshold int) (bool, int) {
	if threshold <= 0 {
		threshold = 5 // 默认阈值
	}

	if len(calls) < threshold {
		return false, 0
	}

	repeatCount := 1
	for i := len(calls) - 1; i > 0; i-- {
		if toolCallsEqual(calls[i], calls[i-1]) {
			repeatCount++
			if repeatCount >= threshold {
				return true, repeatCount
			}
		} else {
			repeatCount = 1
		}
	}

	return false, repeatCount
}

// toolCallsEqual 比较两个工具调用是否相同（名称和参数均一致）。
func toolCallsEqual(a, b ToolCall) bool {
	if a.ToolName != b.ToolName {
		return false
	}
	return argsEqual(a.Args, b.Args)
}

// argsEqual 比较两个参数 map 是否相等。
// 使用规范化的 JSON 序列化进行比较，忽略键顺序。
func argsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	normalizeArgs := func(m map[string]interface{}) string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var parts []string
		for _, k := range keys {
			v := m[k]
			// 将值序列化为 JSON 以确保一致性
			vBytes, err := json.Marshal(v)
			if err != nil {
				vBytes = []byte(fmt.Sprintf("%v", v))
			}
			parts = append(parts, fmt.Sprintf("%s=%s", k, string(vBytes)))
		}
		return strings.Join(parts, "&")
	}

	return normalizeArgs(a) == normalizeArgs(b)
}
