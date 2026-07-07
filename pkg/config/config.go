// Package config 提供 CoW（Copy-on-Write）配置系统。
// Store 通过原子指针交换实现无锁读取，支持 JSON 持久化。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// OpenCodeConfig 是 OpenCode Zen Provider 的配置
type OpenCodeConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"` // 默认 "big-pickle"
}

// BedrockConfig 是 Amazon Bedrock Provider 的配置
type BedrockConfig struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"` // 默认 "us-east-1"
	Model     string `json:"model"`
}

// AzureConfig 是 Azure OpenAI Provider 的配置
type AzureConfig struct {
	Resource   string `json:"resource"`
	Deployment string `json:"deployment"`
	APIKey     string `json:"api_key"`
	APIVersion string `json:"api_version"` // 默认 "2024-02-01"
}

// CopilotConfig 是 GitHub Copilot Provider 的配置
type CopilotConfig struct {
	Model     string `json:"model"` // 默认 "gpt-4"
	TokenPath string `json:"token_path"`
}

// OllamaConfig 是 Ollama Provider 的配置
type OllamaConfig struct {
	Endpoint string `json:"endpoint"` // 默认 "http://localhost:11434"
	Model    string `json:"model"`
}

// Config 是运行时配置。
type Config struct {
	Provider         string                 `json:"provider"`
	Model            string                 `json:"model"`
	Temperature      float64                `json:"temperature"`
	MaxTokens        int                    `json:"max_tokens"`
	SystemPrompt     string                 `json:"system_prompt,omitempty"`
	TopP             float64                `json:"top_p,omitempty"`
	FrequencyPenalty float64                `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                `json:"presence_penalty,omitempty"`
	StopSequences    []string               `json:"stop_sequences,omitempty"`
	MaxIterations    int                    `json:"max_iterations,omitempty"`
	Timeout          int                    `json:"timeout,omitempty"`
	Verbose          bool                   `json:"verbose,omitempty"`
	MCPConfigs       map[string]interface{} `json:"mcp_configs,omitempty"`
	MaxToolCalls     int                    `json:"max_tool_calls,omitempty"`
	MaxContextTokens int                    `json:"max_context_tokens,omitempty"`

	// 上下文压缩配置
	Compaction CompactionConfig `json:"compaction,omitempty"`
	// 重试配置
	Retry RetryConfig `json:"retry,omitempty"`
	// 权限配置
	Permission PermissionConfig `json:"permission,omitempty"`
	// 子代理配置
	SubAgent SubAgentConfig `json:"sub_agent,omitempty"`
	// 循环检测配置
	LoopDetect LoopDetectConfig `json:"loop_detect,omitempty"`
	// 可观测性配置
	Observability ObservabilityConfig `json:"observability,omitempty"`
	// Prompt 缓存配置
	PromptCache PromptCacheConfig `json:"prompt_cache,omitempty"`
	// OAuth 配置
	OAuth OAuthConfig `json:"oauth,omitempty"`
	// 工具配置
	Tools ToolsConfig `json:"tools,omitempty"`
	// 会话配置
	Session SessionConfig `json:"session,omitempty"`
	// 数据库配置
	Database DatabaseConfig `json:"database,omitempty"`
	// 安全配置
	Security SecurityConfig `json:"security,omitempty"`
	// 性能分析配置
	Profiling ProfilingConfig `json:"profiling,omitempty"`

	// 主题配置
	Theme ThemeConfig `json:"theme,omitempty"`
	// 键盘绑定配置
	Keybindings KeybindingsConfig `json:"keybindings,omitempty"`
	// 模板配置
	Templates TemplatesConfig `json:"templates,omitempty"`

	// Provider 特定配置
	OpenCode OpenCodeConfig `json:"opencode,omitempty"`
	Bedrock  BedrockConfig  `json:"bedrock,omitempty"`
	Azure    AzureConfig    `json:"azure,omitempty"`
	Copilot  CopilotConfig  `json:"copilot,omitempty"`
	Ollama   OllamaConfig   `json:"ollama,omitempty"`
}

// CompactionConfig 是上下文压缩模块的配置
type CompactionConfig struct {
	Enabled    bool    `json:"enabled"`
	Strategy   string  `json:"strategy"`    // sliding_window / summarization / selective
	Threshold  float64 `json:"threshold"`   // 自动触发阈值 0.0-1.0，默认 0.8
	WindowSize int     `json:"window_size"` // 滑动窗口大小，默认 10
}

// RetryConfig 是重试机制的配置
type RetryConfig struct {
	Enabled     bool `json:"enabled"`
	MaxAttempts int  `json:"max_attempts"`  // 最大重试次数，默认 3
	BaseDelayMs int  `json:"base_delay_ms"` // 基础延迟（毫秒），默认 2000
	MaxDelayMs  int  `json:"max_delay_ms"`  // 最大延迟（毫秒），默认 60000
}

// PermissionConfig 是权限系统的配置
type PermissionConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // interactive / yolo / deny-all
	// CommandBlacklist 是命令黑名单配置
	CommandBlacklist CommandBlacklistConfig `json:"command_blacklist,omitempty"`
}

// CommandBlacklistConfig 是命令黑名单的配置
type CommandBlacklistConfig struct {
	// BlockedCommands 是用户自定义的黑名单正则模式列表，与内置黑名单合并检查
	BlockedCommands []string `json:"blocked_commands,omitempty"`
}

// SubAgentConfig 是子代理系统的配置
type SubAgentConfig struct {
	Enabled       bool    `json:"enabled"`
	DefaultType   string  `json:"default_type"`   // general / readonly
	CostLimit     float64 `json:"cost_limit"`     // 成本限制（美元），默认 1.0
	MaxConcurrent int     `json:"max_concurrent"` // 最大并发数，默认 5
}

// LoopDetectConfig 是循环检测的配置
type LoopDetectConfig struct {
	Enabled           bool     `json:"enabled"`
	RepeatedThreshold int      `json:"repeated_threshold"`  // 重复内容阈值，默认 3
	ToolLoopThreshold int      `json:"tool_loop_threshold"` // 工具循环阈值，默认 5
	ResponseStrategy  string   `json:"response_strategy"`   // warn / interrupt / prompt
	CustomPatterns    []string `json:"custom_patterns,omitempty"`
}

// ObservabilityConfig 是可观测性模块的配置
type ObservabilityConfig struct {
	Enabled   bool   `json:"enabled"`
	LogLevel  string `json:"log_level"`  // debug / info / warn / error
	LogOutput string `json:"log_output"` // stdout / stderr 或文件路径
}

// OAuthConfig 是 OAuth 认证模块的配置
type OAuthConfig struct {
	Enabled        bool   `json:"enabled"`
	StorageBackend string `json:"storage_backend"` // file / keychain
	CallbackPort   int    `json:"callback_port"`   // 默认 8080
}

// PromptCacheConfig 是 Prompt 缓存功能的配置
type PromptCacheConfig struct {
	Enabled bool `json:"enabled"` // 是否启用 Prompt 缓存，默认 true
}

// ToolsConfig 是内置工具的配置
type ToolsConfig struct {
	WebSearch WebSearchConfig    `json:"web_search,omitempty"`
	WebFetch  WebFetchConfig     `json:"web_fetch,omitempty"`
	Timeout   ToolsTimeoutConfig `json:"timeout,omitempty"`
}

// WebFetchConfig 是 web_fetch 工具的配置
type WebFetchConfig struct {
	// AllowedInternalHosts 允许访问的内部主机列表（SSRF 白名单）
	AllowedInternalHosts []string `json:"allowed_internal_hosts,omitempty"`
}

// WebSearchConfig 是 web_search 工具的配置
type WebSearchConfig struct {
	Backend string `json:"backend"` // "tavily" 或 "exa"
	APIKey  string `json:"api_key,omitempty"`
}

// ToolsTimeoutConfig 是工具执行超时的配置
type ToolsTimeoutConfig struct {
	// Default 全局默认超时（秒），0 表示不超时
	Default int `json:"default"` // 默认 30
	// Overrides 按工具名称覆盖超时（秒）
	Overrides map[string]int `json:"overrides,omitempty"`
}

// SecurityConfig 是安全相关配置
type SecurityConfig struct {
	// SensitivePaths 敏感路径保护配置
	SensitivePaths SensitivePathsConfig `json:"sensitive_paths,omitempty"`
	// ProtectionLevel 保护级别: strict（禁止）/ warn（记录）/ off（关闭）
	ProtectionLevel string `json:"protection_level"` // 默认 "strict"
	// PermissionStore 权限存储方式: sqlite / memory
	PermissionStore string `json:"permission_store"` // 默认 "sqlite"
	// AuditRetentionDays 审计日志保留天数
	AuditRetentionDays int `json:"audit_retention_days"` // 默认 30
}

// SensitivePathsConfig 是敏感路径保护配置
type SensitivePathsConfig struct {
	// Block 额外的黑名单路径
	Block []string `json:"block,omitempty"`
	// Allow 白名单路径（覆盖默认黑名单）
	Allow []string `json:"allow,omitempty"`
}

// ProfilingConfig 是性能分析配置
type ProfilingConfig struct {
	Enabled bool   `json:"enabled"` // 默认 false
	Host    string `json:"host"`    // 默认 "127.0.0.1"
	Port    int    `json:"port"`    // 默认 6060
}

// ThemeConfig 是 TUI 主题配置
type ThemeConfig struct {
	Name       string `json:"name"`        // "dark", "light", "dracula", "monokai"
	CustomPath string `json:"custom_path"` // 自定义主题目录
}

// KeybindingsConfig 是键盘绑定配置
type KeybindingsConfig struct {
	Path string `json:"path"` // keybindings.json 路径
}

// TemplatesConfig 是模板系统配置
type TemplatesConfig struct {
	CustomDir       string `json:"custom_dir"`       // 用户自定义模板目录
	DefaultProvider string `json:"default_provider"` // 默认模板 provider
}

// SessionConfig 是会话系统的配置
type SessionConfig struct {
	Store      string          `json:"store"`       // "jsonl" 或 "sqlite"
	SQLitePath string          `json:"sqlite_path"` // SQLite 文件路径
	AutoTitle  AutoTitleConfig `json:"auto_title"`
	Queue      QueueConfig     `json:"queue"`
}

// DatabaseConfig 是数据库的配置
type DatabaseConfig struct {
	Mode string `json:"mode"` // "wal" 或 "delete"，默认 "wal"
}

// AutoTitleConfig 是自动标题生成的配置
type AutoTitleConfig struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"` // 用于生成标题的模型
}

