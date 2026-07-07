// Package theme 提供 TUI 主题系统
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = ".basework"
	configFile = "theme.json"
)

// ThemeConfig 主题配置持久化结构
type ThemeConfig struct {
	Name string `json:"name"` // "dark", "light", "dracula", "monokai"
}

// configPath 返回配置文件路径
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

// LoadThemeConfig 加载主题配置
// 从 ~/.basework/theme.json 读取，如果文件不存在返回默认配置
func LoadThemeConfig() (*ThemeConfig, error) {
	path, err := configPath()
	if err != nil {
		return &ThemeConfig{Name: DefaultTheme.Name}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ThemeConfig{Name: DefaultTheme.Name}, nil
		}
		return &ThemeConfig{Name: DefaultTheme.Name}, fmt.Errorf("读取主题配置文件失败: %w", err)
	}

	var cfg ThemeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &ThemeConfig{Name: DefaultTheme.Name}, fmt.Errorf("解析主题配置文件失败: %w", err)
	}

	// 验证主题是否存在
	if _, ok := GetTheme(cfg.Name); !ok {
		cfg.Name = DefaultTheme.Name
	}

	return &cfg, nil
}

// SaveThemeConfig 保存主题配置到 ~/.basework/theme.json
func SaveThemeConfig(cfg *ThemeConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化主题配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入主题配置文件失败: %w", err)
	}

	return nil
}

// SetTheme 设置当前主题并保存
func SetTheme(name string) error {
	if _, ok := GetTheme(name); !ok {
		return fmt.Errorf("未知主题: %s", name)
	}
	cfg := &ThemeConfig{Name: name}
	return SaveThemeConfig(cfg)
}
