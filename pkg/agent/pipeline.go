package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// TurnResult 是单次 turn 的执行结果
type TurnResult struct {
	HasToolCalls bool
	Message      llm.ChatMessage
	ToolCalls    []ToolCallRecord
	Usage        llm.Usage
}

// Pipeline 实现了四阶段 turn 执行管线：setup → callLLM → executeTools → finalize
type Pipeline struct {
	td           TurnD
	sessionID    string
	steeringMsgs []llm.ChatMessage // 从 SteeringManager.Drain() 获取的消息，在 setupTurn 中 prepend
}

// NewPipeline 创建管线
func NewPipeline(td TurnD) *Pipeline {
	return &Pipeline{td: td, sessionID: td.SessionID()}
}

// Run 执行完整的 turn 管线
func (p *Pipeline) Run(ctx context.Context) (*TurnResult, error) {
	messages, tools, err := p.setupTurn(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := p.callLLM(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	results, err := p.executeTools(ctx, resp.Message.ToolCalls)
	if err != nil {
		return nil, err
	}

	return p.finalize(ctx, resp, results)
}

// setupTurn 准备 LLM 请求所需的 messages 和 tools
func (p *Pipeline) setupTurn(_ context.Context) ([]llm.ChatMessage, []llm.ToolDefinition, error) {
	// 1. 获取历史消息
	msgs, err := p.td.History()
	if err != nil {
		return nil, nil, err
	}

	// 2. 如果有 system prompt，prepend 或替换
	if prompt := p.td.SystemPrompt(); prompt != "" {
		sysMsg := llm.ChatMessage{
			Role:    llm.RoleSystem,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: prompt}},
		}
		// 如果已有 system 消息则替换，否则 prepend
		inserted := false
		for i, msg := range msgs {
			if msg.Role == llm.RoleSystem {
				msgs[i] = sysMsg
				inserted = true
				break
			}
		}
		if !inserted {
			msgs = append([]llm.ChatMessage{sysMsg}, msgs...)
		}
	}

	// 3. prepend steering 消息（以 system 角色出现）到 system prompt 之后、历史消息之前
	if len(p.steeringMsgs) > 0 {
		// 在第一条非 system 消息之前插入 steering 消息，如果全是 system 则追加到最后
		insertPos := 0
		for i, msg := range msgs {
			if msg.Role != llm.RoleSystem {
				break
			}
			insertPos = i + 1
		}
		// 在 insertPos 处插入 steering 消息
		msgs = append(msgs[:insertPos], append(p.steeringMsgs, msgs[insertPos:]...)...)
		p.steeringMsgs = nil
	}

	// 4. 获取工具定义
	tools := p.td.ToolRegistry().Materialize()

	return msgs, tools, nil
}

// callLLM 调用 LLM stream 并消费，累积 assistant message 和 tool calls
func (p *Pipeline) callLLM(ctx context.Context, messages []llm.ChatMessage, tools []llm.ToolDefinition) (*llm.Response, error) {
	// 1. 执行 BeforeLLM hooks
	msgs, err := p.td.Hooks().RunBeforeLLM(messages)
	if err != nil {
		p.td.Hooks().RunAfterLLM(nil, err)
		return nil, err
	}

	// 2. 调用 stream
	ch, err := p.td.Model().Stream(ctx, &llm.Request{Messages: msgs, Tools: tools})
	if err != nil {
		p.td.Hooks().RunAfterLLM(nil, err)
		return nil, err
	}

	// 3. 消费 stream
	var textContent string
	type accToolCall struct {
		id       string
		name     string
		argsJSON string
	}
	toolCallMap := make(map[int]*accToolCall)
	var toolCalls []llm.ToolCall
	var usage llm.Usage

	for evt := range ch {
		if evt.Error != nil {
			return nil, fmt.Errorf("stream error: %w", evt.Error)
		}
		switch evt.Type {
		case llm.StreamEventText:
			textContent += evt.Delta
			if cb := p.td.Callback(); cb != nil {
				cb.OnTextDelta(evt.Delta)
			}
		case llm.StreamEventToolCall:
			if evt.ToolCall == nil {
				continue
			}
			tc, exists := toolCallMap[evt.ToolCall.Index]
			if !exists {
				tc = &accToolCall{}
				toolCallMap[evt.ToolCall.Index] = tc
			}
			if evt.ToolCall.ID != "" {
				tc.id = evt.ToolCall.ID
			}
			if evt.ToolCall.Name != "" {
				tc.name = evt.ToolCall.Name
			}
			tc.argsJSON += evt.ToolCall.ArgsJSON
			if evt.ToolCall.Complete {
				fullCall := llm.ToolCall{
					ID:       tc.id,
					Name:     tc.name,
					ArgsJSON: tc.argsJSON,
				}
				toolCalls = append(toolCalls, fullCall)
				delete(toolCallMap, evt.ToolCall.Index)
				if cb := p.td.Callback(); cb != nil {
					cb.OnToolCallStart(fullCall)
				}
			}
		case llm.StreamEventUsage:
			if evt.Usage != nil {
				usage = *evt.Usage
			}
		case llm.StreamEventDone:
			// stream 结束
		}
	}

	// 检查 stream 结束后是否因上下文取消而退出
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 刷新未完成的 tool call：stream 结束时将 toolCallMap 中残留的 tool call 标记为完成
	for _, tc := range toolCallMap {
		fullCall := llm.ToolCall{
			ID:       tc.id,
			Name:     tc.name,
			ArgsJSON: tc.argsJSON,
		}
		toolCalls = append(toolCalls, fullCall)
	}

	// 构建 assistant message
	content := []llm.ContentPart{}
	if textContent != "" {
		content = append(content, llm.ContentPart{Type: llm.ContentTypeText, Text: textContent})
	}
	assistantMsg := llm.ChatMessage{
		Role:      llm.RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	}

	resp := &llm.Response{
		Message: assistantMsg,
		Usage:   usage,
	}

	// 4. 执行 AfterLLM hooks
	p.td.Hooks().RunAfterLLM(resp, nil)

	// 5. 将 assistant message 存储为 session 事件
	store := p.td.Session()

	if textContent != "" {
		deltaData, encodeErr := session.EncodeData(&session.TextDeltaData{Delta: textContent})
		if encodeErr != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventTextDelta)
		} else if err := store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventTextDelta,
			Data:      deltaData,
		}); err != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventTextDelta)
		}
	}

	for _, tc := range toolCalls {
		callData, encodeErr := session.EncodeData(&session.ToolCalledData{ToolCall: tc})
		if encodeErr != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventToolCalled)
		} else if err := store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventToolCalled,
			Data:      callData,
		}); err != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventToolCalled)
		}
	}

	// TextEnded 标记 assistant 消息结束
	if textContent != "" || len(toolCalls) > 0 {
		if err := store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventTextEnded,
		}); err != nil {
			log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventTextEnded)
		}
	}

	return resp, nil
}

