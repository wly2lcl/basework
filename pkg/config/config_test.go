package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// 测试辅助函数
// ---------------------------------------------------------------------------

// tempConfigDir 创建临时目录用于测试，返回清理函数。
func tempConfigDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "basework-config-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	return dir, func() { os.RemoveAll(dir) }
}

// writeConfigFile 在指定目录写入 config.json。
func writeConfigFile(t *testing.T, dir string, cfg *Config) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// chdir 切换工作目录，返回恢复函数。
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	return func() { os.Chdir(oldDir) }
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestConfigDefaults 验证默认配置的字段值。
func TestConfigDefaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Provider != "openai" {
		t.Errorf("expected Provider 'openai', got %q", cfg.Provider)
	}
	if cfg.Model != "gpt-4" {
		t.Errorf("expected Model 'gpt-4', got %q", cfg.Model)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected Temperature 0.7, got %f", cfg.Temperature)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("expected MaxTokens 4096, got %d", cfg.MaxTokens)
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", cfg.MaxIterations)
	}
	if cfg.Timeout != 60 {
		t.Errorf("expected Timeout 60, got %d", cfg.Timeout)
	}
	if cfg.MaxToolCalls != 20 {
		t.Errorf("expected MaxToolCalls 20, got %d", cfg.MaxToolCalls)
	}
	if cfg.MaxContextTokens != 128000 {
		t.Errorf("expected MaxContextTokens 128000, got %d", cfg.MaxContextTokens)
	}
	if cfg.Verbose {
		t.Error("expected Verbose false")
	}
}

// TestConfigGet 验证 Get() 返回配置的深拷贝。
func TestConfigGet(t *testing.T) {
	store := NewStore("")
	cfg1 := store.Get()
	cfg2 := store.Get()

	// 应为不同指针（深拷贝）
	if cfg1 == cfg2 {
		t.Error("Get() should return a deep copy (different pointer)")
	}

	// 值应相等
	if cfg1.Provider != cfg2.Provider {
		t.Errorf("values should be equal, got %q vs %q", cfg1.Provider, cfg2.Provider)
	}
}

// TestConfigMutate 验证 Mutate 原子地修改配置。
func TestConfigMutate(t *testing.T) {
	store := NewStore("")

	if err := store.Mutate(func(c *Config) {
		c.Provider = "anthropic"
		c.Model = "claude-3-opus"
		c.Temperature = 0.5
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	cfg := store.Get()
	if cfg.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic', got %q", cfg.Provider)
	}
	if cfg.Model != "claude-3-opus" {
		t.Errorf("expected Model 'claude-3-opus', got %q", cfg.Model)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("expected Temperature 0.5, got %f", cfg.Temperature)
	}
	// 其他字段应保持默认
	if cfg.MaxTokens != 4096 {
		t.Errorf("expected MaxTokens 4096, got %d", cfg.MaxTokens)
	}
}

// TestConfigMutatePanicRollback 验证 Mutate panic 时原配置不受影响。
func TestConfigMutatePanicRollback(t *testing.T) {
	store := NewStore("")
	original := store.Get()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from Mutate")
		}
		// 验证配置未改变
		cfg := store.Get()
		if cfg.Provider != original.Provider {
			t.Errorf("expected Provider %q, got %q", original.Provider, cfg.Provider)
		}
	}()

	store.Mutate(func(c *Config) {
		c.Provider = "should-not-be-set"
		panic("something went wrong")
	})
}

// TestConfigMutateMCPConfigs 验证 MCPConfigs 的深拷贝。
func TestConfigMutateMCPConfigs(t *testing.T) {
	store := NewStore("")

	// 设置 MCP 配置
	if err := store.Mutate(func(c *Config) {
		c.MCPConfigs = map[string]interface{}{
			"server1": map[string]interface{}{
				"command": "node",
				"args":    []interface{}{"server.js"},
			},
		}
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	cfg := store.Get()
	if len(cfg.MCPConfigs) != 1 {
		t.Fatalf("expected 1 MCP config, got %d", len(cfg.MCPConfigs))
	}

	// 再次 Mutate 应不影响原配置
	if err := store.Mutate(func(c *Config) {
		c.MCPConfigs = nil
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	// 验证上一个 Mutate 创建的快照仍然有效
	if cfg.MCPConfigs != nil {
		// 原来的 map 应该没有被清除
		if len(cfg.MCPConfigs) != 1 {
			t.Errorf("original snapshot should have 1 MCP config, got %d", len(cfg.MCPConfigs))
		}
	}
}

// TestConfigCoW 验证并发读取不阻塞。
func TestConfigCoW(t *testing.T) {
	store := NewStore("")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := store.Get()
			if cfg == nil {
				t.Error("Get() returned nil")
			}
		}()
	}
	wg.Wait()
}

// TestConfigConcurrentReadWrite 验证并发读写安全。
func TestConfigConcurrentReadWrite(t *testing.T) {
	store := NewStore("")

	var wg sync.WaitGroup
	// 并发读取
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cfg := store.Get()
				_ = cfg.Provider
			}
		}()
	}

	// 并发写入
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				store.Mutate(func(c *Config) {
					c.Temperature = float64(j) / 100.0
				})
			}
		}()
	}

	wg.Wait()
}

