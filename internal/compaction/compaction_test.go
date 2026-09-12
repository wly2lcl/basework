package compaction

import (
	"context"
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// mockSummarizer 模拟摘要生成器，用于测试 SummarizationStrategy
type mockSummarizer struct {
	summary string
	err     error
}

func (m *mockSummarizer) Summarize(_ context.Context, text string) (string, error) {
	return m.summary, m.err
}

// 辅助函数：创建一条文本消息
func textMsg(role llm.Role, text string) llm.ChatMessage {
	return llm.ChatMessage{
		Role:    role,
		Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: text}},
	}
}

// 辅助函数：创建多条消息
func makeMessages(count int, role llm.Role) []llm.ChatMessage {
	msgs := make([]llm.ChatMessage, count)
	for i := 0; i < count; i++ {
		msgs[i] = textMsg(role, "message content")
	}
	return msgs
}

// ========== EstimateTokens ==========

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "空字符串返回 0",
			text:     "",
			expected: 0,
		},
		{
			name:     "4 字符 = 1 token",
			text:     "abcd",
			expected: 1,
		},
		{
			name:     "5 字符 = 2 token（向上取整）",
			text:     "abcde",
			expected: 2,
		},
		{
			name:     "1000 字符 ≈ 250 tokens",
			text:     string(make([]byte, 1000)),
			expected: 250,
		},
		{
			name:     "1 字符 ≈ 1 token",
			text:     "a",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTokens(tt.text)
			if got != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, 期望 %d", tt.text, got, tt.expected)
			}
		})
	}
}

func TestEstimateConversationTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.ChatMessage
		expected int
	}{
		{
			name:     "空对话返回 0",
			messages: nil,
			expected: 0,
		},
		{
			name: "多条消息累加 token",
			messages: []llm.ChatMessage{
				textMsg(llm.RoleUser, "abcd"),        // 1 token
				textMsg(llm.RoleAssistant, "abcdef"), // 2 tokens
			},
			expected: 3,
		},
		{
			name: "5000 字符 ≈ 1250 tokens",
			messages: []llm.ChatMessage{
				textMsg(llm.RoleUser, string(make([]byte, 2500))),
				textMsg(llm.RoleAssistant, string(make([]byte, 2500))),
			},
			expected: 1250,
		},
		{
			name: "消息包含非文本内容，文本部分仍然计入",
			messages: []llm.ChatMessage{
				{
					Role: llm.RoleUser,
					Content: []llm.ContentPart{
						{Type: llm.ContentTypeText, Text: "abcd"},
						{Type: llm.ContentTypeImage, Text: ""},
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateConversationTokens(tt.messages)
			if got != tt.expected {
				t.Errorf("EstimateConversationTokens() = %d, 期望 %d", got, tt.expected)
			}
		})
	}
}

// ========== SlidingWindowStrategy ==========

func TestSlidingWindowStrategy_Compact(t *testing.T) {
	tests := []struct {
		name       string
		windowSize int
		messages   []llm.ChatMessage
		wantLenMin int // 期望结果至少包含的消息数
		wantLenMax int // 期望结果至多包含的消息数
		checkSys   bool
	}{
		{
			name:       "消息未超过窗口大小，不压缩",
			windowSize: 10,
			messages:   makeMessages(5, llm.RoleUser),
			wantLenMin: 5,
			wantLenMax: 5,
		},
		{
			name:       "20 条消息压缩到最近 10 条",
			windowSize: 10,
			messages:   makeMessages(20, llm.RoleUser),
			wantLenMin: 10,
			wantLenMax: 10,
		},
		{
			name:       "系统消息不受窗口限制",
			windowSize: 3,
			messages: []llm.ChatMessage{
				textMsg(llm.RoleSystem, "system prompt"),
				textMsg(llm.RoleUser, "msg1"),
				textMsg(llm.RoleUser, "msg2"),
				textMsg(llm.RoleUser, "msg3"),
				textMsg(llm.RoleUser, "msg4"),
				textMsg(llm.RoleUser, "msg5"),
			},
			wantLenMin: 4, // 1 条系统 + 3 条非系统
			wantLenMax: 4,
			checkSys:   true,
		},
		{
			name:       "默认窗口大小（0 时使用 10）",
			windowSize: 0,
			messages:   makeMessages(15, llm.RoleUser),
			wantLenMin: 10,
			wantLenMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSlidingWindowStrategy(tt.windowSize)
			got, err := s.Compact(tt.messages, 0)
			if err != nil {
				t.Fatalf("Compact() 返回错误: %v", err)
			}
			if len(got.Messages) < tt.wantLenMin || len(got.Messages) > tt.wantLenMax {
				t.Errorf("Compact() 返回 %d 条消息, 期望在 [%d, %d] 范围内",
					len(got.Messages), tt.wantLenMin, tt.wantLenMax)
			}
			if tt.checkSys {
				hasSys := false
				for _, msg := range got.Messages {
					if msg.Role == llm.RoleSystem {
						hasSys = true
						break
					}
				}
				if !hasSys {
					t.Error("Compact() 结果缺少系统消息")
				}
			}
		})
	}
}

