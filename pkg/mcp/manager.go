package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wly2lcl/basework/pkg/tool"
)

// Manager 管理多个 MCP 服务器连接。
type Manager struct {
	servers map[string]*ServerConnection
	mu      sync.RWMutex
	closed  atomic.Bool
	wg      sync.WaitGroup // 追踪进行中的工具/资源/提示调用

	// 大小限制（0 表示不限制）
	maxResourceSize int
	maxPromptSize   int
}

// ServerConnection 表示到 MCP 服务器的连接。
type ServerConnection struct {
	Name       string
	Transport  Transport
	Tools      []mcpTool
	Resources  []mcpResource
	Prompts    []mcpPrompt
	MaxRetries int

	serverStatus ServerStatus
	statusMu     sync.RWMutex
}

// mcpTool 描述一个 MCP 服务器提供的工具。
type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// NewManager 创建一个新的 Manager。
// maxResourceSize 和 maxPromptSize 为 0 表示使用默认值（10MB）。
func NewManager() *Manager {
	return &Manager{
		servers:         make(map[string]*ServerConnection),
		maxResourceSize: 10 * 1024 * 1024, // 默认 10MB
		maxPromptSize:   10 * 1024 * 1024, // 默认 10MB
	}
}

// SetMaxResourceSize 设置资源大小限制（0 表示不限制）。
func (m *Manager) SetMaxResourceSize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxResourceSize = size
}

// SetMaxPromptSize 设置提示模板大小限制（0 表示不限制）。
func (m *Manager) SetMaxPromptSize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxPromptSize = size
}

// Connect 连接一个 MCP 服务器。
// 执行配置验证、传输层连接、MCP initialize 握手和工具/资源/提示发现。
func (m *Manager) Connect(ctx context.Context, name string, cfg ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	var transport Transport
	switch cfg.TransportType() {
	case "stdio":
		transport = NewStdioTransport(cfg.Command, cfg.Args, cfg.Env)
	case "http":
		transport = NewHTTPTransport(cfg.URL, cfg.Headers)
	}

	if err := transport.Connect(ctx); err != nil {
		return fmt.Errorf("transport connect: %w", err)
	}

	// MCP initialize 握手
	initResult, err := transport.Call(ctx, "initialize", json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"basework","version":"0.1.0"}}`))
	if err != nil {
		transport.Close()
		return fmt.Errorf("initialize handshake: %w", err)
	}
	_ = initResult // 握手结果暂不处理

	// 发送 initialized 通知
	if err := transport.Notify("initialized", nil); err != nil {
		transport.Close()
		return fmt.Errorf("initialized notification: %w", err)
	}

	// 发现工具
	var tools []mcpTool
	toolsResult, err := transport.Call(ctx, "tools/list", nil)
	if err != nil {
		transport.Close()
		return fmt.Errorf("tools/list: %w", err)
	}
	var toolsList struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal(toolsResult, &toolsList); err != nil {
		transport.Close()
		return fmt.Errorf("parse tools/list response: %w", err)
	}
	tools = toolsList.Tools

	// 发现资源（降级：失败时仅记警告，不中断连接）
	var resources []mcpResource
	resourcesResult, err := transport.Call(ctx, "resources/list", nil)
	if err != nil {
		// resources/list 失败时不中断连接
		resources = nil
	} else {
		var resourcesList struct {
			Resources []mcpResource `json:"resources"`
		}
		if json.Unmarshal(resourcesResult, &resourcesList) == nil {
			resources = resourcesList.Resources
		}
	}

	// 发现提示模板（降级：失败时仅记警告，不中断连接）
	var prompts []mcpPrompt
	promptsResult, err := transport.Call(ctx, "prompts/list", nil)
	if err != nil {
		// prompts/list 失败时不中断连接
		prompts = nil
	} else {
		var promptsList struct {
			Prompts []mcpPrompt `json:"prompts"`
		}
		if json.Unmarshal(promptsResult, &promptsList) == nil {
			prompts = promptsList.Prompts
		}
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	conn := &ServerConnection{
		Name:        name,
		Transport:   transport,
		Tools:       tools,
		Resources:   resources,
		Prompts:     prompts,
		serverStatus: StatusAvailable,
		MaxRetries:  maxRetries,
	}

	m.mu.Lock()
	m.servers[name] = conn
	m.mu.Unlock()

	return nil
}

// Disconnect 断开指定服务器的连接。
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	server, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("server %q not found", name)
	}
	delete(m.servers, name)
	m.mu.Unlock()

	return server.Transport.Close()
}

// Tools 返回所有已连接服务器提供的工具列表。
func (m *Manager) Tools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []tool.Tool
	for serverName, server := range m.servers {
		if !server.IsAvailable() {
			continue
		}
		for _, mt := range server.Tools {
			result = append(result, &mcpToolAdapter{
				server:  serverName,
				mcpTool: mt,
				manager: m,
			})
		}
	}
	return result
}

// CallTool 调用指定服务器上的工具。
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, arguments json.RawMessage) (*tool.Result, error) {
	if m.closed.Load() {
		return nil, errors.New("manager is closed")
	}

	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q not found", serverName)
	}

	if !server.IsAvailable() {
		return nil, fmt.Errorf("server unavailable: %s", serverName)
	}

	m.wg.Add(1)
	defer m.wg.Done()

	// 解析参数
	var argsMap map[string]any
	if arguments != nil && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &argsMap); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	params := map[string]any{
		"name":      toolName,
		"arguments": argsMap,
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	result, err := server.Transport.Call(ctx, "tools/call", paramsRaw)
	if err != nil {
		return nil, fmt.Errorf("tools/call: %w", err)
	}

	var callResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &callResp); err != nil {
		return nil, fmt.Errorf("parse tools/call response: %w", err)
	}

	// 合并所有 text 内容
	var texts []string
	for _, c := range callResp.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}

	return &tool.Result{
		Content: strings.Join(texts, "\n"),
		IsError: callResp.IsError,
	}, nil
}

// GetServer 获取指定名称的服务器连接。
func (m *Manager) GetServer(name string) (*ServerConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[name]
	return server, ok
}

// GetServers 返回所有服务器连接的副本。
func (m *Manager) GetServers() map[string]*ServerConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ServerConnection, len(m.servers))
	for k, v := range m.servers {
		result[k] = v
	}
	return result
}

// Close 关闭 Manager，等待所有进行中的调用完成，然后关闭所有连接（幂等）。
func (m *Manager) Close() error {
	if m.closed.Swap(true) {
		return nil
	}

	// 等待所有进行中的工具调用完成
	m.wg.Wait()

	// 关闭所有服务器连接
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		server.Transport.Close()
		delete(m.servers, name)
	}
	return nil
}