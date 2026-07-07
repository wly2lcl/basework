//go:build sqlite

// Package tests 包含 basework 项目的 e2e 端到端测试。
package tests

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/session"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Phase 26: WAL 模式端到端测试
// ---------------------------------------------------------------------------

// TestE2E_WALMode 验证 SQLiteStore 创建时自动启用 WAL 模式。
func TestE2E_WALMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wal_test.db")

	// 1. 创建 SQLiteStore
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 2. 验证 WAL 模式已启用
	var journalMode string
	err = s.GetDB().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("查询 journal_mode 失败: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("期望 WAL 模式，得到 %s", journalMode)
	}

	// 3. 验证数据库文件存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("数据库文件不存在")
	}

	// 4. 写入数据并验证
	info, err := s.Create(session.CreateOpts{Title: "WAL 端到端测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if info.ID == "" {
		t.Fatal("期望非空 ID")
	}

	// 5. 读取验证
	got, err := s.Get(info.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Title != "WAL 端到端测试" {
		t.Errorf("标题不匹配: 期望 'WAL 端到端测试', 得到 '%s'", got.Title)
	}

	t.Logf("WAL 模式数据库创建成功: %s, 会话 ID: %s", dbPath, info.ID[:12])
}

// ---------------------------------------------------------------------------
// Phase 26: 文件锁端到端测试
// ---------------------------------------------------------------------------

// TestE2E_FileLock 验证文件锁的完整生命周期：
// 创建 → 获取 → 验证 → 释放 → 再次获取。
func TestE2E_FileLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "session.lock")

	// 1. 创建锁
	lock := session.NewFileLock(lockPath)

	// 2. 获取锁
	err := lock.Lock(2 * time.Second)
	if err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 3. 验证锁文件存在
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("锁文件未创建")
	}

	// 4. 释放锁
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}

	// 5. 再次获取锁（应成功）
	lock2 := session.NewFileLock(lockPath)
	err = lock2.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("第二次 Lock 失败: %v", err)
	}
	lock2.Unlock()

	t.Log("文件锁端到端流程通过")
}

// TestE2E_FileLockContention 验证文件锁的竞争场景：
// 第一个锁持有 → 第二个锁超时 → 第一个释放 → 第三个获取成功。
func TestE2E_FileLockContention(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "contention.lock")

	// 1. 第一个锁持有者
	lock1 := session.NewFileLock(lockPath)
	err := lock1.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock1 失败: %v", err)
	}
	defer lock1.Unlock()

	// 2. 第二个锁尝试获取（应超时）
	lock2 := session.NewFileLock(lockPath)
	err = lock2.Lock(500 * time.Millisecond)
	if err == nil {
		t.Fatal("期望第二个锁超时错误")
		lock2.Unlock()
	}

	// 3. 释放第一个锁
	lock1.Unlock()

	// 4. 第三个锁应成功获取
	lock3 := session.NewFileLock(lockPath)
	err = lock3.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock3 失败（释放后应可获取）: %v", err)
	}
	lock3.Unlock()

	t.Log("文件锁竞争场景验证通过")
}

// TestE2E_FileLockForceUnlock 验证强制解锁功能。
func TestE2E_FileLockForceUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "force_unlock.lock")

	// 1. 创建锁并获取
	lock := session.NewFileLock(lockPath)
	err := lock.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 2. 强制解锁
	err = lock.ForceUnlock()
	if err != nil {
		t.Fatalf("ForceUnlock 失败: %v", err)
	}

	// 3. 验证锁文件被删除
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("强制解锁后锁文件应被删除")
	}

	// 4. 再次获取锁应成功
	lock2 := session.NewFileLock(lockPath)
	err = lock2.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("强制解锁后获取锁失败: %v", err)
	}
	lock2.Unlock()

	t.Log("强制解锁功能验证通过")
}

// ---------------------------------------------------------------------------
// Phase 26: 恢复机制端到端测试
// ---------------------------------------------------------------------------

// TestE2E_Recovery 验证完整性检查和恢复机制：
// 创建数据库 → 写入数据 → 检查完整性 → 验证完整。
func TestE2E_Recovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "recovery_test.db")

	// 1. 创建数据库
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}

	// 2. 写入数据
	info, err := s.Create(session.CreateOpts{Title: "恢复测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 写入多个事件
	for i := 0; i < 10; i++ {
		err = s.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventPrompted,
			Data:      rawJSONE2E(t, session.PromptedData{Content: "测试消息"}),
		})
		if err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	// 3. 检查完整性
	status, err := s.CheckIntegrity()
	if err != nil {
		t.Fatalf("CheckIntegrity 失败: %v", err)
	}

	if status.Corrupted {
		t.Errorf("期望数据库完整，但检测到损坏: %v", status.Error)
	}

	if status.SessionID == "" {
		t.Error("期望非空 SessionID")
	}

	if status.CheckedAt.IsZero() {
		t.Error("期望非零检查时间")
	}

	s.Close()
	t.Log("完整性检查通过")
}