func TestSlidingWindowStrategy_Name(t *testing.T) {
	s := NewSlidingWindowStrategy(10)
	if s.Name() != "sliding_window" {
		t.Errorf("Name() = %q, 期望 %q", s.Name(), "sliding_window")
	}
}

// ========== SummarizationStrategy ==========

func TestSummarizationStrategy_Compact(t *testing.T) {
	tests := []struct {
		name       string
		messages   []llm.ChatMessage
		summary    string
		summaryErr error
		wantErr    bool
		wantMinLen int
		wantMaxLen int
		checkSys   bool
	}{
		{
			name:       "消息太少，不触发摘要",
			messages:   makeMessages(3, llm.RoleUser),
			wantMinLen: 3,
			wantMaxLen: 3,
		},
		{
			name: "20 条消息，摘要前 10 条，保留后 10 条 + 摘要 + 系统消息",
			messages: func() []llm.ChatMessage {
				msgs := make([]llm.ChatMessage, 20)
				for i := 0; i < 20; i++ {
					msgs[i] = textMsg(llm.RoleUser, "content")
				}
				return msgs
			}(),
			summary:    "这是一个摘要",
			wantMinLen: 11, // 1 摘要 + 10 保留
			wantMaxLen: 11,
		},
		{
			name: "系统消息保留，不参与摘要",
			messages: []llm.ChatMessage{
				textMsg(llm.RoleSystem, "system prompt"),
				textMsg(llm.RoleUser, "user1"),
				textMsg(llm.RoleUser, "user2"),
				textMsg(llm.RoleUser, "user3"),
				textMsg(llm.RoleUser, "user4"),
				textMsg(llm.RoleUser, "user5"),
				textMsg(llm.RoleUser, "user6"),
			},
			summary:    "摘要内容",
			wantMinLen: 5, // 1 系统 + 1 摘要 + 3 保留 (7*0.5=3.5, split=3)
			wantMaxLen: 5,
			checkSys:   true,
		},
		{
			name: "摘要生成失败时返回错误",
			messages: func() []llm.ChatMessage {
				msgs := make([]llm.ChatMessage, 10)
				for i := 0; i < 10; i++ {
					msgs[i] = textMsg(llm.RoleUser, "content")
				}
				return msgs
			}(),
			summaryErr: errors.New("LLM 调用失败"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSummarizer{summary: tt.summary, err: tt.summaryErr}
			s := NewSummarizationStrategy(mock)
			got, err := s.Compact(tt.messages, 0)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Compact() 期望错误，但没有返回")
				}
				return
			}
			if err != nil {
				t.Fatalf("Compact() 返回意外错误: %v", err)
			}
			if len(got.Messages) < tt.wantMinLen || len(got.Messages) > tt.wantMaxLen {
				t.Errorf("Compact() 返回 %d 条消息, 期望在 [%d, %d] 范围内",
					len(got.Messages), tt.wantMinLen, tt.wantMaxLen)
			}
			// 摘要必须通过 Result.Summary 回传，否则它永远到不了模型面前。
			if tt.summary != "" && got.Summary != tt.summary {
				t.Errorf("Summary = %q, 期望 %q", got.Summary, tt.summary)
			}
			if tt.checkSys {
				hasSys := false
				for _, msg := range got.Messages {
					if msg.Role == llm.RoleSystem {
						hasSys = true
						break
					}
				}
				if !hasSys {
					t.Error("Compact() 结果缺少系统消息")
				}
			}
		})
	}
}

func TestSummarizationStrategy_Name(t *testing.T) {
	s := NewSummarizationStrategy(&mockSummarizer{})
	if s.Name() != "summarization" {
		t.Errorf("Name() = %q, 期望 %q", s.Name(), "summarization")
	}
}

// ========== SelectiveStrategy ==========

