package permission

import (
	"fmt"
	"sort"
	"sync"
)

// Cache 是线程安全的权限决策缓存。
type Cache struct {
	mu    sync.RWMutex
	store map[string]bool
}

// NewCache 创建一个新的权限缓存。
func NewCache() *Cache {
	return &Cache{
		store: make(map[string]bool),
	}
}

// Get 获取指定键的缓存决策。
// 如果键不存在，返回 nil。
func (c *Cache) Get(key string) *bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.store[key]; ok {
		return &v
	}
	return nil
}

// Set 设置指定键的缓存决策。
func (c *Cache) Set(key string, allow bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = allow
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