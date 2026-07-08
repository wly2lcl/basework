package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// ErrAgentClosed 表示 agent 已关闭的错误
var ErrAgentClosed = errors.New("agent: closed")

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

	// 压缩状态标记
	compressionApplied bool

	// 集成模块接口
	compactor    Compactor
	permChecker  PermissionChecker
	loopDetector LoopDetector
	eventBus     EventPublisher
	obsEnabled   bool

	// steeringManager 转向管理器
	steeringManager *SteeringManager
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
		model:           cfg.model,
		session:         sess,
		sessionID:       info.ID,
		cfg:             cfg,
		chain:           chain,
		plugins:         cfg.plugins,
		observer:        cfg.observer,
		callback:        cfg.callback,
		compactor:       cfg.compactor,
		permChecker:     cfg.permChecker,
		loopDetector:    cfg.loopDetector,
		eventBus:        cfg.eventBus,
		obsEnabled:      cfg.obsEnabled,
		steeringManager: cfg.steeringManager,
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
		return nil, ErrAgentClosed
	}
	a.mu.Unlock()

	// 观测器通知
	if a.observer != nil {
		a.observer.OnAgentStart(ctx, input)
	}

	// 1. 追加 Prompted 事件
	promptData, encodeErr := session.EncodeData(&session.PromptedData{Content: input})
	if encodeErr != nil {
		log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventPrompted)
	} else if err := a.session.AppendEvent(session.Event{
		SessionID: a.sessionID,
		Type:      session.EventPrompted,
		Data:      promptData,
	}); err != nil {
		log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventPrompted)
	}

	// 新消息到来时重置压缩标记，允许再次压缩
	a.compressionApplied = false

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
		return nil, ErrAgentClosed
	}
	a.mu.Unlock()

	// 新消息到来时重置压缩标记，允许再次压缩
	a.compressionApplied = false

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
			data, encodeErr := session.EncodeData(&session.PromptedData{Content: text})
			if encodeErr != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventPrompted)
			} else if err := a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventPrompted,
				Data:      data,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventPrompted)
			}

		case llm.RoleAssistant:
			text := ""
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					text += part.Text
				}
			}
			if text != "" {
				deltaData, encodeErr := session.EncodeData(&session.TextDeltaData{Delta: text})
				if encodeErr != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventTextDelta)
				} else if err := a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventTextDelta,
					Data:      deltaData,
				}); err != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventTextDelta)
				}
			}
			for _, tc := range msg.ToolCalls {
				callData, encodeErr := session.EncodeData(&session.ToolCalledData{ToolCall: tc})
				if encodeErr != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventToolCalled)
				} else if err := a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventToolCalled,
					Data:      callData,
				}); err != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventToolCalled)
				}
			}
			if text != "" || len(msg.ToolCalls) > 0 {
				if err := a.session.AppendEvent(session.Event{
					SessionID: a.sessionID,
					Type:      session.EventTextEnded,
				}); err != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventTextEnded)
				}
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
			succData, encodeErr := session.EncodeData(&session.ToolSuccessData{
				ToolCallID: msg.ToolCallID,
				Content:    text,
			})
			if encodeErr != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventToolSuccess)
			} else if err := a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventToolSuccess,
				Data:      succData,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventToolSuccess)
			}
		}
	}

	return a.runLoop(ctx)
}