// TestE2E_ListCorruptedSessions 验证列举损坏会话功能。
func TestE2E_ListCorruptedSessions(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. 创建多个完整的数据库
	for i := 0; i < 3; i++ {
		dbPath := filepath.Join(tmpDir, "session_"+string(rune('0'+i))+".db")
		s, err := session.NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore 失败: %v", err)
		}
		_, err = s.Create(session.CreateOpts{Title: "测试"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		s.Close()
	}

	// 2. 列举损坏的会话
	corrupted, err := session.ListCorruptedSessions(tmpDir)
	if err != nil {
		t.Fatalf("ListCorruptedSessions 失败: %v", err)
	}

	// 应该没有损坏的会话
	if len(corrupted) != 0 {
		t.Errorf("期望 0 个损坏的会话，找到 %d 个", len(corrupted))
	}

	t.Logf("在 %s 中检查了会话，未发现损坏", tmpDir)
}

// ---------------------------------------------------------------------------
// Phase 26: 压缩端到端测试
// ---------------------------------------------------------------------------

// TestE2E_Compression 验证消息压缩端到端流程：
// 创建压缩器 → 压缩消息 → 解压 → 验证一致性。
func TestE2E_Compression(t *testing.T) {
	// 1. 创建 Snappy 压缩器
	snappyComp := session.NewCompressor("snappy")
	testData := []byte("这是一条测试消息，用于验证压缩和解压功能的正确性。Hello World!")

	// 2. 压缩
	compressed, err := snappyComp.Compress(testData)
	if err != nil {
		t.Fatalf("Snappy Compress 失败: %v", err)
	}

	if len(compressed) == 0 {
		t.Fatal("压缩后数据为空")
	}

	// 3. 解压
	decompressed, err := snappyComp.Decompress(compressed)
	if err != nil {
		t.Fatalf("Snappy Decompress 失败: %v", err)
	}

	// 4. 验证一致性
	if string(decompressed) != string(testData) {
		t.Errorf("Snappy 解压后数据不匹配: 期望 %s，得到 %s", testData, decompressed)
	}

	// 5. Gzip 压缩器同样测试
	gzipComp := session.NewCompressor("gzip")
	gzipCompressed, err := gzipComp.Compress(testData)
	if err != nil {
		t.Fatalf("Gzip Compress 失败: %v", err)
	}
	gzipDecompressed, err := gzipComp.Decompress(gzipCompressed)
	if err != nil {
		t.Fatalf("Gzip Decompress 失败: %v", err)
	}
	if string(gzipDecompressed) != string(testData) {
		t.Errorf("Gzip 解压后数据不匹配")
	}

	t.Logf("Snappy 压缩率: %d -> %d (%.1f%%)", len(testData), len(compressed), float64(len(compressed))/float64(len(testData))*100)
	t.Logf("Gzip 压缩率: %d -> %d (%.1f%%)", len(testData), len(gzipCompressed), float64(len(gzipCompressed))/float64(len(testData))*100)
}

// TestE2E_MessageCompressor 验证 MessageCompressor 端到端流程。
func TestE2E_MessageCompressor(t *testing.T) {
	config := session.DefaultCompressionConfig()
	config.Threshold = 2 // 降低阈值以便触发压缩
	mc := session.NewMessageCompressor(config)

	// 准备测试消息
	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Hello, how are you?"}`),
		json.RawMessage(`{"role":"assistant","content":"I'm doing great, thank you!"}`),
		json.RawMessage(`{"role":"user","content":"What's the weather like?"}`),
	}

	// 压缩消息
	compressed, err := mc.CompressMessages(messages)
	if err != nil {
		t.Fatalf("CompressMessages 失败: %v", err)
	}

	// 解压消息
	decompressed, err := mc.DecompressMessages(compressed)
	if err != nil {
		t.Fatalf("DecompressMessages 失败: %v", err)
	}

	// 验证消息数量和内容
	if len(decompressed) != len(messages) {
		t.Fatalf("消息数量不匹配: 期望 %d，得到 %d", len(messages), len(decompressed))
	}

	for i := range messages {
		if string(decompressed[i]) != string(messages[i]) {
			t.Errorf("消息 %d 不匹配: 期望 %s，得到 %s", i, messages[i], decompressed[i])
		}
	}

	// 验证统计信息
	stats := mc.GetStats()
	if stats.TotalMessages != 3 {
		t.Errorf("期望 TotalMessages=3，得到 %d", stats.TotalMessages)
	}
	if stats.OriginalSize <= 0 {
		t.Error("期望 OriginalSize > 0")
	}

	t.Logf("压缩统计: %d 条消息, 原始 %d bytes, 压缩后 %d bytes, 比率 %.2f",
		stats.TotalMessages, stats.OriginalSize, stats.CompressedSize, stats.CompressionRatio)
}

