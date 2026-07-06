package benchmark

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ---------------------------------------------------------------------------
// 模拟流式模型
// ---------------------------------------------------------------------------

// mockStreamModel 实现 llm.Model 接口，模拟流式响应。
type mockStreamModel struct {
	id            string
	tokenCount    int
	tokenInterval time.Duration // 每个 token 的间隔时间
	ttfb          time.Duration // 首 token 延迟
}

func (m *mockStreamModel) ID() string { return m.id }

func (m *mockStreamModel) Generate(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	return &llm.Response{
		Message: llm.ChatMessage{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "模拟响应"}},
		},
		Usage: llm.Usage{
			PromptTokens:     10,
			CompletionTokens: m.tokenCount,
			TotalTokens:      10 + m.tokenCount,
		},
	}, nil
}

func (m *mockStreamModel) Stream(_ context.Context, _ *llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, m.tokenCount+2)

	// 模拟首 token 延迟
	if m.ttfb > 0 {
		time.Sleep(m.ttfb)
	}

	// 发送文本事件
	for i := 0; i < m.tokenCount; i++ {
		if m.tokenInterval > 0 {
			time.Sleep(m.tokenInterval)
		}
		ch <- llm.StreamEvent{
			Type:  llm.StreamEventText,
			Delta: fmt.Sprintf("token-%d ", i),
		}
	}

	// 发送 usage 和 done 事件
	ch <- llm.StreamEvent{
		Type: llm.StreamEventUsage,
		Usage: &llm.Usage{
			PromptTokens:     10,
			CompletionTokens: m.tokenCount,
			TotalTokens:      10 + m.tokenCount,
		},
	}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)

	return ch, nil
}

func (m *mockStreamModel) Supports(cap llm.Capability) bool {
	return cap == llm.CapStreaming
}

// BenchmarkStreamingLatency_TTFT 测量首 token 到达时间（TTFT）。
// 使用零间隔的 mock 模型模拟流式响应。
func BenchmarkStreamingLatency_TTFT(b *testing.B) {
	tokenCounts := []int{10, 50, 100, 500}

	for _, tc := range tokenCounts {
		b.Run(fmt.Sprintf("Tokens_%d", tc), func(b *testing.B) {
			model := &mockStreamModel{
				id:            "mock-stream",
				tokenCount:    tc,
				tokenInterval: 0,
				ttfb:          0,
			}

			start := time.Now()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				ch, err := model.Stream(context.Background(), &llm.Request{
					Messages: []llm.ChatMessage{
						{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "测试提示"}}},
					},
				})
				if err != nil {
					b.Fatalf("Stream 失败: %v", err)
				}

				// 消费所有事件
				for ev := range ch {
					if ev.Type == llm.StreamEventDone {
						break
					}
				}
			}

			b.ReportMetric(float64(time.Since(start).Microseconds())/float64(b.N), "ns/op")
		})
	}
}

// BenchmarkStreamingThroughput 测量流式响应吞吐量。
// 模拟不同 token 数量下的流式传输性能。
func BenchmarkStreamingThroughput(b *testing.B) {
	sizes := []struct {
		name       string
		tokenCount int
	}{
		{"Small_10", 10},
		{"Medium_100", 100},
		{"Large_1000", 1000},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			model := &mockStreamModel{
				id:            "mock-stream",
				tokenCount:    size.tokenCount,
				tokenInterval: 0,
				ttfb:          0,
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(size.tokenCount))

			for i := 0; i < b.N; i++ {
				ch, err := model.Stream(context.Background(), &llm.Request{})
				if err != nil {
					b.Fatalf("Stream 失败: %v", err)
				}
				for ev := range ch {
					if ev.Type == llm.StreamEventDone {
						break
					}
				}
			}
		})
	}
}

// BenchmarkStreamingOverhead 对比有/无流式的开销差异。
// 分别测量 Generate（非流式）和 Stream（流式）的性能。
func BenchmarkStreamingOverhead(b *testing.B) {
	tokenCount := 100
	model := &mockStreamModel{
		id:            "mock",
		tokenCount:    tokenCount,
		tokenInterval: 0,
		ttfb:          0,
	}

	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "测试"}}},
		},
	}

	// 非流式
	b.Run("Generate_NonStreaming", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := model.Generate(context.Background(), req)
			if err != nil {
				b.Fatalf("Generate 失败: %v", err)
			}
		}
	})

	// 流式
	b.Run("Stream", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ch, err := model.Stream(context.Background(), req)
			if err != nil {
				b.Fatalf("Stream 失败: %v", err)
			}
			for ev := range ch {
				if ev.Type == llm.StreamEventDone {
					break
				}
			}
		}
	})
}

// BenchmarkStreamingConcurrent 测试并发流式传输性能。
// 多个 goroutine 同时进行流式请求。
func BenchmarkStreamingConcurrent(b *testing.B) {
	concurrencyLevels := []int{1, 4, 8, 16}

	for _, c := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", c), func(b *testing.B) {
			var wg sync.WaitGroup

			b.ResetTimer()
			b.ReportAllocs()

			for g := 0; g < c; g++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					model := &mockStreamModel{
						id:            fmt.Sprintf("mock-%d", id),
						tokenCount:    50,
						tokenInterval: 0,
						ttfb:          0,
					}
					for i := 0; i < b.N/c; i++ {
						ch, err := model.Stream(context.Background(), &llm.Request{})
						if err != nil {
							b.Errorf("Stream 失败: %v", err)
							return
						}
						for ev := range ch {
							if ev.Type == llm.StreamEventDone {
								break
							}
						}
					}
				}(g)
			}
			wg.Wait()
		})
	}
}

// BenchmarkStreamEventChannel 测试 StreamEvent channel 的传输开销。
// 模拟大量事件通过 channel 传递的性能。
func BenchmarkStreamEventChannel(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Events_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				ch := make(chan llm.StreamEvent, size+1)

				// 发送
				for j := 0; j < size; j++ {
					ch <- llm.StreamEvent{
						Type:  llm.StreamEventText,
						Delta: "x",
					}
				}
				ch <- llm.StreamEvent{Type: llm.StreamEventDone}
				close(ch)

				// 接收
				for ev := range ch {
					_ = ev
				}
			}
		})
	}
}

// ensure mockStreamModel implements llm.Model
var _ llm.Model = (*mockStreamModel)(nil)