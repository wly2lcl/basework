package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wly2lcl/basework/internal/compaction"
	"github.com/wly2lcl/basework/internal/loopdetect"
	"github.com/wly2lcl/basework/internal/observability"
	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/internal/subagent"
	runtimetools "github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/lsp"
	"github.com/wly2lcl/basework/pkg/mcp"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/skill"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

type runtimeAgentOptions struct {
	Callback agent.Callback
}

type runtimeAgent struct {
	Agent        agent.Agent
	ModelName    string
	ProviderName string
}

func newRuntimeAgent(cfg *config.Config, opts runtimeAgentOptions) (*runtimeAgent, error) {
	providerType := cfg.Provider
	if envProvider := os.Getenv("BASEWORK_PROVIDER"); envProvider != "" {
		providerType = envProvider
	}

	model, err := provider.Create(provider.Config{
		Type:    providerType,
		ModelID: cfg.Model,
		APIKey:  providerAPIKey(cfg, providerType),
		Options: buildProviderOptions(cfg, providerType),
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LLM 模型失败: %w", err)
	}

	sess, err := session.NewJSONLStore(getSessionDir())
	if err != nil {
		return nil, fmt.Errorf("创建会话存储失败: %w", err)
	}

	if err := configureBuiltinToolRuntime(cfg); err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("获取工作目录失败: %w", err)
	}

	var permissionChecker *runtimePermissionChecker
	if cfg.Permission.Enabled {
		permissionChecker, err = newPermissionChecker(cfg)
		if err != nil {
			return nil, err
		}
	}

	var permissionCleanups []func() error
	var coreChecker *permission.Checker
	if permissionChecker != nil {
		coreChecker = permissionChecker.Core
		permissionCleanups = permissionChecker.Cleanups
	}
	plugin, extraTools := initializeRuntimeExtensions(context.Background(), cfg, cwd, permissionCleanups...)
	subAgentCoordinator := newRuntimeSubAgentCoordinator(cfg, model, cwd, coreChecker, permissionChecker, extraTools)

	agentOpts := []agent.Option{
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithSystemPrompt(runtimeSystemPrompt(cfg, cwd)),
		agent.WithMaxSteps(cfg.MaxIterations),
		agent.WithTools(runtimeTools(cfg, cwd, coreChecker, extraTools, subAgentCoordinator)...),
	}
	agentOpts = append(agentOpts, runtimeBehaviorOptions(cfg, model, subAgentCoordinator)...)
	if plugin != nil {
		agentOpts = append(agentOpts, agent.WithPlugin(plugin))
	}
	if opts.Callback != nil {
		agentOpts = append(agentOpts, agent.WithCallback(opts.Callback))
	}
	if cfg.Permission.Enabled {
		agentOpts = append(agentOpts, agent.WithPermissionChecker(permissionChecker.Adapter))
	}

	agt, err := agent.New(agentOpts...)
	if err != nil {
		return nil, fmt.Errorf("创建 Agent 失败: %w", err)
	}

	return &runtimeAgent{
		Agent:        agt,
		ModelName:    cfg.Model,
		ProviderName: providerType,
	}, nil
}

func configureBuiltinToolRuntime(cfg *config.Config) error {
	builtin.SetTimeoutConfig(builtin.TimeoutConfig{
		DefaultTimeout: cfg.Tools.Timeout.Default,
		Overrides:      cfg.Tools.Timeout.Overrides,
	})
	if !cfg.Observability.Enabled {
		builtin.SetEventBus(nil)
	}

	level := cfg.Security.ProtectionLevel
	if level == "" {
		level = "strict"
	}
	protection, err := permission.ParseProtectionLevel(level)
	if err != nil {
		return fmt.Errorf("解析敏感路径保护级别失败: %w", err)
	}
	builtin.SetPathChecker(permission.NewPathChecker(
		cfg.Security.SensitivePaths.Block,
		cfg.Security.SensitivePaths.Allow,
		protection,
	))
	return nil
}

func baseTools(cfg *config.Config) []tool.Tool {
	tools := builtin.All()
	for _, t := range tools {
		if bash, ok := t.(*builtin.BashTool); ok {
			bash.PermissionMode = "default"
			if cfg.Permission.Enabled && cfg.Permission.Mode != "" {
				bash.PermissionMode = cfg.Permission.Mode
			}
			bash.BlockedCommands = cfg.Permission.CommandBlacklist.BlockedCommands
		}
	}
	return tools
}

