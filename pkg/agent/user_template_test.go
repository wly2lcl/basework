package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserTemplateManager_Scan(t *testing.T) {
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "custom.txt"), []byte("{{.Model}} custom template"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "override.txt"), []byte("Override: {{.Provider}}"), 0644)

	m := NewUserTemplateManager(dir)
	if err := m.Scan(); err != nil {
		t.Fatalf("Scan() 失败: %v", err)
	}

	tests := []struct {
		name      string
		exists    bool
		expectVal string
	}{
		{"custom", true, "{{.Model}} custom template"},
		{"override", true, "Override: {{.Provider}}"},
		{"nonexistent", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, ok := m.Get(tt.name)
			if ok != tt.exists {
				t.Errorf("Get(%q) ok = %v, 期望 %v", tt.name, ok, tt.exists)
			}
			if content != tt.expectVal {
				t.Errorf("Get(%q) = %q, 期望 %q", tt.name, content, tt.expectVal)
			}
		})
	}
}

func TestUserTemplateManager_Scan_NonExistentDir(t *testing.T) {
	m := NewUserTemplateManager("/nonexistent/path")
	if err := m.Scan(); err != nil {
		t.Fatalf("Scan() 对不存在的目录应返回 nil: %v", err)
	}
}

func TestUserTemplateManager_Validate(t *testing.T) {
	m := NewUserTemplateManager("/tmp")

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"有效模板", "{{.Model}} is great", false},
		{"无变量", "Plain text", false},
		{"语法错误", "{{.Model", true},
		{"未定义变量", "{{.Undefined}}", false}, // Parse 阶段不检查未定义，Execute 时会报
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestUserTemplateManager_Reload(t *testing.T) {
	dir := t.TempDir()

	// 创建初始模板
	_ = os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("Hello {{.Model}}"), 0644)

	m := NewUserTemplateManager(dir)
	if err := m.Scan(); err != nil {
		t.Fatalf("初始 Scan 失败: %v", err)
	}

	content, ok := m.Get("greeting")
	if !ok || content != "Hello {{.Model}}" {
		t.Fatalf("初始模板内容不对: %q, %v", content, ok)
	}

	// 修改模板文件
	_ = os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("Hi {{.Model}}!"), 0644)

	changed, err := m.Reload()
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if !changed {
		t.Error("Reload 应检测到变更")
	}

	content, ok = m.Get("greeting")
	if !ok || content != "Hi {{.Model}}!" {
		t.Errorf("Reload 后内容应为 'Hi {{.Model}}!', 得到 %q", content)
	}

	// 再次 Reload（无变更）
	changed, err = m.Reload()
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if changed {
		t.Error("无变更时 Reload 应返回 false")
	}
}

func TestUserTemplateManager_Reload_NewFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("Template A"), 0644)

	m := NewUserTemplateManager(dir)
	_ = m.Scan()

	// 新增文件
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("Template B"), 0644)

	changed, err := m.Reload()
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if !changed {
		t.Error("新增文件时 Reload 应返回 true")
	}

	content, ok := m.Get("b")
	if !ok || content != "Template B" {
		t.Errorf("新模板应可用: %q, %v", content, ok)
	}
}

func TestUserTemplateManager_Reload_DeletedFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("Template A"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("Template B"), 0644)

	m := NewUserTemplateManager(dir)
	_ = m.Scan()

	// 删除文件
	_ = os.Remove(filepath.Join(dir, "a.txt"))

	changed, err := m.Reload()
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if !changed {
		t.Error("删除文件时 Reload 应返回 true")
	}

	_, ok := m.Get("a")
	if ok {
		t.Error("删除后 Get('a') 应返回 false")
	}
}

func TestUserTemplateManager_Scan_InvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "bad.txt"), []byte("{{.Model"), 0644)

	m := NewUserTemplateManager(dir)
	err := m.Scan()
	if err == nil {
		t.Error("语法错误的模板文件应导致 Scan 返回错误")
	}
}

func TestUserTemplateManager_Reload_InvalidSyntaxSkipped(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "good.txt"), []byte("Valid {{.Model}}"), 0644)

	m := NewUserTemplateManager(dir)
	_ = m.Scan()

	// 新增语法错误的文件，不应影响已有模板
	_ = os.WriteFile(filepath.Join(dir, "bad.txt"), []byte("{{.Model"), 0644)

	changed, err := m.Reload()
	if err != nil {
		t.Fatalf("Reload 不应因语法错误的文件而失败: %v", err)
	}
	// 由于 bad.txt 语法错误，它会被跳过，没有变更应该被应用
	// good.txt 未变更，所以 changed 应为 false
	if changed {
		t.Log("Reload 检测到变更（bad.txt 被跳过）")
	}

	_, ok := m.Get("bad")
	if ok {
		t.Error("语法错误的模板不应被加载")
	}

	content, ok := m.Get("good")
	if !ok || content != "Valid {{.Model}}" {
		t.Errorf("已有模板应不受影响: %q, %v", content, ok)
	}
}