// QueueConfig 是会话队列的配置
type QueueConfig struct {
	Enabled bool `json:"enabled"`
	MaxSize int  `json:"max_size"` // 最大队列大小
}

// defaultConfig 返回默认配置。
func defaultConfig() *Config {
	return &Config{
		Provider:         "openai",
		Model:            "gpt-4",
		Temperature:      0.7,
		MaxTokens:        4096,
		TopP:             1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		StopSequences:    nil,
		MaxIterations:    10,
		Timeout:          60,
		Verbose:          false,
		MCPConfigs:       nil,
		MaxToolCalls:     20,
		MaxContextTokens: 128000,
		// 压缩：默认不启用
		Compaction: CompactionConfig{
			Enabled:    false,
			Strategy:   "sliding_window",
			Threshold:  0.8,
			WindowSize: 10,
		},
		// 重试：默认启用
		Retry: RetryConfig{
			Enabled:     true,
			MaxAttempts: 3,
			BaseDelayMs: 2000,
			MaxDelayMs:  60000,
		},
		// 权限：默认 yolo 模式（不检查）
		Permission: PermissionConfig{
			Enabled: false,
			Mode:    "yolo",
		},
		// 子代理：默认启用
		SubAgent: SubAgentConfig{
			Enabled:       true,
			DefaultType:   "general",
			CostLimit:     1.0,
			MaxConcurrent: 5,
		},
		// 循环检测：默认启用
		LoopDetect: LoopDetectConfig{
			Enabled:           true,
			RepeatedThreshold: 3,
			ToolLoopThreshold: 5,
			ResponseStrategy:  "warn",
			CustomPatterns:    nil,
		},
		// 可观测性：默认不启用
		Observability: ObservabilityConfig{
			Enabled:   false,
			LogLevel:  "info",
			LogOutput: "stdout",
		},
		// 会话：默认 JSONL 存储，启用自动标题和队列
		Session: SessionConfig{
			Store:      "jsonl",
			SQLitePath: "",
			AutoTitle: AutoTitleConfig{
				Enabled: true,
				Model:   "",
			},
			Queue: QueueConfig{
				Enabled: true,
				MaxSize: 100,
			},
		},
		// 数据库：默认 WAL 模式
		Database: DatabaseConfig{
			Mode: "wal",
		},
		// 安全：默认 strict 保护
		Security: SecurityConfig{
			SensitivePaths: SensitivePathsConfig{
				Block: nil,
				Allow: nil,
			},
			ProtectionLevel:    "strict",
			PermissionStore:    "sqlite",
			AuditRetentionDays: 30,
		},
		// 性能分析：默认关闭
		Profiling: ProfilingConfig{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    6060,
		},
		// 主题：默认 dark
		Theme: ThemeConfig{
			Name: "dark",
		},
		// 模板：默认无自定义目录
		Templates: TemplatesConfig{
			DefaultProvider: "default",
		},
		// 工具超时：默认 30s
		Tools: ToolsConfig{
			Timeout: ToolsTimeoutConfig{
				Default:   30,
				Overrides: map[string]int{"bash": 60},
			},
		},
		// Prompt 缓存：默认启用
		PromptCache: PromptCacheConfig{
			Enabled: true,
		},
		// OAuth：默认不启用
		OAuth: OAuthConfig{
			Enabled:        false,
			StorageBackend: "file",
			CallbackPort:   8080,
		},
	}
}

