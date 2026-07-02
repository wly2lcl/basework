package agent

import (
	"context"
	"sync"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// AgentLoop 是 Agent 接口的具体实现，管理工具循环和生命周期
type AgentLoop struct {
	model     llm.Model
	session   session.Store
	sessionID string
	cfg       *config
	chain     *hook.Chain
	plugins   []Plugin
	observer  Observer
	callback  Callback
	pipeline  *Pipeline
	mu        sync.Mutex
	closed    bool
}

// agentInstance 实现 Instance 接口
type agentInstance struct {
	agent *AgentLoop
}

func (a *agentInstance) ID() string {
	return a.agent.sessionID
}

func (a *agentInstance) Model() llm.Model {
	return a.agent.model
}

func (a *agentInstance) TurnD() TurnD {
	return &agentTurnD{agent: a.agent}
}

// agentTurnD 实现 TurnD 接口
type agentTurnD struct {
	agent *AgentLoop
}

func (d *agentTurnD) SystemPrompt() string {
	return d.agent.cfg.systemPrompt
}

func (d *agentTurnD) MaxSteps() int {
	return d.agent.cfg.maxSteps
}

func (d *agentTurnD) Model() llm.Model {
	return d.agent.model
}

func (d *agentTurnD) Session() session.Store {
	return d.agent.session
}

func (d *agentTurnD) SessionID() string {
	return d.agent.sessionID
}

func (d *agentTurnD) History() ([]llm.ChatMessage, error) {
	events, err := d.agent.session.Events(session.EventFilter{SessionID: d.agent.sessionID})
	if err != nil {
		return nil, err
	}
	return session.ProjectMessages(events), nil
}

func (d *agentTurnD) Hooks() *hook.Chain {
	return d.agent.chain
}

func (d *agentTurnD) ToolRegistry() *tool.Registry {
	return d.agent.cfg.registry
}

func (d *agentTurnD) Callback() Callback {
	return d.agent.callback
}

// newAgentLoop 创建 AgentLoop，初始化 session 和 pipeline
func newAgentLoop(cfg *config, chain *hook.Chain) (*AgentLoop, error) {
	sess := cfg.session
	if sess == nil {
		sess = session.NewMemoryStore()
	}

	info, err := sess.Create(session.CreateOpts{Title: "agent session"})
	if err != nil {
		return nil, err
	}

	a := &AgentLoop{
		model:     cfg.model,
		session:   sess,
		sessionID: info.ID,
		cfg:       cfg,
		chain:     chain,
		plugins:   cfg.plugins,
		observer:  cfg.observer,
		callback:  cfg.callback,
	}

	// 初始化插件（失败时回滚已初始化的）
	for i, p := range cfg.plugins {
		if err := p.Initialize(context.Background(), a); err != nil {
			// 回滚：逆序关闭已初始化的插件
			for j := i - 1; j >= 0; j-- {
				_ = cfg.plugins[j].Shutdown(context.Background())
			}
			return nil, err
		}
	}

	// 创建 pipeline
	inst := &agentInstance{agent: a}
	a.pipeline = NewPipeline(inst.TurnD())

	return a, nil
}

// HandleMessage 处理单条文本输入
func (a *AgentLoop) HandleMessage(ctx context.Context, input string) (*Response, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, nil
	}
	a.mu.Unlock()

	// 观测器通知
	if a.observer != nil {
		a.observer.OnAgentStart(ctx, input)
	}

	// 1. 追加 Prompted 事件
	promptData, _ := session.EncodeData(&session.PromptedData{Content: input})
	_ = a.session.AppendEvent(session.Event{
		SessionID: a.sessionID,
		Type:      session.EventPrompted,
		Data:      promptData,
	})

	// 2. 运行工具循环
	resp, err := a.runLoop(ctx)

	// 通知观测器
	if a.observer != nil {
		a.observer.OnAgentEnd(ctx, resp, err)
	}

	// 通知错误回调
	if err != nil && a.callback != nil {
		a.callback.OnError(err)
	}

	if resp != nil {
		resp.SessionID = a.sessionID
		if a.callback != nil {
			a.callback.OnTurnEnd(resp)
		}
	}

	return resp, err
}