func runtimeTools(cfg *config.Config, workDir string, checker *permission.Checker, extraTools []tool.Tool, subAgentCoordinator *subagent.Coordinator) []tool.Tool {
	tools := baseTools(cfg)
	tools = append(tools,
		runtimetools.NewApplyPatchTool(workDir),
		runtimetools.NewTodoWriteTool("basework-runtime"),
		runtimetools.NewQuestionTool(checker),
		runtimetools.NewWebFetchTool(cfg),
		runtimetools.NewWebSearchTool(cfg),
	)
	if subAgentCoordinator != nil {
		tools = append(tools, subagent.NewSubAgentTool(subAgentCoordinator))
	}
	tools = append(tools, extraTools...)
	return tools
}

func runtimeBehaviorOptions(cfg *config.Config, model llm.Model, subAgentCoordinator *subagent.Coordinator) []agent.Option {
	var opts []agent.Option
	if c := newRuntimeCompactor(cfg, model); c != nil {
		opts = append(opts, agent.WithCompactor(c))
	}
	if d := newRuntimeLoopDetector(cfg); d != nil {
		opts = append(opts, agent.WithLoopDetector(d))
	}
	if bus := newRuntimeEventBus(cfg); bus != nil {
		adapter := observability.NewEventBusAdapter(bus)
		builtin.SetEventBus(adapter)
		opts = append(opts, agent.WithEventBus(adapter))
	}
	if subAgentCoordinator != nil {
		opts = append(opts, agent.WithSubAgentRunner(subagent.NewSubAgentRunnerAdapter(subAgentCoordinator)))
	}
	return opts
}

func newRuntimeCompactor(cfg *config.Config, model llm.Model) agent.Compactor {
	if !cfg.Compaction.Enabled {
		return nil
	}
	compactionCfg := compaction.Config{
		Enabled:    cfg.Compaction.Enabled,
		Strategy:   cfg.Compaction.Strategy,
		Threshold:  cfg.Compaction.Threshold,
		WindowSize: cfg.Compaction.WindowSize,
	}
	var strategy compaction.Strategy
	switch cfg.Compaction.Strategy {
	case "summarization":
		strategy = compaction.NewSummarizationStrategy(compaction.NewLLMSummarizer(model))
	case "selective":
		strategy = compaction.NewSelectiveStrategy(cfg.Compaction.WindowSize)
	default:
		strategy = compaction.NewSlidingWindowStrategy(cfg.Compaction.WindowSize)
	}
	return compaction.NewEngine(compactionCfg, strategy)
}

func newRuntimeLoopDetector(cfg *config.Config) agent.LoopDetector {
	if !cfg.LoopDetect.Enabled {
		return nil
	}
	loopCfg := loopdetect.Config{
		Enabled:           cfg.LoopDetect.Enabled,
		RepeatedThreshold: cfg.LoopDetect.RepeatedThreshold,
		ToolLoopThreshold: cfg.LoopDetect.ToolLoopThreshold,
		ResponseStrategy:  cfg.LoopDetect.ResponseStrategy,
		CustomPatterns:    cfg.LoopDetect.CustomPatterns,
	}
	return loopdetect.NewLoopDetectorAdapter(loopdetect.NewDetector(loopCfg))
}

func newRuntimeEventBus(cfg *config.Config) *observability.EventBus {
	if !cfg.Observability.Enabled {
		return nil
	}
	return observability.NewEventBus()
}

func newRuntimeSubAgentCoordinator(cfg *config.Config, model llm.Model, workDir string, checker *permission.Checker, permissionChecker *runtimePermissionChecker, extraTools []tool.Tool) *subagent.Coordinator {
	if !cfg.SubAgent.Enabled {
		return nil
	}
	subAgentCfg := subagent.Config{
		Enabled:       cfg.SubAgent.Enabled,
		DefaultType:   cfg.SubAgent.DefaultType,
		CostLimit:     cfg.SubAgent.CostLimit,
		MaxConcurrent: cfg.SubAgent.MaxConcurrent,
	}
	factory := func(ctx context.Context, task *subagent.Task) (subagent.AgentTaskRunner, error) {
		childTools := runtimeSubAgentTools(cfg, workDir, checker, extraTools, task.AgentType)
		childOpts := []agent.Option{
			agent.WithModel(model),
			agent.WithSession(session.NewMemoryStore()),
			agent.WithSystemPrompt(runtimeSubAgentPrompt(cfg, workDir, task)),
			agent.WithMaxSteps(cfg.MaxIterations),
			agent.WithTools(childTools...),
		}
		if permissionChecker != nil {
			childOpts = append(childOpts, agent.WithPermissionChecker(permissionChecker.Adapter))
		}
		if c := newRuntimeCompactor(cfg, model); c != nil {
			childOpts = append(childOpts, agent.WithCompactor(c))
		}
		if d := newRuntimeLoopDetector(cfg); d != nil {
			childOpts = append(childOpts, agent.WithLoopDetector(d))
		}
		child, err := agent.New(childOpts...)
		if err != nil {
			return nil, err
		}
		return &runtimeSubAgentRunner{agent: child}, nil
	}
	return subagent.NewCoordinator(factory, subAgentCfg)
}