// clone 深拷贝 Config，确保 map 和 slice 独立。
func (c *Config) clone() *Config {
	if c == nil {
		return nil
	}
	cp := *c
	if cp.MCPConfigs != nil {
		cp.MCPConfigs = make(map[string]interface{}, len(c.MCPConfigs))
		for k, v := range c.MCPConfigs {
			cp.MCPConfigs[k] = v
		}
	}
	if cp.StopSequences != nil {
		cp.StopSequences = make([]string, len(c.StopSequences))
		copy(cp.StopSequences, c.StopSequences)
	}
	// 深拷贝 LoopDetectConfig.CustomPatterns
	if cp.LoopDetect.CustomPatterns != nil {
		cp.LoopDetect.CustomPatterns = make([]string, len(c.LoopDetect.CustomPatterns))
		copy(cp.LoopDetect.CustomPatterns, c.LoopDetect.CustomPatterns)
	}
	// 深拷贝 Tools.Timeout.Overrides
	if cp.Tools.Timeout.Overrides != nil {
		cp.Tools.Timeout.Overrides = make(map[string]int, len(c.Tools.Timeout.Overrides))
		for k, v := range c.Tools.Timeout.Overrides {
			cp.Tools.Timeout.Overrides[k] = v
		}
	}
	// 深拷贝 Security.SensitivePaths
	if cp.Security.SensitivePaths.Block != nil {
		cp.Security.SensitivePaths.Block = make([]string, len(c.Security.SensitivePaths.Block))
		copy(cp.Security.SensitivePaths.Block, c.Security.SensitivePaths.Block)
	}
	if cp.Security.SensitivePaths.Allow != nil {
		cp.Security.SensitivePaths.Allow = make([]string, len(c.Security.SensitivePaths.Allow))
		copy(cp.Security.SensitivePaths.Allow, c.Security.SensitivePaths.Allow)
	}
	return &cp
}

