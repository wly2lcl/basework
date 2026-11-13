package permission

import (
	"fmt"
	"path"
	"sort"
)

// Rule 定义一条权限规则。
type Rule struct {
	// ToolPattern 工具名称的 glob 匹配模式（如 "read_*"、"write_file"）。
	ToolPattern string `json:"tool_pattern"`
	// ArgPattern 工具参数的 glob 匹配模式（如 "*.txt"），为空时仅匹配工具名称。
	ArgPattern string `json:"arg_pattern,omitempty"`
	// Allow true 表示允许，false 表示拒绝。
	Allow bool `json:"allow"`
}

// MatchRule 检查单条规则是否匹配给定的工具名称和参数。
// 参数匹配使用 path.Match（仅匹配 arg 值的字符串表示）。
func MatchRule(rule Rule, toolName string, args map[string]interface{}) bool {
	// 匹配工具名称
	matched, err := path.Match(rule.ToolPattern, toolName)
	if err != nil || !matched {
		return false
	}

	// 如果没有配置参数模式，仅匹配工具名称即可
	if rule.ArgPattern == "" {
		return true
	}

	// 将 args 序列化为字符串进行匹配
	argsStr := formatArgs(args)
	matched, err = path.Match(rule.ArgPattern, argsStr)
	return err == nil && matched
}

// formatArgs 将参数映射格式化为用于匹配的字符串。
// 保持确定性顺序以使缓存键一致。
func formatArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result string
	for i, k := range keys {
		if i > 0 {
			result += " "
		}
		result += fmt.Sprintf("%s=%v", k, args[k])
	}
	return result
}

// MatchRules 按顺序匹配规则，返回第一条匹配的规则。
// 如果没有规则匹配，返回 nil。
func MatchRules(rules []Rule, toolName string, args map[string]interface{}) *Rule {
	for _, rule := range rules {
		if MatchRule(rule, toolName, args) {
			return &rule
		}
	}
	return nil
}