// TestConfigSaveLoadRoundtrip 验证 Save 后 Load 能正确恢复。
func TestConfigSaveLoadRoundtrip(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 修改配置
	if err := store.Mutate(func(c *Config) {
		c.Provider = "anthropic"
		c.Model = "claude-3-opus"
		c.Temperature = 0.3
		c.MaxTokens = 8192
		c.Verbose = true
		c.MCPConfigs = map[string]interface{}{
			"filesystem": map[string]interface{}{
				"command": "npx",
				"args":    []interface{}{"-y", "@modelcontextprotocol/server-filesystem"},
			},
		}
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	// 保存
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 加载
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg := loaded.Get()
	if cfg.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic', got %q", cfg.Provider)
	}
	if cfg.Model != "claude-3-opus" {
		t.Errorf("expected Model 'claude-3-opus', got %q", cfg.Model)
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("expected Temperature 0.3, got %f", cfg.Temperature)
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("expected MaxTokens 8192, got %d", cfg.MaxTokens)
	}
	if !cfg.Verbose {
		t.Error("expected Verbose true")
	}
	if cfg.MCPConfigs == nil {
		t.Fatal("expected MCPConfigs to be loaded")
	}
	if _, ok := cfg.MCPConfigs["filesystem"]; !ok {
		t.Error("expected filesystem MCP config")
	}
}

// TestConfigLoadFileNotExists 验证文件不存在时返回默认配置。
func TestConfigLoadFileNotExists(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "nonexistent.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load should not return error for non-existent file: %v", err)
	}
	if store == nil {
		t.Fatal("Load should return a Store for non-existent file")
	}

	cfg := store.Get()
	if cfg.Provider != "openai" {
		t.Errorf("expected default Provider 'openai', got %q", cfg.Provider)
	}
}

// TestConfigLoadInvalidJSON 验证非法 JSON 文件返回错误。
func TestConfigLoadInvalidJSON(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not valid json{"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load 应返回错误，但得到了 nil")
	}
}

