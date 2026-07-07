package benchmark

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/internal/observability"
)

// BenchmarkTokenRecordUsage 测试 TokenTracker.RecordUsage 的写入吞吐量。
// 模拟连续记录不同模型的 token 使用量。
func BenchmarkTokenRecordUsage(b *testing.B) {
	tt := observability.NewTokenTracker()
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		model := models[i%len(models)]
		input := rand.Intn(1000) + 100
		output := rand.Intn(2000) + 50
		tt.RecordUsage(model, input, output)
	}
}

// BenchmarkTokenGetTotalUsage 测试 TokenTracker.GetTotalUsage 的读取吞吐量。
// 预先填充大量记录后，测量总计查询性能。
func BenchmarkTokenGetTotalUsage(b *testing.B) {
	tt := observability.NewTokenTracker()
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"}

	// 预填充 10000 条记录
	const recordCount = 10000
	for i := 0; i < recordCount; i++ {
		model := models[i%len(models)]
		tt.RecordUsage(model, i, i*2)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tt.GetTotalUsage()
	}
}

// BenchmarkTokenGetUsageByModel 测试按模型查询 token 使用量的性能。
// 预先填充大量记录后，测量按模型过滤的查询性能。
func BenchmarkTokenGetUsageByModel(b *testing.B) {
	tt := observability.NewTokenTracker()
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"}

	// 预填充 10000 条记录
	const recordCount = 10000
	for i := 0; i < recordCount; i++ {
		model := models[i%len(models)]
		tt.RecordUsage(model, i, i*2)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tt.GetUsageByModel(models[i%len(models)])
	}
}

// BenchmarkTokenMixedWorkload 测试混合读写场景下的 TokenTracker 性能。
// 模拟真实场景：一边记录使用量，一边查询统计信息。
func BenchmarkTokenMixedWorkload(b *testing.B) {
	tt := observability.NewTokenTracker()
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"}

	var wg sync.WaitGroup

	// 写入 goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			model := models[i%len(models)]
			tt.RecordUsage(model, i, i*2)
		}
	}()

	// 读取 goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N/2; i++ {
			tt.GetTotalUsage()
			tt.GetUsageByModel(models[i%len(models)])
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()

	wg.Wait()
}

// BenchmarkTokenTrackerReset 测试 TokenTracker.Reset 的性能。
// 预先填充大量记录后，测量重置操作的耗时。
func BenchmarkTokenTrackerReset(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tt := observability.NewTokenTracker()
		for j := 0; j < 1000; j++ {
			tt.RecordUsage("gpt-4", j, j*2)
		}
		b.StartTimer()

		tt.Reset()
	}
}

// BenchmarkTokenRecordUsage_DifferentConcurrency 测试不同并发度下的记录性能。
func BenchmarkTokenRecordUsage_DifferentConcurrency(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16}

	for _, c := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", c), func(b *testing.B) {
			tt := observability.NewTokenTracker()

			var wg sync.WaitGroup
			perGoroutine := b.N / c
			if perGoroutine < 1 {
				perGoroutine = 1
			}

			b.ResetTimer()
			b.ReportAllocs()

			for g := 0; g < c; g++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for i := 0; i < perGoroutine; i++ {
						model := fmt.Sprintf("model-%d", id)
						tt.RecordUsage(model, i, i*2)
					}
				}(g)
			}
			wg.Wait()
		})
	}
}
