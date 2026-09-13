package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/internal/subagent"
	runtimetools "github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/lsp"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestRuntimeToolsIncludesEnhancedAndLSPTools(t *testing.T) {
	cfg := config.NewStore("").Get()
	subAgentCoordinator := subagent.NewCoordinator(nil, subagent.DefaultConfig())
	tools := runtimeTools(cfg, t.TempDir(), nil, lsp.Tools(lsp.NewManager(lsp.Config{})), subAgentCoordinator, runtimeJobWiring{})
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
	tools := runtimeTools(cfg, t.TempDir(), nil, nil, newRuntimeSubAgentCoordinator(cfg, nil, t.TempDir(), nil, nil, nil, nil), runtimeJobWiring{})
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

	tools := runtimeTools(cfg, t.TempDir(), nil, lsp.Tools(lsp.NewManager(lsp.Config{})), nil, runtimeJobWiring{})
	if len(tools) == 0 {
		t.Fatal("opencode free model without API key should keep runtime tools enabled")
	}
}

// TestRuntimeJobToolsFollowWiring 确认后台任务工具只在给了 Manager 时注册。
//
// 这是"不注册假能力"的回归防线：Manager 为 nil（例如子代理路径）时若仍出现
// bash_background / job_*，模型会看到一组调用即报错的工具。
func TestRuntimeJobToolsFollowWiring(t *testing.T) {
	cfg := config.NewStore("").Get()

	withoutJobs := toolNameSet(runtimeTools(cfg, t.TempDir(), nil, nil, nil, runtimeJobWiring{}))
	for _, name := range []string{"bash_background", "job_list", "job_output", "job_cancel"} {
		if withoutJobs[name] {
			t.Fatalf("未接 Manager 时不应注册 %q", name)
		}
	}

	owner := &jobOwnerHolder{}
	owner.Set("session-abc")
	mgr := newRuntimeJobManagerAt(owner, filepath.Join(t.TempDir(), "jobs.jsonl"), t.TempDir())
	t.Cleanup(mgr.Close)

	withJobs := toolNameSet(runtimeTools(cfg, t.TempDir(), nil, nil, nil, runtimeJobWiring{
		manager: mgr,
		owner:   owner.Get,
	}))
	for _, name := range []string{"bash_background", "job_list", "job_output", "job_cancel"} {
		if !withJobs[name] {
			t.Fatalf("接了 Manager 后应注册 %q", name)
		}
	}
}

// TestRuntimeEditToolFollowsWiring 确认 edit_files 只在主 Agent 接线时注册。
//
// editTool 为 nil（子代理路径）时不注册，避免子代理拿到一套"事件无处落"的
// 编辑能力；接入时权限口径用同一个 pathChecker。
func TestRuntimeEditToolFollowsWiring(t *testing.T) {
	cfg := config.NewStore("").Get()

	withoutEdit := toolNameSet(runtimeTools(cfg, t.TempDir(), nil, nil, nil, runtimeJobWiring{}))
	if withoutEdit["edit_files"] {
		t.Fatalf("未接 editTool 时不应注册 edit_files")
	}

	pathChecker := permission.NewPathChecker(nil, nil, permission.ProtectionStrict)
	withEdit := toolNameSet(runtimeTools(cfg, t.TempDir(), nil, nil, nil, runtimeJobWiring{
		editTool:    runtimetools.NewEditFilesTool(t.TempDir()),
		pathChecker: pathChecker,
	}))
	if !withEdit["edit_files"] {
		t.Fatalf("接了 editTool 后应注册 edit_files")
	}
}

// TestEditToolUsesRuntimePathChecker 确认接线后 edit_files 的路径检查走的是
// 运行时 pathChecker（同一份敏感路径配置），而不是 planner 的默认放行。
func TestEditToolUsesRuntimePathChecker(t *testing.T) {
	root := t.TempDir()
	// 编辑工具内部会把工作区解析成真实路径（macOS 的 TempDir 带 /var → /private/var
	// 软链），黑名单条目必须用同一口径，否则匹配不上。
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	blocked := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(blocked, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("准备文件: %v", err)
	}
	// 严格保护级别 + 显式 Block 列表，确保拦截可判定。
	pathChecker := permission.NewPathChecker([]string{blocked}, nil, permission.ProtectionStrict)

	editTool := runtimetools.NewEditFilesTool(root)
	editTool.CheckPath = pathChecker.CheckPath

	raw, err := json.Marshal(map[string]interface{}{
		"action": "preview",
		"edits": []interface{}{
			map[string]interface{}{"path": "secret.txt", "old": "x", "new": "y"},
		},
	})
	if err != nil {
		t.Fatalf("序列化参数: %v", err)
	}
	res, err := editTool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("被拦截路径的预览应报错: %s", res.Content)
	}
	if !strings.Contains(res.Content, "路径检查拒绝") {
		t.Fatalf("报错应来自路径检查: %s", res.Content)
	}
}

// TestRuntimeBackgroundTimeoutMirrorsBuiltin 确认后台 bash 与同步 bash 读同一份超时口径。
func TestRuntimeBackgroundTimeoutMirrorsBuiltin(t *testing.T) {
	cfg := config.NewStore("").Get()
	// 默认配置里 overrides.bash = 60，后台 bash 不该自己发明一个 30。
	if seconds, disabled := runtimeBackgroundTimeout(cfg); disabled || seconds != 60 {
		t.Fatalf("默认配置下后台 bash 超时 = (%d, disabled=%v)，期望 (60, false)", seconds, disabled)
	}

	// 配置显式关闭超时（0 = 不超时）时必须表达为 disabled，而不是回落成 30 秒。
	cfg.Tools.Timeout.Overrides = map[string]int{"bash": 0}
	if seconds, disabled := runtimeBackgroundTimeout(cfg); !disabled || seconds != 0 {
		t.Fatalf("overrides.bash=0 时应为 (0, true)，得到 (%d, %v)", seconds, disabled)
	}

	// 无 overrides 时用全局默认。
	cfg.Tools.Timeout.Overrides = nil
	cfg.Tools.Timeout.Default = 45
	if seconds, disabled := runtimeBackgroundTimeout(cfg); disabled || seconds != 45 {
		t.Fatalf("无 overrides 时应为 (45, false)，得到 (%d, %v)", seconds, disabled)
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
	coord := newRuntimeSubAgentCoordinator(cfg, model, t.TempDir(), nil, nil, nil, nil)
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
	tools := readonlyRuntimeTools(cfg, lsp.Tools(lsp.NewManager(lsp.Config{})), nil)
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
	prompt := runtimeSystemPrompt(cfg, workDir, nil)
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
