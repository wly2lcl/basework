package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Validate() 单元测试
// ---------------------------------------------------------------------------

func TestValidate_DefaultConfig_Passes(t *testing.T) {
	cfg := defaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidate_NegativeTemperature_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.Temperature = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative temperature")
	}
}

func TestValidate_TemperatureTooHigh_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.Temperature = 2.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for temperature > 2.0")
	}
}

func TestValidate_TemperatureBoundary_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.Temperature = 0.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("temperature 0.0 should be valid: %v", err)
	}
	cfg.Temperature = 2.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("temperature 2.0 should be valid: %v", err)
	}
}

func TestValidate_NegativeMaxTokens_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.MaxTokens = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative max_tokens")
	}
}

func TestValidate_ZeroMaxTokens_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.MaxTokens = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero max_tokens should be valid: %v", err)
	}
}

func TestValidate_NegativeMaxIterations_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.MaxIterations = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative max_iterations")
	}
}

func TestValidate_NegativeTimeout_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.Timeout = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative timeout")
	}
}

func TestValidate_InvalidProfilingPort_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiling.Port = 80
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for profiling port < 1024")
	}
}

func TestValidate_ValidProfilingPort_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiling.Port = 8080
	if err := cfg.Validate(); err != nil {
		t.Errorf("profiling port 8080 should be valid: %v", err)
	}
}

func TestValidate_ZeroProfilingPort_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiling.Port = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("profiling port 0 (unset) should be valid: %v", err)
	}
}

func TestValidate_InvalidCallbackPort_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.OAuth.CallbackPort = 999
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for callback port < 1024")
	}
}

func TestValidate_ValidCallbackPort_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.OAuth.CallbackPort = 8443
	if err := cfg.Validate(); err != nil {
		t.Errorf("callback port 8443 should be valid: %v", err)
	}
}

func TestValidate_TopPTooLow_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.TopP = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative top_p")
	}
}

func TestValidate_TopPTooHigh_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.TopP = 1.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for top_p > 1.0")
	}
}

func TestValidate_TopPBoundary_Passes(t *testing.T) {
	cfg := defaultConfig()
	cfg.TopP = 0.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("top_p 0.0 should be valid: %v", err)
	}
	cfg.TopP = 1.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("top_p 1.0 should be valid: %v", err)
	}
}

func TestValidate_FrequencyPenaltyTooLow_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.FrequencyPenalty = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative frequency_penalty")
	}
}

func TestValidate_FrequencyPenaltyTooHigh_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.FrequencyPenalty = 2.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for frequency_penalty > 2.0")
	}
}

func TestValidate_PresencePenaltyTooLow_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.PresencePenalty = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative presence_penalty")
	}
}

func TestValidate_PresencePenaltyTooHigh_Fails(t *testing.T) {
	cfg := defaultConfig()
	cfg.PresencePenalty = 2.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for presence_penalty > 2.0")
	}
}

// ---------------------------------------------------------------------------
// Load() 验证集成
// ---------------------------------------------------------------------------

func TestLoad_InvalidTemperature_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"temperature": -1.0}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when loading config with invalid temperature")
	}
}

func TestLoad_InvalidPort_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"profiling": {"port": 80}}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when loading config with invalid profiling port")
	}
}

// ---------------------------------------------------------------------------
// Reload() 验证集成
// ---------------------------------------------------------------------------

func TestReload_InvalidTemperature_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 先保存有效配置
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	// 写入无效配置
	data := `{"temperature": 3.0}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	if err := store.Reload(); err == nil {
		t.Fatal("expected error when reloading config with invalid temperature")
	}
}

func TestReload_InvalidMaxTokens_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	data := `{"max_tokens": -100}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	if err := store.Reload(); err == nil {
		t.Fatal("expected error when reloading config with negative max_tokens")
	}
}

func TestReload_ValidConfig_Passes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 写入有效完整配置
	cfg := &Config{
		Provider:    "anthropic",
		Model:       "claude-3-opus",
		Temperature: 0.5,
		MaxTokens:   8192,
		TopP:        0.9,
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := store.Reload(); err != nil {
		t.Errorf("reload with valid config should pass: %v", err)
	}

	got := store.Get()
	if got.Provider != "anthropic" {
		t.Errorf("expected provider=anthropic, got %q", got.Provider)
	}
}

// ---------------------------------------------------------------------------
// 多字段验证
// ---------------------------------------------------------------------------

func TestValidate_MultipleErrors_FirstReturned(t *testing.T) {
	cfg := defaultConfig()
	cfg.Temperature = -1.0
	cfg.MaxTokens = -1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	// 第一个错误应该是 temperature
	if err.Error() != "temperature must be between 0.0 and 2.0, got -1.000000" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Load() partial config: 缺失字段应保持零值但验证不应崩溃
// ---------------------------------------------------------------------------

func TestLoad_PartialConfig_Passes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// 只设置提供者，其他为零值（应通过验证，因为零值在范围内）
	data := `{"provider": "test", "model": "test-model"}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("load partial config should pass: %v", err)
	}

	cfg := store.Get()
	if cfg.Provider != "test" {
		t.Errorf("expected provider=test, got %q", cfg.Provider)
	}
}
