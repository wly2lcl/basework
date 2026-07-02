package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Registry 管理工具集合
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]Tool
	version  int64
	disabled map[string]bool
}

// NewRegistry 创建并初始化一个 Registry
func NewRegistry() *Registry {
	return &Registry{
		tools:    make(map[string]Tool),
		disabled: make(map[string]bool),
	}
}

// Register 注册工具，重名返回 error
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q 已注册", name)
	}
	r.tools[name] = t
	r.version++
	return nil
}

// Get 查找工具，不存在返回 nil
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// Settle 查找并执行工具调用
func (r *Registry) Settle(ctx context.Context, call llm.ToolCall) (*Result, error) {
	r.mu.RLock()
	t, ok := r.tools[call.Name]
	if !ok {
		r.mu.RUnlock()
		return nil, fmt.Errorf("工具 %q 不存在", call.Name)
	}
	if r.disabled[call.Name] {
		r.mu.RUnlock()
		return nil, fmt.Errorf("工具 %q 已被禁用", call.Name)
	}
	r.mu.RUnlock()

	return t.Execute(ctx, json.RawMessage(call.ArgsJSON))
}

// Clone 深拷贝 Registry（子 agent 作用域隔离）
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clone := &Registry{
		tools:    make(map[string]Tool, len(r.tools)),
		version:  r.version,
		disabled: make(map[string]bool, len(r.disabled)),
	}
	for k, v := range r.tools {
		clone.tools[k] = v
	}
	for k, v := range r.disabled {
		clone.disabled[k] = v
	}
	return clone
}

// Materialize 生成给 LLM 的工具定义列表（排除 disabled）
func (r *Registry) Materialize() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		if r.disabled[t.Name()] {
			continue
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// Disable 禁用工具
func (r *Registry) Disable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disabled[name] = true
}