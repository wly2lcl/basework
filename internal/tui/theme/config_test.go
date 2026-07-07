// Package theme 测试
package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	oldUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return tmpDir, nil
	}
	t.Cleanup(func() {
		userHomeDir = oldUserHomeDir
	})

	return tmpDir
}

// TestLoadThemeConfig 测试加载主题配置
func TestLoadThemeConfig(t *testing.T) {
	withTempHome(t)

	// 默认返回 dark
	cfg, err := LoadThemeConfig()
	if err != nil {
		t.Fatalf("LoadThemeConfig 失败: %v", err)
	}
	if cfg.Name != "dark" {
		t.Fatalf("期望主题名为 dark，得到 %s", cfg.Name)
	}
}

// TestSaveThemeConfig 测试保存主题配置
func TestSaveThemeConfig(t *testing.T) {
	tmpDir := withTempHome(t)

	// 保存配置
	cfg := &ThemeConfig{Name: "light"}
	if err := SaveThemeConfig(cfg); err != nil {
		t.Fatalf("SaveThemeConfig 失败: %v", err)
	}

	// 验证文件存在
	configPath := filepath.Join(tmpDir, ".basework", "theme.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("配置文件未创建")
	}

	// 重新加载验证
	loaded, err := LoadThemeConfig()
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if loaded.Name != "light" {
		t.Fatalf("期望主题名为 light，得到 %s", loaded.Name)
	}
}

// TestSetTheme 测试设置主题
func TestSetTheme(t *testing.T) {
	withTempHome(t)

	// 设置有效主题
	if err := SetTheme("dracula"); err != nil {
		t.Fatalf("SetTheme(dracula) 失败: %v", err)
	}

	// 设置无效主题
	if err := SetTheme("nonexistent"); err == nil {
		t.Fatal("SetTheme(nonexistent) 应返回错误")
	}
}

// TestLoadThemeConfigInvalidFile 测试损坏的配置文件
func TestLoadThemeConfigInvalidFile(t *testing.T) {
	tmpDir := withTempHome(t)

	// 创建损坏的配置文件
	configDir := filepath.Join(tmpDir, ".basework")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("创建配置目录失败: %v", err)
	}
	configPath := filepath.Join(configDir, "theme.json")
	if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("写入损坏配置失败: %v", err)
	}

	// 应返回默认配置
	cfg, err := LoadThemeConfig()
	if err != nil {
		t.Logf("损坏文件应返回默认而非错误: %v", err)
	}
	if cfg.Name != "dark" {
		t.Fatalf("损坏文件应返回默认主题 'dark'，得到 %s", cfg.Name)
	}
}