// HandleMessages 处理预构建的多条消息
func (a *AgentLoop) HandleMessages(ctx context.Context, messages []llm.ChatMessage) (*Response, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, nil
	}
	a.mu.Unlock()

	// 将消息转换为事件存储
	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleUser:
			text := ""
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					text += part.Text
				}
			}
			data, _ := session.EncodeData(&session.PromptedData{Content: text})
			_ = a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventPrompted,
				Data:      data,
			})

		case llm.RoleAssistant:
			text := ""
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					text += part.Text
				}
			}
			if text != "" {
				deltaData, _ := session.EncodeData(&session.TextDeltaData{Delta: text})
				_ = a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventTextDelta,
					Data:      deltaData,
				})
			}
			for _, tc := range msg.ToolCalls {
				callData, _ := session.EncodeData(&session.ToolCalledData{ToolCall: tc})
				_ = a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventToolCalled,
					Data:      callData,
				})
			}
			if text != "" || len(msg.ToolCalls) > 0 {
				_ = a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventTextEnded,
				})
			}

		case llm.RoleTool:
			if msg.ToolCallID == "" {
				continue
			}
			text := ""
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					text += part.Text
				}
			}
			succData, _ := session.EncodeData(&session.ToolSuccessData{
				ToolCallID: msg.ToolCallID,
				Content:    text,
			})
			_ = a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventToolSuccess,
				Data:      succData,
			})
		}
	}

	return a.runLoop(ctx)
}

// runLoop 执行工具循环，每次迭代调用 pipeline.Run()
func (a *AgentLoop) runLoop(ctx context.Context) (*Response, error) {
	var allToolCalls []ToolCallRecord
	var finalUsage llm.Usage
	var finalMessage llm.ChatMessage

	for step := 0; step < a.cfg.maxSteps; step++ {
		// 每轮重新创建 pipeline（TurnD 会从 session 读取最新状态）
		inst := &agentInstance{agent: a}
		p := NewPipeline(inst.TurnD())

		// 追加 turn started 事件
		turnData, _ := session.EncodeData(&session.TurnStartedData{Step: step})
		_ = a.session.AppendEvent(session.Event{
			SessionID: a.sessionID,
			Type:      session.EventTurnStarted,
			Data:      turnData,
		})

		result, err := p.Run(ctx)
		if err != nil {
			// 追加 turn failed 事件
			_ = a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventTurnFailed,
			})
			return nil, err
		}

		// 追加 turn ended 事件
		endData, _ := session.EncodeData(&session.TurnEndedData{Usage: result.Usage})
		_ = a.session.AppendEvent(session.Event{
			SessionID: a.sessionID,
			Type:      session.EventTurnEnded,
			Data:      endData,
		})

		finalMessage = result.Message
		finalUsage = result.Usage
		allToolCalls = append(allToolCalls, result.ToolCalls...)

		if !result.HasToolCalls {
			return &Response{
				Message:   finalMessage,
				ToolCalls: allToolCalls,
				Usage:     finalUsage,
				SessionID: a.sessionID,
			}, nil
		}
	}

	return nil, ErrMaxStepsExceeded
}

// Close 关闭 agent，逆序关闭插件
func (a *AgentLoop) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	// 逆序关闭插件
	for i := len(a.plugins) - 1; i >= 0; i-- {
		_ = a.plugins[i].Shutdown(context.Background())
	}

	return nil
}

// 编译期接口检查
var _ Agent = (*AgentLoop)(nil)
var _ Instance = (*agentInstance)(nil)
var _ TurnD = (*agentTurnD)(nil)