func TestSelectiveStrategy_Compact(t *testing.T) {
	tests := []struct {
		name               string
		assistantKeepCount int
		messages           []llm.ChatMessage
		wantUserCount      int // 期望保留的用户消息数
		wantAssistantCount int // 期望保留的助手消息数
		wantSysCount       int // 期望保留的系统消息数
		wantLen            int // 总长度
	}{
		{
			name:               "所有用户消息保留，助手消息裁剪到 5 条",
			assistantKeepCount: 5,
			messages: func() []llm.ChatMessage {
				var msgs []llm.ChatMessage
				msgs = append(msgs, textMsg(llm.RoleSystem, "system"))
				msgs = append(msgs, textMsg(llm.RoleUser, "user1"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant1"))
				msgs = append(msgs, textMsg(llm.RoleUser, "user2"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant2"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant3"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant4"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant5"))
				msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant6"))
				msgs = append(msgs, textMsg(llm.RoleTool, "tool_result"))
				return msgs
			}(),
			wantUserCount:      2,
			wantAssistantCount: 5,
			wantSysCount:       1,
			wantLen:            9, // 1 sys + 2 user + 1 tool + 5 assistant
		},
		{
			name:               "助手消息未超过限制，全部保留",
			assistantKeepCount: 10,
			messages: []llm.ChatMessage{
				textMsg(llm.RoleUser, "user1"),
				textMsg(llm.RoleAssistant, "assistant1"),
			},
			wantUserCount:      1,
			wantAssistantCount: 1,
			wantLen:            2,
		},
		{
			name:               "默认值（keppCount <= 0 时使用 5）",
			assistantKeepCount: 0,
			messages: func() []llm.ChatMessage {
				var msgs []llm.ChatMessage
				msgs = append(msgs, textMsg(llm.RoleUser, "user1"))
				for i := 0; i < 10; i++ {
					msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant"))
				}
				return msgs
			}(),
			wantUserCount:      1,
			wantAssistantCount: 5,
			wantLen:            6, // 1 user + 5 assistant
		},
		{
			name:               "助手消息恰好等于限制，保留全部",
			assistantKeepCount: 3,
			messages: func() []llm.ChatMessage {
				var msgs []llm.ChatMessage
				msgs = append(msgs, textMsg(llm.RoleUser, "user1"))
				for i := 0; i < 3; i++ {
					msgs = append(msgs, textMsg(llm.RoleAssistant, "assistant"))
				}
				return msgs
			}(),
			wantUserCount:      1,
			wantAssistantCount: 3,
			wantLen:            4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSelectiveStrategy(tt.assistantKeepCount)
			got, err := s.Compact(tt.messages, 0)
			if err != nil {
				t.Fatalf("Compact() 返回错误: %v", err)
			}
			if len(got.Messages) != tt.wantLen {
				t.Errorf("Compact() 返回 %d 条消息, 期望 %d 条", len(got.Messages), tt.wantLen)
			}

			userCount := 0
			assistantCount := 0
			sysCount := 0
			for _, msg := range got.Messages {
				switch msg.Role {
				case llm.RoleUser:
					userCount++
				case llm.RoleAssistant:
					assistantCount++
				case llm.RoleSystem:
					sysCount++
				}
			}
			if userCount != tt.wantUserCount {
				t.Errorf("用户消息数 = %d, 期望 %d", userCount, tt.wantUserCount)
			}
			if assistantCount != tt.wantAssistantCount {
				t.Errorf("助手消息数 = %d, 期望 %d", assistantCount, tt.wantAssistantCount)
			}
			if sysCount != tt.wantSysCount {
				t.Errorf("系统消息数 = %d, 期望 %d", sysCount, tt.wantSysCount)
			}
		})
	}
}

func TestSelectiveStrategy_Name(t *testing.T) {
	s := NewSelectiveStrategy(5)
	if s.Name() != "selective" {
		t.Errorf("Name() = %q, 期望 %q", s.Name(), "selective")
	}
}

// ========== Engine.ShouldCompact ==========

func TestEngine_ShouldCompact(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		messages  []llm.ChatMessage
		maxTokens int
		expected  bool
	}{
		{
			name:      "压缩禁用时返回 false",
			config:    Config{Enabled: false, Threshold: 0.8},
			messages:  makeMessages(10, llm.RoleUser),
			maxTokens: 100,
			expected:  false,
		},
		{
			name:      "maxTokens <= 0 时返回 false",
			config:    Config{Enabled: true, Threshold: 0.8},
			messages:  makeMessages(10, llm.RoleUser),
			maxTokens: 0,
			expected:  false,
		},
		{
			name:      "token 使用率 80% 时触发压缩",
			config:    Config{Enabled: true, Threshold: 0.8},
			messages:  []llm.ChatMessage{textMsg(llm.RoleUser, string(make([]byte, 320)))}, // 80 tokens
			maxTokens: 100,
			expected:  true,
		},
		{
			name:      "token 使用率低于阈值时不触发",
			config:    Config{Enabled: true, Threshold: 0.8},
			messages:  []llm.ChatMessage{textMsg(llm.RoleUser, string(make([]byte, 160)))}, // 40 tokens
			maxTokens: 100,
			expected:  false,
		},
		{
			name:      "自定义阈值 70% 触发",
			config:    Config{Enabled: true, Threshold: 0.7},
			messages:  []llm.ChatMessage{textMsg(llm.RoleUser, string(make([]byte, 280)))}, // 70 tokens
			maxTokens: 100,
			expected:  true,
		},
		{
			name:      "自定义阈值 70% 未触发",
			config:    Config{Enabled: true, Threshold: 0.7},
			messages:  []llm.ChatMessage{textMsg(llm.RoleUser, string(make([]byte, 240)))}, // 60 tokens
			maxTokens: 100,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine(tt.config, nil)
			got := e.ShouldCompact(tt.messages, tt.maxTokens)
			if got != tt.expected {
				t.Errorf("ShouldCompact() = %v, 期望 %v", got, tt.expected)
			}
		})
	}
}