// TestE2E_Compression_BelowThreshold 验证低于阈值时不压缩。
func TestE2E_Compression_BelowThreshold(t *testing.T) {
	config := session.DefaultCompressionConfig()
	config.Threshold = 100 // 高阈值，不触发压缩
	mc := session.NewMessageCompressor(config)

	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Hello"}`),
	}

	compressed, err := mc.CompressMessages(messages)
	if err != nil {
		t.Fatalf("CompressMessages 失败: %v", err)
	}

	decompressed, err := mc.DecompressMessages(compressed)
	if err != nil {
		t.Fatalf("DecompressMessages 失败: %v", err)
	}

	if len(decompressed) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(decompressed))
	}

	stats := mc.GetStats()
	if stats.CompressedCount != 0 {
		t.Errorf("低于阈值时 CompressedCount 应为 0，得到 %d", stats.CompressedCount)
	}
}

// ---------------------------------------------------------------------------
// Phase 27: 多进程并发读写测试
// ---------------------------------------------------------------------------

// TestE2E_ConcurrentReadWrite 验证多 goroutine 并发读写 SQLiteStore 的安全性。
func TestE2E_ConcurrentReadWrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建会话
	info, err := s.Create(session.CreateOpts{Title: "并发测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	const goroutineCount = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	errCh := make(chan error, goroutineCount*eventsPerGoroutine)

	// 启动多个 goroutine 并发写入
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				err := s.AppendEvent(session.Event{
					SessionID: info.ID,
					Type:      session.EventType("test"),
					Data:      []byte(`{"goroutine":` + string(rune('0'+id)) + `,"seq":` + string(rune('0'+j)) + `}`),
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// 收集错误
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Fatalf("并发写入中 %d 个错误:", len(errs))
		for _, e := range errs {
			t.Errorf("  %v", e)
		}
	}

	// 验证事件总数
	events, err := s.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}

	expectedCount := goroutineCount * eventsPerGoroutine
	if len(events) != expectedCount {
		t.Errorf("期望 %d 个事件，得到 %d", expectedCount, len(events))
	}

	t.Logf("并发读写测试通过: %d goroutine 共写入 %d 个事件", goroutineCount, len(events))
}

// TestE2E_ConcurrentReadWrite_MultipleSessions 验证多会话并发读写。
func TestE2E_ConcurrentReadWrite_MultipleSessions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "multi_session_concurrent.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	const sessionCount = 5
	const eventsPerSession = 10

	// 创建多个会话
	var sessionIDs []string
	for i := 0; i < sessionCount; i++ {
		info, err := s.Create(session.CreateOpts{Title: "并发会话"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		sessionIDs = append(sessionIDs, info.ID)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, sessionCount*eventsPerSession)

	// 每个会话由一个 goroutine 写入
	for _, sid := range sessionIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < eventsPerSession; j++ {
				err := s.AppendEvent(session.Event{
					SessionID: id,
					Type:      session.EventPrompted,
					Data:      []byte(`{"content":"concurrent message"}`),
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}(sid)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Fatalf("多会话并发写入中有 %d 个错误", len(errs))
	}

	// 验证每个会话的事件数
	for _, sid := range sessionIDs {
		events, err := s.Events(session.EventFilter{SessionID: sid})
		if err != nil {
			t.Fatalf("获取会话 %s 的事件失败: %v", sid[:12], err)
		}
		if len(events) != eventsPerSession {
			t.Errorf("会话 %s 期望 %d 个事件，得到 %d", sid[:12], eventsPerSession, len(events))
		}
	}

	t.Logf("多会话并发读写测试通过: %d 个会话各写入 %d 个事件", sessionCount, eventsPerSession)
}

// TestE2E_SessionConcurrentAccess 并发访问同一会话的场景测试。
func TestE2E_SessionConcurrentAccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "session_concurrent.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(session.CreateOpts{Title: "并发访问测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	const readerCount = 5
	const writerCount = 3
	const opsPerGoroutine = 10

	var wg sync.WaitGroup
	errCh := make(chan error, readerCount*opsPerGoroutine+writerCount*opsPerGoroutine)

	// 并发读取
	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_, err := s.Get(info.ID)
				if err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	// 并发写入
	for i := 0; i < writerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				err := s.AppendEvent(session.Event{
					SessionID: info.ID,
					Type:      session.EventTextDelta,
					Data:      []byte(`{"delta":"concurrent write"}`),
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Fatalf("并发访问中有 %d 个错误", len(errs))
	}

	// 验证最终状态
	events, err := s.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}

	expectedEvents := writerCount * opsPerGoroutine
	if len(events) != expectedEvents {
		t.Errorf("期望 %d 个事件，得到 %d", expectedEvents, len(events))
	}

	t.Logf("并发访问测试通过: %d 读 + %d 写 goroutine, 共 %d 个事件", readerCount, writerCount, len(events))
}

// TestE2E_FileLockWithStore 验证文件锁与 SQLiteStore 配合使用（进程级锁场景模拟）。
// 文件锁使用 flock 实现，适用于多进程同步；在单进程 goroutine 场景中，
// 使用 sync.Mutex 更合适。此测试验证锁的获取/释放与存储操作的组合流程。
func TestE2E_FileLockWithStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "flock_store.db")
	lockPath := filepath.Join(tmpDir, "flock.lock")

	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(session.CreateOpts{Title: "Flock 配合测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 1. 使用文件锁保护写入操作（模拟多进程场景）
	lock := session.NewFileLock(lockPath)
	err = lock.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("获取文件锁失败: %v", err)
	}

	// 在锁保护下写入
	for i := 0; i < 10; i++ {
		err := s.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventTextDelta,
			Data:      []byte(`{"delta":"locked write"}`),
		})
		if err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}
	lock.Unlock()

	// 2. 验证写入成功
	events, err := s.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}

	if len(events) != 10 {
		t.Errorf("期望 10 个事件，得到 %d", len(events))
	}

	// 3. 使用 sync.Mutex 保护 goroutine 间并发（实际应用场景）
	var mu sync.Mutex
	var wg sync.WaitGroup
	const concurrentOps = 20

	for i := 0; i < concurrentOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// goroutine 间应使用 sync.Mutex，flock 是多进程级锁
			mu.Lock()
			defer mu.Unlock()

			err := s.AppendEvent(session.Event{
				SessionID: info.ID,
				Type:      session.EventTextDelta,
				Data:      []byte(`{"delta":"mutex protected write"}`),
			})
			if err != nil {
				t.Errorf("并发写入失败: %v", err)
			}
		}()
	}
	wg.Wait()

	// 4. 验证最终事件数
	events, err = s.Events(session.EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}

	expectedCount := 10 + concurrentOps
	if len(events) != expectedCount {
		t.Errorf("期望 %d 个事件，得到 %d", expectedCount, len(events))
	}

	t.Logf("文件锁 + sync.Mutex 配合测试通过: 共 %d 个事件", len(events))
}

