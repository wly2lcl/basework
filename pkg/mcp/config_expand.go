// Package mcp 提供 MCP（Model Context Protocol）客户端实现。
package mcp

import "os"

// expandConfig 对 ServerConfig 中的 command、args、env 字段执行 Shell 变量展开。
// 使用 os.ExpandEnv 支持 $VAR 和 ${VAR} 两种语法。
// 展开仅在配置加载时执行一次，运行时不再重复。
func expandConfig(cfg *ServerConfig) *ServerConfig {
	if cfg == nil {
		return nil
	}

	expanded := *cfg

	// 展开 command
	expanded.Command = os.ExpandEnv(cfg.Command)

	// 展开 args
	if cfg.Args != nil {
		expanded.Args = make([]string, len(cfg.Args))
		for i, arg := range cfg.Args {
			expanded.Args[i] = os.ExpandEnv(arg)
		}
	}

	// 展开 env
	if cfg.Env != nil {
		expanded.Env = make(map[string]string, len(cfg.Env))
		for k, v := range cfg.Env {
			expanded.Env[k] = os.ExpandEnv(v)
		}
	}

	return &expanded
}