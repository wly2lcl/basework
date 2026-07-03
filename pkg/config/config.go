// Package config 提供 CoW（Copy-on-Write）配置系统。
// Store 通过原子指针交换实现无锁读取，支持 JSON 持久化。
package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

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
	return &cp
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

// Get 返回当前配置的不可变快照（零拷贝读取）。
// 调用方不应修改返回的 Config。
func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
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

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	s.mu.Lock()
	s.config = &cfg
	s.mu.Unlock()

	return nil
}

// Load 从 JSON 文件加载配置。
// 文件不存在时返回使用默认配置的 Store。
// 文件内容非法 JSON 时记录警告并返回默认配置的 Store。
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewStore(path), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("warning: config file %q has invalid JSON: %v, using defaults", path, err)
		return NewStore(path), nil
	}

	return &Store{
		config: &cfg,
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