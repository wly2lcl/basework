package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/subagent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/lsp"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestRuntimeToolsIncludesEnhancedAndLSPTools(t *testing.T) {
	cfg := config.NewStore("").Get()
	subAgentCoordinator := subagent.NewCoordinator(nil, subagent.DefaultConfig())
	tools := runtimeTools(cfg, t.TempDir(), nil, lsp.Tools(lsp.NewManager(lsp.Config{})), subAgentCoordinator)
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
		"sub_agent",
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

func TestRuntimeToolsOmitsSubAgentWhenDisabled(t *testing.T) {
	cfg := config.NewStore("").Get()
	cfg.SubAgent.Enabled = false
	tools := runtimeTools(cfg, t.TempDir(), nil, nil, newRuntimeSubAgentCoordinator(cfg, nil, t.TempDir(), nil, nil, nil))
	names := toolNameSet(tools)
	if names["sub_agent"] {
		t.Fatal("runtime tools should omit sub_agent when disabled")
	}
}

func TestRuntimeToolsEnabledForOpenCodeFreeModelWithoutAPIKey(t *testing.T) {
	t.Setenv("BASEWORK_PROVIDER", "")
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OG_API_KEY", "")

	cfg := config.NewStore("").Get()
	cfg.Provider = "opencode"
	cfg.Model = "big-pickle"
	cfg.OpenCode.APIKey = ""

	tools := runtimeTools(cfg, t.TempDir(), nil, lsp.Tools(lsp.NewManager(lsp.Config{})), nil)
	if len(tools) == 0 {
		t.Fatal("opencode free model without API key should keep runtime tools enabled")
	}
}

func TestRuntimeBehaviorOptionsFollowConfig(t *testing.T) {
	cfg := config.NewStore("").Get()
	model := &runtimeMockModel{}

	if got := runtimeBehaviorOptions(cfg, model, nil); len(got) != 1 {
		t.Fatalf("default behavior options = %d, want 1", len(got))
	}

	cfg.Compaction.Enabled = true
	cfg.Observability.Enabled = true
	coord := subagent.NewCoordinator(nil, subagent.DefaultConfig())
	opts := runtimeBehaviorOptions(cfg, model, coord)
	if len(opts) != 4 {
		t.Fatalf("enabled behavior options = %d, want 4", len(opts))
	}
}

func TestRuntimeCompactorAndLoopDetectorRespectConfig(t *testing.T) {
	cfg := config.NewStore("").Get()
	model := &runtimeMockModel{}
	if newRuntimeCompactor(cfg, model) != nil {
		t.Fatal("compactor should be nil when compaction is disabled")
	}
	cfg.Compaction.Enabled = true
	cfg.Compaction.Strategy = "sliding_window"
	if newRuntimeCompactor(cfg, model) == nil {
		t.Fatal("compactor should be created when compaction is enabled")
	}

	if newRuntimeLoopDetector(cfg) == nil {
		t.Fatal("loop detector should be enabled by default")
	}
	cfg.LoopDetect.Enabled = false
	if newRuntimeLoopDetector(cfg) != nil {
		t.Fatal("loop detector should be nil when disabled")
	}
}

func TestRuntimeSubAgentCoordinatorExecutesChildAgent(t *testing.T) {
	cfg := config.NewStore("").Get()
	model := &runtimeMockModel{
		responses: []llm.Response{{
			Message: llm.ChatMessage{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "child done"}},
			},
			Usage: llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		}},
	}
	coord := newRuntimeSubAgentCoordinator(cfg, model, t.TempDir(), nil, nil, nil)
	if coord == nil {
		t.Fatal("expected sub-agent coordinator")
	}

	st := subagent.NewSubAgentTool(coord)
	result, err := st.Execute(context.Background(), json.RawMessage(`{"description":"do child work","agent_type":"general"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("sub-agent tool returned error: %s", result.Content)
	}
	if !containsAll(result.Content, `"success":true`, `"output":"child done"`, `"total_tokens":5`) {
		t.Fatalf("unexpected sub-agent result: %s", result.Content)
	}
}

func TestReadonlyRuntimeToolsExcludeWritableTools(t *testing.T) {
	cfg := config.NewStore("").Get()
	tools := readonlyRuntimeTools(cfg, lsp.Tools(lsp.NewManager(lsp.Config{})))
	names := toolNameSet(tools)
	for _, name := range []string{"read", "grep", "glob", "lsp_definition", "lsp_diagnostics"} {
		if !names[name] {
			t.Fatalf("readonly tools missing %q", name)
		}
	}
	for _, name := range []string{"bash", "write", "edit", "apply_patch", "web_fetch", "web_search", "sub_agent"} {
		if names[name] {
			t.Fatalf("readonly tools should exclude %q", name)
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

type runtimeMockModel struct {
	responses []llm.Response
	idx       int
}

func (m *runtimeMockModel) ID() string { return "runtime-mock" }

func (m *runtimeMockModel) Generate(context.Context, *llm.Request) (*llm.Response, error) {
	return m.next(), nil
}

func (m *runtimeMockModel) Stream(context.Context, *llm.Request) (<-chan llm.StreamEvent, error) {
	resp := m.next()
	ch := make(chan llm.StreamEvent, 4+len(resp.Message.ToolCalls)*2)
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText && part.Text != "" {
			ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: part.Text}
		}
	}
	for i, tc := range resp.Message.ToolCalls {
		ch <- llm.StreamEvent{
			Type: llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{
				Index: i,
				ID:    tc.ID,
				Name:  tc.Name,
			},
		}
		ch <- llm.StreamEvent{
			Type: llm.StreamEventToolCall,
			ToolCall: &llm.ToolCallDelta{
				Index:    i,
				ArgsJSON: tc.ArgsJSON,
				Complete: true,
			},
		}
	}
	ch <- llm.StreamEvent{Type: llm.StreamEventUsage, Usage: &resp.Usage}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

func (m *runtimeMockModel) Supports(llm.Capability) bool { return true }

func (m *runtimeMockModel) next() *llm.Response {
	if len(m.responses) == 0 {
		return &llm.Response{
			Message: llm.ChatMessage{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "ok"}},
			},
		}
	}
	if m.idx >= len(m.responses) {
		return &m.responses[len(m.responses)-1]
	}
	resp := &m.responses[m.idx]
	m.idx++
	return resp
}

var _ llm.Model = (*runtimeMockModel)(nil)
var _ subagent.AgentTaskRunner = (*runtimeSubAgentRunner)(nil)
