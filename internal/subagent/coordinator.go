package subagent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// AgentFactory 创建子代理实例的函数类型
type AgentFactory func(ctx context.Context, task *Task) (AgentTaskRunner, error)

// Config 子代理系统配置
type Config struct {
	// Enabled 是否启用子代理功能
	Enabled bool `json:"enabled"`
	// DefaultType 默认子代理类型
	DefaultType string `json:"default_type"`
	// CostLimit 每个子代理的最大成本限制（美元）
	CostLimit float64 `json:"cost_limit"`
	// MaxConcurrent 最大并发子代理数量
	MaxConcurrent int `json:"max_concurrent"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		DefaultType:   string(TypeGeneral),
		CostLimit:     1.0,
		MaxConcurrent: 5,
	}
}

// Coordinator 子代理协调器，负责任务创建、分发和结果聚合
type Coordinator struct {
	// AgentFactory 创建子代理实例的工厂函数
	AgentFactory AgentFactory
	// Config 子代理系统配置
	Config Config
	// progressReporter 进度报告器
	progressReporter ProgressReporter
	// costTracker 成本追踪器
	costTracker *CostTracker
	mu          sync.RWMutex
}

// NewCoordinator 创建新的子代理协调器
func NewCoordinator(factory AgentFactory, cfg Config) *Coordinator {
	return &Coordinator{
		AgentFactory:     factory,
		Config:           cfg,
		costTracker:      NewCostTracker(),
		progressReporter: &NopProgressReporter{},
	}
}

// SetProgressReporter 设置进度报告器
func (c *Coordinator) SetProgressReporter(reporter ProgressReporter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progressReporter = reporter
}

// ProgressReporter 返回当前的进度报告器
func (c *Coordinator) ProgressReporter() ProgressReporter {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.progressReporter
}

// CostTracker 返回成本追踪器
func (c *Coordinator) CostTracker() *CostTracker {
	return c.costTracker
}

// CreateTask 创建子代理任务
func (c *Coordinator) CreateTask(description string, agentType AgentType, context map[string]interface{}) (*Task, error) {
	if !c.Config.Enabled {
		return nil, errors.New("子代理系统已禁用")
	}
	if description == "" {
		return nil, errors.New("任务描述不能为空")
	}
	if agentType == "" {
		agentType = AgentType(c.Config.DefaultType)
	}
	if agentType != TypeGeneral && agentType != TypeReadonly {
		return nil, fmt.Errorf("不支持的子代理类型: %s", agentType)
	}

	return NewTask(description, agentType, context), nil
}

// ExecuteTask 执行单个子代理任务
func (c *Coordinator) ExecuteTask(ctx context.Context, task *Task) (*Result, error) {
	if !c.Config.Enabled {
		return nil, errors.New("子代理系统已禁用")
	}
	if c.AgentFactory == nil {
		return nil, errors.New("AgentFactory 未设置")
	}

	// 报告任务启动
	c.ProgressReporter().OnStart(task)

	// 创建子代理实例
	agent, err := c.AgentFactory(ctx, task)
	if err != nil {
		result := &Result{
			ID:      task.ID,
			Success: false,
			Error:   fmt.Sprintf("创建子代理失败: %v", err),
		}
		c.ProgressReporter().OnComplete(task, result)
		return result, nil
	}

	// 检查成本限制
	if c.Config.CostLimit > 0 {
		taskCost := c.costTracker.GetTaskCost(task.ID)
		if taskCost >= c.Config.CostLimit {
			result := &Result{
				ID:      task.ID,
				Success: false,
				Error:   fmt.Sprintf("成本超限: %.4f (限制: %.4f)", taskCost, c.Config.CostLimit),
			}
			c.ProgressReporter().OnComplete(task, result)
			return result, nil
		}
	}

	// 执行任务（通过 Agent）
	result, err := c.executeAgentTask(ctx, agent, task)
	if err != nil {
		result = &Result{
			ID:      task.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	// 记录成本
	if result.TokenUsage.TotalTokens > 0 || result.Cost > 0 {
		c.costTracker.TrackUsage(task.ID, result.TokenUsage, result.Cost)
	}

	// 报告任务完成
	c.ProgressReporter().OnComplete(task, result)

	return result, nil
}

// executeAgentTask 与子代理交互执行任务
func (c *Coordinator) executeAgentTask(ctx context.Context, agent AgentTaskRunner, task *Task) (*Result, error) {
	// 默认超时 30 秒
	timeout := 30 * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := agent.HandleMessage(execCtx, task.Description)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &Result{
				ID:      task.ID,
				Success: false,
				Error:   fmt.Sprintf("任务执行超时（%v）: %s", timeout, task.Description),
				TokenUsage: TokenUsage{
					InputTokens:  0,
					OutputTokens: 0,
					TotalTokens:  0,
				},
				Cost: 0,
			}, nil
		}
		return nil, fmt.Errorf("子代理执行失败: %w", err)
	}

	return &Result{
		ID:      task.ID,
		Success: true,
		Output:  resp.Content,
		TokenUsage: TokenUsage{
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
			TotalTokens:  resp.TotalTokens,
		},
		Cost: 0,
	}, nil
}

// ExecuteTasks 并发执行多个子代理任务
func (c *Coordinator) ExecuteTasks(ctx context.Context, tasks []*Task) ([]*Result, error) {
	if !c.Config.Enabled {
		return nil, errors.New("子代理系统已禁用")
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	// 使用 errgroup 管理并发
	g, gctx := errgroup.WithContext(ctx)

	// 控制并发数的信号量
	sem := make(chan struct{}, c.Config.MaxConcurrent)
	if c.Config.MaxConcurrent <= 0 {
		sem = make(chan struct{}, len(tasks)) // 无限制
	}

	results := make([]*Result, len(tasks))
	var mu sync.Mutex

	for i, task := range tasks {
		i, task := i, task
		g.Go(func() error {
			// 获取信号量
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()

			// 执行单个任务
			result, err := c.ExecuteTask(gctx, task)
			mu.Lock()
			results[i] = result
			mu.Unlock()
			return err
		})
	}

	// 等待所有任务完成
	if err := g.Wait(); err != nil {
		return results, err
	}

	return results, nil
}