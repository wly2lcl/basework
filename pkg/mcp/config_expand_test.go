package mcp

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Config Expand 单元测试
// ---------------------------------------------------------------------------

// TestConfigExpandCommand 测试 command 字段中的 Shell 变量展开。
func TestConfigExpandCommand(t *testing.T) {
	os.Setenv("MCP_TEST_HOME", "/home/testuser")
	defer os.Unsetenv("MCP_TEST_HOME")

	cfg := &ServerConfig{
		Command: "${MCP_TEST_HOME}/.local/bin/mcp-server",
	}

	expanded := expandConfig(cfg)
	if expanded.Command != "/home/testuser/.local/bin/mcp-server" {
		t.Errorf("expected '/home/testuser/.local/bin/mcp-server', got %q", expanded.Command)
	}

	// 原始配置不改变
	if cfg.Command == expanded.Command {
		t.Error("expandConfig should return a new copy, not modify original")
	}
}

// TestConfigExpandArgs 测试 args 字段中的 Shell 变量展开。
func TestConfigExpandArgs(t *testing.T) {
	os.Setenv("MCP_TEST_DIR", "/etc/mcp")
	defer os.Unsetenv("MCP_TEST_DIR")

	cfg := &ServerConfig{
		Command: "mcp-server",
		Args:    []string{"--config", "${MCP_TEST_DIR}/mcp.yaml", "--verbose"},
	}

	expanded := expandConfig(cfg)
	if len(expanded.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(expanded.Args))
	}
	if expanded.Args[1] != "/etc/mcp/mcp.yaml" {
		t.Errorf("expected '/etc/mcp/mcp.yaml', got %q", expanded.Args[1])
	}
	if expanded.Args[2] != "--verbose" {
		t.Errorf("expected '--verbose', got %q", expanded.Args[2])
	}
}

// TestConfigExpandEnv 测试 env 字段中的 Shell 变量展开（$VAR 和 ${VAR} 两种语法）。
func TestConfigExpandEnv(t *testing.T) {
	os.Setenv("MCP_TEST_PATH", "/usr/bin")
	os.Setenv("MCP_TEST_HOME", "/home/testuser")
	defer os.Unsetenv("MCP_TEST_PATH")
	defer os.Unsetenv("MCP_TEST_HOME")

	cfg := &ServerConfig{
		Command: "mcp-server",
		Env: map[string]string{
			"PATH":     "$MCP_TEST_PATH:/custom/bin",
			"DATA_DIR": "${MCP_TEST_HOME}/data",
		},
	}

	expanded := expandConfig(cfg)
	if expanded.Env["PATH"] != "/usr/bin:/custom/bin" {
		t.Errorf("expected '/usr/bin:/custom/bin', got %q", expanded.Env["PATH"])
	}
	if expanded.Env["DATA_DIR"] != "/home/testuser/data" {
		t.Errorf("expected '/home/testuser/data', got %q", expanded.Env["DATA_DIR"])
	}
}

// TestConfigExpandOnlyOnce 测试展开仅在启动时执行，运行时不受环境变量变化影响。
func TestConfigExpandOnlyOnce(t *testing.T) {
	os.Setenv("MCP_TEST_VAR", "initial_value")
	defer os.Unsetenv("MCP_TEST_VAR")

	cfg := &ServerConfig{
		Command: "${MCP_TEST_VAR}/bin/tool",
	}

	expanded := expandConfig(cfg)
	if expanded.Command != "initial_value/bin/tool" {
		t.Errorf("expected 'initial_value/bin/tool', got %q", expanded.Command)
	}

	// 修改环境变量
	os.Setenv("MCP_TEST_VAR", "changed_value")

	// 再次展开应使用新的值（因为 expandConfig 每次调用都使用 os.ExpandEnv）
	// 但生产环境中，expandConfig 仅在启动时被调用一次
	reExpanded := expandConfig(cfg)
	if reExpanded.Command != "changed_value/bin/tool" {
		t.Errorf("expected 'changed_value/bin/tool', got %q", reExpanded.Command)
	}

	// 验证第一次展开的结果没有被后续影响
	if expanded.Command != "initial_value/bin/tool" {
		t.Errorf("original expanded value changed: %q", expanded.Command)
	}
}

// TestConfigExpandNil 测试 nil 配置安全。
func TestConfigExpandNil(t *testing.T) {
	result := expandConfig(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// TestConfigLoadConfig 测试 LoadConfig 方法。
func TestConfigLoadConfig(t *testing.T) {
	os.Setenv("MCP_TEST_HOME", "/tmp")
	defer os.Unsetenv("MCP_TEST_HOME")

	cfg := &ServerConfig{
		Command: "${MCP_TEST_HOME}/server",
		Args:    []string{"--port", "${MCP_TEST_PORT:-8080}"},
		Env: map[string]string{
			"VAR": "$MCP_TEST_HOME/data",
		},
	}

	cfg.LoadConfig()

	if cfg.Command != "/tmp/server" {
		t.Errorf("expected '/tmp/server', got %q", cfg.Command)
	}
}
