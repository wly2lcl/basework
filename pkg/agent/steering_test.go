package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// captureModel 记录每次 Stream 调用时的请求消息
type captureModel struct {
	mockModel
	lastMessages []llm.ChatMessage
}

func (m *captureModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	m.lastMessages = make([]llm.ChatMessage, len(req.Messages))
	copy(m.lastMessages, req.Messages)
	return m.mockModel.Stream(ctx, req)
}

func TestSteering_InjectSteer(t *testing.T) {
	sm := NewSteeringManager()

	err := sm.InjectMessage(context.Background(), "steer消息", Steer)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	msgs := sm.Drain()
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息, 得到 %d", len(msgs))
	}
	if msgs[0] != "steer消息" {
		t.Errorf("期望消息内容='steer消息', 得到 '%s'", msgs[0])
	}
}

func TestSteering_InjectQueue(t *testing.T) {
	sm := NewSteeringManager()

	err := sm.InjectMessage(context.Background(), "queue消息", Queue)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	msgs := sm.Drain()
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息, 得到 %d", len(msgs))
	}
	if msgs[0] != "queue消息" {
		t.Errorf("期望消息内容='queue消息', 得到 '%s'", msgs[0])
	}
}

func TestSteering_MultipleMessages(t *testing.T) {
	sm := NewSteeringManager()

	_ = sm.InjectMessage(context.Background(), "msg1", Steer)
	_ = sm.InjectMessage(context.Background(), "msg2", Queue)
	_ = sm.InjectMessage(context.Background(), "msg3", Steer)

	msgs := sm.Drain()
	if len(msgs) != 3 {
		t.Fatalf("期望 3 条消息, 得到 %d", len(msgs))
	}
}

func TestSteering_EmptyDrain(t *testing.T) {
	sm := NewSteeringManager()

	msgs := sm.Drain()
	if len(msgs) != 0 {
		t.Errorf("未注入消息时 drainSteering 应返回空切片, 得到 %d", len(msgs))
	}
}

func TestSteering_CancelSequence(t *testing.T) {
	sm := NewSteeringManager()

	// 注入第一条消息
	err := sm.InjectMessage(context.Background(), "msg1", Steer)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	// cancel 当前 seq
	sm.mu.Lock()
	seq := sm.seqGen
	sm.mu.Unlock()
	sm.Cancel(seq)

	// drain 应该没有消息（被 cancel 了）
	msgs := sm.Drain()
	if len(msgs) != 0 {
		t.Errorf("cancel 后 drainSteering 应返回空, 得到 %d 条消息", len(msgs))
	}
}

func TestSteering_CancelAfterDrainAndPersistenceFailure(t *testing.T) {
	sm := NewSteeringManager()
	if err := sm.InjectMessage(context.Background(), "cancel me", Queue); err != nil {
		t.Fatal(err)
	}

	drained := sm.drainPending()
	if len(drained) != 1 {
		t.Fatalf("drainPending() returned %d messages, want 1", len(drained))
	}
	sm.Cancel(drained[0].seq)
	sm.restorePending(drained)

	if got := sm.Drain(); len(got) != 0 {
		t.Fatalf("canceled message reappeared after persistence retry: %#v", got)
	}
}

func TestSteering_ConcurrentInjection(t *testing.T) {
	sm := NewSteeringManager()
	var wg sync.WaitGroup

	// 并发注入 20 条消息
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content := string(rune('A' + i))
			mode := Steer
			if i%2 == 0 {
				mode = Queue
			}
			_ = sm.InjectMessage(context.Background(), content, mode)
		}(i)
	}
	wg.Wait()

	msgs := sm.Drain()
	// 只测试不会 panic/data race，不验证具体消息数量
	_ = msgs
}

func TestSteering_InjectSteerThenDrainIsolation(t *testing.T) {
	sm := NewSteeringManager()

	// Steer 模式应该立即在 drain 中可见
	_ = sm.InjectMessage(context.Background(), "steer1", Steer)
	_ = sm.InjectMessage(context.Background(), "queue1", Queue)

	msgs := sm.Drain()
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息, 得到 %d", len(msgs))
	}
}

