package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// UserTemplateManager 管理用户自定义模板。
// 从指定目录扫描 .txt 文件，支持热重载和模板优先级（用户 > 内置）。
type UserTemplateManager struct {
	dir       string            // .basework/prompts/ 目录
	templates map[string]string // name -> content
	modTimes  map[string]time.Time
}

// NewUserTemplateManager 创建用户模板管理器。
// dir 是用户自定义模板目录（例如 .basework/prompts/）。
func NewUserTemplateManager(dir string) *UserTemplateManager {
	return &UserTemplateManager{
		dir:       dir,
		templates: make(map[string]string),
		modTimes:  make(map[string]time.Time),
	}
}

// Scan 扫描用户模板目录，加载所有 .txt 文件。
// 文件名（不含扩展名）作为模板名称。
func (m *UserTemplateManager) Scan() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在不是错误
		}
		return fmt.Errorf("读取模板目录 %s 失败: %w", m.dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".txt")
		data, err := os.ReadFile(filepath.Join(m.dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("读取模板文件 %s 失败: %w", entry.Name(), err)
		}

		content := string(data)
		if err := m.Validate(content); err != nil {
			return fmt.Errorf("模板 %s 语法验证失败: %w", entry.Name(), err)
		}

		m.templates[name] = content

		info, err := entry.Info()
		if err == nil {
			m.modTimes[name] = info.ModTime()
		}
	}

	return nil
}

// Get 获取模板内容。优先返回用户自定义模板，不存在时返回 false。
func (m *UserTemplateManager) Get(name string) (string, bool) {
	content, ok := m.templates[name]
	return content, ok
}

// Reload 热重载：检查模板文件是否变更，如有变更重新加载。
// 返回是否有任何模板被更新。
func (m *UserTemplateManager) Reload() (bool, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("读取模板目录 %s 失败: %w", m.dir, err)
	}

	changed := false
	seen := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".txt")
		seen[name] = true

		info, err := entry.Info()
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		prevModTime, exists := m.modTimes[name]

		if exists && !modTime.After(prevModTime) {
			continue // 未变更
		}

		data, err := os.ReadFile(filepath.Join(m.dir, entry.Name()))
		if err != nil {
			continue
		}

		content := string(data)
		if err := m.Validate(content); err != nil {
			continue // 语法错误时跳过
		}

		m.templates[name] = content
		m.modTimes[name] = modTime
		changed = true
	}

	// 清理已删除的模板
	for name := range m.templates {
		if !seen[name] {
			delete(m.templates, name)
			delete(m.modTimes, name)
			changed = true
		}
	}

	return changed, nil
}

// Validate 验证模板语法是否正确。
// 使用 text/template 解析，不执行（不传入数据）。
func (m *UserTemplateManager) Validate(content string) error {
	_, err := template.New("validate").Option("missingkey=error").Parse(content)
	return err
}
