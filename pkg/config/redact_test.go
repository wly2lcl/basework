package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSensitiveKey 钉死判定规则：宁可多脱，但 token_path 这类「引用位置」
// 不是秘密，漏判会毁掉解释输出的排查价值。
func TestSensitiveKey(t *testing.T) {
	sensitive := []string{
		"api_key", "APIKey", "apikey",
		"access_key", "secret_key", "client_secret", "SecretKey",
		"password", "passwd",
		"private_key", "credentials",
		"token", "access_token", "auth_token", "refresh_token",
		"authorization", "proxy_authorization",
		"key",
	}
	for _, k := range sensitive {
		if !SensitiveKey(k) {
			t.Errorf("SensitiveKey(%q) = false，期望 true", k)
		}
	}
	plain := []string{
		"token_path", // 引用位置，不是秘密
		"model", "provider", "temperature", "max_tokens", "endpoint",
		"region", "resource", "deployment", "backend", "callback_port",
		"authorization_endpoint", // 是端点 URL，不是凭据
	}
	for _, k := range plain {
		if SensitiveKey(k) {
			t.Errorf("SensitiveKey(%q) = true，期望 false", k)
		}
	}
}

// TestRedactedView_RedactsSecrets 确认嵌套的秘密全部被替换。
func TestRedactedView_RedactsSecrets(t *testing.T) {
	cfg := defaultConfig()
	cfg.OpenCode.APIKey = "sk-super-secret-value-001"
	cfg.Azure.APIKey = "azure-key-002"
	cfg.Bedrock.AccessKey = "AKIA-003"
	cfg.Bedrock.SecretKey = "bedrock-secret-004"
	cfg.OAuth.Providers = map[string]OAuthProviderConfig{
		"acme": {
			AuthorizationEndpoint: "https://acme.example/authorize",
			TokenEndpoint:         "https://acme.example/token",
			ClientID:              "client-id-public",
			ClientSecret:          "acme-client-secret-005",
		},
	}
	cfg.MCPConfigs = map[string]interface{}{
		"server1": map[string]interface{}{
			"command": "npx",
			"env": map[string]interface{}{
				"GITHUB_TOKEN":    "ghp-mcp-token-006",
				"API_KEY":         "mcp-api-key-007",
				"普通变量":            "可以保留",
				"SESSION_DB_PATH": "/tmp/db.sqlite",
			},
		},
	}
	cfg.Tools.WebSearch.APIKey = "tavily-key-008"

	view, err := RedactedView(cfg)
	if err != nil {
		t.Fatalf("RedactedView: %v", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(encoded)

	// 每一个秘密都不能出现在脱敏视图里。
	for _, secret := range []string{
		"sk-super-secret-value-001", "azure-key-002", "AKIA-003",
		"bedrock-secret-004", "acme-client-secret-005",
		"ghp-mcp-token-006", "mcp-api-key-007", "tavily-key-008",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("秘密 %q 泄漏进脱敏视图", secret)
		}
	}

	// 非敏感值必须原样保留，否则解释输出失去意义。
	for _, want := range []string{
		"https://acme.example/authorize", "client-id-public",
		"可以保留", "/tmp/db.sqlite", "npx",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("非敏感值 %q 不应在脱敏视图中丢失", want)
		}
	}

	// 已设置的秘密以统一标记出现；未设置的字段保留空值。
	if !strings.Contains(text, RedactionMarker) {
		t.Errorf("脱敏视图应包含标记 %s", RedactionMarker)
	}
	if !strings.Contains(text, `"token_path":""`) {
		t.Errorf("未设置的非敏感字段应保留空值以便区分: %s", text)
	}

	// token_path 类字段保留原值。
	if cfg.Copilot.TokenPath != "" {
		if !strings.Contains(text, cfg.Copilot.TokenPath) {
			t.Errorf("token_path 不应被脱敏: %s", text)
		}
	}
}

// TestRedactedView_DoesNotMutateOriginal 确认脱敏不改原配置对象。
func TestRedactedView_DoesNotMutateOriginal(t *testing.T) {
	cfg := defaultConfig()
	cfg.OpenCode.APIKey = "sk-original-secret"

	if _, err := RedactedView(cfg); err != nil {
		t.Fatalf("RedactedView: %v", err)
	}
	if cfg.OpenCode.APIKey != "sk-original-secret" {
		t.Fatalf("原配置对象被改写: %q", cfg.OpenCode.APIKey)
	}
}

// TestRedactedView_StructurePreserved 确认脱敏只换值不换结构：
// 字段一个不少（空值也算信息），嵌套层级不变。
func TestRedactedView_StructurePreserved(t *testing.T) {
	full := defaultConfig()
	view, err := RedactedView(full)
	if err != nil {
		t.Fatalf("RedactedView: %v", err)
	}
	rawFull, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal full: %v", err)
	}
	var wantKeys int
	countKeys(rawFull, &wantKeys)
	var gotKeys int
	encoded, _ := json.Marshal(view)
	countKeys(encoded, &gotKeys)
	if gotKeys != wantKeys {
		t.Fatalf("脱敏后字段数变化: %d → %d", wantKeys, gotKeys)
	}
}

func countKeys(b []byte, n *int) {
	var tree map[string]any
	if err := json.Unmarshal(b, &tree); err != nil {
		return
	}
	walkCount(tree, n)
}

func walkCount(v any, n *int) {
	switch typed := v.(type) {
	case map[string]any:
		for k, val := range typed {
			*n++
			_ = k
			walkCount(val, n)
		}
	case []any:
		for _, val := range typed {
			walkCount(val, n)
		}
	}
}
