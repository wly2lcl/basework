package loopdetect

import "strings"

// defaultPatterns 是默认的循环指示短语列表。
var defaultPatterns = []string{
	"I'll try again",
	"Let me retry",
	"Let me try again",
	"让我再试一次",
	"我再次尝试",
}

// CheckLoopPattern 检测文本中是否包含循环指示短语。
// 如果提供了自定义模式列表，则使用自定义模式；否则使用默认模式。
// 返回 (是否匹配, 匹配到的模式)。
func CheckLoopPattern(text string, patterns []string) (bool, string) {
	if len(patterns) == 0 {
		patterns = defaultPatterns
	}

	lowerText := strings.ToLower(text)

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if strings.Contains(lowerText, strings.ToLower(pattern)) {
			return true, pattern
		}
	}

	return false, ""
}

// SetDefaultPatterns 替换默认模式列表。如果不安全则不执行替换。
// 用于测试或自定义默认值。
func SetDefaultPatterns(patterns []string) {
	if len(patterns) > 0 {
		defaultPatterns = patterns
	}
}