// executeTools 执行所有工具调用，单工具失败不影响其他
func (p *Pipeline) executeTools(ctx context.Context, calls []llm.ToolCall) ([]ToolCallRecord, error) {
	store := p.td.Session()
	records := make([]ToolCallRecord, 0, len(calls))

	// 通过类型断言获取 agent 的权限检查器和事件总线
	var permChecker PermissionChecker
	var eventBus EventPublisher
	obsEnabled := false
	if atd, ok := p.td.(*agentTurnD); ok {
		permChecker = atd.agent.permChecker
		eventBus = atd.agent.eventBus
		obsEnabled = atd.agent.obsEnabled
	}

	for _, call := range calls {
		// 1. 权限检查
		if permChecker != nil {
			var args map[string]interface{}
			if call.ArgsJSON != "" {
				_ = json.Unmarshal([]byte(call.ArgsJSON), &args)
			}
			allowed, pErr := permChecker.Check(ctx, call.Name, args)
			if pErr != nil {
				log.Printf("[权限] 检查失败: %v", pErr)
			} else if !allowed {
				errMsg := "权限拒绝: 工具 " + call.Name + " 未被允许执行"
				records = append(records, ToolCallRecord{Call: call, Err: errors.New(errMsg)})
				failData, encodeErr := session.EncodeData(&session.ToolFailedData{
					ToolCallID: call.ID,
					Error:      errMsg,
				})
				if encodeErr != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventToolFailed)
				} else if err := store.AppendEvent(session.Event{
					SessionID: p.sessionID,
					Type:      session.EventToolFailed,
					Data:      failData,
				}); err != nil {
					log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventToolFailed)
				}
				continue
			}
		}

		// 2. BeforeTool hook
		modifiedCall, err := p.td.Hooks().RunBeforeTool(call)
		if err != nil {
			records = append(records, ToolCallRecord{Call: call, Err: err})
			// 追加失败事件
			failData, encodeErr := session.EncodeData(&session.ToolFailedData{
				ToolCallID: call.ID,
				Error:      err.Error(),
			})
			if encodeErr != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventToolFailed)
			} else if err := store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolFailed,
				Data:      failData,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventToolFailed)
			}
			continue
		}
		if modifiedCall != nil {
			call = *modifiedCall
		}

		// 可观测性：发布 tool.start 事件
		if obsEnabled && eventBus != nil {
			eventBus.PublishEvent("tool.start", map[string]interface{}{
				"tool_name": call.Name,
				"tool_id":   call.ID,
			})
		}

		// 3. 执行工具
		result, execErr := p.td.ToolRegistry().Settle(ctx, call)

		// 可观测性：发布 tool.end 事件
		if obsEnabled && eventBus != nil {
			data := map[string]interface{}{
				"tool_name": call.Name,
				"tool_id":   call.ID,
			}
			if execErr != nil {
				data["error"] = execErr.Error()
			}
			eventBus.PublishEvent("tool.end", data)
		}

		// 4. AfterTool hook
		p.td.Hooks().RunAfterTool(call, result, execErr)

		// 5. 回调通知
		if cb := p.td.Callback(); cb != nil {
			cb.OnToolCallEnd(call, result, execErr)
		}

		// 6. 追加事件到 session
		if execErr != nil {
			failData, encodeErr := session.EncodeData(&session.ToolFailedData{
				ToolCallID: call.ID,
				Error:      execErr.Error(),
			})
			if encodeErr != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventToolFailed)
			} else if err := store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolFailed,
				Data:      failData,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventToolFailed)
			}
		} else {
			content := ""
			if result != nil {
				content = result.Content
			}
			succData, encodeErr := session.EncodeData(&session.ToolSuccessData{
				ToolCallID: call.ID,
				Content:    content,
			})
			if encodeErr != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", encodeErr, p.sessionID, session.EventToolSuccess)
			} else if err := store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolSuccess,
				Data:      succData,
			}); err != nil {
				log.Printf("event persistence failed: %v (session=%s, type=%s)", err, p.sessionID, session.EventToolSuccess)
			}
		}

		records = append(records, ToolCallRecord{
			Call:   call,
			Result: result,
			Err:    execErr,
		})
	}

	return records, nil
}

// finalize 构造 TurnResult
func (p *Pipeline) finalize(_ context.Context, resp *llm.Response, records []ToolCallRecord) (*TurnResult, error) {
	return &TurnResult{
		HasToolCalls: len(records) > 0,
		Message:      resp.Message,
		ToolCalls:    records,
		Usage:        resp.Usage,
	}, nil
}
