package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
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

	// system prompt 落盘状态：cfg.systemPrompt 变化时才写新事件。
	// 「是否已落盘」用独立布尔量而非与空串比较，否则空 prompt 会被反复写事件。
	systemPromptLogged bool
	lastSystemPrompt   string
}

// agentInstance 实现 Instance 接口
type agentInstance struct {
	agent *AgentLoop
}

func (a *agentInstance) ID() string {
	return a.agent.sessionID
}

// SessionID 实现 SessionIDProvider。
func (a *agentInstance) SessionID() string {
	return a.agent.sessionID
}

// SessionID 让 *AgentLoop 自身也满足 SessionIDProvider。
//
// New 返回的具体类型是 *AgentLoop（agentInstance 只是它的包装）。产品层的
// bindRuntimeJobOwner 用类型断言取 SessionIDProvider，若只有包装类型实现了
// 该接口，断言就会静默失败、后台任务永远拿不到归属（SHIP-003 发现的缺陷）。
func (a *AgentLoop) SessionID() string {
	return a.sessionID
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

// AuditMode 实现 AuditPolicyProvider，把配置里的审计策略传给 Pipeline。
func (d *agentTurnD) AuditMode() AuditMode {
	return d.agent.cfg.auditMode.Normalize()
}

// newAgentLoop 创建 AgentLoop，初始化 session 和 pipeline
func newAgentLoop(cfg *config, chain *hook.Chain) (*AgentLoop, error) {
	sess := cfg.session
	if sess == nil {
		sess = session.NewMemoryStore()
	}

	var info *session.Info
	var err error
	if cfg.sessionID != "" {
		info, err = sess.Get(cfg.sessionID)
	} else {
		info, err = sess.Create(session.CreateOpts{Title: "agent session"})
	}
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
	a.refreshSystemPrompt()
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
	a.refreshSystemPrompt()

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

// refreshSystemPrompt 在每轮请求开始前读取动态 prompt。配置里的静态 prompt
// 仍由 WithSystemPrompt 提供；动态提供者用于把本轮最新事实写入同一套
// system.prompt_set 事件，避免「事实已经落盘但下一请求仍使用启动快照」。
func (a *AgentLoop) refreshSystemPrompt() {
	if a.cfg != nil && a.cfg.systemPromptProvider != nil {
		a.cfg.systemPrompt = a.cfg.systemPromptProvider()
	}
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
				compacted, summary, cerr := a.runCompaction(history)
				if cerr == nil && !reflect.DeepEqual(compacted, history) {
					log.Printf("[压缩] 上下文已从 %d 条消息压缩至 %d 条，摘要 %d 字符",
						len(history), len(compacted), len(summary))

					// 写 EventCompacted。
					//
					// 摘要必须随事件落盘：请求由事件日志投影重建，摘要只留在内存里
					// 到不了模型面前，"压缩"会退化成"静默丢消息"。
					//
					// 保存压缩器的完整输出，而不是猜测它删除了多少条前缀消息。
					// 摘要替换、选择性保留和重排都无法用单个截断边界准确表达。
					snapshot := make([]llm.ChatMessage, len(compacted))
					copy(snapshot, compacted)
					compactData, err := session.EncodeData(&session.CompactedData{
						Summary:  summary,
						Snapshot: snapshot,
					})
					if err != nil {
						log.Printf("event persistence failed: %v", err)
					} else if err := a.session.AppendEvent(session.Event{
						SessionID: a.sessionID,
						Type:      session.EventCompacted,
						Data:      compactData,
					}); err != nil {
						log.Printf("event persistence failed: %v", err)
					} else {
						a.compressionApplied = true
					}
				}
			}
		}
	}

	for step := 0; step < a.cfg.maxSteps; step++ {
		// 每轮重新创建 pipeline（TurnD 会从 session 读取最新状态）
		inst := &agentInstance{agent: a}
		p := NewPipeline(inst.TurnD())

		// 把请求级上下文落成事件：它们会进入发给 provider 的请求，因此必须先入日志，
		// 否则「模型看得到、日志里没有」，事后无法解释模型为什么那样回答。
		if err := a.persistSystemPrompt(); err != nil {
			return nil, err
		}
		if err := a.persistSteering(); err != nil {
			return nil, err
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

// runCompaction 执行一次压缩，并尽可能取得摘要文本。
//
// Compactor 若实现了 CompactReporter（可选扩展），则连摘要一起返回；
// 否则摘要为空——这与引入摘要之前的行为一致，属于降级而非破坏。
func (a *AgentLoop) runCompaction(history []llm.ChatMessage) ([]llm.ChatMessage, string, error) {
	if reporter, ok := a.compactor.(CompactReporter); ok {
		report, err := reporter.CompactWithReport(history)
		if err != nil {
			return nil, "", err
		}
		return report.Messages, report.Summary, nil
	}

	compacted, err := a.compactor.Compact(history)
	if err != nil {
		return nil, "", err
	}
	return compacted, "", nil
}

// persistSystemPrompt 在 system prompt 发生变化时写入 system.prompt_set 事件。
//
// system prompt 是请求的一部分，但此前只存在于配置（cfg.systemPrompt）中，不进日志。
// 于是「模型看到的上下文」与「日志记录的历史」之间存在一段无法核对的差：同一份
// 日志换一个配置打开，就会重建出不同的请求，而且没有任何提示。
//
// 只在内容变化时写：prompt 通常一辈子只设一次，每步都写会让日志被重复事件淹没。
func (a *AgentLoop) persistSystemPrompt() error {
	prompt := a.cfg.systemPrompt
	if a.systemPromptLogged && a.lastSystemPrompt == prompt {
		return nil
	}

	// 首次使用空 prompt 也写入空值：会话可能已有更早的 system.prompt_set，
	// 空值才能明确清除它，避免接续日志时重放过期 prompt。
	data, err := session.EncodeData(&session.SystemPromptSetData{
		Content: prompt,
		Hash:    contentHash(prompt),
	})
	if err != nil {
		return fmt.Errorf("agent: encode system prompt event: %w", err)
	}
	if err := a.session.AppendEvent(session.Event{
		SessionID: a.sessionID,
		Type:      session.EventSystemPromptSet,
		Data:      data,
	}); err != nil {
		return fmt.Errorf("agent: persist system prompt event: %w", err)
	}
	a.lastSystemPrompt = prompt
	a.systemPromptLogged = true
	return nil
}

// persistSteering 把本步 Drain 到的 steering 消息写入 steered 事件。
//
// 此前 Drain() 之后消息即被丢弃：模型在那一轮看得到，日志里却没有，事后无法解释
// 模型为什么那样回答。落成事件后它成为历史的一部分，请求可以完整重建。
func (a *AgentLoop) persistSteering() error {
	if a.steeringManager == nil {
		return nil
	}
	pending := a.steeringManager.drainPending()
	if len(pending) == 0 {
		return nil
	}
	msgs := make([]string, len(pending))
	for i, msg := range pending {
		msgs[i] = msg.content
	}

	data, err := session.EncodeData(&session.SteeredData{Messages: msgs})
	if err != nil {
		a.steeringManager.restorePending(pending)
		return fmt.Errorf("agent: encode steering event: %w", err)
	}
	if err := a.session.AppendEvent(session.Event{
		SessionID: a.sessionID,
		Type:      session.EventSteered,
		Data:      data,
	}); err != nil {
		a.steeringManager.restorePending(pending)
		return fmt.Errorf("agent: persist steering event: %w", err)
	}
	return nil
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
