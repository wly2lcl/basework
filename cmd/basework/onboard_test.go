package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withInitGlobals(t *testing.T, cfgPath, providerName, model, preset string, force bool) {
	t.Helper()
	oldPath, oldProvider, oldModel, oldPreset, oldForce := cfgFile, initProvider, initModel, initPreset, initForce
	cfgFile, initProvider, initModel, initPreset, initForce = cfgPath, providerName, model, preset, force
	t.Cleanup(func() {
		cfgFile, initProvider, initModel, initPreset, initForce = oldPath, oldProvider, oldModel, oldPreset, oldForce
	})
}

func withTestStdin(t *testing.T, contents string, keepOpen bool) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})
	if contents != "" {
		if _, err := w.WriteString(contents); err != nil {
			t.Fatal(err)
		}
	}
	if !keepOpen {
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunInitNonInteractiveCreatesDeterministicConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withInitGlobals(t, path, "opencode", "", "readonly", false)
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENCODE_API_KEY", "OG_API_KEY"} {
		t.Setenv(name, "")
	}
	if err := runInitNonInteractive(); err != nil {
		t.Fatalf("runInitNonInteractive() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("配置不是 JSON: %v", err)
	}
	if got["provider"] != "opencode" || got["model"] != "big-pickle" || got["preset"] != "readonly" {
		t.Fatalf("配置默认值不符合预期: %#v", got)
	}
}

func TestRunInitNonInteractiveRejectsMissingRequiredKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withInitGlobals(t, path, "openai", "gpt-test", "", false)
	t.Setenv("OPENAI_API_KEY", "")
	if err := runInitNonInteractive(); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("缺 key 应明确失败，得到 %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("失败初始化不应创建配置，stat=%v", err)
	}
}

func TestRunInitNonInteractiveKeepsExistingConfigByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"provider":"openai","model":"old-model","temperature":0.2}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	withInitGlobals(t, path, "opencode", "new-model", "coding", false)
	if err := runInitNonInteractive(); err != nil {
		t.Fatalf("已有配置应直接保留: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("已有配置被覆盖:\n got %s\nwant %s", got, original)
	}
}

func TestRunInitNonInteractiveDoesNotReadOpenPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withInitGlobals(t, path, "opencode", "", "readonly", false)
	withTestStdin(t, "", true)

	done := make(chan error, 1)
	go func() { done <- runInitNonInteractive() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("非交互初始化不应失败: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("非交互初始化读取了保持打开的 stdin")
	}
}

func TestRunInitInteractiveEOFHasDeterministicResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENCODE_API_KEY", "OG_API_KEY"} {
		t.Setenv(name, "")
	}
	withTestStdin(t, "", false)
	if err := runInit(); err != nil {
		t.Fatalf("EOF 下交互初始化不应失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "basework", "config.json")); err != nil {
		t.Fatalf("EOF 下没有生成配置: %v", err)
	}
}

func TestRunInitInteractiveInvalidInputFallsBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENCODE_API_KEY", "OG_API_KEY"} {
		t.Setenv(name, "")
	}
	withTestStdin(t, "not-a-provider\nnot-a-model\n", false)
	if err := runInit(); err != nil {
		t.Fatalf("异常输入应回退默认值而非失败: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "basework", "config.json"))
	if err != nil {
		t.Fatalf("异常输入后没有配置: %v", err)
	}
	if !strings.Contains(string(data), `"provider": "opencode"`) {
		t.Fatalf("异常输入没有回退到稳定 provider: %s", data)
	}
}
