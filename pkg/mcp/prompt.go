// Package mcp 提供 MCP（Model Context Protocol）客户端实现。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// mcpPrompt 描述一个 MCP 服务器提供的提示模板。
type mcpPrompt struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Arguments   []mcpPromptArg `json:"arguments,omitempty"`
}

// mcpPromptArg 描述提示模板的一个参数。
type mcpPromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage 是 prompts/get 返回的消息项。
type PromptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// GetPromptResult 是 prompts/get 的响应结果。
type GetPromptResult struct {
	Messages  []PromptMessage `json:"messages"`
	Truncated bool            `json:"truncated,omitempty"`
}

// Prompts 返回指定服务器的提示模板列表。
func (m *Manager) Prompts(serverName string) ([]mcpPrompt, error) {
	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q not found", serverName)
	}

	m.mu.RLock()
	prompts := server.Prompts
	m.mu.RUnlock()
	return prompts, nil
}

// GetPrompt 获取指定名称的提示模板内容，应用大小限制。
func (m *Manager) GetPrompt(ctx context.Context, serverName, promptName string) (*GetPromptResult, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("manager is closed")
	}

	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q not found", serverName)
	}

	m.wg.Add(1)
	defer m.wg.Done()

	params := map[string]string{
		"name": promptName,
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	result, err := server.Transport.Call(ctx, "prompts/get", paramsRaw)
	if err != nil {
		return nil, fmt.Errorf("prompts/get: %w", err)
	}

	var getResp struct {
		Messages []PromptMessage `json:"messages"`
	}
	if err := json.Unmarshal(result, &getResp); err != nil {
		return nil, fmt.Errorf("parse prompts/get response: %w", err)
	}

	return applyPromptContentLimit(&GetPromptResult{Messages: getResp.Messages}, m.maxPromptSize), nil
}

// applyPromptContentLimit 对提示模板内容应用大小限制并标记截断。
// maxSize <= 0 表示不限制。
func applyPromptContentLimit(result *GetPromptResult, maxSize int) *GetPromptResult {
	if result == nil {
		return nil
	}
	if maxSize <= 0 {
		return result
	}

	truncated := false
	totalSize := 0
	for i, msg := range result.Messages {
		contentBytes, err := json.Marshal(msg.Content)
		if err != nil {
			continue
		}
		totalSize += len(contentBytes)
		if totalSize > maxSize {
			result.Messages = result.Messages[:i]
			truncated = true
			break
		}
	}
	result.Truncated = truncated
	return result
}