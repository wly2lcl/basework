// Package mcp 提供 MCP（Model Context Protocol）客户端实现。
package mcp

import (
	"context"
	"fmt"
	"time"
)

// ServerStatus 表示 MCP 服务器的连接状态。
type ServerStatus string

const (
	StatusAvailable   ServerStatus = "available"
	StatusReconnecting ServerStatus = "reconnecting"
	StatusUnavailable ServerStatus = "unavailable"
)

const (
	// defaultMaxRetries 默认最大重试次数。
	defaultMaxRetries = 3
	// initialBackoff 初始退避时间。
	initialBackoff = 1 * time.Second
)

// Reconnect 对指定服务器执行指数退避重连。
// 退避序列：1s, 2s, 4s, 8s（最长），最大重试次数由配置决定。
// 重连成功时重置重试计数；超过最大重试次数后标记为 unavailable。
func (m *Manager) Reconnect(ctx context.Context, serverName string) error {
	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("server %q not found", serverName)
	}

	maxRetries := server.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	server.setStatus(StatusReconnecting)

	backoff := initialBackoff
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			server.setStatus(StatusUnavailable)
			return ctx.Err()
		case <-time.After(backoff):
		}

		// 尝试重新连接 transport
		if err := server.Transport.Connect(ctx); err != nil {
			lastErr = err
			backoff *= 2
			continue
		}

		// 重新执行 MCP initialize 握手
		initResult, err := server.Transport.Call(ctx, "initialize", nil)
		if err != nil {
			lastErr = err
			backoff *= 2
			continue
		}
		_ = initResult

		// 重连成功，重置状态
		server.setStatus(StatusAvailable)
		return nil
	}

	// 所有重试失败，标记为 unavailable
	server.setStatus(StatusUnavailable)
	return fmt.Errorf("reconnect failed after %d retries: %w", maxRetries, lastErr)
}

// setStatus 设置服务器状态（线程安全）。
func (s *ServerConnection) setStatus(status ServerStatus) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.serverStatus = status
}

// Status 返回服务器当前状态。
func (s *ServerConnection) Status() ServerStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.serverStatus
}

// IsAvailable 检查服务器是否可用。
func (s *ServerConnection) IsAvailable() bool {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.serverStatus == StatusAvailable
}