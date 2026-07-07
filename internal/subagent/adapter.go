package subagent

import (
	"context"
	"errors"

	"github.com/wly2lcl/basework/pkg/agent"
)

// SubAgentRunnerAdapter 将 *Coordinator 适配为 agent.SubAgentRunner 接口。
type SubAgentRunnerAdapter struct {
	inner *Coordinator
}

// NewSubAgentRunnerAdapter 创建适配器。
func NewSubAgentRunnerAdapter(c *Coordinator) *SubAgentRunnerAdapter {
	return &SubAgentRunnerAdapter{inner: c}
}

// Run 实现 agent.SubAgentRunner。
func (a *SubAgentRunnerAdapter) Run(ctx context.Context, task string) (string, error) {
	if a.inner == nil {
		return "", errors.New("subagent: coordinator 未设置")
	}
	if !a.inner.Config.Enabled {
		return "", errors.New("subagent: 系统已禁用")
	}

	t, err := a.inner.CreateTask(task, AgentType(a.inner.Config.DefaultType), nil)
	if err != nil {
		return "", err
	}

	result, err := a.inner.ExecuteTask(ctx, t)
	if err != nil {
		return "", err
	}

	if !result.Success {
		return result.Output, errors.New(result.Error)
	}
	return result.Output, nil
}

// Enabled 实现 agent.SubAgentRunner。
func (a *SubAgentRunnerAdapter) Enabled() bool {
	if a.inner == nil {
		return false
	}
	return a.inner.Config.Enabled
}

// Ensure SubAgentRunnerAdapter implements agent.SubAgentRunner.
var _ agent.SubAgentRunner = (*SubAgentRunnerAdapter)(nil)