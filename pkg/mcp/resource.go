// Package mcp 提供 MCP（Model Context Protocol）客户端实现。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// mcpResource 描述一个 MCP 服务器提供的资源。
type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        int    `json:"size,omitempty"`
}

// ResourceContent 是 resources/read 返回的资源内容项。
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ReadResourceResult 是 resources/read 的响应结果。
type ReadResourceResult struct {
	Contents  []ResourceContent `json:"contents"`
	Truncated bool              `json:"truncated,omitempty"`
}

// Resources 返回指定服务器的资源列表。
func (m *Manager) Resources(serverName string) ([]mcpResource, error) {
	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q not found", serverName)
	}

	m.mu.RLock()
	resources := server.Resources
	m.mu.RUnlock()
	return resources, nil
}

// ReadResource 读取指定 URI 的资源内容，应用大小限制。
func (m *Manager) ReadResource(ctx context.Context, serverName, resourceURI string) (*ReadResourceResult, error) {
	m.mu.RLock()
	if m.closed.Load() {
		m.mu.RUnlock()
		return nil, fmt.Errorf("manager is closed")
	}
	server, ok := m.servers[serverName]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("server %q not found", serverName)
	}
	m.wg.Add(1)
	m.mu.RUnlock()
	defer m.wg.Done()

	params := map[string]string{
		"uri": resourceURI,
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	result, err := server.Transport.Call(ctx, "resources/read", paramsRaw)
	if err != nil {
		return nil, fmt.Errorf("resources/read: %w", err)
	}

	var readResp struct {
		Contents []ResourceContent `json:"contents"`
	}
	if err := json.Unmarshal(result, &readResp); err != nil {
		return nil, fmt.Errorf("parse resources/read response: %w", err)
	}

	return applyResourceContentLimit(&ReadResourceResult{Contents: readResp.Contents}, m.maxResourceSize), nil
}

// applyResourceContentLimit 对资源/提示内容应用大小限制并标记截断。
// maxSize <= 0 表示不限制。
func applyResourceContentLimit(result *ReadResourceResult, maxSize int) *ReadResourceResult {
	if result == nil {
		return nil
	}
	if maxSize <= 0 {
		return result
	}

	truncated := false
	for i, c := range result.Contents {
		if len(c.Text) > maxSize {
			c.Text = c.Text[:maxSize]
			truncated = true
		}
		if len(c.Blob) > maxSize {
			c.Blob = c.Blob[:maxSize]
			truncated = true
		}
		result.Contents[i] = c
	}
	result.Truncated = truncated
	return result
}