// TestConfigDiscoverCurrentDir 验证 Discover 优先查找当前目录。
func TestConfigDiscoverCurrentDir(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	// 在当前目录创建 config.json
	writeConfigFile(t, dir, &Config{
		Provider: "test-provider",
		Model:    "test-model",
	})

	restore := chdir(t, dir)
	defer restore()

	path, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// 解析 symlink（macOS /var → /private/var）
	expected, _ := filepath.EvalSymlinks(filepath.Join(dir, "config.json"))
	got, _ := filepath.EvalSymlinks(path)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestConfigDiscoverFallbackToGlobal 验证无配置文件时返回全局路径。
func TestConfigDiscoverFallbackToGlobal(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	// 确保当前目录没有 config.json
	restore := chdir(t, dir)
	defer restore()

	path, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	expected := filepath.Join(home, ".config", "basework", "config.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// TestConfigDiscoverParentDir 验证 Discover 向上查找父目录。
func TestConfigDiscoverParentDir(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	// 在父目录创建 config.json（子目录中不应有）
	writeConfigFile(t, dir, &Config{
		Provider: "parent-provider",
	})

	subDir := filepath.Join(dir, "subdir", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	restore := chdir(t, subDir)
	defer restore()

	path, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// 解析 symlink（macOS /var → /private/var）
	expected, _ := filepath.EvalSymlinks(filepath.Join(dir, "config.json"))
	got, _ := filepath.EvalSymlinks(path)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestConfigReload 验证 Reload 能重新加载文件内容。
func TestConfigReload(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 初始配置
	cfg := store.Get()
	if cfg.Provider != "openai" {
		t.Errorf("expected default Provider 'openai', got %q", cfg.Provider)
	}

	// 直接写入文件（模拟外部修改）
	writeConfigFile(t, dir, &Config{
		Provider: "anthropic",
		Model:    "claude-3-opus",
	})

	// 重新加载
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	cfg = store.Get()
	if cfg.Provider != "anthropic" {
		t.Errorf("expected Provider 'anthropic' after reload, got %q", cfg.Provider)
	}
}

// TestConfigReloadReadError 验证 Reload 文件不存在时返回错误。
func TestConfigReloadReadError(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	store := NewStore(filepath.Join(dir, "nonexistent.json"))
	if err := store.Reload(); err == nil {
		t.Fatal("expected error when reloading non-existent file")
	}
}

// TestConfigSaveCreatesDirectory 验证 Save 自动创建目录。
func TestConfigSaveCreatesDirectory(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	deepPath := filepath.Join(dir, "deep", "nested", "dir", "config.json")
	store := NewStore(deepPath)

	if err := store.Save(); err != nil {
		t.Fatalf("Save should create directories: %v", err)
	}

	if _, err := os.Stat(deepPath); os.IsNotExist(err) {
		t.Error("expected config.json to be created")
	}
}

// TestConfigAtomicSave 验证原子保存不破坏现有文件。
func TestConfigAtomicSave(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 先保存一次
	if err := store.Mutate(func(c *Config) {
		c.Provider = "initial"
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 修改并再次保存
	if err := store.Mutate(func(c *Config) {
		c.Provider = "updated"
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 验证文件内容为更新后的值
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Get().Provider != "updated" {
		t.Errorf("expected 'updated', got %q", loaded.Get().Provider)
	}
}

// TestConfigNewStore 验证 NewStore 不共享指针。
func TestConfigNewStore(t *testing.T) {
	s1 := NewStore("/tmp/test1.json")
	s2 := NewStore("/tmp/test2.json")

	// 修改 s1 不应影响 s2
	s1.Mutate(func(c *Config) {
		c.Provider = "custom"
	})

	if s2.Get().Provider != "openai" {
		t.Error("s2 should not be affected by s1 mutation")
	}
}

// TestConfigMutateZeroFields 验证 Mutate 设置零值字段。
func TestConfigMutateZeroFields(t *testing.T) {
	store := NewStore("")

	// 先设置非零值
	store.Mutate(func(c *Config) {
		c.Provider = "test"
		c.MaxTokens = 100
		c.Verbose = true
	})

	// 再设置为零值
	store.Mutate(func(c *Config) {
		c.Provider = ""
		c.MaxTokens = 0
		c.Verbose = false
	})

	cfg := store.Get()
	if cfg.Provider != "" {
		t.Errorf("expected empty Provider, got %q", cfg.Provider)
	}
	if cfg.MaxTokens != 0 {
		t.Errorf("expected MaxTokens 0, got %d", cfg.MaxTokens)
	}
	if cfg.Verbose {
		t.Error("expected Verbose false")
	}
}

// TestConfigMutateCopyOnWrite 验证 Mutate 创建副本，不修改原配置。
func TestConfigMutateCopyOnWrite(t *testing.T) {
	store := NewStore("")
	original := store.Get()

	store.Mutate(func(c *Config) {
		c.Provider = "modified"
	})

	// 之前的指针应保持不变
	if original.Provider != "openai" {
		t.Errorf("original snapshot should have 'openai', got %q", original.Provider)
	}
}

// TestConfigGetDeepCopy 验证 Get() 返回的深拷贝修改不影响内部状态。
func TestConfigGetDeepCopy(t *testing.T) {
	store := NewStore("")

	// 获取深拷贝并修改
	cfg := store.Get()
	cfg.Provider = "hacked"
	cfg.MaxTokens = 9999
	cfg.MaxContextTokens = 9999
	cfg.MCPConfigs = map[string]interface{}{
		"evil": "value",
	}
	cfg.StopSequences = []string{"evil"}
	cfg.Tools.Timeout.Overrides = map[string]int{"evil": 999}

	// 重新获取，内部状态应不受影响
	cfg2 := store.Get()
	if cfg2.Provider != "openai" {
		t.Errorf("internal state Provider should be 'openai', got %q", cfg2.Provider)
	}
	if cfg2.MaxTokens != 4096 {
		t.Errorf("internal state MaxTokens should be 4096, got %d", cfg2.MaxTokens)
	}
	if cfg2.MaxContextTokens != 128000 {
		t.Errorf("internal state MaxContextTokens should be 128000, got %d", cfg2.MaxContextTokens)
	}
	if cfg2.MCPConfigs != nil {
		t.Error("internal state MCPConfigs should be nil")
	}
	if cfg2.StopSequences != nil {
		t.Error("internal state StopSequences should be nil")
	}
	if cfg2.Tools.Timeout.Overrides["bash"] != 60 {
		t.Errorf("internal state Overrides should still have bash=60, got %v", cfg2.Tools.Timeout.Overrides)
	}
	if _, ok := cfg2.Tools.Timeout.Overrides["evil"]; ok {
		t.Error("internal state should not have 'evil' override")
	}
}

// TestConfigMutateStopSequences 验证 StopSequences 的深拷贝。
func TestConfigMutateStopSequences(t *testing.T) {
	store := NewStore("")

	store.Mutate(func(c *Config) {
		c.StopSequences = []string{"\n", "stop"}
	})

	cfg := store.Get()
	if len(cfg.StopSequences) != 2 {
		t.Fatalf("expected 2 stop sequences, got %d", len(cfg.StopSequences))
	}

	// 第二次 Mutate 不应影响之前的快照
	store.Mutate(func(c *Config) {
		c.StopSequences = nil
	})

	if len(cfg.StopSequences) != 2 {
		t.Error("original snapshot StopSequences should still be 2")
	}
}

// TestConfigDiscoverNoParentConfig 验证父目录无配置时跳过。
func TestConfigDiscoverNoParentConfig(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	// 在子目录创建 config.json，但父目录没有
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	restore := chdir(t, subDir)
	defer restore()

	// 没有 config.json 在子目录或父目录，应返回全局路径
	path, err := Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "basework", "config.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// TestConfigReloadNotCalled 验证不调用 Reload 时配置不变。
func TestConfigReloadNotCalled(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	store := NewStore(path)

	// 保存初始配置
	store.Mutate(func(c *Config) {
		c.Provider = "initial"
	})
	store.Save()

	// 外部修改文件
	writeConfigFile(t, dir, &Config{
		Provider: "external-change",
	})

	// 未调用 Reload，配置应保持为 initial
	cfg := store.Get()
	if cfg.Provider != "initial" {
		t.Errorf("expected 'initial' without Reload, got %q", cfg.Provider)
	}
}

// TestConfigLoadComplexConfig 验证加载复杂配置正确。
func TestConfigLoadComplexConfig(t *testing.T) {
	dir, cleanup := tempConfigDir(t)
	defer cleanup()

	path := filepath.Join(dir, "config.json")
	writeConfigFile(t, dir, &Config{
		Provider:         "azure",
		Model:            "gpt-4-turbo",
		Temperature:      0.2,
		MaxTokens:        16384,
		SystemPrompt:     "You are a helpful assistant.",
		TopP:             0.95,
		FrequencyPenalty: 0.1,
		PresencePenalty:  0.1,
		StopSequences:    []string{"<|end|>", "<|stop|>"},
		MaxIterations:    5,
		Timeout:          120,
		Verbose:          true,
		MCPConfigs: map[string]interface{}{
			"fs": map[string]interface{}{
				"command": "npx",
			},
		},
		MaxToolCalls:     50,
		MaxContextTokens: 200000,
	})

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg := store.Get()
	if cfg.Provider != "azure" {
		t.Errorf("Provider: %q", cfg.Provider)
	}
	if cfg.Model != "gpt-4-turbo" {
		t.Errorf("Model: %q", cfg.Model)
	}
	if cfg.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("SystemPrompt: %q", cfg.SystemPrompt)
	}
	if cfg.TopP != 0.95 {
		t.Errorf("TopP: %f", cfg.TopP)
	}
	if len(cfg.StopSequences) != 2 {
		t.Errorf("StopSequences: %v", cfg.StopSequences)
	}
	if cfg.MaxIterations != 5 {
		t.Errorf("MaxIterations: %d", cfg.MaxIterations)
	}
	if cfg.Timeout != 120 {
		t.Errorf("Timeout: %d", cfg.Timeout)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
	if cfg.MaxToolCalls != 50 {
		t.Errorf("MaxToolCalls: %d", cfg.MaxToolCalls)
	}
	if cfg.MaxContextTokens != 200000 {
		t.Errorf("MaxContextTokens: %d", cfg.MaxContextTokens)
	}
}