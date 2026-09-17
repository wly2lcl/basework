//go:build sqlite

package permission

import (
	"encoding/json"
	"fmt"
)

// MigrateFromMemory 从内存缓存迁移到 SQLite。
// 检测内存缓存非空 + SQLite 表为空时自动迁移。
// 返回迁移的规则数量。
func MigrateFromMemory(cache *Cache, store Store) (int, error) {
	if cache == nil {
		return 0, fmt.Errorf("permission: cache 不能为 nil")
	}
	if store == nil {
		return 0, fmt.Errorf("permission: store 不能为 nil")
	}

	// 检查 store 中是否已有规则
	existing, err := store.List()
	if err != nil {
		return 0, fmt.Errorf("permission: 检查 store 失败: %w", err)
	}
	if len(existing) > 0 {
		return 0, fmt.Errorf("permission: store 中已有 %d 条规则，禁止覆盖迁移", len(existing))
	}

	// 从 cache 中提取所有规则
	// 注意：Cache 只存储键值对，我们需要从键中解析出规则信息
	rules := cache.dump()
	if len(rules) == 0 {
		return 0, nil
	}

	var migrated int
	for key, allow := range rules {
		toolName, args := parseCacheKey(key)
		ruleType := "deny"
		if allow {
			ruleType = "allow"
		}

		// 构建 pattern（包含参数模式）
		pattern := toolName
		if len(args) > 0 {
			argsStr := formatArgs(args)
			pattern = toolName + ":" + escapeGlob(argsStr)
		}

		storedRule := &StoredRule{
			RuleType: ruleType,
			Pattern:  pattern,
			Scope:    "session",
			Source:   "migration",
		}
		// Preserve the cache's owner when known. An unbound legacy cache stays
		// session-scoped without an ID and therefore cannot match by accident.
		scope := cache.ScopeContext()
		if scope.SessionID != "" {
			storedRule.SessionID = scope.SessionID
		} else if scope.ProjectID != "" {
			storedRule.Scope = "project"
			storedRule.ProjectID = scope.ProjectID
		}

		if err := store.Create(storedRule); err != nil {
			return migrated, fmt.Errorf("permission: 迁移规则失败: %w", err)
		}
		migrated++
	}

	return migrated, nil
}

// dump 返回 Cache 中所有键值对（内部使用）。
func (c *Cache) dump() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]bool, len(c.store))
	for k, v := range c.store {
		result[k] = v
	}
	return result
}

// ExportRules 将 store 中的规则导出为 JSON 格式。
func ExportRules(store Store) ([]byte, error) {
	rules, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("permission: 导出规则失败: %w", err)
	}

	if rules == nil {
		rules = []StoredRule{}
	}

	// 序列化为 JSON
	jsonBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("permission: 序列化规则失败: %w", err)
	}

	return jsonBytes, nil
}

// ImportRules 从 JSON 数据导入规则到 store。
func ImportRules(store Store, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("permission: 导入数据为空")
	}

	var rules []StoredRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return 0, fmt.Errorf("permission: 解析规则数据失败: %w", err)
	}

	if len(rules) == 0 {
		return 0, nil
	}

	var imported int
	for i := range rules {
		// 清空 ID 以重新生成
		rules[i].ID = ""
		// 设置来源
		rules[i].Source = "migration"

		if err := store.Create(&rules[i]); err != nil {
			return imported, fmt.Errorf("permission: 导入规则失败 (索引 %d): %w", i, err)
		}
		imported++
	}

	return imported, nil
}

// VerifyImport 验证导入的规则数量与预期一致。
func VerifyImport(store Store, expectedCount int) error {
	rules, err := store.List()
	if err != nil {
		return fmt.Errorf("permission: 验证导入失败: %w", err)
	}

	if len(rules) != expectedCount {
		return fmt.Errorf("permission: 导入验证失败: 期望 %d 条规则, 实际 %d 条", expectedCount, len(rules))
	}

	return nil
}
