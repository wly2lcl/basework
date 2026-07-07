//go:build sqlite

package session

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// BenchmarkWALMode_Write — WAL 模式写入性能基准
// ---------------------------------------------------------------------------

// BenchmarkWALMode_Write 测量 WAL 模式下 SQLiteStore 的顺序写入性能。
// 每次迭代创建新会话，写入 100 个事件。
func BenchmarkWALMode_Write(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), "bench_write.db")
		s, err := NewSQLiteStore(dbPath)
		if err != nil {
			b.Fatalf("NewSQLiteStore 失败: %v", err)
		}

		info, err := s.Create(CreateOpts{Title: "基准测试"})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}
		b.StartTimer()

		const eventsPerIteration = 100
		for j := 0; j < eventsPerIteration; j++ {
			err := s.AppendEvent(Event{
				SessionID: info.ID,
				Type:      EventPrompted,
				Data:      []byte(`{"content":"benchmark message for WAL mode write performance testing"}`),
			})
			if err != nil {
				b.Fatalf("AppendEvent 失败: %v", err)
			}
		}

		b.StopTimer()
		s.Close()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkWALMode_ReadWrite — WAL 模式并发读写性能基准
// ---------------------------------------------------------------------------

// BenchmarkWALMode_ReadWrite 测量 WAL 模式下并发读写性能。
// 启动多个写入 goroutine 和读取 goroutine 同时操作。
func BenchmarkWALMode_ReadWrite(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench_rw.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(CreateOpts{Title: "并发读写基准"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	const writerCount = 4
	const readerCount = 4

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		errCh := make(chan error, writerCount+readerCount)

		// 写入 goroutines
		for w := 0; w < writerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.AppendEvent(Event{
					SessionID: info.ID,
					Type:      EventTextDelta,
					Data:      []byte(`{"delta":"concurrent write"}`),
				})
				if err != nil {
					errCh <- err
				}
			}()
		}

		// 读取 goroutines
		for r := 0; r < readerCount; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := s.List(ListFilter{Limit: 10})
				if err != nil {
					errCh <- err
				}
			}()
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				b.Errorf("并发操作失败: %v", err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSnappy_Compress — Snappy 压缩速度基准
// ---------------------------------------------------------------------------

// BenchmarkSnappy_Compress 测量 Snappy 压缩器的压缩性能。
func BenchmarkSnappy_Compress(b *testing.B) {
	compressor := &SnappyCompressor{}

	// 准备测试数据：10KB 的 JSON 数据
	dataSize := 10 * 1024
	testData := makeTestData(b, dataSize)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compressor.Compress(testData)
		if err != nil {
			b.Fatalf("Compress 失败: %v", err)
		}
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(testData)))
}

// ---------------------------------------------------------------------------
// BenchmarkSnappy_Decompress — Snappy 解压速度基准
// ---------------------------------------------------------------------------

// BenchmarkSnappy_Decompress 测量 Snappy 压缩器的解压性能。
func BenchmarkSnappy_Decompress(b *testing.B) {
	compressor := &SnappyCompressor{}

	dataSize := 10 * 1024
	testData := makeTestData(b, dataSize)

	// 预压缩
	compressed, err := compressor.Compress(testData)
	if err != nil {
		b.Fatalf("预压缩失败: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compressor.Decompress(compressed)
		if err != nil {
			b.Fatalf("Decompress 失败: %v", err)
		}
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(testData)))
}

// ---------------------------------------------------------------------------
// BenchmarkGzip_Compress — Gzip 压缩速度基准（对比）
// ---------------------------------------------------------------------------

// BenchmarkGzip_Compress 测量 Gzip 压缩器的压缩性能，与 Snappy 对比。
func BenchmarkGzip_Compress(b *testing.B) {
	compressor := &GzipCompressor{}

	dataSize := 10 * 1024
	testData := makeTestData(b, dataSize)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compressor.Compress(testData)
		if err != nil {
			b.Fatalf("Gzip Compress 失败: %v", err)
		}
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(testData)))
}

// ---------------------------------------------------------------------------
// BenchmarkGzip_Decompress — Gzip 解压速度基准（对比）
// ---------------------------------------------------------------------------

// BenchmarkGzip_Decompress 测量 Gzip 压缩器的解压性能，与 Snappy 对比。
func BenchmarkGzip_Decompress(b *testing.B) {
	compressor := &GzipCompressor{}

	dataSize := 10 * 1024
	testData := makeTestData(b, dataSize)

	// 预压缩
	compressed, err := compressor.Compress(testData)
	if err != nil {
		b.Fatalf("预压缩失败: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := compressor.Decompress(compressed)
		if err != nil {
			b.Fatalf("Gzip Decompress 失败: %v", err)
		}
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(testData)))
}

// ---------------------------------------------------------------------------
// BenchmarkCompression_LargeMessage — 大消息压缩性能基准
// ---------------------------------------------------------------------------

