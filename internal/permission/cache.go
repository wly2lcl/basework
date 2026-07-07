package permission

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Cache 是线程安全的权限决策缓存。
// 支持可选的持久化存储后端：
//   - 有 store 时：Set 同步写入 store，Get miss 时查 store
//   - 无 store 时：纯内存模式（向后兼容）
type Cache struct {
	mu    sync.RWMutex
	store map[string]bool
	pers  Store // 可选持久化存储
}

// NewCache 创建一个新的纯内存权限缓存。
// 无持久化后端，保持向后兼容。
func NewCache() *Cache {
	return &Cache{
		store: make(map[string]bool),
	}
}

// NewCacheWithStore 创建一个带持久化存储的权限缓存。
// Set 同时写入 store，Get miss 时自动查 store。
func NewCacheWithStore(store Store) *Cache {
	return &Cache{
		store: make(map[string]bool),
		pers:  store,
	}
}

// Get 获取指定键的缓存决策。
// 如果内存中不存在，有 store 时尝试从 store 读取。
// 如果键不存在，返回 nil。
func (c *Cache) Get(key string) *bool {
	c.mu.RLock()
	if v, ok := c.store[key]; ok {
		c.mu.RUnlock()
		return &v
	}
	c.mu.RUnlock()

	// 内存 miss，尝试查 store
	if c.pers != nil {
		// 尝试从 store 找匹配的规则
		// key 格式：toolName:arg1=val1 arg2=val2 ...
		// 需要解析出 toolName 和 args
		toolName, args := parseCacheKey(key)
		rule, err := c.pers.FindByPattern(toolName, args)
		if err == nil && rule != nil {
			if rule.RuleType == "ask" {
				return nil
			}
			allow := rule.RuleType == "allow"
			c.mu.Lock()
			c.store[key] = allow
			c.mu.Unlock()
			return &allow
		}
	}
	return nil
}

// Set 设置指定键的缓存决策。
// 有 store 时同步写入持久化存储。
func (c *Cache) Set(key string, allow bool) {
	c.mu.Lock()
	c.store[key] = allow
	c.mu.Unlock()

	// 同步写入 store
	if c.pers != nil {
		toolName, _ := parseCacheKey(key)
		ruleType := "deny"
		if allow {
			ruleType = "allow"
		}
		rule := &StoredRule{
			RuleType: ruleType,
			Pattern:  toolName,
			Scope:    "session",
			Source:   "auto",
		}
		// 同步写入，不阻塞权限决策
		if err := c.pers.Create(rule); err != nil {
			fmt.Fprintf(os.Stderr, "permission: 持久化规则失败: %v\n", err)
		}
	}
}

// Clear 清除所有缓存的决策。
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]bool)
}

// Delete 删除指定键的缓存决策。
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, key)
}

// CacheKey 生成用于缓存权限决策的键。
// 键格式：toolName:arg1=val1 arg2=val2 ...
// 参数按键名排序以保证键的确定性。
func CacheKey(toolName string, args map[string]interface{}) string {
	if len(args) == 0 {
		return toolName + ":"
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	key := toolName + ":"
	for i, k := range keys {
		if i > 0 {
			key += " "
		}
		key += fmt.Sprintf("%s=%v", k, args[k])
	}
	return key
}

// parseCacheKey 解析缓存键为工具名和参数映射。
func parseCacheKey(key string) (string, map[string]interface{}) {
	idx := strings.Index(key, ":")
	if idx == -1 {
		return key, nil
	}
	toolName := key[:idx]
	argsStr := key[idx+1:]
	if argsStr == "" {
		return toolName, nil
	}

	args := make(map[string]interface{})
	pairs := strings.Split(argsStr, " ")
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		eqIdx := strings.Index(pair, "=")
		if eqIdx == -1 {
			continue
		}
		k := pair[:eqIdx]
		v := pair[eqIdx+1:]
		args[k] = v
	}
	return toolName, args
}