func runtimeSubAgentTools(cfg *config.Config, workDir string, checker *permission.Checker, extraTools []tool.Tool, agentType subagent.AgentType) []tool.Tool {
	if agentType == subagent.TypeReadonly {
		return readonlyRuntimeTools(cfg, extraTools)
	}
	return runtimeTools(cfg, workDir, checker, extraTools, nil)
}

func readonlyRuntimeTools(cfg *config.Config, extraTools []tool.Tool) []tool.Tool {
	allowed := map[string]bool{
		"read": true,
		"grep": true,
		"glob": true,
	}
	var tools []tool.Tool
	for _, t := range baseTools(cfg) {
		if allowed[t.Name()] {
			tools = append(tools, t)
		}
	}
	for _, t := range extraTools {
		name := t.Name()
		if strings.HasPrefix(name, "lsp_") || name == "mcp_read" || name == "mcp_prompt" {
			tools = append(tools, t)
		}
	}
	return tools
}

func runtimeSubAgentPrompt(cfg *config.Config, workDir string, task *subagent.Task) string {
	prompt := runtimeSystemPrompt(cfg, workDir)
	if task.AgentType == subagent.TypeReadonly {
		prompt += "\n\n你是只读子代理。只能读取和分析信息，不要修改文件、执行写入操作或运行破坏性命令。"
	}
	if len(task.Context) > 0 {
		if data, err := json.Marshal(task.Context); err == nil {
			prompt += "\n\n子代理上下文：" + string(data)
		}
	}
	return strings.TrimSpace(prompt)
}

type runtimeSubAgentRunner struct {
	agent agent.Agent
}

func (r *runtimeSubAgentRunner) HandleMessage(ctx context.Context, input string) (*subagent.TaskResult, error) {
	resp, err := r.agent.HandleMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return &subagent.TaskResult{
		Content:      responseText(resp),
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	}, nil
}

func (r *runtimeSubAgentRunner) Close() error {
	return r.agent.Close()
}

type runtimePermissionChecker struct {
	Core     *permission.Checker
	Adapter  agent.PermissionChecker
	Cleanups []func() error
}

func newPermissionChecker(cfg *config.Config) (*runtimePermissionChecker, error) {
	mode := cfg.Permission.Mode
	if mode == "" {
		mode = "deny-all"
	}
	parsedMode, err := permission.ParseMode(mode)
	if err != nil {
		return nil, fmt.Errorf("解析权限模式失败: %w", err)
	}
	persistence, err := openRuntimePermissionPersistence(cfg)
	if err != nil {
		return nil, err
	}

	prompt := newRuntimePermissionPrompt(os.Stdin, os.Stdout)
	var checker *permission.Checker
	if persistence.Store != nil {
		checker = permission.NewCheckerWithStore(parsedMode, persistence.Store, prompt)
	} else {
		checker = permission.NewChecker(parsedMode, nil, prompt)
	}
	if persistence.AuditLogger != nil {
		checker = checker.WithAudit(persistence.AuditLogger)
	}

	level := cfg.Security.ProtectionLevel
	if level == "" {
		level = "strict"
	}
	protection, err := permission.ParseProtectionLevel(level)
	if err != nil {
		return nil, fmt.Errorf("解析敏感路径保护级别失败: %w", err)
	}
	pathChecker := permission.NewPathChecker(
		cfg.Security.SensitivePaths.Block,
		cfg.Security.SensitivePaths.Allow,
		protection,
	)
	return &runtimePermissionChecker{
		Core:     checker,
		Adapter:  permission.NewPathPermissionAdapter(checker, pathChecker),
		Cleanups: persistence.Cleanups,
	}, nil
}