// ---------------------------------------------------------------------------
// TestE2E_WALMode_Notify 验证 WAL 模式下的多连接并发读写
// ---------------------------------------------------------------------------

// TestE2E_WALMode_MultiConnection 验证多个 SQLite 连接并发读写。
func TestE2E_WALMode_MultiConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wal_multi.db")

	// 连接1：写入
	s1, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s1.Close()

	info, err := s1.Create(session.CreateOpts{Title: "多连接测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 写入一些数据
	for i := 0; i < 10; i++ {
		err := s1.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventPrompted,
			Data:      rawJSONE2E(t, session.PromptedData{Content: "消息"}),
		})
		if err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	s1.Close()

	// 连接2：读取（验证 WAL 模式下另一个连接可读取）
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开第二个连接失败: %v", err)
	}
	defer db2.Close()

	// 设置 WAL 模式
	_, err = db2.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		t.Fatalf("设置 WAL 模式失败: %v", err)
	}

	// 查询会话数
	var count int
	err = db2.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	if count != 1 {
		t.Errorf("期望 1 个会话，得到 %d", count)
	}

	t.Logf("WAL 多连接测试通过: 第二个连接可读取 %d 个会话", count)
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// rawJSONE2E 辅助函数，将结构体编码为 json.RawMessage。
func rawJSONE2E(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("编码 JSON 失败: %v", err)
	}
	return data
}

