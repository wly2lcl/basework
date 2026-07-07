package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
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

	agentOpts := []agent.Option{
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithSystemPrompt(cfg.SystemPrompt),
		agent.WithMaxSteps(cfg.MaxIterations),
		agent.WithTools(baseTools(cfg)...),
	}
	if opts.Callback != nil {
		agentOpts = append(agentOpts, agent.WithCallback(opts.Callback))
	}
	if cfg.Permission.Enabled {
		checker, err := newPermissionChecker(cfg)
		if err != nil {
			return nil, err
		}
		agentOpts = append(agentOpts, agent.WithPermissionChecker(checker))
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

func newPermissionChecker(cfg *config.Config) (agent.PermissionChecker, error) {
	mode := cfg.Permission.Mode
	if mode == "" {
		mode = "deny-all"
	}
	parsedMode, err := permission.ParseMode(mode)
	if err != nil {
		return nil, fmt.Errorf("解析权限模式失败: %w", err)
	}
	checker := permission.NewChecker(parsedMode, nil, func(context.Context, string, map[string]interface{}) (bool, bool, error) {
		return false, false, nil
	})

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
	return permission.NewPathPermissionAdapter(checker, pathChecker), nil
}
