package permission

import "strings"

// ScopeContext identifies the execution context used to filter persisted rules.
// A scoped rule never matches when its corresponding context value is missing.
type ScopeContext struct {
	SessionID string
	ProjectID string
}

func (c ScopeContext) normalized() ScopeContext {
	return ScopeContext{
		SessionID: strings.TrimSpace(c.SessionID),
		ProjectID: strings.TrimSpace(c.ProjectID),
	}
}

// RuleScope returns the normalized scope name. Empty scope is treated as global
// for compatibility with older rows that predate scope metadata.
func RuleScope(rule StoredRule) string {
	scope := strings.ToLower(strings.TrimSpace(rule.Scope))
	if scope == "" {
		return "global"
	}
	return scope
}

// ScopeRank orders matching rules from most specific to least specific.
// session > project > global. Unknown scopes are never eligible.
func ScopeRank(scope string) int {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "session":
		return 0
	case "project":
		return 1
	case "", "global":
		return 2
	default:
		return -1
	}
}

// RuleApplies reports whether a stored rule belongs to the supplied context.
// Missing context is a fail-closed condition for session/project rules.
func RuleApplies(rule StoredRule, context ScopeContext) bool {
	context = context.normalized()
	switch RuleScope(rule) {
	case "global":
		return true
	case "session":
		return context.SessionID != "" && strings.TrimSpace(rule.SessionID) == context.SessionID
	case "project":
		return context.ProjectID != "" && strings.TrimSpace(rule.ProjectID) == context.ProjectID
	default:
		return false
	}
}
