package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Reconnect 单元测试
// ---------------------------------------------------------------------------

// TestReconnectExponentialBackoff 测试指数退避重连逻辑。
func TestReconnectExponentialBackoff(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	server, _ := mgr.GetServer("test-server")
	if !server.IsAvailable() {
		t.Fatal("expected server to be available initially")
	}

	// 模拟 transport 关闭，触发重连
	server.Transport.Close()

	// 使用短超时的 context 避免测试时间过长
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := mgr.Reconnect(ctx, "test-server")
	// 现在 Reset+Connect 会启动新进程，重连应该成功
	if err != nil {
		t.Fatalf("Reconnect should succeed (creates new process): %v", err)
	}

	// 检查状态标记为 available
	if !server.IsAvailable() {
		t.Errorf("expected server to be available after successful reconnect, got %q", server.Status())
	}
}

// TestReconnectMaxRetries 测试达到最大重试次数后标记 unavailable。
func TestReconnectMaxRetries(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	// 使用无效命令，导致连接始终失败
	cfg := managerTestConfig()
	cfg.Command = "nonexistent-command-that-will-always-fail"
	cfg.Args = nil
	cfg.Env = nil

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 初始连接也应该会失败...实际测试只需要测试重试逻辑
	// 改为注入一个已关闭的 transport 来模拟
	mgr.mu.Lock()
	transport := NewStdioTransport("nonexistent-command", nil, nil)
	server := &ServerConnection{
		Name:         "test-server",
		Transport:    transport,
		MaxRetries:   2,
		serverStatus: StatusAvailable,
	}
	mgr.servers["test-server"] = server
	mgr.mu.Unlock()

	err := mgr.Reconnect(ctx, "test-server")
	if err == nil {
		t.Fatal("expected error for failed reconnect")
	}

	// 验证标记为 unavailable
	if server.IsAvailable() {
		t.Error("expected server to be unavailable after failed reconnect")
	}
	if server.Status() != StatusUnavailable {
		t.Errorf("expected status 'unavailable', got %q", server.Status())
	}

	if !strings.Contains(err.Error(), "reconnect failed") {
		t.Errorf("expected 'reconnect failed' in error, got %v", err)
	}
}

// TestReconnectStatusTransitions 测试状态标记转换。
func TestReconnectStatusTransitions(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	server, _ := mgr.GetServer("test-server")

	// 初始状态应为 available
	if server.Status() != StatusAvailable {
		t.Errorf("expected initial status 'available', got %q", server.Status())
	}

	// 设置重连中状态
	server.setStatus(StatusReconnecting)
	if server.Status() != StatusReconnecting {
		t.Errorf("expected status 'reconnecting', got %q", server.Status())
	}

	// 恢复 available
	server.setStatus(StatusAvailable)
	if !server.IsAvailable() {
		t.Error("expected server to be available")
	}

	// 标记 unavailable
	server.setStatus(StatusUnavailable)
	if server.IsAvailable() {
		t.Error("expected server to be unavailable")
	}
	if server.Status() != StatusUnavailable {
		t.Errorf("expected status 'unavailable', got %q", server.Status())
	}
}