func initializeRuntimeExtensions(ctx context.Context, cfg *config.Config, workDir string, extraCleanups ...func() error) (agent.Plugin, []tool.Tool) {
	var tools []tool.Tool
	cleanups := append([]func() error{}, extraCleanups...)

	if lspManager, err := startLSPManager(ctx, workDir); err != nil {
		log.Printf("[runtime] warning: LSP disabled: %v", err)
	} else {
		tools = append(tools, lsp.Tools(lspManager)...)
		cleanups = append(cleanups, lspManager.Stop)
	}

	if mcpManager, err := startMCPManager(ctx, cfg); err != nil {
		log.Printf("[runtime] warning: MCP disabled: %v", err)
	} else if mcpManager != nil {
		tools = append(tools, runtimetools.NewMCPReadTool(mcpManager), runtimetools.NewMCPPromptTool(mcpManager))
		tools = append(tools, mcpManager.Tools()...)
		cleanups = append(cleanups, mcpManager.Close)
	}

	return newRuntimeCleanupPlugin(cleanups...), tools
}

type runtimePermissionPersistence struct {
	Store       permission.Store
	AuditLogger *permission.AuditLogger
	Cleanups    []func() error
}

func newRuntimePermissionPrompt(stdin *os.File, stdout *os.File) permission.PromptFunc {
	return func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		default:
		}

		fmt.Fprintf(stdout, "\n权限请求: 工具 %s\n", toolName)
		if len(args) > 0 {
			fmt.Fprintf(stdout, "参数:\n%s\n", permission.MarshalArgs(args))
		}
		fmt.Fprint(stdout, "允许执行？[y] 本次允许 / [a] 始终允许 / [n] 本次拒绝 / [d] 始终拒绝: ")

		reader := bufio.NewReader(stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false, false, fmt.Errorf("读取权限输入失败: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, false, nil
		case "a", "always":
			return true, true, nil
		case "d", "deny", "never":
			return false, true, nil
		default:
			return false, false, nil
		}
	}
}

func startLSPManager(ctx context.Context, workDir string) (*lsp.Manager, error) {
	manager := lsp.NewManager(lsp.Config{})
	if err := manager.Start(ctx, workDir); err != nil {
		return nil, err
	}
	return manager, nil
}

func startMCPManager(ctx context.Context, cfg *config.Config) (*mcp.Manager, error) {
	if len(cfg.MCPConfigs) == 0 {
		return nil, nil
	}
	manager := mcp.NewManager()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for name, raw := range cfg.MCPConfigs {
		serverCfg, err := parseMCPServerConfig(raw)
		if err != nil {
			log.Printf("[runtime] warning: skip MCP server %q: %v", name, err)
			continue
		}
		if err := manager.Connect(connectCtx, name, serverCfg); err != nil {
			log.Printf("[runtime] warning: MCP server %q connect failed: %v", name, err)
			continue
		}
	}
	return manager, nil
}

func parseMCPServerConfig(raw interface{}) (mcp.ServerConfig, error) {
	var cfg mcp.ServerConfig
	data, err := json.Marshal(raw)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.LoadConfig()
	return cfg, cfg.Validate()
}

func runtimeSystemPrompt(cfg *config.Config, workDir string) string {
	prompt := cfg.SystemPrompt
	skillPrompt := runtimeSkillPrompt(cfg, workDir)
	if skillPrompt == "" {
		return prompt
	}
	if prompt == "" {
		return skillPrompt
	}
	return prompt + "\n\n" + skillPrompt
}

func runtimeSkillPrompt(cfg *config.Config, workDir string) string {
	paths := runtimeSkillPaths(cfg, workDir)
	loader := skill.NewLoader(paths...)
	if err := loader.Discover(); err != nil {
		log.Printf("[runtime] warning: skill discovery failed: %v", err)
		return ""
	}
	xml := skill.ToPromptXML(loader.Active())
	if xml == "" {
		return ""
	}
	return "可用技能定义如下；当任务匹配技能描述时优先遵循对应技能指令：\n" + xml
}

func runtimeSkillPaths(cfg *config.Config, workDir string) []string {
	paths := []string{filepath.Join(workDir, ".basework", "skills")}
	if cfg.Templates.CustomDir != "" {
		paths = append(paths, filepath.Join(cfg.Templates.CustomDir, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, ".basework", "skills"),
			filepath.Join(home, ".config", "basework", "skills"),
		)
	}
	return paths
}

type runtimeCleanupPlugin struct {
	cleanups []func() error
}

func newRuntimeCleanupPlugin(cleanups ...func() error) agent.Plugin {
	if len(cleanups) == 0 {
		return nil
	}
	return &runtimeCleanupPlugin{cleanups: cleanups}
}

func (p *runtimeCleanupPlugin) Name() string {
	return "runtime-cleanup"
}

func (p *runtimeCleanupPlugin) Initialize(context.Context, agent.Agent) error {
	return nil
}

func (p *runtimeCleanupPlugin) Shutdown(context.Context) error {
	var firstErr error
	for i := len(p.cleanups) - 1; i >= 0; i-- {
		if err := p.cleanups[i](); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
