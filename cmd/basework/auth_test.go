package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetOAuthConfigLoadsProviders(t *testing.T) {
	oldCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = oldCfgFile })

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgFile = filepath.Join(dir, "config.json")
	data := []byte(`{
		"oauth": {
			"enabled": true,
			"storage_backend": "file",
			"callback_port": 8181,
			"providers": {
				"copilot": {
					"authorization_endpoint": "https://github.com/login/oauth/authorize",
					"token_endpoint": "https://github.com/login/oauth/access_token",
					"client_id": "client-id",
					"client_secret": "secret",
					"scopes": ["read:user"],
					"redirect_uri": "http://127.0.0.1:8181/callback"
				}
			}
		}
	}`)
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	oauthCfg, store, err := getOAuthConfig()
	if err != nil {
		t.Fatalf("getOAuthConfig: %v", err)
	}
	if store == nil {
		t.Fatal("expected token store")
	}
	if !oauthCfg.Enabled {
		t.Fatal("expected oauth enabled")
	}
	if oauthCfg.CallbackPort != 8181 {
		t.Fatalf("expected callback port 8181, got %d", oauthCfg.CallbackPort)
	}
	provider, ok := oauthCfg.Providers["copilot"]
	if !ok {
		t.Fatal("expected copilot provider")
	}
	if provider.ClientID != "client-id" || provider.ClientSecret != "secret" {
		t.Fatalf("unexpected client config: %+v", provider)
	}
	if len(provider.Scopes) != 1 || provider.Scopes[0] != "read:user" {
		t.Fatalf("unexpected scopes: %v", provider.Scopes)
	}
}

func TestGetOAuthConfigRejectsKeychainBackend(t *testing.T) {
	oldCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = oldCfgFile })

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgFile = filepath.Join(dir, "config.json")
	data := []byte(`{
		"oauth": {
			"enabled": true,
			"storage_backend": "keychain",
			"callback_port": 8181
		}
	}`)
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := getOAuthConfig()
	if err == nil {
		t.Fatal("expected keychain backend error")
	}
	if !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("expected keychain error, got %v", err)
	}
}
