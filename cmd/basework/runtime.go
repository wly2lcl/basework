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

	"github.com/wly2lcl/basework/internal/permission"
	runtimetools "github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
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
		APIKey:  lookupAPIKey(providerType),
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

	agentOpts := []agent.Option{
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithSystemPrompt(runtimeSystemPrompt(cfg, cwd)),
		agent.WithMaxSteps(cfg.MaxIterations),
		agent.WithTools(runtimeTools(cfg, cwd, coreChecker, extraTools)...),
	}
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

func runtimeTools(cfg *config.Config, workDir string, checker *permission.Checker, extraTools []tool.Tool) []tool.Tool {
	tools := baseTools(cfg)
	tools = append(tools,
		runtimetools.NewApplyPatchTool(workDir),
		runtimetools.NewTodoWriteTool("basework-runtime"),
		runtimetools.NewQuestionTool(checker),
		runtimetools.NewWebFetchTool(cfg),
		runtimetools.NewWebSearchTool(cfg),
	)
	tools = append(tools, extraTools...)
	return tools
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
