package permission

import "time"

// Store 是权限规则持久化存储接口。
type Store interface {
	// Create 创建权限规则。
	Create(rule *StoredRule) error
	// Get 按 ID 获取权限规则。
	Get(id string) (*StoredRule, error)
	// Update 更新权限规则。
	Update(rule *StoredRule) error
	// Delete 删除权限规则。
	Delete(id string) error
	// List 列出所有权限规则。
	List() ([]StoredRule, error)
	// FindByPattern 按模式查找匹配的权限规则。
	FindByPattern(toolName string, args map[string]interface{}) (*StoredRule, error)
	// Close 关闭存储。
	Close() error
}

// StoredRule 是持久化的权限规则。
type StoredRule struct {
	ID        string    `json:"id"`
	RuleType  string    `json:"rule_type"`   // "allow" | "deny" | "ask"
	Pattern   string    `json:"pattern"`     // 工具名或路径模式
	Scope     string    `json:"scope"`       // "global" | "session" | "project"
	SessionID string    `json:"session_id,omitempty"` // 会话 ID（session scope 时使用）
	ProjectID string    `json:"project_id,omitempty"` // 项目 ID（project scope 时使用）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"`      // "user" | "auto" | "migration"
}