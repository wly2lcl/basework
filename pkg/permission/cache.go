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
	mu      sync.RWMutex
	store   map[string]bool
	pers    Store // 可选持久化存储
	context ScopeContext
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

// NewCacheWithStoreAndContext creates a persistent cache scoped to one context.
func NewCacheWithStoreAndContext(store Store, context ScopeContext) *Cache {
	return &Cache{
		store:   make(map[string]bool),
		pers:    store,
		context: context.normalized(),
	}
}

// SetContext switches the cache context and invalidates all in-memory entries.
func (c *Cache) SetContext(context ScopeContext) {
	c.mu.Lock()
	c.context = context.normalized()
	c.store = make(map[string]bool)
	c.mu.Unlock()
}

// ScopeContext returns the context associated with this cache.
func (c *Cache) ScopeContext() ScopeContext {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.context
}

// Get 获取指定键的缓存决策。
// 如果内存中不存在，有 store 时尝试从 store 读取。
// 如果键不存在，返回 nil。
func (c *Cache) Get(key string) *bool {
	c.mu.RLock()
	context := c.context
	c.mu.RUnlock()
	return c.getForContext(key, context)
}

// getForContext reads a decision using an explicit context snapshot. This is
// used by Checker so a context switch cannot make one in-flight check read
// from one context and persist into another.
func (c *Cache) getForContext(key string, context ScopeContext) *bool {
	context = context.normalized()
	c.mu.RLock()
	// Entries in the in-memory map belong to the context that was active when
	// they were written. Do not reuse them after a concurrent context switch.
	if c.context == context {
		if v, ok := c.store[key]; ok {
			c.mu.RUnlock()
			return &v
		}
	}
	c.mu.RUnlock()

	// 内存 miss，尝试从 store 读取。
	if c.pers != nil {
		// key 格式：toolName:arg1=val1 arg2=val2 ...
		toolName, args := parseCacheKey(key)
		rule, err := findStoredRule(c.pers, toolName, args, context)
		if err == nil && rule != nil {
			if rule.RuleType == "ask" {
				return nil
			}
			allow := rule.RuleType == "allow"
			c.mu.Lock()
			// A context switch may have happened while the persistent lookup
			// was in flight. Only warm the in-memory cache for the same
			// context; the persistent result remains correctly scoped.
			if c.context == context {
				c.store[key] = allow
			}
			c.mu.Unlock()
			return &allow
		}
	}
	return nil
}

// setForContext stores a decision using an explicit context snapshot.
func (c *Cache) setForContext(key string, allow bool, context ScopeContext) {
	context = context.normalized()
	c.mu.Lock()
	// Do not put an old-context result into the current in-memory map after a
	// concurrent switch. It is still safe to persist the result under its
	// original context below.
	if c.context == context {
		c.store[key] = allow
	}
	c.mu.Unlock()

	if c.pers != nil {
		toolName, args := parseCacheKey(key)
		ruleType := "deny"
		if allow {
			ruleType = "allow"
		}
		pattern := toolName
		if len(args) > 0 {
			// Cache keys represent one concrete argument set. Escape glob
			// metacharacters before storing it as a rule so a cached approval
			// for `path=*.txt` cannot become a wildcard permission.
			pattern += ":" + escapeGlob(formatArgs(args))
		}
		rule := &StoredRule{
			RuleType: ruleType,
			Pattern:  pattern,
			Source:   "auto",
		}
		// A cache decision without a bound context must never become a
		// cross-session persisted rule. It remains in memory for this checker.
		if context.SessionID != "" {
			rule.Scope = "session"
			rule.SessionID = context.SessionID
		} else if context.ProjectID != "" {
			rule.Scope = "project"
			rule.ProjectID = context.ProjectID
		} else {
			return
		}
		if err := c.pers.Create(rule); err != nil {
			fmt.Fprintf(os.Stderr, "permission: 持久化规则失败: %v\n", err)
		}
	}
}

// escapeGlob quotes path.Match metacharacters so a concrete cache decision
// remains limited to the exact argument text it represents.
func escapeGlob(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch r {
		case '\\', '*', '?', '[', ']':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Set 设置指定键的缓存决策。
// 有 store 时同步写入持久化存储。
func (c *Cache) Set(key string, allow bool) {
	c.mu.Lock()
	context := c.context
	c.mu.Unlock()
	c.setForContext(key, allow, context)
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
