//go:build sqlite

package main

import (
	"path/filepath"
	"testing"
)

// TestResolveMigrateSQLitePath_Default 验证迁移的 SQLite 默认路径与规范目录一致。
func TestResolveMigrateSQLitePath_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := migrateSQLitePath
	migrateSQLitePath = ""
	t.Cleanup(func() { migrateSQLitePath = original })

	got := resolveMigrateSQLitePath()
	want := filepath.Join(getSessionDir(), "sessions.db")
	if got != want {
		t.Fatalf("resolveMigrateSQLitePath() = %q, want %q", got, want)
	}
}

// TestResolveMigrateSQLitePath_Explicit 验证显式指定路径时不被覆盖。
func TestResolveMigrateSQLitePath_Explicit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	custom := filepath.Join(t.TempDir(), "custom.db")
	original := migrateSQLitePath
	migrateSQLitePath = custom
	t.Cleanup(func() { migrateSQLitePath = original })

	if got := resolveMigrateSQLitePath(); got != custom {
		t.Fatalf("显式路径应原样返回 %q，得到 %q", custom, got)
	}
}
