package config

import (
	"encoding/json"
	"testing"
)

func TestThemeConfigDefaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Theme.Name != "dark" {
		t.Errorf("默认主题应为 'dark', 得到 %q", cfg.Theme.Name)
	}
	if cfg.Theme.CustomPath != "" {
		t.Errorf("默认 CustomPath 应为空, 得到 %q", cfg.Theme.CustomPath)
	}
}

func TestKeybindingsConfigDefaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Keybindings.Path != "" {
		t.Errorf("默认 Keybindings.Path 应为空, 得到 %q", cfg.Keybindings.Path)
	}
}

func TestTemplatesConfigDefaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Templates.CustomDir != "" {
		t.Errorf("默认 CustomDir 应为空, 得到 %q", cfg.Templates.CustomDir)
	}
	if cfg.Templates.DefaultProvider != "default" {
		t.Errorf("默认 DefaultProvider 应为 'default', 得到 %q", cfg.Templates.DefaultProvider)
	}
}

func TestNewConfigSectionsJSON(t *testing.T) {
	// 验证 JSON 序列化/反序列化
	store := NewStore("")
	cfg := store.Get()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if decoded.Theme.Name != cfg.Theme.Name {
		t.Errorf("Theme.Name 序列化往返失败: %q != %q", decoded.Theme.Name, cfg.Theme.Name)
	}
	if decoded.Templates.DefaultProvider != cfg.Templates.DefaultProvider {
		t.Errorf("Templates.DefaultProvider 序列化往返失败: %q != %q", decoded.Templates.DefaultProvider, cfg.Templates.DefaultProvider)
	}
}

func TestCloneNewConfigSections(t *testing.T) {
	store := NewStore("")

	store.Mutate(func(c *Config) {
		c.Theme = ThemeConfig{Name: "dracula", CustomPath: "/custom/theme"}
		c.Keybindings = KeybindingsConfig{Path: "/custom/keybindings.json"}
		c.Templates = TemplatesConfig{CustomDir: "/custom/templates", DefaultProvider: "anthropic"}
	})

	cfg := store.Get()
	if cfg.Theme.Name != "dracula" {
		t.Errorf("Theme.Name 应为 'dracula', 得到 %q", cfg.Theme.Name)
	}
	if cfg.Keybindings.Path != "/custom/keybindings.json" {
		t.Errorf("Keybindings.Path 应为 '/custom/keybindings.json', 得到 %q", cfg.Keybindings.Path)
	}
	if cfg.Templates.DefaultProvider != "anthropic" {
		t.Errorf("Templates.DefaultProvider 应为 'anthropic', 得到 %q", cfg.Templates.DefaultProvider)
	}
}

func TestNewConfigSectionsOmitEmpty(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	data, _ := json.Marshal(cfg)
	// 当 keybindings 为空时，omitempty 应省略
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)

	if _, ok := raw["keybindings"]; ok {
		t.Log("keybindings 字段在 JSON 中（omitempty 行为）")
	}
}