func TestSteeringIntegration(t *testing.T) {
	sm := NewSteeringManager()

	model := &captureModel{}
	model.mockModel.responses = []llm.Response{
		{
			Message: llm.ChatMessage{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "好的"}},
			},
			Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}

	a, err := New(
		WithModel(model),
		WithSteeringManager(sm),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	// 注入 steering 消息
	err = sm.InjectMessage(context.Background(), "请使用中文回答", Queue)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	// 发送用户消息
	resp, err := a.HandleMessage(nil, "你好")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}
	if resp == nil {
		t.Fatal("期望非空 response")
	}

	// 验证 steering 消息出现在 LLM 请求中
	if len(model.lastMessages) == 0 {
		t.Fatal("期望 LLM 被调用, 但 lastMessages 为空")
	}

	found := false
	for _, msg := range model.lastMessages {
		if msg.Role == llm.RoleSystem {
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText && part.Text == "请使用中文回答" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("期望 LLM 请求中包含 steering 消息 '请使用中文回答' 作为 system 消息")
	}

	// 验证 steering 消息出现在 system prompt 之前（prepend 位置）
	// system prompt 空时，steering 消息应为第一条 system 消息
	var sysRoles []string
	for _, msg := range model.lastMessages {
		if msg.Role == llm.RoleSystem {
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					sysRoles = append(sysRoles, part.Text)
				}
			}
		}
	}
	if len(sysRoles) == 0 {
		t.Fatal("期望至少有一条 system 消息")
	}
	if sysRoles[0] != "请使用中文回答" {
		t.Errorf("期望第一条 system 消息为 '请使用中文回答', 得到 '%s'", sysRoles[0])
	}
}

func TestSteeringIntegration_MultipleMessages(t *testing.T) {
	sm := NewSteeringManager()

	model := &captureModel{}
	model.mockModel.responses = []llm.Response{
		{
			Message: llm.ChatMessage{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "好的"}},
			},
			Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}

	a, err := New(
		WithModel(model),
		WithSteeringManager(sm),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	// 注入多条 steering 消息
	_ = sm.InjectMessage(context.Background(), "第一条", Queue)
	_ = sm.InjectMessage(context.Background(), "第二条", Steer)

	_, err = a.HandleMessage(nil, "测试")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	systemTexts := make(map[string]bool)
	for _, msg := range model.lastMessages {
		if msg.Role == llm.RoleSystem {
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					systemTexts[part.Text] = true
				}
			}
		}
	}
	if !systemTexts["第一条"] {
		t.Error("期望 LLM 请求中包含 steering 消息 '第一条'")
	}
	if !systemTexts["第二条"] {
		t.Error("期望 LLM 请求中包含 steering 消息 '第二条'")
	}
}

func TestSteeringIntegration_WithSystemPrompt(t *testing.T) {
	sm := NewSteeringManager()

	model := &captureModel{}
	model.mockModel.responses = []llm.Response{
		{
			Message: llm.ChatMessage{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "好的"}},
			},
			Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}

	a, err := New(
		WithModel(model),
		WithSteeringManager(sm),
		WithSystemPrompt("你是助手"),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_ = sm.InjectMessage(context.Background(), "steer指令", Queue)

	_, err = a.HandleMessage(nil, "测试")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	// 验证 system prompt 和 steering 消息都在请求中
	systemTexts := make(map[string]bool)
	for _, msg := range model.lastMessages {
		if msg.Role == llm.RoleSystem {
			for _, part := range msg.Content {
				if part.Type == llm.ContentTypeText {
					systemTexts[part.Text] = true
				}
			}
		}
	}
	if !systemTexts["你是助手"] {
		t.Error("期望 LLM 请求中包含 system prompt '你是助手'")
	}
	if !systemTexts["steer指令"] {
		t.Error("期望 LLM 请求中包含 steering 消息 'steer指令'")
	}
}