// BenchmarkCompression_LargeMessage 测量大消息（100KB）的压缩性能。
// 对比 Snappy 和 Gzip 在不同数据大小下的表现。
func BenchmarkCompression_LargeMessage(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1 * 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, size := range sizes {
		testData := makeTestData(b, size.size)

		// Snappy 压缩
		b.Run(fmt.Sprintf("Snappy_Compress_%s", size.name), func(b *testing.B) {
			compressor := &SnappyCompressor{}
			b.SetBytes(int64(len(testData)))
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := compressor.Compress(testData)
				if err != nil {
					b.Fatalf("Compress 失败: %v", err)
				}
			}
		})

		// Snappy 解压
		b.Run(fmt.Sprintf("Snappy_Decompress_%s", size.name), func(b *testing.B) {
			compressor := &SnappyCompressor{}
			compressed, _ := compressor.Compress(testData)
			b.SetBytes(int64(len(testData)))
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := compressor.Decompress(compressed)
				if err != nil {
					b.Fatalf("Decompress 失败: %v", err)
				}
			}
		})

		// Gzip 压缩
		b.Run(fmt.Sprintf("Gzip_Compress_%s", size.name), func(b *testing.B) {
			compressor := &GzipCompressor{}
			b.SetBytes(int64(len(testData)))
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := compressor.Compress(testData)
				if err != nil {
					b.Fatalf("Gzip Compress 失败: %v", err)
				}
			}
		})

		// Gzip 解压
		b.Run(fmt.Sprintf("Gzip_Decompress_%s", size.name), func(b *testing.B) {
			compressor := &GzipCompressor{}
			compressed, _ := compressor.Compress(testData)
			b.SetBytes(int64(len(testData)))
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := compressor.Decompress(compressed)
				if err != nil {
					b.Fatalf("Gzip Decompress 失败: %v", err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkMessageCompressor — 消息压缩器整体性能基准
// ---------------------------------------------------------------------------

// BenchmarkMessageCompressor 测量 MessageCompressor 的压缩/解压性能。
func BenchmarkMessageCompressor(b *testing.B) {
	config := DefaultCompressionConfig()
	config.Threshold = 10
	mc := NewMessageCompressor(config)

	// 准备 100 条消息
	messages := make([]json.RawMessage, 100)
	for i := range messages {
		messages[i] = json.RawMessage(
			fmt.Sprintf(`{"role":"user","content":"这是一条基准测试消息，编号 %d，用于测试消息压缩的整体性能","seq":%d}`, i, i),
		)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		compressed, err := mc.CompressMessages(messages)
		if err != nil {
			b.Fatalf("CompressMessages 失败: %v", err)
		}

		decompressed, err := mc.DecompressMessages(compressed)
		if err != nil {
			b.Fatalf("DecompressMessages 失败: %v", err)
		}

		if len(decompressed) != len(messages) {
			b.Fatalf("消息数量不匹配: 期望 %d，得到 %d", len(messages), len(decompressed))
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSQLite_ConcurrentWrite — SQLite 并发写入基准
// ---------------------------------------------------------------------------

// BenchmarkSQLite_ConcurrentWrite 测量不同并发度下 SQLiteStore 的写入性能。
func BenchmarkSQLite_ConcurrentWrite(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			dbPath := filepath.Join(b.TempDir(), "bench_conc_write.db")
			s, err := NewSQLiteStore(dbPath)
			if err != nil {
				b.Fatalf("NewSQLiteStore 失败: %v", err)
			}
			defer s.Close()

			info, err := s.Create(CreateOpts{Title: "并发写入基准"})
			if err != nil {
				b.Fatalf("Create 失败: %v", err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				for j := 0; j < concurrency; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_ = s.AppendEvent(Event{
							SessionID: info.ID,
							Type:      EventTextDelta,
							Data:      []byte(`{"delta":"concurrent"}`),
						})
					}()
				}
				wg.Wait()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// makeTestData 生成指定大小的测试数据，模拟 JSON 消息。
func makeTestData(b *testing.B, size int) []byte {
	b.Helper()

	// 生成类似 JSON 的重复数据，模拟会话消息
	const baseMessage = `{"role":"user","content":"这是一条测试消息，用于基准测试。","seq":%d,"timestamp":"2024-01-01T00:00:00Z"}`
	baseLen := len(baseMessage) - 2 // 减去 %d 的长度

	// 计算需要的消息数量
	msgCount := size / baseLen
	if msgCount < 1 {
		msgCount = 1
	}

	var result []byte
	result = append(result, '[')
	for i := 0; i < msgCount; i++ {
		if i > 0 {
			result = append(result, ',')
		}
		msg := fmt.Sprintf(baseMessage, i)
		result = append(result, []byte(msg)...)
	}
	result = append(result, ']')

	// 如果还不够大，用随机字节填充到目标大小
	if len(result) < size {
		extra := make([]byte, size-len(result))
		rand.Read(extra)
		result = append(result, extra...)
	}

	return result
}
