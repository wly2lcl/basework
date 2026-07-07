package tools

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
)

// TestSSRF_ProtocolWhitelist 测试协议白名单
func TestSSRF_ProtocolWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"file protocol", "file:///etc/passwd", true},
		{"ftp protocol", "ftp://example.com/file", true},
		{"http protocol", "http://example.com", false},
		{"https protocol", "https://example.com", false},
	}

	tool := NewWebFetchTool(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), []byte(`{"url": "`+tt.url+`"}`))
			if err != nil {
				t.Fatalf("Execute 返回错误: %v", err)
			}
			if tt.blocked {
				if !result.IsError {
					t.Errorf("期望 %s 被阻止，但请求成功", tt.url)
				}
				if !strings.Contains(result.Content, "不支持的协议") {
					t.Errorf("期望包含'不支持的协议', 得到: %s", result.Content)
				}
			} else {
				// 允许的协议可能因 DNS 解析失败而出错，但不应因协议被拒绝
				if result.IsError && strings.Contains(result.Content, "不支持的协议") {
					t.Errorf("期望 %s 不被协议白名单拒绝，但被拒绝: %s", tt.url, result.Content)
				}
			}
		})
	}
}

// TestSSRF_BlockedIPs 测试 SSRF IP 黑名单
func TestSSRF_BlockedIPs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"loopback 127.0.0.1", "http://127.0.0.1:8080/", true},
		{"link-local 169.254.169.254", "http://169.254.169.254/", true},
		{"private 192.168.1.1", "http://192.168.1.1/", true},
		{"private 10.0.0.1", "http://10.0.0.1/", true},
		{"private 172.16.0.1", "http://172.16.0.1/", true},
		{"public example.com", "https://example.com", false},
	}

	tool := NewWebFetchTool(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), []byte(`{"url": "`+tt.url+`"}`))
			if err != nil {
				t.Fatalf("Execute 返回错误: %v", err)
			}
			if tt.blocked {
				if !result.IsError {
					t.Errorf("期望 %s 被 SSRF 阻止，但请求成功", tt.url)
				}
				if !strings.Contains(result.Content, "SSRF 已被阻止") {
					t.Errorf("期望包含'SSRF 已被阻止', 得到: %s", result.Content)
				}
			}
			// 不检查未阻止的 case — 可能因网络/超时失败，但不应被 SSRF 阻止
		})
	}
}

// TestSSRF_AllowedHostBypass 测试白名单绕过
func TestSSRF_AllowedHostBypass(t *testing.T) {
	// 当 127.0.0.1 被添加到白名单时，应允许访问
	tool := NewWebFetchTool(&config.Config{
		Tools: config.ToolsConfig{
			WebFetch: config.WebFetchConfig{
				AllowedInternalHosts: []string{"127.0.0.1"},
			},
		},
	})

	// 测试 127.0.0.1 被允许（白名单绕过）
	// 由于没有实际服务器在监听，预计连接失败，但不应被 SSRF 阻止
	result, err := tool.Execute(context.Background(), []byte(`{"url": "http://127.0.0.1:19999/test"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}

	if strings.Contains(result.Content, "SSRF 已被阻止") {
		t.Errorf("白名单中的 127.0.0.1 不应被 SSRF 阻止，得到: %s", result.Content)
	}
}

// TestIsBlockedIP 单元测试 isBlockedIP 函数
func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name  string
		ip    string
		block bool
	}{
		{"loopback IPv4", "127.0.0.1", true},
		{"loopback IPv6", "::1", true},
		{"private 10.x", "10.0.0.1", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"link-local 169.254.x", "169.254.1.1", true},
		{"public IP", "8.8.8.8", false},
		{"public IP 2", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("无法解析 IP: %s", tt.ip)
			}
			got := isBlockedIP(ip)
			if got != tt.block {
				t.Errorf("isBlockedIP(%s) = %v, 期望 %v", tt.ip, got, tt.block)
			}
		})
	}
}