// runLoop 执行工具循环，每次迭代调用 pipeline.Run()
func (a *AgentLoop) runLoop(ctx context.Context) (*Response, error) {
	var allToolCalls []ToolCallRecord
	var loopToolCalls []LoopToolCall
	var finalUsage llm.Usage
	var finalMessage llm.ChatMessage

	// 可观测性：发布 agent.start 事件
	a.publishEvent("agent.start", nil)

	// 1. 压缩：检查是否需要压缩上下文
	if !a.compressionApplied {
		history, err := a.td().History()
		if err == nil && a.compactor != nil {
			if a.compactor.ShouldCompact(history, a.cfg.maxContextTokens) {
				compacted, cerr := a.compactor.Compact(history)
				if cerr == nil && len(compacted) < len(history) {
					log.Printf("[压缩] 上下文已从 %d 条消息压缩至 %d 条", len(history), len(compacted))
					// 写入 EventCompacted 事件
					keepFrom := len(history) - len(compacted)
					compactData, err := session.EncodeData(&session.CompactedData{KeepFrom: keepFrom})
					if err != nil {
						log.Printf("event persistence failed: %v", err)
					} else if err := a.session.AppendEvent(session.Event{
						SessionID: a.sessionID,
						Type:      session.EventCompacted,
						Data:      compactData,
					}); err != nil {
						log.Printf("event persistence failed: %v", err)
					}
					// 设置标记，跳过后续压缩直到新消息到达
					a.compressionApplied = true
				}
			}
		}
	}

	for step := 0; step < a.cfg.maxSteps; step++ {
		// 每轮重新创建 pipeline（TurnD 会从 session 读取最新状态）
		inst := &agentInstance{agent: a}
		p := NewPipeline(inst.TurnD())

		// 注入 steering 消息：在 setupTurn 中 prepend 到历史消息之前
		if a.steeringManager != nil {
			msgs := a.steeringManager.Drain()
			if len(msgs) > 0 {
				p.steeringMsgs = steeringToChatMessages(msgs)
			}
		}

		// 追加 turn started 事件
		turnData, encodeErr := session.EncodeData(&session.TurnStartedData{Step: step})
		if encodeErr != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventTurnStarted)
		} else if err := a.session.AppendEvent(session.Event{
			SessionID: a.sessionID,
			Type:      session.EventTurnStarted,
			Data:      turnData,
		}); err != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventTurnStarted)
		}

		result, err := p.Run(ctx)
		if err != nil {
			// 可观测性：发布 agent.error 事件
			a.publishEvent("agent.error", map[string]interface{}{
				"error": err.Error(),
				"step":  step,
			})

			// 追加 turn failed 事件
			if err := a.session.AppendEvent(session.Event{
				SessionID: a.sessionID,
				Type:      session.EventTurnFailed,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventTurnFailed)
			}
			return nil, err
		}

		// 2. 循环检测：检查每次 assistant 响应后的循环
		if a.loopDetector != nil {
			// 从 session 获取当前消息历史
			messages, _ := a.td().History()
			for _, tc := range result.ToolCalls {
				loopToolCalls = append(loopToolCalls, loopToolCallFromRecord(tc))
			}

			detectResult, dErr := a.loopDetector.Check(messages, loopToolCalls)
			if dErr != nil {
				// 中断策略返回的错误，直接返回
				return nil, dErr
			}
			if detectResult.IsLoop {
				log.Printf("[循环检测] 检测到循环: %s", detectResult.Details)
				// warn 策略已记录日志，继续执行
			}
		}

		// 追加 turn ended 事件
		endData, encodeErr := session.EncodeData(&session.TurnEndedData{Usage: result.Usage})
		if encodeErr != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, a.sessionID, session.EventTurnEnded)
		} else if err := a.session.AppendEvent(session.Event{
			SessionID: a.sessionID,
			Type:      session.EventTurnEnded,
			Data:      endData,
		}); err != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", err, a.sessionID, session.EventTurnEnded)
		}

		finalMessage = result.Message
		finalUsage = result.Usage
		allToolCalls = append(allToolCalls, result.ToolCalls...)

		if !result.HasToolCalls {
			// 可观测性：发布 agent.end 事件
			a.publishEvent("agent.end", map[string]interface{}{
				"total_tokens": finalUsage.TotalTokens,
				"tool_calls":   len(allToolCalls),
			})
			return &Response{
				Message:   finalMessage,
				ToolCalls: allToolCalls,
				Usage:     finalUsage,
				SessionID: a.sessionID,
			}, nil
		}
	}

	// 可观测性：发布 agent.end 事件（步数超限）
	a.publishEvent("agent.end", map[string]interface{}{
		"error":      ErrMaxStepsExceeded.Error(),
		"tool_calls": len(allToolCalls),
	})
	return nil, ErrMaxStepsExceeded
}

// td 返回当前会话的 TurnD 实现
func (a *AgentLoop) td() TurnD {
	return (&agentInstance{agent: a}).TurnD()
}

// publishEvent 发布可观测性事件（仅在启用时）
func (a *AgentLoop) publishEvent(eventType string, data map[string]interface{}) {
	if a.eventBus != nil && a.obsEnabled {
		a.eventBus.PublishEvent(eventType, data)
	}
}

func loopToolCallFromRecord(record ToolCallRecord) LoopToolCall {
	var args map[string]interface{}
	if record.Call.ArgsJSON != "" {
		_ = json.Unmarshal([]byte(record.Call.ArgsJSON), &args)
	}
	return LoopToolCall{
		ToolName: record.Call.Name,
		Args:     args,
	}
}

// Tools 返回已注册的工具列表
func (a *AgentLoop) Tools() []tool.Tool {
	return a.cfg.registry.MaterializeAsTools()
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