// Validate 验证配置字段是否在合法范围内。
func (c *Config) Validate() error {
	if c.Temperature < 0.0 || c.Temperature > 2.0 {
		return fmt.Errorf("temperature must be between 0.0 and 2.0, got %f", c.Temperature)
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative, got %d", c.MaxTokens)
	}
	if c.MaxIterations < 0 {
		return fmt.Errorf("max_iterations must be non-negative, got %d", c.MaxIterations)
	}
	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative, got %d", c.Timeout)
	}
	if c.Profiling.Port != 0 && (c.Profiling.Port < 1024 || c.Profiling.Port > 65535) {
		return fmt.Errorf("profiling port must be between 1024 and 65535, got %d", c.Profiling.Port)
	}
	if c.OAuth.CallbackPort != 0 && (c.OAuth.CallbackPort < 1024 || c.OAuth.CallbackPort > 65535) {
		return fmt.Errorf("callback port must be between 1024 and 65535, got %d", c.OAuth.CallbackPort)
	}
	if c.TopP < 0.0 || c.TopP > 1.0 {
		return fmt.Errorf("top_p must be between 0.0 and 1.0, got %f", c.TopP)
	}
	if c.FrequencyPenalty < 0.0 || c.FrequencyPenalty > 2.0 {
		return fmt.Errorf("frequency_penalty must be between 0.0 and 2.0, got %f", c.FrequencyPenalty)
	}
	if c.PresencePenalty < 0.0 || c.PresencePenalty > 2.0 {
		return fmt.Errorf("presence_penalty must be between 0.0 and 2.0, got %f", c.PresencePenalty)
	}
	return nil
}

// Store 管理配置的原子读写和持久化。
type Store struct {
	mu     sync.RWMutex
	config *Config
	path   string
}

// NewStore 创建一个新的 Store，使用默认配置。
func NewStore(path string) *Store {
	return &Store{
		config: defaultConfig(),
		path:   path,
	}
}

// Get 返回当前配置的深拷贝，调用方修改返回的 Config 不会影响内部状态。
func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.clone()
}

// Mutate 原子地修改配置。
// fn 接收当前配置的深拷贝，修改后原子交换为新配置。
// 如果 fn panic，原配置保持不变（回滚）。
func (s *Store) Mutate(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	newConfig := s.config.clone()
	fn(newConfig)
	s.config = newConfig
	return nil
}

// Save 将配置原子地写入 JSON 文件（临时文件 + 重命名）。
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.config, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// Reload 从文件重新加载配置。
func (s *Store) Reload() error {
	s.mu.RLock()
	path := s.path
	s.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()

	return nil
}

// Load 从 JSON 文件加载配置。
// 文件不存在时返回使用默认配置的 Store。
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewStore(path), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &Store{
		config: cfg,
		path:   path,
	}, nil
}

// Discover 查找配置文件路径。
// 搜索顺序：当前目录 → 父目录（逐级向上）→ ~/.config/basework/config.json。
// 均不存在时返回 ~/.config/basework/config.json 作为默认路径。
func Discover() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}

	for {
		cfgPath := filepath.Join(dir, "config.json")
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // 已到根目录
		}
		dir = parent
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "basework", "config.json"), nil
}