// ========== Engine.Compact ==========

func TestEngine_Compact(t *testing.T) {
	// 使用滑动窗口策略
	slidingStrategy := NewSlidingWindowStrategy(3)

	tests := []struct {
		name     string
		config   Config
		messages []llm.ChatMessage
		wantErr  bool
		checkFn  func(t *testing.T, result []llm.ChatMessage)
	}{
		{
			name:   "压缩禁用时返回原消息",
			config: Config{Enabled: false},
			messages: []llm.ChatMessage{
				textMsg(llm.RoleSystem, "system"),
				textMsg(llm.RoleUser, "user1"),
				textMsg(llm.RoleUser, "user2"),
				textMsg(llm.RoleUser, "user3"),
				textMsg(llm.RoleUser, "user4"),
			},
			checkFn: func(t *testing.T, result []llm.ChatMessage) {
				if len(result) != 5 {
					t.Errorf("Compact() 返回 %d 条消息, 期望 5 条", len(result))
				}
			},
		},
		{
			name:   "启用压缩后执行策略压缩",
			config: Config{Enabled: true, Threshold: 0.8},
			messages: []llm.ChatMessage{
				textMsg(llm.RoleUser, "msg1"),
				textMsg(llm.RoleUser, "msg2"),
				textMsg(llm.RoleUser, "msg3"),
				textMsg(llm.RoleUser, "msg4"),
				textMsg(llm.RoleUser, "msg5"),
			},
			checkFn: func(t *testing.T, result []llm.ChatMessage) {
				// 滑动窗口 3，5 条消息压缩到 3 条
				if len(result) != 3 {
					t.Errorf("Compact() 返回 %d 条消息, 期望 3 条", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine(tt.config, slidingStrategy)
			got, err := e.Compact(tt.messages)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Compact() 期望错误，但没有返回")
				}
				return
			}
			if err != nil {
				t.Fatalf("Compact() 返回意外错误: %v", err)
			}
			tt.checkFn(t, got)
		})
	}
}

// ========== NewEngine 默认值 ==========

func TestNewEngine_Defaults(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		wantThreshold  float64
		wantWindowSize int
	}{
		{
			name:           "Threshold <= 0 时使用默认值 0.8",
			config:         Config{Threshold: 0, WindowSize: 10},
			wantThreshold:  0.8,
			wantWindowSize: 10,
		},
		{
			name:           "Threshold > 1.0 时使用默认值 0.8",
			config:         Config{Threshold: 1.5, WindowSize: 10},
			wantThreshold:  0.8,
			wantWindowSize: 10,
		},
		{
			name:           "WindowSize <= 0 时使用默认值 10",
			config:         Config{Threshold: 0.8, WindowSize: 0},
			wantThreshold:  0.8,
			wantWindowSize: 10,
		},
		{
			name:           "自定义值有效时保留原值",
			config:         Config{Threshold: 0.5, WindowSize: 20},
			wantThreshold:  0.5,
			wantWindowSize: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine(tt.config, nil)
			if e.config.Threshold != tt.wantThreshold {
				t.Errorf("Threshold = %f, 期望 %f", e.config.Threshold, tt.wantThreshold)
			}
			if e.config.WindowSize != tt.wantWindowSize {
				t.Errorf("WindowSize = %d, 期望 %d", e.config.WindowSize, tt.wantWindowSize)
			}
		})
	}
}
