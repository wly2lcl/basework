package hook

import (
	"fmt"
	"path"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Effect 表示规则的效果
type Effect string

const (
	Allow Effect = "allow"
	Deny  Effect = "deny"
	Ask   Effect = "ask"
)

// Rule 是一条权限规则
type Rule struct {
	Action   string // "tool:write", "tool:bash" 等
	Resource string // 通配符匹配，如 "tool:*"
	Effect   Effect
}

// PermissionHook 实现权限 = Hook 模式
type PermissionHook struct {
	NopHook              // 嵌入空实现
	rules  []Rule
	onAsk  func(action, resource string) (bool, error) // 用户确认回调
}

// NewPermissionHook 创建新的 PermissionHook
func NewPermissionHook(rules []Rule, onAsk func(string, string) (bool, error)) *PermissionHook {
	return &PermissionHook{
		rules: rules,
		onAsk: onAsk,
	}
}

// ruleMatches 检查 rule 是否匹配给定的 action 和 resource
func ruleMatches(rule Rule, action, resource string) bool {
	// action 匹配：call.Name 前缀匹配（如 "tool:write" 匹配 "write"）
	actionMatch := false
	if rule.Action == action {
		actionMatch = true
	} else {
		// 尝试前缀匹配：rule.Action 为 "tool:write" 时匹配 action="write"
		ruleMatched, err := path.Match(rule.Action, action)
		if err == nil && ruleMatched {
			actionMatch = true
		}
		// 也尝试反向匹配：action="*" 或通配匹配 rule.Action
		actionMatched, err := path.Match(action, rule.Action)
		if err == nil && actionMatched {
			actionMatch = true
		}
	}

	if !actionMatch {
		return false
	}

	// resource 匹配：使用 path.Match 通配符
	matched, err := path.Match(rule.Resource, resource)
	if err != nil {
		return false
	}
	return matched
}

// BeforeTool 实现权限检查
func (p *PermissionHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
	action := call.Name
	resource := call.Name

	for _, rule := range p.rules {
		if !ruleMatches(rule, action, resource) {
			continue
		}

		switch rule.Effect {
		case Allow:
			return &call, nil
		case Deny:
			return nil, fmt.Errorf("permission denied: %s", call.Name)
		case Ask:
			if p.onAsk == nil {
				return &call, nil
			}
			allowed, err := p.onAsk(action, resource)
			if err != nil {
				return nil, err
			}
			if allowed {
				return &call, nil
			}
			return nil, fmt.Errorf("permission denied by user: %s", call.Name)
		}
	}

	// 无匹配规则 → 默认 Allow
	return &call, nil
}