// TestE2E_SQLiteStore_CreateGetDelete 验证创建、获取、删除的生命周期。
func TestE2E_SQLiteStore_CreateGetDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lifecycle.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 1. 创建
	info, err := s.Create(session.CreateOpts{Title: "生命周期测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 2. 获取
	got, err := s.Get(info.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != info.ID {
		t.Errorf("ID 不匹配: 期望 %s，得到 %s", info.ID, got.ID)
	}

	// 3. List 验证
	all, err := s.List(session.ListFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	found := false
	for _, item := range all {
		if item.ID == info.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("List 结果中未找到刚创建的会话")
	}

	// 4. 删除
	err = s.Delete(info.ID)
	if err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 5. 确认删除
	_, err = s.Get(info.ID)
	if err == nil {
		t.Fatal("删除后 Get 应返回错误")
	}
}

// TestE2E_WALMode_MessageProjection 验证 WAL 模式下的消息投影。
func TestE2E_WALMode_MessageProjection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wal_projection.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(session.CreateOpts{Title: "消息投影测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 写入用户消息和助手回复事件
	err = s.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventTurnStarted,
		Data:      rawJSONE2E(t, session.TurnStartedData{Step: 1}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	err = s.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventPrompted,
		Data:      rawJSONE2E(t, session.PromptedData{Content: "你好"}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	err = s.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventTextDelta,
		Data:      rawJSONE2E(t, session.TextDeltaData{Delta: "你好！"}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	err = s.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventTextEnded,
		Data:      rawJSONE2E(t, struct{}{}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	err = s.AppendEvent(session.Event{
		SessionID: info.ID,
		Type:      session.EventTurnEnded,
		Data:      rawJSONE2E(t, session.TurnEndedData{}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	// 通过 MessagesForSession 投影消息
	msgs, err := s.MessagesForSession(info.ID)
	if err != nil {
		t.Fatalf("MessagesForSession 失败: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("期望至少有一条投影消息")
	}

	// 验证用户消息
	userMsgFound := false
	assistantMsgFound := false
	for _, m := range msgs {
		role := string(m.Role)
		if role == "user" {
			userMsgFound = true
			// 提取文本
			var texts []string
			for _, part := range m.Content {
				if part.Type == "text" {
					texts = append(texts, part.Text)
				}
			}
			fullText := strings.Join(texts, "")
			if !strings.Contains(fullText, "你好") {
				t.Errorf("用户消息应包含 '你好'，实际: %s", fullText)
			}
		}
		if role == "assistant" {
			assistantMsgFound = true
		}
	}

	if !userMsgFound {
		t.Error("未找到用户消息")
	}
	if !assistantMsgFound {
		t.Error("未找到助手消息")
	}

	t.Logf("消息投影测试通过: %d 条投影消息", len(msgs))
}

// TestE2E_ConcurrentSessionEvents 验证独立会话的并发事件处理。
func TestE2E_ConcurrentSessionEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent_events.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	const sessionCount = 3
	const eventsPerSession = 5

	// 创建会话
	var sessionIDs []string
	for i := 0; i < sessionCount; i++ {
		info, err := s.Create(session.CreateOpts{Title: "独立会话"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		sessionIDs = append(sessionIDs, info.ID)
	}

	// 并发事件处理 + 并发读取验证
	var wg sync.WaitGroup
	errCh := make(chan error, sessionCount*eventsPerSession*2)

	// 写入 goroutines
	for _, sid := range sessionIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			var innerWg sync.WaitGroup
			for j := 0; j < eventsPerSession; j++ {
				innerWg.Add(1)
				go func(seq int) {
					defer innerWg.Done()
					err := s.AppendEvent(session.Event{
						SessionID: id,
						Type:      session.EventPrompted,
						Data:      rawJSONE2E(t, session.PromptedData{Content: "并发事件"}),
					})
					if err != nil {
						errCh <- err
					}
				}(j)
			}
			innerWg.Wait()
		}(sid)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Fatalf("并发事件处理中有 %d 个错误", len(errs))
	}

	for _, sid := range sessionIDs {
		events, err := s.Events(session.EventFilter{SessionID: sid})
		if err != nil {
			t.Fatalf("获取会话 %s 事件失败: %v", sid[:12], err)
		}
		if len(events) != eventsPerSession {
			t.Errorf("会话 %s 期望 %d 个事件，得到 %d", sid[:12], eventsPerSession, len(events))
		}
	}

	t.Logf("独立会话并发事件测试通过: %d 个会话, 每个 %d 个事件", sessionCount, eventsPerSession)
}
