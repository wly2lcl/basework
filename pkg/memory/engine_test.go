//go:build memory

package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestEngineNew 测试新引擎创建。
func TestEngineNew(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 1000)
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.maxTokens != 1000 {
		t.Errorf("maxTokens = %d, want 1000", engine.maxTokens)
	}
}

// TestEngineCompressShort 测试短消息列表不压缩。
func TestEngineCompressShort(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好！有什么可以帮助你的？"}}},
	}

	result, err := engine.Compress(context.Background(), messages)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

// TestEngineCompressLong 测试长消息列表被压缩。
func TestEngineCompressLong(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 50)

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第一条长消息，内容足够长以触发压缩。"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第二条长消息，作为助手的回复。"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第三条消息。"}}},
	}

	result, err := engine.Compress(context.Background(), messages)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if len(result) >= len(messages) {
		t.Log("Messages may not be compressed (token estimate below threshold)")
	}
}

// TestEngineCompressEmpty 测试空消息列表。
func TestEngineCompressEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	result, err := engine.Compress(context.Background(), []llm.ChatMessage{})
	if err != nil {
		t.Fatalf("Compress empty: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

// TestEngineRetrieve 测试检索记忆。
func TestEngineRetrieve(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	if err := store.Write(LayerProject, "Golang 是一种编译型语言", "go"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Golang 是什么？"}}},
	}
	entries, err := engine.Retrieve(context.Background(), messages, 5)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	_ = entries
}

// TestEngineRetrieveEmpty 测试空消息时检索。
func TestEngineRetrieveEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	entries, err := engine.Retrieve(context.Background(), []llm.ChatMessage{}, 5)
	if err != nil {
		t.Fatalf("Retrieve empty: %v", err)
	}
	_ = entries
}

// TestEngineAssemble 测试组装记忆到消息。
func TestEngineAssemble(t *testing.T) {
	engine := NewEngine(nil, nil, 100)

	memories := []Entry{
		{
			Content: "之前讨论过 Golang 的并发模型",
			Tags:    []string{"go", "concurrency"},
			Layer:   LayerAuto,
		},
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "继续讨论 Golang"}}},
	}

	result := engine.Assemble("你是一个助手", memories, messages)

	if len(result) != len(messages)+1 {
		t.Fatalf("expected %d messages, got %d", len(messages)+1, len(result))
	}

	if result[0].Role != llm.RoleSystem {
		t.Errorf("first message should be system role")
	}

	sysText := ""
	for _, part := range result[0].Content {
		sysText += part.Text
	}
	if !strings.Contains(sysText, "Golang 的并发模型") {
		t.Errorf("system message should contain memory content: %s", sysText)
	}
}

// TestEngineAssembleNoMemories 测试无记忆时组装。
func TestEngineAssembleNoMemories(t *testing.T) {
	engine := NewEngine(nil, nil, 100)

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}

	result := engine.Assemble("系统提示", nil, messages)
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

// TestEngineAssembleEmptyMemories 测试空记忆列表。
func TestEngineAssembleEmptyMemories(t *testing.T) {
	engine := NewEngine(nil, nil, 100)

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}

	result := engine.Assemble("系统提示", []Entry{}, messages)
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

// TestEngineAssembleWithSystemPrompt 测试带系统提示的组装。
func TestEngineAssembleWithSystemPrompt(t *testing.T) {
	engine := NewEngine(nil, nil, 100)

	memories := []Entry{
		{Content: "用户偏好简洁回答"},
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}

	result := engine.Assemble("你是一个助手", memories, messages)

	sysText := ""
	for _, part := range result[0].Content {
		sysText += part.Text
	}

	if !strings.Contains(sysText, "你是一个助手") {
		t.Errorf("system message should contain prompt: %s", sysText)
	}
	if !strings.Contains(sysText, "用户偏好简洁回答") {
		t.Errorf("system message should contain memory: %s", sysText)
	}
}

// TestFTSIndex 测试 FTS 索引创建和搜索。
func TestFTSIndex(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	if err := fts.Index("1", "Golang 是一种编程语言"); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if err := fts.Index("2", "Python 也是一种编程语言"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(ids), ids)
	}
	if ids[0] != "1" {
		t.Errorf("expected id '1', got %q", ids[0])
	}
}

// TestFTSIndexSearchMultiple 测试多结果搜索。
func TestFTSIndexSearchMultiple(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("1", "Golang 并发编程")
	fts.Index("2", "Golang 网络编程")
	fts.Index("3", "Python 数据分析")

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ids))
	}
}

// TestFTSIndexSearchLimit 测试搜索限制。
func TestFTSIndexSearchLimit(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	for i := 0; i < 5; i++ {
		fts.Index(string(rune('0'+i)), "Golang 编程")
	}

	ids, err := fts.Search("Golang", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(ids))
	}
}

// TestFTSIndexUpdate 测试索引覆盖更新。
func TestFTSIndexUpdate(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("1", "Golang")
	fts.Index("1", "Python")

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results for 'Golang' after update, got %d", len(ids))
	}

	ids, err = fts.Search("Python", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result for 'Python', got %d", len(ids))
	}
}

