// Package observability 提供结构化日志、成本追踪、token 使用量追踪和事件总线功能。
package observability

// Config 是可观测性模块的配置
type Config struct {
	// Enabled 是否启用可观测性
	Enabled bool `json:"enabled"`
	// LogLevel 日志级别：debug / info / warn / error
	LogLevel string `json:"log_level"`
	// LogOutput 日志输出目标：stdout / stderr 或文件路径
	LogOutput string `json:"log_output"`
}
