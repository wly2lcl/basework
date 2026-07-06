package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityConfig_Defaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Security.ProtectionLevel != "strict" {
		t.Errorf("expected protection_level=strict, got %q", cfg.Security.ProtectionLevel)
	}
	if cfg.Security.PermissionStore != "sqlite" {
		t.Errorf("expected permission_store=sqlite, got %q", cfg.Security.PermissionStore)
	}
	if cfg.Security.AuditRetentionDays != 30 {
		t.Errorf("expected audit_retention_days=30, got %d", cfg.Security.AuditRetentionDays)
	}
}

func TestProfilingConfig_Defaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Profiling.Enabled {
		t.Error("expected profiling enabled=false")
	}
	if cfg.Profiling.Host != "127.0.0.1" {
		t.Errorf("expected profiling host=127.0.0.1, got %q", cfg.Profiling.Host)
	}
	if cfg.Profiling.Port != 6060 {
		t.Errorf("expected profiling port=6060, got %d", cfg.Profiling.Port)
	}
}

func TestToolsTimeoutConfig_Defaults(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	if cfg.Tools.Timeout.Default != 30 {
		t.Errorf("expected tools.timeout.default=30, got %d", cfg.Tools.Timeout.Default)
	}
	if bash, ok := cfg.Tools.Timeout.Overrides["bash"]; !ok || bash != 60 {
		t.Errorf("expected tools.timeout.overrides.bash=60, got %d", bash)
	}
}

func TestSecurityConfig_SensitivePaths(t *testing.T) {
	// 测试自定义敏感路径配置
	jsonData := `{
		"security": {
			"protection_level": "warn",
			"permission_store": "memory",
			"audit_retention_days": 7,
			"sensitive_paths": {
				"block": ["/custom/secret/"],
				"allow": ["~/.ssh/config"]
			}
		}
	}`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()

	if cfg.Security.ProtectionLevel != "warn" {
		t.Errorf("expected protection_level=warn, got %q", cfg.Security.ProtectionLevel)
	}
	if cfg.Security.PermissionStore != "memory" {
		t.Errorf("expected permission_store=memory, got %q", cfg.Security.PermissionStore)
	}
	if cfg.Security.AuditRetentionDays != 7 {
		t.Errorf("expected audit_retention_days=7, got %d", cfg.Security.AuditRetentionDays)
	}
	if len(cfg.Security.SensitivePaths.Block) != 1 || cfg.Security.SensitivePaths.Block[0] != "/custom/secret/" {
		t.Errorf("expected block=[/custom/secret/], got %v", cfg.Security.SensitivePaths.Block)
	}
	if len(cfg.Security.SensitivePaths.Allow) != 1 || cfg.Security.SensitivePaths.Allow[0] != "~/.ssh/config" {
		t.Errorf("expected allow=[~/.ssh/config], got %v", cfg.Security.SensitivePaths.Allow)
	}
}

func TestToolsTimeoutConfig_Overrides(t *testing.T) {
	jsonData := `{
		"tools": {
			"timeout": {
				"default": 60,
				"overrides": {
					"bash": 120,
					"read": 10
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()

	if cfg.Tools.Timeout.Default != 60 {
		t.Errorf("expected default=60, got %d", cfg.Tools.Timeout.Default)
	}
	if cfg.Tools.Timeout.Overrides["bash"] != 120 {
		t.Errorf("expected bash override=120, got %d", cfg.Tools.Timeout.Overrides["bash"])
	}
	if cfg.Tools.Timeout.Overrides["read"] != 10 {
		t.Errorf("expected read override=10, got %d", cfg.Tools.Timeout.Overrides["read"])
	}
}

func TestProfilingConfig_Custom(t *testing.T) {
	jsonData := `{
		"profiling": {
			"enabled": true,
			"host": "0.0.0.0",
			"port": 9090
		}
	}`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()

	if !cfg.Profiling.Enabled {
		t.Error("expected profiling enabled=true")
	}
	if cfg.Profiling.Host != "0.0.0.0" {
		t.Errorf("expected host=0.0.0.0, got %q", cfg.Profiling.Host)
	}
	if cfg.Profiling.Port != 9090 {
		t.Errorf("expected port=9090, got %d", cfg.Profiling.Port)
	}
}

func TestSecurityConfig_Serialization(t *testing.T) {
	store := NewStore("")
	cfg := store.Get()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.Security.ProtectionLevel != cfg.Security.ProtectionLevel {
		t.Errorf("protection_level mismatch after round-trip")
	}
	if loaded.Profiling.Enabled != cfg.Profiling.Enabled {
		t.Errorf("profiling.enabled mismatch after round-trip")
	}
	if loaded.Tools.Timeout.Default != cfg.Tools.Timeout.Default {
		t.Errorf("tools.timeout.default mismatch after round-trip")
	}
}

func TestConfig_Clone_SecurityFields(t *testing.T) {
	store := NewStore("")
	store.Mutate(func(c *Config) {
		c.Security.SensitivePaths.Block = []string{"/test/"}
		c.Security.SensitivePaths.Allow = []string{"/test/ok"}
		c.Tools.Timeout.Overrides = map[string]int{"test": 99}
	})

	original := store.Get()
	cloned := original.clone()

	// 修改 clone 不应影响 original
	cloned.Security.SensitivePaths.Block[0] = "/modified/"
	cloned.Tools.Timeout.Overrides["test"] = 0

	if original.Security.SensitivePaths.Block[0] != "/test/" {
		t.Error("clone modified original Security.SensitivePaths.Block")
	}
	if original.Tools.Timeout.Overrides["test"] != 99 {
		t.Error("clone modified original Tools.Timeout.Overrides")
	}
}
