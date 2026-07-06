// Package theme 测试
package theme

import (
	"testing"
)

// TestDefaultTheme 测试默认主题不为空
func TestDefaultTheme(t *testing.T) {
	if DefaultTheme == nil {
		t.Fatal("DefaultTheme 不应为 nil")
	}
	if DefaultTheme.Name != "dark" {
		t.Fatalf("期望主题名为 dark，得到 %s", DefaultTheme.Name)
	}
}

// TestGetTheme 测试获取主题
func TestGetTheme(t *testing.T) {
	tests := []struct {
		name    string
		wantOk  bool
		wantNil bool
	}{
		{"dark", true, false},
		{"light", true, false},
		{"dracula", true, false},
		{"monokai", true, false},
		{"nonexistent", false, true},
	}

	for _, tt := range tests {
		got, ok := GetTheme(tt.name)
		if ok != tt.wantOk {
			t.Fatalf("GetTheme(%q) ok = %v，期望 %v", tt.name, ok, tt.wantOk)
		}
		if !tt.wantNil && got == nil {
			t.Fatalf("GetTheme(%q) 返回了 nil", tt.name)
		}
	}
}

// TestListThemes 测试列出主题
func TestListThemes(t *testing.T) {
	themes := ListThemes()
	if len(themes) < 4 {
		t.Fatalf("至少应有 4 个内置主题，得到 %d", len(themes))
	}
}

// TestThemeGet 测试获取颜色值
func TestThemeGet(t *testing.T) {
	color := DarkTheme.Get(ColorBackground)
	if color == "" {
		t.Fatal("DarkTheme.Get(ColorBackground) 不应为空")
	}

	// 测试不存在的角色
	unknown := DarkTheme.Get(ColorRole("nonexistent"))
	if unknown == "" {
		t.Fatal("不存在的角色应返回默认值")
	}
}

// TestThemeLipglossColor 测试 LipglossColor
func TestThemeLipglossColor(t *testing.T) {
	color := DarkTheme.LipglossColor(ColorAccent)
	if color == "" {
		t.Fatal("LipglossColor 不应为空字符串")
	}
}

// TestAllThemesHaveRequiredColors 测试所有主题都有必需颜色
func TestAllThemesHaveRequiredColors(t *testing.T) {
	themes := []*Theme{DarkTheme, LightTheme, DraculaTheme, MonokaiTheme}
	for _, tm := range themes {
		if err := ValidateTheme(tm); err != nil {
			t.Fatalf("主题 %q 验证失败: %v", tm.Name, err)
		}
	}
}

// TestThemeDeepCopy 通过 AdaptTheme 验证主题不被修改
func TestThemeAdaptNoMutation(t *testing.T) {
	original := DarkTheme.Get(ColorAccent)
	_ = AdaptTheme(DarkTheme, ColorDepthTrueColor)
	if DarkTheme.Get(ColorAccent) != original {
		t.Fatal("AdaptTheme 不应修改原始主题")
	}
}