package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/lsp"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestRuntimeToolsIncludesEnhancedAndLSPTools(t *testing.T) {
	cfg := config.NewStore("").Get()
	tools := runtimeTools(cfg, t.TempDir(), nil, lsp.Tools(lsp.NewManager(lsp.Config{})))
	names := toolNameSet(tools)

	for _, name := range []string{
		"bash",
		"read",
		"write",
		"edit",
		"grep",
		"glob",
		"apply_patch",
		"todowrite",
		"question",
		"web_fetch",
		"web_search",
		"lsp_definition",
		"lsp_references",
		"lsp_hover",
		"lsp_diagnostics",
		"lsp_document_symbols",
		"lsp_workspace_symbols",
	} {
		if !names[name] {
			t.Fatalf("runtime tools missing %q", name)
		}
	}
}

func TestRuntimeSkillPromptDiscoversWorkspaceSkills(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".basework", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: review
description: Review code
---
Use a review-first response.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	prompt := runtimeSkillPrompt(config.NewStore("").Get(), workDir)
	if prompt == "" {
		t.Fatal("expected skill prompt")
	}
	if !containsAll(prompt, "<skills>", "review", "Use a review-first response.") {
		t.Fatalf("unexpected skill prompt: %s", prompt)
	}
}

func TestRuntimeSystemPromptAppendsSkills(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".basework", "skills", "writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: writer
description: Write docs
---
Write concise documentation.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := config.NewStore("").Get()
	cfg.SystemPrompt = "Base prompt."
	prompt := runtimeSystemPrompt(cfg, workDir)
	if !containsAll(prompt, "Base prompt.", "<skills>", "writer") {
		t.Fatalf("unexpected system prompt: %s", prompt)
	}
}

func TestProviderAPIKeyPrefersOpenCodeConfig(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "env-key")
	cfg := config.NewStore("").Get()
	cfg.OpenCode.APIKey = "config-key"

	if got := providerAPIKey(cfg, "opencode"); got != "config-key" {
		t.Fatalf("expected config key, got %q", got)
	}
}

func TestLookupAPIKeySupportsLegacyOpenCodeEnv(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OG_API_KEY", "legacy-key")

	if got := lookupAPIKey("opencode"); got != "legacy-key" {
		t.Fatalf("expected legacy OpenCode key, got %q", got)
	}
}

func toolNameSet(tools []tool.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	return names
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
