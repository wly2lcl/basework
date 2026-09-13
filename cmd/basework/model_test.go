package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
)

func TestRunModelListFreeOnly(t *testing.T) {
	var buf bytes.Buffer

	if err := runModelList(&buf, true); err != nil {
		t.Fatalf("runModelList: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"big-pickle", "mimo-v2.5-free", "yes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected free model list to contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "gpt-4o") {
		t.Fatalf("free-only model list should not contain paid model, got:\n%s", out)
	}
}

func TestRunModelUseOpenCodeFreeModel(t *testing.T) {
	oldCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = oldCfgFile })

	path := filepath.Join(t.TempDir(), "config.json")
	cfgFile = path
	store := config.NewStore(path)
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := runModelUse("mimo-v2.5-free"); err != nil {
		t.Fatalf("runModelUse: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg := loaded.Get()
	if cfg.Provider != "opencode" {
		t.Fatalf("expected provider opencode, got %q", cfg.Provider)
	}
	if cfg.Model != "mimo-v2.5-free" {
		t.Fatalf("expected model mimo-v2.5-free, got %q", cfg.Model)
	}
}

func TestRunModelUseUnknownModel(t *testing.T) {
	err := runModelUse("not-a-real-model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	if !strings.Contains(err.Error(), "model list") {
		t.Fatalf("expected model list hint, got %v", err)
	}
}

// TestRunModelListShowsCapabilitiesAndKey 验证列表同时展示能力结论与 key 要求，
// 且“免费”与“需要 key”是两个独立列。
func TestRunModelListShowsCapabilitiesAndKey(t *testing.T) {
	var buf bytes.Buffer
	if err := runModelList(&buf, false); err != nil {
		t.Fatalf("runModelList: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Tools", "Streaming", "Vision", "API key", "Free"} {
		if !strings.Contains(out, want) {
			t.Errorf("列表缺少列 %q:\n%s", want, out)
		}
	}
	// 纯文本模型必须显式标为 no，而不是留空或标成 yes。
	if !strings.Contains(out, "deepseek-chat") || !strings.Contains(out, "unknown") {
		t.Errorf("列表应同时出现有依据的模型与 unknown 结论:\n%s", out)
	}
}

// TestRunModelInfoReportsSourceAndUnknownNote 验证单模型信息给出依据来源。
func TestRunModelInfoReportsSourceAndUnknownNote(t *testing.T) {
	var buf bytes.Buffer
	if err := runModelInfo(&buf, "deepseek-chat"); err != nil {
		t.Fatalf("runModelInfo: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"deepseek-chat", "vision", "unsupported", "declared", "需要 API key: required"} {
		if !strings.Contains(out, want) {
			t.Errorf("model info 缺少 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "需要 API key: required（环境变量 OPENAI_API_KEY") == false {
		t.Errorf("model info 应给出实际读取的环境变量:\n%s", out)
	}
}

// TestRunModelInfoUnknownModelIsHonest 验证未知模型的能力结论是 unknown 而非支持。
func TestRunModelInfoUnknownModelIsHonest(t *testing.T) {
	var buf bytes.Buffer
	if err := runModelInfo(&buf, "mimo-v2.5-free"); err != nil {
		t.Fatalf("runModelInfo: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "unknown") {
		t.Errorf("未知模型应标为 unknown:\n%s", out)
	}
	if strings.Contains(out, "vision        supported") {
		t.Errorf("未知模型的视觉能力不应被声明为 supported:\n%s", out)
	}
}

// TestRunModelUseRefusesModelWithoutKey 验证缺 key 时给出明确错误与可选替代，
// 并且不会先把配置改掉（不静默换模型、不留下一个跑不起来的配置）。
func TestRunModelUseRefusesModelWithoutKey(t *testing.T) {
	oldCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = oldCfgFile })

	// 确保所有可能被读取的 key 环境变量为空。
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OG_API_KEY", "")
	t.Setenv("AZURE_API_KEY", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")

	path := filepath.Join(t.TempDir(), "config.json")
	cfgFile = path
	store := config.NewStore(path)
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err := runModelUse("deepseek-chat")
	if err == nil {
		t.Fatal("缺少 API key 时应返回错误")
	}
	msg := err.Error()
	for _, want := range []string{"OPENAI_API_KEY", "可选替代", "big-pickle"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q:\n%s", want, msg)
		}
	}

	loaded, loadErr := config.Load(path)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	cfg := loaded.Get()
	if cfg.Model == "deepseek-chat" {
		t.Error("缺 key 时不应写入一个无法运行的模型配置")
	}
}

// TestModelEntryRequiresKeySeparatesFreeFromKey 验证免费与 key 需求分离。
func TestModelEntryRequiresKeySeparatesFreeFromKey(t *testing.T) {
	free := modelEntry{Provider: "opencode", ModelID: "big-pickle", Free: true}
	if free.requiresKey() {
		t.Error("免费模型不应要求 key")
	}
	if got := free.keyStatus(); got != "not required" {
		t.Errorf("免费模型 keyStatus = %q", got)
	}
	paid := modelEntry{Provider: "openai", ModelID: "gpt-4o"}
	if !paid.requiresKey() {
		t.Error("付费 provider 应要求 key")
	}
	if got := paid.keyStatus(); got != "required" {
		t.Errorf("付费模型 keyStatus = %q", got)
	}
}
