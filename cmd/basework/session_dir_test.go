package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setTestHome 统一设置测试 HOME。
// Windows 上 os.UserHomeDir 读 USERPROFILE 而忽略 HOME，必须两个都设，
// 否则生产代码拿到的是真实用户目录而不是 t.TempDir()。
func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
}

// TestGetSessionDir 验证规范会话目录位于 $HOME/.local/share/basework/sessions。
func TestGetSessionDir(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	got := getSessionDir()
	want := filepath.Join(home, ".local", "share", "basework", "sessions")
	if got != want {
		t.Fatalf("getSessionDir() = %q, want %q", got, want)
	}
}

// TestLegacySessionDir 验证历史目录为 $HOME/.basework/sessions。
func TestLegacySessionDir(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	got := legacySessionDir()
	want := filepath.Join(home, ".basework", "sessions")
	if got != want {
		t.Fatalf("legacySessionDir() = %q, want %q", got, want)
	}
}

// TestResolveSessionDir_PrefersCanonical 验证两个目录都存在时优先规范目录。
func TestResolveSessionDir_PrefersCanonical(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	canonical := getSessionDir()
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatalf("MkdirAll canonical: %v", err)
	}
	if err := os.MkdirAll(legacySessionDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}

	if got := resolveSessionDir(); got != canonical {
		t.Fatalf("两个目录都存在时应优先规范目录，得到 %q", got)
	}
}

// TestResolveSessionDir_FallsBackToLegacy 验证仅旧目录存在时回退到旧目录，
// 以免升级后已有用户的会话读取不到。
func TestResolveSessionDir_FallsBackToLegacy(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	legacy := legacySessionDir()
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}

	if got := resolveSessionDir(); got != legacy {
		t.Fatalf("仅旧目录存在时应回退到旧目录 %q，得到 %q", legacy, got)
	}
}

// TestResolveSessionDir_DefaultsToCanonical 验证两个目录都不存在时返回规范目录。
func TestResolveSessionDir_DefaultsToCanonical(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	if got := resolveSessionDir(); got != getSessionDir() {
		t.Fatalf("目录都不存在时应返回规范目录 %q，得到 %q", getSessionDir(), got)
	}
}

// TestShortSessionID 验证短 ID 不会 panic。
// 旧实现直接做 sessionID[:12]，ID 不足 12 字符时越界 panic。
func TestShortSessionID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空字符串", "", ""},
		{"短于 12", "abc", "abc"},
		{"恰好 12", "123456789012", "123456789012"},
		{"长于 12", "1234567890123456", "123456789012"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortSessionID(tc.in); got != tc.want {
				t.Fatalf("shortSessionID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
