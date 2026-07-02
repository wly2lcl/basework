package agent

import (
	"context"

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
	td        TurnD
	sessionID string
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

	// 2. 如果有 system prompt，prepend
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

	// 3. 获取工具定义
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
		deltaData, _ := session.EncodeData(&session.TextDeltaData{Delta: textContent})
		_ = store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventTextDelta,
			Data:      deltaData,
		})
	}

	for _, tc := range toolCalls {
		callData, _ := session.EncodeData(&session.ToolCalledData{ToolCall: tc})
		_ = store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventToolCalled,
			Data:      callData,
		})
	}

	// TextEnded 标记 assistant 消息结束
	if textContent != "" || len(toolCalls) > 0 {
		_ = store.AppendEvent(session.Event{
			SessionID: p.sessionID,
			Type:      session.EventTextEnded,
		})
	}

	return resp, nil
}

// executeTools 执行所有工具调用，单工具失败不影响其他
func (p *Pipeline) executeTools(ctx context.Context, calls []llm.ToolCall) ([]ToolCallRecord, error) {
	store := p.td.Session()
	records := make([]ToolCallRecord, 0, len(calls))

	for _, call := range calls {
		// 1. BeforeTool hook
		modifiedCall, err := p.td.Hooks().RunBeforeTool(call)
		if err != nil {
			records = append(records, ToolCallRecord{Call: call, Err: err})
			// 追加失败事件
			failData, _ := session.EncodeData(&session.ToolFailedData{
				ToolCallID: call.ID,
				Error:      err.Error(),
			})
			_ = store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolFailed,
				Data:      failData,
			})
			continue
		}
		if modifiedCall != nil {
			call = *modifiedCall
		}

		// 2. 执行工具
		result, execErr := p.td.ToolRegistry().Settle(ctx, call)

		// 3. AfterTool hook
		p.td.Hooks().RunAfterTool(call, result, execErr)

		// 4. 回调通知
		if cb := p.td.Callback(); cb != nil {
			cb.OnToolCallEnd(call, result, execErr)
		}

		// 5. 追加事件到 session
		if execErr != nil {
			failData, _ := session.EncodeData(&session.ToolFailedData{
				ToolCallID: call.ID,
				Error:      execErr.Error(),
			})
			_ = store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolFailed,
				Data:      failData,
			})
		} else {
			content := ""
			if result != nil {
				content = result.Content
			}
			succData, _ := session.EncodeData(&session.ToolSuccessData{
				ToolCallID: call.ID,
				Content:    content,
			})
			_ = store.AppendEvent(session.Event{
				SessionID: p.sessionID,
				Type:      session.EventToolSuccess,
				Data:      succData,
			})
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
	hasToolCalls := false
	for _, r := range records {
		if r.Err == nil {
			hasToolCalls = true
			break
		}
	}

	return &TurnResult{
		HasToolCalls: hasToolCalls,
		Message:      resp.Message,
		ToolCalls:    records,
		Usage:        resp.Usage,
	}, nil
}