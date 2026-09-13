package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveEndpoint_Precedence 端点优先级：环境变量 > 配置文件 > 默认。
//
// 这是 CFG-004 的核心契约，`config explain` 与 runtime 都依赖它给出同一答案。
func TestResolveEndpoint_Precedence(t *testing.T) {
	cfg := defaultConfig()
	cfg.Providers = map[string]ProviderEndpoint{
		"openai": {BaseURL: "https://from-config.example.com/v1", APIKey: "sk-config"},
	}

	cases := []struct {
		name         string
		providerType string
		envBaseURL   string
		wantURL      string
		wantSource   EndpointSource
	}{
		{"环境变量覆盖配置文件", "openai", "https://from-env.example.com/v1", "https://from-env.example.com/v1", EndpointSourceEnv},
		{"无环境变量时取配置文件", "openai", "", "https://from-config.example.com/v1", EndpointSourceConfig},
		{"环境变量为空白视同未设置", "openai", "   ", "https://from-config.example.com/v1", EndpointSourceConfig},
		{"未配置的 provider 用默认端点", "anthropic", "", "", EndpointSourceDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cfg.ResolveEndpoint(tc.providerType, tc.envBaseURL)
			if got.BaseURL != tc.wantURL {
				t.Errorf("BaseURL = %q, want %q", got.BaseURL, tc.wantURL)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

// TestEffectiveProviderName_EnvWins 生效 provider 的判定与既有行为一致。
func TestEffectiveProviderName_EnvWins(t *testing.T) {
	cfg := defaultConfig()
	cfg.Provider = "openai"
	if got := cfg.EffectiveProviderName("anthropic"); got != "anthropic" {
		t.Errorf("环境变量应覆盖配置: got %q", got)
	}
	if got := cfg.EffectiveProviderName(""); got != "openai" {
		t.Errorf("无环境变量时应取配置值: got %q", got)
	}
}

// TestResolveEndpoint_FollowsEffectiveProvider providers.<name> 的键必须按
// 生效 provider 取值：BASEWORK_PROVIDER 覆盖后不能仍读配置里的 provider 那一块，
// 否则会把 A 的端点用在 B 上。
func TestResolveEndpoint_FollowsEffectiveProvider(t *testing.T) {
	cfg := defaultConfig()
	cfg.Provider = "openai"
	cfg.Providers = map[string]ProviderEndpoint{
		"openai":    {BaseURL: "https://openai-gw.example.com/v1"},
		"anthropic": {BaseURL: "https://anthropic-gw.example.com"},
	}

	effective := cfg.EffectiveProviderName("anthropic")
	got := cfg.ResolveEndpoint(effective, "")
	if got.BaseURL != "https://anthropic-gw.example.com" {
		t.Fatalf("生效 provider 为 anthropic 时应取其端点，得到 %q", got.BaseURL)
	}
}

// TestValidateBaseURL 端点形态校验：写错必须在加载/启动时报错，
// 不允许静默退回默认端点。
func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"空值合法（表示不覆盖）", "", false},
		{"标准 https", "https://gw.example.com/v1", false},
		{"本地 http 带端口", "http://127.0.0.1:11434/v1", false},
		{"漏掉 scheme", "127.0.0.1:11434", true},
		{"非 http 协议", "ftp://gw.example.com", true},
		{"只有 scheme 没有主机", "https://", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURL("providers.openai.base_url", tc.raw)
			if tc.wantErr && err == nil {
				t.Fatalf("期望报错，得到 nil（输入 %q）", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("不应报错，得到 %v（输入 %q）", err, tc.raw)
			}
			if err != nil && !strings.Contains(err.Error(), "providers.openai.base_url") {
				t.Errorf("错误信息应指明来源标签: %v", err)
			}
		})
	}
}

// TestLoad_RejectsMalformedProviderBaseURL 加载坏端点必须失败，而不是带着它启动。
func TestLoad_RejectsMalformedProviderBaseURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw, err := json.Marshal(map[string]any{
		"provider":  "openai",
		"providers": map[string]any{"openai": map[string]any{"base_url": "127.0.0.1:11434"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写文件: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("漏掉 scheme 的 base_url 应导致加载失败")
	} else if !strings.Contains(err.Error(), "providers.openai.base_url") {
		t.Errorf("错误应指明具体字段: %v", err)
	}
}

// TestLoad_AcceptsValidProviderBaseURL 合法的自定义端点必须能加载并解析出来。
func TestLoad_AcceptsValidProviderBaseURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw, err := json.Marshal(map[string]any{
		"provider": "openai",
		"model":    "agnes-2.5-flash",
		"providers": map[string]any{
			"openai": map[string]any{
				"base_url": "https://newapi.example.com/v1",
				"api_key":  "sk-load-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写文件: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("合法配置应加载成功: %v", err)
	}
	cfg := store.Get()
	ep := cfg.ProviderEndpointFor("openai")
	if ep.BaseURL != "https://newapi.example.com/v1" || ep.APIKey != "sk-load-test" {
		t.Fatalf("providers 未按预期解析: %+v", ep)
	}
}

// TestClone_ProvidersIndependent clone 必须深拷贝 Providers：
// 调用方拿到 Get() 的快照后改它，不能影响 Store 内部状态。
func TestClone_ProvidersIndependent(t *testing.T) {
	store := NewStore("")
	if err := store.Mutate(func(c *Config) {
		c.Providers = map[string]ProviderEndpoint{"openai": {BaseURL: "https://a.example.com/v1"}}
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	snapshot := store.Get()
	snapshot.Providers["openai"] = ProviderEndpoint{BaseURL: "https://tampered.example.com/v1"}
	snapshot.Providers["anthropic"] = ProviderEndpoint{BaseURL: "https://injected.example.com"}

	again := store.Get()
	if again.Providers["openai"].BaseURL != "https://a.example.com/v1" {
		t.Errorf("clone 与内部状态共享了 map: %+v", again.Providers["openai"])
	}
	if _, ok := again.Providers["anthropic"]; ok {
		t.Error("新增键竟然写进了内部状态，说明 map 未深拷贝")
	}
}

// TestRedactedView_ProviderCredentialsRedacted 凭据必须脱敏，端点 URL 保留
// （URL 是排查必需信息，不是秘密）。
func TestRedactedView_ProviderCredentialsRedacted(t *testing.T) {
	const secret = "sk-cfg004-redact-9d2f"
	cfg := defaultConfig()
	cfg.Providers = map[string]ProviderEndpoint{
		"openai": {BaseURL: "https://gw.example.com/v1", APIKey: secret},
	}

	view, err := RedactedView(cfg)
	if err != nil {
		t.Fatalf("RedactedView: %v", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal 视图: %v", err)
	}
	text := string(encoded)

	if strings.Contains(text, secret) {
		t.Fatalf("providers.*.api_key 未脱敏:\n%s", text)
	}
	if !strings.Contains(text, RedactionMarker) {
		t.Errorf("应出现脱敏占位符:\n%s", text)
	}
	if !strings.Contains(text, "https://gw.example.com/v1") {
		t.Errorf("端点 URL 不是秘密，应保留以便排查:\n%s", text)
	}
}

// TestDisplayURL_StripsUserInfo 内嵌凭据的 URL 在展示时必须去掉 userinfo。
func TestDisplayURL_StripsUserInfo(t *testing.T) {
	if got := DisplayURL("https://user:pass@gw.example.com/v1"); strings.Contains(got, "pass") {
		t.Errorf("展示用 URL 不应包含内嵌凭据: %q", got)
	}
	if got := DisplayURL("https://gw.example.com/v1"); got != "https://gw.example.com/v1" {
		t.Errorf("无 userinfo 的 URL 应原样返回: %q", got)
	}
}
