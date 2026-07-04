// Package mcp 提供 MCP（Model Context Protocol）客户端实现。
package mcp

import (
	"errors"
	"fmt"
)

// ServerConfig 是单个 MCP 服务器的配置。
type ServerConfig struct {
	// 传输类型（"stdio" 或 "http"）。为空时自动检测：
	// 有 Command 则用 stdio，有 URL 则用 http。
	// 当两者同时存在时，Command 优先。
	Type string `json:"type,omitempty"`

	// stdio 传输
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// http 传输
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	// 重连配置
	MaxRetries int `json:"max_retries,omitempty"`
}

// LoadConfig 加载并展开 MCP 服务器配置。
// 在加载阶段对 command、args、env 执行 Shell 变量展开（os.ExpandEnv）。
func (c *ServerConfig) LoadConfig() {
	expanded := expandConfig(c)
	if expanded != nil {
		*c = *expanded
	}
}

// TransportType 返回实际使用的传输类型（自动检测）。
// 优先级：显式 Type > Command > URL。
func (c *ServerConfig) TransportType() string {
	if c.Type != "" {
		return c.Type
	}
	if c.Command != "" {
		return "stdio"
	}
	if c.URL != "" {
		return "http"
	}
	return ""
}

// Validate 检查配置是否有效。
func (c *ServerConfig) Validate() error {
	switch c.TransportType() {
	case "stdio":
		if c.Command == "" {
			return errors.New("stdio transport requires Command")
		}
	case "http":
		if c.URL == "" {
			return errors.New("http transport requires URL")
		}
	case "":
		return errors.New("transport type is empty: set Command for stdio, URL for http, or specify Type explicitly")
	default:
		return fmt.Errorf("unsupported transport type: %s", c.Type)
	}
	return nil
}