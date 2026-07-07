package agent

import (
	"testing"
)

func TestRouteForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"gemini", "gemini"},
		{"opencode", "openai"},   // OpenCode 使用 OpenAI 兼容协议
		{"bedrock", "anthropic"}, // Bedrock 常用 Claude
		{"azure", "default"},
		{"ollama", "default"},
		{"copilot", "default"},
		{"", "default"},
		{"unknown", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+">"+tt.want, func(t *testing.T) {
			got := RouteForProvider(tt.provider)
			if got != tt.want {
				t.Errorf("RouteForProvider(%q) = %q, 期望 %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestRouteForProvider_CombinedWithLoad(t *testing.T) {
	// 验证 RouteForProvider 的结果可以传给 LoadBuiltinTemplate
	providers := []string{"anthropic", "openai", "gemini", "opencode", "bedrock", "ollama", ""}
	for _, p := range providers {
		route := RouteForProvider(p)
		tmpl, err := LoadBuiltinTemplate(route)
		if err != nil {
			t.Errorf("RouteForProvider(%q) -> %q -> LoadBuiltinTemplate 失败: %v", p, route, err)
			continue
		}
		if tmpl == "" {
			t.Errorf("RouteForProvider(%q) -> %q 返回空模板", p, route)
		}
	}
}
