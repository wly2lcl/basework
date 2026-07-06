package agent

import (
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		vars    TemplateVars
		want    string
		wantErr bool
	}{
		{
			name: "基本变量替换",
			tmpl: "Model: {{.Model}}, Provider: {{.Provider}}",
			vars: TemplateVars{
				Model:    "claude-3.5-sonnet",
				Provider: "anthropic",
			},
			want:    "Model: claude-3.5-sonnet, Provider: anthropic",
			wantErr: false,
		},
		{
			name: "空模板",
			tmpl: "",
			vars: TemplateVars{},
			want:    "",
			wantErr: false,
		},
		{
			name: "未定义变量应报错",
			tmpl: "{{.UndefinedVar}}",
			vars: TemplateVars{},
			wantErr: true,
		},
		{
			name: "语法错误应报错",
			tmpl: "{{.Model",
			vars: TemplateVars{},
			wantErr: true,
		},
		{
			name: "全部变量替换",
			tmpl: "Model={{.Model}} Provider={{.Provider}} Dir={{.WorkingDir}} Date={{.Date}} Platform={{.Platform}} Arch={{.Arch}} Shell={{.Shell}}",
			vars: TemplateVars{
				Model:      "gpt-4",
				Provider:   "openai",
				WorkingDir: "/home/user",
				Date:       "2026-07-06",
				Platform:   "darwin",
				Arch:       "arm64",
				Shell:      "zsh",
			},
			want:    "Model=gpt-4 Provider=openai Dir=/home/user Date=2026-07-06 Platform=darwin Arch=arm64 Shell=zsh",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.tmpl, tt.vars)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderTemplate() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RenderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadBuiltinTemplate(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"gemini", "gemini"},
		{"default", "default"},
		{"空字符串走默认", ""},
		{"未知名称走默认", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := LoadBuiltinTemplate(tt.in)
			if err != nil {
				t.Fatalf("LoadBuiltinTemplate(%q) 返回错误: %v", tt.in, err)
			}
			if tmpl == "" {
				t.Errorf("LoadBuiltinTemplate(%q) 返回空模板", tt.in)
			}
		})
	}
}

func TestRenderTemplate_WithContext(t *testing.T) {
	vars := NewTemplateVars()
	vars.Model = "test-model"
	vars.Provider = "test-provider"
	vars.Context = "Extra context here"

	result, err := RenderTemplate("{{.Model}} - {{.Context}}", vars)
	if err != nil {
		t.Fatalf("RenderTemplate 失败: %v", err)
	}
	if result != "test-model - Extra context here" {
		t.Errorf("期望 'test-model - Extra context here', 得到 %q", result)
	}
}

func TestNewTemplateVars(t *testing.T) {
	vars := NewTemplateVars()
	if vars.Date == "" {
		t.Error("NewTemplateVars() 应设置 Date 字段")
	}
}

func TestBuiltinTemplatesRenderable(t *testing.T) {
	builtins := []string{"anthropic", "openai", "gemini", "default"}
	vars := TemplateVars{
		Model:      "test-model",
		Provider:   "test-provider",
		WorkingDir: "/test",
		Date:       "2026-07-06",
		Platform:   "linux",
		Arch:       "amd64",
		Shell:      "bash",
		Context:    "test context",
	}

	for _, name := range builtins {
		t.Run(name, func(t *testing.T) {
			tmpl, err := LoadBuiltinTemplate(name)
			if err != nil {
				t.Fatalf("LoadBuiltinTemplate(%q) 失败: %v", name, err)
			}
			result, err := RenderTemplate(tmpl, vars)
			if err != nil {
				t.Fatalf("RenderTemplate(%q) 失败: %v", name, err)
			}
			if result == "" {
				t.Errorf("RenderTemplate(%q) 返回空字符串", name)
			}
		})
	}
}