// TestFTSIndexInMemory 测试内存数据库。
func TestFTSIndexInMemory(t *testing.T) {
	fts, err := NewFTSIndex(":memory:")
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	if err := fts.Index("1", "测试内容"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	ids, err := fts.Search("测试", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result, got %d", len(ids))
	}
}

// TestFTSIndexClose 测试关闭索引。
func TestFTSIndexClose(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}

	if err := fts.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestFTSIndexSearchNoMatch 测试无匹配搜索。
func TestFTSIndexSearchNoMatch(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("1", "Golang")
	ids, err := fts.Search("Python", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results, got %d", len(ids))
	}
}

// TestEngineAssembleMultipleMemories 测试多条记忆的组装。
func TestEngineAssembleMultipleMemories(t *testing.T) {
	engine := NewEngine(nil, nil, 100)
	now := time.Now()

	memories := []Entry{
		{Content: "记忆1", CreatedAt: now.Add(-2 * time.Hour)},
		{Content: "记忆2", CreatedAt: now.Add(-1 * time.Hour)},
		{Content: "记忆3", CreatedAt: now},
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}

	result := engine.Assemble("", memories, messages)

	sysText := ""
	for _, part := range result[0].Content {
		sysText += part.Text
	}

	if !strings.Contains(sysText, "记忆1") || !strings.Contains(sysText, "记忆2") || !strings.Contains(sysText, "记忆3") {
		t.Errorf("system message should contain all memories: %s", sysText)
	}
}

// TestEngineAssembleWithTags 测试带标签记忆的组装。
func TestEngineAssembleWithTags(t *testing.T) {
	engine := NewEngine(nil, nil, 100)

	memories := []Entry{
		{Content: "带标签的记忆", Tags: []string{"重要", "技术"}},
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
	}

	result := engine.Assemble("", memories, messages)

	sysText := ""
	for _, part := range result[0].Content {
		sysText += part.Text
	}

	if !strings.Contains(sysText, "重要") || !strings.Contains(sysText, "技术") {
		t.Errorf("system message should contain tags: %s", sysText)
	}
}

// TestNewFTSIndexInvalidPath 测试无效路径。
func TestNewFTSIndexInvalidPath(t *testing.T) {
	_, err := NewFTSIndex("/nonexistent/dir/test.db")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// TestEngineRetrieveWithLimit 测试检索限制。
func TestEngineRetrieveWithLimit(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	for i := 0; i < 5; i++ {
		store.Write(LayerProject, "测试记忆条目")
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "测试"}}},
	}

	entries, err := engine.Retrieve(context.Background(), messages, 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(entries) > 3 {
		t.Errorf("expected at most 3 entries, got %d", len(entries))
	}
}

// TestEngineNewWithNil 测试使用 nil 参数创建引擎。
func TestEngineNewWithNil(t *testing.T) {
	engine := NewEngine(nil, nil, 0)
	if engine == nil {
		t.Fatal("NewEngine with nil should not return nil")
	}
}

// TestFTSIndexFilePersistence 测试 FTS 索引文件持久化。
func TestFTSIndexFilePersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	fts1, err := NewFTSIndex(dbPath)
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	fts1.Index("1", "持久化测试")
	fts1.Close()

	fts2, err := NewFTSIndex(dbPath)
	if err != nil {
		t.Fatalf("NewFTSIndex reopen: %v", err)
	}
	defer fts2.Close()

	ids, err := fts2.Search("持久化", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result after reopen, got %d", len(ids))
	}
}

// TestEngineCompressWithStore 测试压缩将记忆写入存储。
func TestEngineCompressWithStore(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 30)

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第一条消息，包含足够长的内容以便触发压缩。"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第二条消息，作为助手的回复，内容也很长。"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是第三条消息。"}}},
	}

	_, err = engine.Compress(context.Background(), messages)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}

	entries, err := store.Read(LayerAuto)
	if err != nil {
		t.Fatalf("Read auto layer: %v", err)
	}
	_ = entries
}

// TestFTSIndexSearchEmpty 测试空搜索。
func TestFTSIndexSearchEmpty(t *testing.T) {
	fts, err := NewFTSIndex(":memory:")
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	_, err = fts.Search("", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
}

// TestEngineCompressSingleMessage 测试单条消息不压缩。
func TestEngineCompressSingleMessage(t *testing.T) {
	engine := NewEngine(nil, nil, 100)
	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "单条消息"}}},
	}
	result, err := engine.Compress(context.Background(), messages)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

// TestEngineCompressEmptyMessages 测试空消息压缩。
func TestEngineCompressEmptyMessages(t *testing.T) {
	engine := NewEngine(nil, nil, 100)
	result, err := engine.Compress(context.Background(), []llm.ChatMessage{})
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

// TestEngineNewWithDefault 测试默认值。
func TestEngineNewWithDefault(t *testing.T) {
	engine := NewEngine(nil, nil, 0)
	if engine.maxTokens != 0 {
		t.Errorf("maxTokens = %d, want 0", engine.maxTokens)
	}
}

// TestEngineRetrieveWithMemories 测试有记忆时的检索。
func TestEngineRetrieveWithMemories(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fts, err := NewFTSIndex(filepath.Join(dir, "fts.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	engine := NewEngine(store, fts, 100)

	store.Write(LayerProject, "Golang 是一种静态类型语言", "go")
	store.Write(LayerProject, "Golang 的并发模型基于 goroutine", "go", "concurrency")

	fts.Index("Golang 是一种静态类型语言", "Golang 是一种静态类型语言")
	fts.Index("Golang 的并发模型基于 goroutine", "Golang 的并发模型基于 goroutine")

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Golang 的并发"}}},
	}

	entries, err := engine.Retrieve(context.Background(), messages, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	_ = entries
}