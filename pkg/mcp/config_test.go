package mcp

import (
	"testing"
)

// ---------------------------------------------------------------------------
// TransportType 自动检测
// ---------------------------------------------------------------------------

// TestTransportTypeExplicit 测试显式设置 Type 字段。
func TestTransportTypeExplicit(t *testing.T) {
	c := &ServerConfig{Type: "http", Command: "node", URL: "http://localhost:8080"}
	if got := c.TransportType(); got != "http" {
		t.Errorf("explicit Type=http: got %q, want %q", got, "http")
	}
}

// TestTransportTypeStdioByCommand 测试通过 Command 自动检测为 stdio。
func TestTransportTypeStdioByCommand(t *testing.T) {
	c := &ServerConfig{Command: "node"}
	if got := c.TransportType(); got != "stdio" {
		t.Errorf("Command only: got %q, want %q", got, "stdio")
	}
}

// TestTransportTypeHTTPByURL 测试通过 URL 自动检测为 http。
func TestTransportTypeHTTPByURL(t *testing.T) {
	c := &ServerConfig{URL: "http://localhost:8080"}
	if got := c.TransportType(); got != "http" {
		t.Errorf("URL only: got %q, want %q", got, "http")
	}
}

// TestTransportTypeCommandPriority 测试 Command 和 URL 同时存在时 Command 优先。
func TestTransportTypeCommandPriority(t *testing.T) {
	c := &ServerConfig{Command: "node", URL: "http://localhost:8080"}
	if got := c.TransportType(); got != "stdio" {
		t.Errorf("Command+URL: got %q, want %q", got, "stdio")
	}
}

// TestTransportTypeEmpty 测试空配置返回空字符串。
func TestTransportTypeEmpty(t *testing.T) {
	c := &ServerConfig{}
	if got := c.TransportType(); got != "" {
		t.Errorf("empty config: got %q, want %q", got, "")
	}
}

// TestTransportTypeExplicitOverride 测试显式 Type 覆盖自动检测。
func TestTransportTypeExplicitOverride(t *testing.T) {
	c := &ServerConfig{Type: "stdio", URL: "http://localhost:8080"}
	if got := c.TransportType(); got != "stdio" {
		t.Errorf("Type=stdio overrides URL: got %q, want %q", got, "stdio")
	}
}

// ---------------------------------------------------------------------------
// Validate 验证逻辑
// ---------------------------------------------------------------------------

// TestValidateValidStdio 测试有效的 stdio 配置。
func TestValidateValidStdio(t *testing.T) {
	c := &ServerConfig{Command: "node", Args: []string{"server.js"}}
	if err := c.Validate(); err != nil {
		t.Errorf("valid stdio config: got error %v, want nil", err)
	}
}

// TestValidateValidHTTP 测试有效的 http 配置。
func TestValidateValidHTTP(t *testing.T) {
	c := &ServerConfig{URL: "http://localhost:8080/mcp"}
	if err := c.Validate(); err != nil {
		t.Errorf("valid http config: got error %v, want nil", err)
	}
}

// TestValidateStdioMissingCommand 测试 stdio 缺少 Command。
func TestValidateStdioMissingCommand(t *testing.T) {
	c := &ServerConfig{Type: "stdio", Args: []string{"server.js"}}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for stdio without Command, got nil")
	}
	if err.Error() != "stdio transport requires Command" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestValidateHTTPMissingURL 测试 http 缺少 URL。
func TestValidateHTTPMissingURL(t *testing.T) {
	c := &ServerConfig{Type: "http"}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for http without URL, got nil")
	}
	if err.Error() != "http transport requires URL" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestValidateInvalidType 测试无效的传输类型。
func TestValidateInvalidType(t *testing.T) {
	c := &ServerConfig{Type: "websocket"}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
}

// TestValidateEmpty 测试空配置。
func TestValidateEmpty(t *testing.T) {
	c := &ServerConfig{}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty config, got nil")
	}
}

// TestValidateExplicitStdioOverridesURL 测试显式 Type=stdio 时即使有 URL 也不报错。
func TestValidateExplicitStdioOverridesURL(t *testing.T) {
	c := &ServerConfig{Type: "stdio", Command: "node", URL: "http://localhost:8080"}
	if err := c.Validate(); err != nil {
		t.Errorf("explicit stdio with Command: got error %v, want nil", err)
	}
}

// TestValidateExplicitHTTPOverridesCommand 测试显式 Type=http 时即使有 Command 也不报错。
func TestValidateExplicitHTTPOverridesCommand(t *testing.T) {
	c := &ServerConfig{Type: "http", URL: "http://localhost:8080", Command: "node"}
	if err := c.Validate(); err != nil {
		t.Errorf("explicit http with URL: got error %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Validate 辅助验证：自动检测路径
// ---------------------------------------------------------------------------

// TestValidateAutoStdio 测试自动检测为 stdio 时的验证。
func TestValidateAutoStdio(t *testing.T) {
	c := &ServerConfig{Command: "node"}
	if err := c.Validate(); err != nil {
		t.Errorf("auto stdio config: got error %v, want nil", err)
	}
}

// TestValidateAutoHTTP 测试自动检测为 http 时的验证。
func TestValidateAutoHTTP(t *testing.T) {
	c := &ServerConfig{URL: "http://localhost:8080"}
	if err := c.Validate(); err != nil {
		t.Errorf("auto http config: got error %v, want nil", err)
	}
}

// TestValidateCommandWithoutURL 测试有 Command 无 URL 自动 stdio，应通过。
func TestValidateCommandWithoutURL(t *testing.T) {
	c := &ServerConfig{Command: "node", Args: []string{"index.js"}, Env: map[string]string{"KEY": "val"}}
	if err := c.Validate(); err != nil {
		t.Errorf("full stdio config: got error %v, want nil", err)
	}
}

// TestValidateURLWithoutCommand 测试有 URL 无 Command 自动 http，应通过。
func TestValidateURLWithoutCommand(t *testing.T) {
	c := &ServerConfig{URL: "https://api.example.com/mcp", Headers: map[string]string{"Auth": "token"}}
	if err := c.Validate(); err != nil {
		t.Errorf("full http config: got error %v, want nil", err)
	}
}
