// Package theme 提供 TUI 主题系统
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadCustomThemes 从指定目录加载自定义主题
// 扫描 dir/*.json，每个 JSON 文件应包含一个 Theme 结构
func LoadCustomThemes(dir string) ([]*Theme, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取自定义主题目录失败: %w", err)
	}

	var themes []*Theme
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		theme, err := loadThemeFile(path)
		if err != nil {
			// 跳过无效文件但不中断
			continue
		}
		themes = append(themes, theme)
	}

	return themes, nil
}

// loadThemeFile 从 JSON 文件加载单个主题
func loadThemeFile(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取主题文件 %s 失败: %w", path, err)
	}

	var t Theme
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("解析主题文件 %s 失败: %w", path, err)
	}

	if err := ValidateTheme(&t); err != nil {
		return nil, fmt.Errorf("主题文件 %s 验证失败: %w", path, err)
	}

	return &t, nil
}

// ValidateTheme 检查主题是否包含所有必需的颜色角色
func ValidateTheme(t *Theme) error {
	if t.Name == "" {
		return fmt.Errorf("主题名称不能为空")
	}
	if t.Colors == nil {
		return fmt.Errorf("主题颜色映射不能为空")
	}

	var missing []ColorRole
	for _, role := range requiredRoles {
		if _, ok := t.Colors[role]; !ok {
			missing = append(missing, role)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("主题 %q 缺少必需颜色角色: %v", t.Name, missing)
	}

	return nil
}
