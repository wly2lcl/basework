package lsp

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

// Manager 管理多个 LSP 客户端的生命周期。
type Manager struct {
	clients   map[string]*Client
	mu        sync.RWMutex
	config    Config
	workspace string
}

// NewManager 创建一个新的 LSP Manager。
// 如果 config.Servers 为空，使用 DefaultServers。
func NewManager(config Config) *Manager {
	if config.Servers == nil {
		config.Servers = DefaultServers
	}
	return &Manager{
		clients: make(map[string]*Client),
		config:  config,
	}
}

// Start 启动 Manager，调用 Detect 自动检测，但不预启动任何客户端。
// workspacePath 保存用于后续懒启动。
func (m *Manager) Start(_ context.Context, workspacePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspace = workspacePath
	// 调用 Detect 自动检测（结果仅用于日志/诊断，不预启动客户端）
	_ = Detect(workspacePath)
	return nil
}

// envToSlice 将 map 格式的环境变量转换为 []string 格式（"KEY=VALUE"）。
func envToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// getClient 根据文件路径找到对应的客户端，懒启动。
// 如果文件扩展名无匹配语言，返回 nil。
// 如果客户端未启动，调用 Start 启动。
// 如果客户端启动失败或处于 Error 状态，返回 nil。
func (m *Manager) getClient(ctx context.Context, file string) *Client {
	ext := strings.ToLower(filepath.Ext(file))

	m.mu.RLock()
	lang, ok := extensionToLanguage[ext]
	if !ok {
		m.mu.RUnlock()
		return nil
	}

	client, exists := m.clients[lang]
	if exists {
		m.mu.RUnlock()
		if client != nil && client.Ready() {
			return client
		}
		return nil
	}
	m.mu.RUnlock()

	// 客户端未创建，检查 config 中是否有该语言的配置
	m.mu.RLock()
	cfg, hasCfg := m.config.Servers[lang]
	workspace := m.workspace
	m.mu.RUnlock()

	if !hasCfg {
		return nil
	}

	// 创建并启动客户端（写锁）
	m.mu.Lock()
	// 双重检查：另一个 goroutine 可能已经创建了
	client, exists = m.clients[lang]
	if exists {
		m.mu.Unlock()
		if client != nil && client.Ready() {
			return client
		}
		return nil
	}

	client = NewClient(lang, cfg.Command, cfg.Args, envToSlice(cfg.Env))
	m.clients[lang] = client
	m.mu.Unlock()

	// 启动客户端
	if err := client.Start(ctx, workspace); err != nil {
		return nil // 优雅降级
	}

	return client
}

// Definition 跳转到定义，自动路由到正确的客户端。
func (m *Manager) Definition(ctx context.Context, file string, pos Position) ([]Location, error) {
	client := m.getClient(ctx, file)
	if client == nil {
		return nil, nil
	}
	return client.Definition(ctx, file, pos)
}

// References 查找引用。
func (m *Manager) References(ctx context.Context, file string, pos Position) ([]Location, error) {
	client := m.getClient(ctx, file)
	if client == nil {
		return nil, nil
	}
	return client.References(ctx, file, pos)
}

// Hover 获取悬停信息。
func (m *Manager) Hover(ctx context.Context, file string, pos Position) (string, error) {
	client := m.getClient(ctx, file)
	if client == nil {
		return "", nil
	}
	return client.Hover(ctx, file, pos)
}

// Diagnostics 获取诊断信息。
func (m *Manager) Diagnostics(file string) ([]Diagnostic, uint64) {
	client := m.getClient(context.Background(), file)
	if client == nil {
		return nil, 0
	}
	return client.Diagnostics(file)
}

// DocumentSymbols 获取文档符号。
func (m *Manager) DocumentSymbols(ctx context.Context, file string) ([]SymbolInfo, error) {
	client := m.getClient(ctx, file)
	if client == nil {
		return nil, nil
	}
	return client.DocumentSymbols(ctx, file)
}

// WorkspaceSymbols 搜索工作区符号。
func (m *Manager) WorkspaceSymbols(ctx context.Context, query string) ([]SymbolInfo, error) {
	// WorkspaceSymbols 不需要文件参数，尝试所有已就绪的客户端
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	var allSymbols []SymbolInfo
	for _, client := range clients {
		if client != nil && client.Ready() {
			symbols, err := client.WorkspaceSymbols(ctx, query)
			if err == nil {
				allSymbols = append(allSymbols, symbols...)
			}
		}
	}
	return allSymbols, nil
}

// Stop 停止所有客户端。
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for lang, client := range m.clients {
		if client != nil {
			_ = client.Stop()
		}
		delete(m.clients, lang)
	}
	return nil
}
