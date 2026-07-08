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
