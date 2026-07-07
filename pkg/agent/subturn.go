package agent

import (
	"context"
	"errors"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// SubTurnConfig 子代理配置（精简版 8 字段）
type SubTurnConfig struct {
	// Prompt 子代理任务提示
	Prompt string
	// Tools 可选，覆盖父 agent 工具
	Tools []tool.Tool
	// Model 可选，覆盖父 agent 模型
	Model llm.Model
	// MaxSteps 可选，默认 10
	MaxSteps int
	// Async 异步执行
	Async bool
	// Budget token 预算（共享指针）
	Budget *int64
	// Session 可选，独立 session
	Session session.Store
	// Hooks 可选，覆盖父 agent hooks
	Hooks []hook.Hook
}

// SubTurnResult 子代理结果
type SubTurnResult struct {
	// Response 子代理响应
	Response *Response
	// Done 异步模式使用，关闭时表示执行完成
	Done <-chan struct{}
	// Err 子代理执行错误
	Err error
}

// RunSubTurn 执行子代理任务
func RunSubTurn(ctx context.Context, parent Agent, cfg SubTurnConfig) (*SubTurnResult, error) {
	if parent == nil {
		return nil, errors.New("subturn: parent agent 不能为 nil")
	}

	// 从 parent 获取 model
	model := cfg.Model
	if model == nil {
		// 通过类型断言获取 AgentLoop 的 model
		loop, ok := parent.(*AgentLoop)
		if !ok {
			return nil, errors.New("subturn: 无法从 parent agent 获取 model，请通过 cfg.Model 指定")
		}
		model = loop.model
	}

	// 构造 options
	opts := []Option{
		WithModel(model),
		WithMaxSteps(cfg.MaxSteps),
	}

	// 默认 maxSteps
	if cfg.MaxSteps <= 0 {
		opts = append(opts, WithMaxSteps(10))
	}

	// 工具
	if len(cfg.Tools) > 0 {
		opts = append(opts, WithTools(cfg.Tools...))
	}

	// session
	if cfg.Session != nil {
		opts = append(opts, WithSession(cfg.Session))
	}

	// hooks
	for _, h := range cfg.Hooks {
		opts = append(opts, WithHook(h))
	}

	// 异步执行
	if cfg.Async {
		done := make(chan struct{})
		result := &SubTurnResult{Done: done}

		go func() {
			defer close(done)
			resp, err := executeSubTurn(ctx, opts, cfg.Prompt)
			result.Response = resp
			result.Err = err
		}()

		return result, nil
	}

	// 同步执行
	resp, err := executeSubTurn(ctx, opts, cfg.Prompt)
	return &SubTurnResult{
		Response: resp,
		Err:      err,
	}, nil
}

// executeSubTurn 内部创建并执行子代理
func executeSubTurn(ctx context.Context, opts []Option, prompt string) (*Response, error) {
	subAgent, err := New(opts...)
	if err != nil {
		return nil, err
	}
	defer subAgent.Close()

	return subAgent.HandleMessage(ctx, prompt)
}
