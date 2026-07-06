package permission

import (
	"context"
	"encoding/json"
	"fmt"
)

// Config 是权限系统的配置。
type Config struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // "interactive", "yolo", "deny-all"
	Rules   []Rule `json:"rules"`
}

// PromptFunc 是用户提示回调函数。
// 返回：
//   - allow: 用户是否允许执行
//   - cacheDecision: 是否缓存该决策
//   - err: 提示过程中的错误
type PromptFunc func(ctx context.Context, toolName string, args map[string]interface{}) (allow bool, cacheDecision bool, err error)

// Checker 是权限检查器。
// 支持可选的持久化存储和审计日志记录。
type Checker struct {
	Mode        Mode
	Rules       []Rule
	PromptFunc  PromptFunc
	Cache       *Cache
	Store       Store       // 可选持久化存储
	AuditLogger *AuditLogger // 可选审计日志记录器
}

// NewChecker 创建一个新的权限检查器。
// 无持久化存储和审计日志，保持向后兼容。
func NewChecker(mode Mode, rules []Rule, promptFunc PromptFunc) *Checker {
	return &Checker{
		Mode:       mode,
		Rules:      rules,
		PromptFunc: promptFunc,
		Cache:      NewCache(),
	}
}

// NewCheckerWithStore 创建一个带持久化存储的权限检查器。
func NewCheckerWithStore(mode Mode, store Store, promptFunc PromptFunc) *Checker {
	return &Checker{
		Mode:       mode,
		Rules:      []Rule{},
		PromptFunc: promptFunc,
		Cache:      NewCacheWithStore(store),
		Store:      store,
	}
}

// NewCheckerFromConfig 根据配置创建权限检查器。
func NewCheckerFromConfig(cfg Config) (*Checker, error) {
	mode, err := ParseMode(cfg.Mode)
	if err != nil {
		return nil, fmt.Errorf("parse permission mode: %w", err)
	}

	return &Checker{
		Mode:  mode,
		Rules: cfg.Rules,
		Cache: NewCache(),
	}, nil
}

// WithAudit 返回一个配置了审计日志记录器的 Checker（不变更原对象）。
func (c *Checker) WithAudit(logger *AuditLogger) *Checker {
	return &Checker{
		Mode:        c.Mode,
		Rules:       c.Rules,
		PromptFunc:  c.PromptFunc,
		Cache:       c.Cache,
		Store:       c.Store,
		AuditLogger: logger,
	}
}

// Check 检查工具执行是否被允许。
//
// 流程：
//  1. Yolo 模式 → 始终允许
//  2. DenyAll 模式 → 始终拒绝
//  3. Interactive 模式 →
//     a. 检查缓存（命中则使用缓存决策）
//     b. 检查规则（匹配则使用规则决策，不提示）
//     c. 通过 PromptFunc 提示用户
func (c *Checker) Check(ctx context.Context, toolName string, args map[string]interface{}) (bool, error) {
	switch c.Mode {
	case ModeYolo:
		c.recordAudit(toolName, "", "allowed", args)
		return true, nil

	case ModeDenyAll:
		c.recordAudit(toolName, "", "denied", args)
		return false, nil

	case ModeInteractive:
		return c.checkInteractive(ctx, toolName, args)

	default:
		return false, fmt.Errorf("unknown permission mode: %q", c.Mode)
	}
}

// checkInteractive 在交互模式下检查权限。
func (c *Checker) checkInteractive(ctx context.Context, toolName string, args map[string]interface{}) (bool, error) {
	// 1. 检查缓存
	cacheKey := CacheKey(toolName, args)
	if cached := c.Cache.Get(cacheKey); cached != nil {
		decision := "denied"
		if *cached {
			decision = "allowed"
		}
		c.recordAudit(toolName, "", decision, args)
		return *cached, nil
	}

	// 2. 检查内存规则
	if rule := MatchRules(c.Rules, toolName, args); rule != nil {
		c.Cache.Set(cacheKey, rule.Allow)
		decision := "denied"
		if rule.Allow {
			decision = "allowed"
		}
		c.recordAudit(toolName, "", decision, args)
		return rule.Allow, nil
	}

	// 3. 检查 store 中的规则
	if c.Store != nil {
		storedRule, err := c.Store.FindByPattern(toolName, args)
		if err == nil && storedRule != nil {
			allow := storedRule.RuleType == "allow"
			c.Cache.Set(cacheKey, allow)
			decision := "denied"
			if allow {
				decision = "allowed"
			}
			c.recordAudit(toolName, storedRule.ID, decision, args)
			return allow, nil
		}
	}

	// 4. 提示用户
	if c.PromptFunc == nil {
		return false, fmt.Errorf("permission checker has no PromptFunc configured for interactive mode")
	}

	allow, cacheDecision, err := c.PromptFunc(ctx, toolName, args)
	if err != nil {
		return false, fmt.Errorf("permission prompt failed: %w", err)
	}

	if cacheDecision {
		c.Cache.Set(cacheKey, allow)
	}

	decision := "denied"
	if allow {
		decision = "allowed"
	}
	c.recordAudit(toolName, "", decision, args)

	return allow, nil
}

// recordAudit 记录审计事件（如果配置了 AuditLogger）。
func (c *Checker) recordAudit(toolName, ruleID, decision string, args map[string]interface{}) {
	if c.AuditLogger == nil {
		return
	}

	contextJSON := "{}"
	if args != nil {
		data, err := json.Marshal(args)
		if err == nil {
			contextJSON = string(data)
		}
	}

	c.AuditLogger.Record(AuditRecord{
		ToolName: toolName,
		RuleID:   ruleID,
		Decision: decision,
		Context:  contextJSON,
	})
}

// WithMode 返回一个使用指定模式的新 Checker（不变更原对象）。
func (c *Checker) WithMode(mode Mode) *Checker {
	return &Checker{
		Mode:        mode,
		Rules:       c.Rules,
		PromptFunc:  c.PromptFunc,
		Cache:       c.Cache,
		Store:       c.Store,
		AuditLogger: c.AuditLogger,
	}
}

// MarshalJSON 实现 json.Marshaler，将 args 序列化为 JSON。
// 用于在提示信息中展示工具参数。
func MarshalArgs(args map[string]interface{}) string {
	data, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	return string(data)
}