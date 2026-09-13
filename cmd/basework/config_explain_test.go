package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
)

// TestConfigExplain_RedactsAndReflectsEnv 验证 explain 的三条验收：
// 秘密不出现在输出、环境覆盖与 runtime 同口径、输出反映有效值。
func TestConfigExplain_RedactsAndReflectsEnv(t *testing.T) {
	const secret = "sk-explain-test-secret-7f3a9c"
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := map[string]interface{}{
		"provider": "opencode",
		"model":    "big-pickle",
		"opencode": map[string]interface{}{"api_key": secret},
		"mcp_configs": map[string]interface{}{
			"deep": map[string]interface{}{
				"env": map[string]interface{}{"DEEP_TOKEN": "deep-token-leak-9b2"},
			},
		},
	}
	raw, err := json.Marshal(cfgJSON)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("写配置: %v", err)
	}
	before := fileHash(t, cfgPath)

	store, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}

	t.Setenv("BASEWORK_PROVIDER", "anthropic")
	t.Setenv("OPENCODE_API_KEY", "") // 隔离宿主环境，来源判定应归到配置文件

	var out bytes.Buffer
	if err := renderConfigExplain(&out, cfgPath, store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	text := out.String()

	// 1. 秘密（含任意嵌套的 mcp_configs）绝不出现。
	for _, leaked := range []string{secret, "deep-token-leak-9b2"} {
		if strings.Contains(text, leaked) {
			t.Errorf("秘密 %q 出现在 explain 输出中", leaked)
		}
	}
	if !strings.Contains(text, config.RedactionMarker) {
		t.Errorf("输出应包含脱敏标记:\n%s", text)
	}

	// 2. 环境覆盖与 runtime 同口径。
	if !strings.Contains(text, `BASEWORK_PROVIDER=anthropic`) {
		t.Errorf("输出应反映 BASEWORK_PROVIDER 覆盖:\n%s", text)
	}
	if !strings.Contains(text, `生效 provider: anthropic`) {
		t.Errorf("输出应给出生效 provider:\n%s", text)
	}

	// 3. key 来源只报有无，不报值。
	if !strings.Contains(text, "已设置（来源：配置文件）") {
		t.Errorf("opencode key 应显示为配置文件来源:\n%s", text)
	}

	// 4. 只读：配置文件字节不变。
	if after := fileHash(t, cfgPath); after != before {
		t.Fatalf("explain 改动了配置文件")
	}
}

// TestConfigExplain_MissingFileUsesDefaults 文件不存在时按默认值解释，且明确说明。
func TestConfigExplain_MissingFileUsesDefaults(t *testing.T) {
	t.Setenv("BASEWORK_PROVIDER", "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "not-exist.json")

	store, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load 对不存在的文件应回退默认值: %v", err)
	}

	var out bytes.Buffer
	if err := renderConfigExplain(&out, cfgPath, store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "文件不存在") {
		t.Errorf("应说明配置文件不存在:\n%s", text)
	}
	if !strings.Contains(text, `生效 provider = 配置值 "opencode"`) {
		t.Errorf("无覆盖时应显示配置值 provider:\n%s", text)
	}
}

// TestConfigExplain_KeySourceEnvUnset 验证「未设置 + 可用环境变量名」的提示路径。
func TestConfigExplain_KeySourceEnvUnset(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStore(filepath.Join(dir, "config.json"))

	t.Setenv("OPENAI_API_KEY", "")
	var out bytes.Buffer
	if err := renderConfigExplain(&out, filepath.Join(dir, "config.json"), store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	if !strings.Contains(out.String(), "未设置（可配置在配置文件，或环境变量 OPENAI_API_KEY）") {
		t.Errorf("未设置的 provider 应给出可用环境变量提示:\n%s", out.String())
	}
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	sum := sha256.Sum256(data)
	return string(sum[:])
}