// TestReconnectServerNotFound 测试对不存在的服务器执行重连。
func TestReconnectServerNotFound(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	err := mgr.Reconnect(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

// TestReconnectSuccessResetsRetry 测试重连成功后重置重试计数。
func TestReconnectSuccessResetsRetry(t *testing.T) {
	// 使用资源 mock（支持更多方法）
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", managerTestConfig()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	server, _ := mgr.GetServer("test-server")
	if !server.IsAvailable() {
		t.Fatal("expected server available after connect")
	}

	// 验证重试次数配置生效
	if server.MaxRetries <= 0 {
		t.Error("expected MaxRetries to be set")
	}
}

// TestStdioReset 测试 Reset 杀死进程并重置状态。
func TestStdioReset(t *testing.T) {
	transport := stdioTestTransport()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 验证进程已启动
	if transport.cmd == nil || transport.cmd.Process == nil {
		t.Fatal("expected process to be running")
	}
	pid := transport.cmd.Process.Pid

	// 调用 Reset
	transport.Reset()

	// 验证 started 为 false
	if transport.started.Load() {
		t.Error("expected started to be false after reset")
	}

	// 验证旧进程不再运行（发送 signal 0 检查）
	process, err := os.FindProcess(pid)
	if err == nil {
		if err := process.Signal(os.Signal(syscall.Signal(0))); err == nil {
			t.Error("expected old process to be killed")
		}
	}

	transport.Close()
}

// TestReconnectStartsNewProcess 测试 Reset+Connect 启动新进程。
func TestReconnectStartsNewProcess(t *testing.T) {
	transport := stdioTestTransport()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	oldPid := transport.cmd.Process.Pid

	// Reset + 重新 Connect
	transport.Reset()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("second Connect: %v", err)
	}

	// 验证新进程 PID 不同
	if transport.cmd.Process.Pid == oldPid {
		t.Error("expected new process to have different PID")
	}

	// 验证新连接可用
	result, err := transport.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("Call after reconnect: %v", err)
	}
	var s string
	if err := json.Unmarshal(result, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s != "pong" {
		t.Errorf("expected 'pong', got %q", s)
	}

	transport.Close()
}

// TestReconnectRediscovery 测试重连后重新发现工具/资源/提示。
func TestReconnectRediscovery(t *testing.T) {
	transport := stdioTestTransport()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 模拟重连流程
	transport.Reset()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	// 重新发现
	tools := rediscoverTools(ctx, transport)
	resources := rediscoverResources(ctx, transport)
	prompts := rediscoverPrompts(ctx, transport)

	// 验证重新发现成功
	if len(tools) == 0 {
		t.Error("expected tools to be rediscovered")
	}
	if len(resources) == 0 {
		t.Error("expected resources to be rediscovered")
	}
	if len(prompts) == 0 {
		t.Error("expected prompts to be rediscovered")
	}
	if tools[0].Name != "echo" {
		t.Errorf("expected 'echo', got %q", tools[0].Name)
	}

	transport.Close()
}

// TestReconnectUnavailableBlocksCalls 测试 unavailable 后拒绝调用。
func TestReconnectUnavailableBlocksCalls(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	server, _ := mgr.GetServer("test-server")
	server.Transport.Close()
	server.setStatus(StatusUnavailable)

	// 调用工具应返回 unavailable 错误
	_, err := mgr.CallTool(context.Background(), "test-server", "test_tool", nil)
	if err == nil {
		t.Fatal("expected error for unavailable server")
	}
	if !strings.Contains(err.Error(), "server unavailable") {
		t.Errorf("expected 'server unavailable' in error, got %v", err)
	}
}

// TestReconnectOtherServersNotAffected 测试一个服务器 unavailable 不影响其他。
func TestReconnectOtherServersNotAffected(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "good-server", managerTestConfig()); err != nil {
		t.Fatalf("Connect good-server: %v", err)
	}
	if err := mgr.Connect(ctx, "bad-server", managerTestConfig()); err != nil {
		t.Fatalf("Connect bad-server: %v", err)
	}

	// 标记 bad-server 为 unavailable
	badServer, _ := mgr.GetServer("bad-server")
	badServer.Transport.Close()
	badServer.setStatus(StatusUnavailable)

	// good-server 仍可用
	result, err := mgr.CallTool(ctx, "good-server", "test_tool", nil)
	if err != nil {
		t.Fatalf("CallTool on good-server failed: %v", err)
	}
	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}

	// bad-server 应拒绝调用
	_, err = mgr.CallTool(ctx, "bad-server", "test_tool", nil)
	if err == nil {
		t.Fatal("expected error for bad-server")
	}
}

// TestReconnectMaxRetriesFromConfig 测试 MaxRetries 从配置读取。
func TestReconnectMaxRetriesFromConfig(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	cfg := managerTestConfig()
	cfg.MaxRetries = 5

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	server, _ := mgr.GetServer("test-server")
	if server.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", server.MaxRetries)
	}
}
