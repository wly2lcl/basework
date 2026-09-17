package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// presetTestConfig 返回默认配置的独立实例（避免用例间共享可变状态）。
func presetTestConfig() *Config {
	return defaultConfig()
}

// writeTempConfig 把原始 JSON 写进临时文件，返回路径（供 Load 用例使用）。
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时配置: %v", err)
	}
	return path
}

func TestApplyPreset_EmptyIsNoOp(t *testing.T) {
	cfg := presetTestConfig()
	before := *cfg
	if err := ApplyPreset(cfg, ""); err != nil {
		t.Fatalf("空预设应直接成功: %v", err)
	}
	if cfg.Tools.Allowed != nil || cfg.Permission.Enabled != before.Permission.Enabled {
		t.Errorf("空预设不得改动配置: allowed=%v perm=%v", cfg.Tools.Allowed, cfg.Permission.Enabled)
	}
}

func TestApplyPreset_ReadOnlyExpandsWhitelistAndPermission(t *testing.T) {
	cfg := presetTestConfig()
	cfg.Permission.Mode = ""
	if err := ApplyPreset(cfg, PresetReadonly); err != nil {
		t.Fatalf("展开 readonly: %v", err)
	}
	if len(cfg.Tools.Allowed) != len(readonlyToolset) {
		t.Fatalf("白名单应整体替换为只读集合: %v", cfg.Tools.Allowed)
	}
	for i, name := range readonlyToolset {
		if cfg.Tools.Allowed[i] != name {
			t.Errorf("白名单[%d]=%q，期望 %q", i, cfg.Tools.Allowed[i], name)
		}
	}
	if !cfg.Permission.Enabled {
		t.Error("readonly 应强制开启权限检查")
	}
	if cfg.Permission.Mode != "deny-all" {
		t.Fatalf("空权限模式应与 runtime 回退一致为 deny-all，得到 %q", cfg.Permission.Mode)
	}
}

func TestApplyPreset_ReadOnlyConflictsWithExplicitAllowed(t *testing.T) {
	cfg := presetTestConfig()
	cfg.Tools.Allowed = []string{"read", "bash"}
	err := ApplyPreset(cfg, PresetReadonly)
	if !errors.Is(err, ErrPresetConflict) {
		t.Fatalf("双重指定应报 ErrPresetConflict，得到: %v", err)
	}
	// 冲突时不得留下半展开状态
	if cfg.Permission.Enabled {
		t.Error("冲突路径不得改动权限配置")
	}
}

func TestApplyPreset_UnknownName(t *testing.T) {
	cfg := presetTestConfig()
	err := ApplyPreset(cfg, "nano-banana")
	if !errors.Is(err, ErrUnknownPreset) {
		t.Fatalf("未知预设应报 ErrUnknownPreset，得到: %v", err)
	}
}

func TestApplyPreset_CodingKeepsEverything(t *testing.T) {
	cfg := presetTestConfig()
	cfg.Tools.Allowed = []string{"read"} // coding 不与白名单冲突
	if err := ApplyPreset(cfg, PresetCoding); err != nil {
		t.Fatalf("coding 应总是成功: %v", err)
	}
	if len(cfg.Tools.Allowed) != 1 || cfg.Tools.Allowed[0] != "read" {
		t.Errorf("coding 不得改动工具白名单: %v", cfg.Tools.Allowed)
	}
}

func TestApplyPreset_ReadOnlyIsIdempotent(t *testing.T) {
	// 回归：Load 展开过一次后，命令行同值预设在运行入口会再次 Apply。
	// 第二次展开不得因「allowed 已填充」而误报冲突。
	cfg := presetTestConfig()
	if err := ApplyPreset(cfg, PresetReadonly); err != nil {
		t.Fatalf("第一次展开: %v", err)
	}
	if err := ApplyPreset(cfg, PresetReadonly); err != nil {
		t.Fatalf("同值二次展开应幂等: %v", err)
	}
	if len(cfg.Tools.Allowed) != len(readonlyToolset) {
		t.Errorf("二次展开后白名单不应变化: %v", cfg.Tools.Allowed)
	}
}

func TestIsKnownPreset(t *testing.T) {
	if !IsKnownPreset(PresetReadonly) || !IsKnownPreset(PresetCoding) {
		t.Error("内置预设应被识别")
	}
	if IsKnownPreset("") {
		t.Error("空串不是预设")
	}
	if IsKnownPreset("turbo") {
		t.Error("未知名不应被识别")
	}
}

func TestToolAllowed(t *testing.T) {
	cases := []struct {
		name      string
		allowlist []string
		tool      string
		want      bool
	}{
		{"空白名单不限制", nil, "bash", true},
		{"精确命中", []string{"read", "grep"}, "read", true},
		{"精确未命中", []string{"read"}, "bash", false},
		{"前缀通配命中", []string{"lsp_*"}, "lsp_diagnostics", true},
		{"前缀通配未命中", []string{"lsp_*"}, "glob", false},
		{"裸星号不是通配", []string{"*"}, "bash", false},
	}
	for _, tc := range cases {
		if got := ToolAllowed(tc.allowlist, tc.tool); got != tc.want {
			t.Errorf("%s: ToolAllowed(%v, %q)=%v，期望 %v", tc.name, tc.allowlist, tc.tool, got, tc.want)
		}
	}
}

func TestLoad_ExpandsFilePreset(t *testing.T) {
	path := writeTempConfig(t, `{
		"provider": "openai",
		"model": "gpt-4o-mini",
		"preset": "readonly"
	}`)
	store, err := Load(path)
	if err != nil {
		t.Fatalf("加载含 preset 的配置: %v", err)
	}
	cfg := store.Get()
	if cfg.Preset != PresetReadonly {
		t.Errorf("Preset 字段应保留声明值: %q", cfg.Preset)
	}
	if len(cfg.Tools.Allowed) == 0 {
		t.Error("加载时应已展开白名单")
	}
	if !cfg.Permission.Enabled {
		t.Error("加载时应已强制权限检查")
	}
}

func TestLoad_UnknownFilePresetFails(t *testing.T) {
	path := writeTempConfig(t, `{
		"provider": "openai",
		"model": "gpt-4o-mini",
		"preset": "turbo"
	}`)
	if _, err := Load(path); !errors.Is(err, ErrUnknownPreset) {
		t.Fatalf("未知文件内预设应在加载时报错，得到: %v", err)
	}
}

func TestReadonlyToolset_ReturnsCopy(t *testing.T) {
	a := ReadonlyToolset()
	a[0] = "bash"
	if ReadonlyToolset()[0] == "bash" {
		t.Fatal("ReadonlyToolset 必须返回拷贝，防止调用方污染全局定义")
	}
}
