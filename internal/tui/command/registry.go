// Package command 提供命令面板系统
package command

import (
	"fmt"
	"sort"
	"strings"
)

// CommandHandler 命令处理函数
type CommandHandler func(args string) error

// Command 表示一条可执行命令
type Command struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Args        string         `json:"args,omitempty"`
	Handler     CommandHandler `json:"-"`
}

// Registry 命令注册表
type Registry struct {
	commands map[string]*Command
}

// NewRegistry 创建新的命令注册表
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]*Command),
	}
}

// Register 注册一个命令
func (r *Registry) Register(cmd *Command) error {
	if cmd.Name == "" {
		return fmt.Errorf("命令名称不能为空")
	}
	if _, exists := r.commands[cmd.Name]; exists {
		return fmt.Errorf("命令 %q 已存在", cmd.Name)
	}
	r.commands[cmd.Name] = cmd
	return nil
}

// Get 获取指定名称的命令
func (r *Registry) Get(name string) (*Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// List 返回所有已注册的命令
func (r *Registry) List() []*Command {
	result := make([]*Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		result = append(result, cmd)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Search 使用 fuzzy 匹配搜索命令
func (r *Registry) Search(query string) []*Command {
	if query == "" {
		return r.List()
	}

	// 先用精确前缀匹配
	var exact []*Command
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd.Name, query) {
			exact = append(exact, cmd)
		}
	}

	// 再用 fuzzy 匹配
	names := make([]string, 0, len(r.commands))
	for _, cmd := range r.commands {
		names = append(names, cmd.Name)
	}

	matched := FuzzyFilter(query, names)
	matchedSet := make(map[string]bool, len(matched))
	for _, name := range matched {
		matchedSet[name] = true
	}

	// 合并结果（前缀匹配优先）
	seen := make(map[string]bool)
	var result []*Command
	for _, cmd := range exact {
		if !seen[cmd.Name] {
			result = append(result, cmd)
			seen[cmd.Name] = true
		}
	}
	for _, name := range matched {
		if !seen[name] {
			if cmd, ok := r.commands[name]; ok {
				result = append(result, cmd)
				seen[name] = true
			}
		}
	}

	return result
}

// Execute 执行命令
func (r *Registry) Execute(input string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// 解析命令名和参数
	parts := strings.SplitN(input, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	cmd, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("未知命令: %q，输入 /help 查看可用命令", name)
	}

	return cmd.Handler(args)
}