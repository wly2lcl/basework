package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactURLRemovesCredentialsAndQuery(t *testing.T) {
	got := redactURL("https://user:secret@example.test/v1/?api_key=do-not-save#fragment")
	if got != "https://example.test/v1" {
		t.Fatalf("redactURL() = %q", got)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "do-not-save") {
		t.Fatalf("脱敏结果泄漏凭据: %q", got)
	}
}

func TestCopyDirCopiesFixtureContents(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.WriteFile(filepath.Join(src, "calc.go"), []byte("return a - b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "case.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"calc.go", filepath.Join("nested", "case.txt")} {
		if _, err := os.Stat(filepath.Join(dst, path)); err != nil {
			t.Fatalf("复制后缺少 %s: %v", path, err)
		}
	}
}
