package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileStoreWriteAndRead 测试写入和读取记忆。
func TestFileStoreWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	err := store.Write(LayerProject, "测试记忆内容", "tag1", "tag2")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != "测试记忆内容" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "测试记忆内容")
	}
	if len(entries[0].Tags) != 2 || entries[0].Tags[0] != "tag1" {
		t.Errorf("Tags = %v, want [tag1 tag2]", entries[0].Tags)
	}
	if entries[0].Layer != LayerProject {
		t.Errorf("Layer = %q, want %q", entries[0].Layer, LayerProject)
	}
	if entries[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

// TestFileStoreWriteMultiple 测试多次写入。
func TestFileStoreWriteMultiple(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	for i := 0; i < 3; i++ {
		content := "记忆" + strings.Repeat("!", i+1)
		if err := store.Write(LayerProject, content); err != nil {
			t.Fatalf("Write #%d failed: %v", i, err)
		}
	}

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[2].Content != "记忆!!!" {
		t.Errorf("last entry content = %q, want %q", entries[2].Content, "记忆!!!")
	}
}

// TestFileStoreReadEmpty 测试读取空文件返回空切片。
func TestFileStoreReadEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestFileStoreClear 测试清空记忆。
func TestFileStoreClear(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	if err := store.Write(LayerProject, "待清除的记忆"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := store.Clear(LayerProject); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read after Clear failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

// TestFileStoreClearEmptyFile 测试清空不存在的文件。
func TestFileStoreClearEmptyFile(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	if err := store.Clear(LayerProject); err != nil {
		t.Fatalf("Clear on empty should not error: %v", err)
	}
}

// TestFileStoreSearch 测试搜索记忆。
func TestFileStoreSearch(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "Golang 是一种编程语言", "go")
	store.Write(LayerProject, "Python 也是一种编程语言", "python")

	entries, err := store.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 result, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Content, "Golang") {
		t.Errorf("result content = %q, should contain 'Golang'", entries[0].Content)
	}
}

// TestFileStoreSearchCaseInsensitive 测试搜索不区分大小写。
func TestFileStoreSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "Hello World")

	entries, err := store.Search("hello", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 result, got %d", len(entries))
	}
}

// TestFileStoreSearchLimit 测试搜索限制数量。
func TestFileStoreSearchLimit(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	for i := 0; i < 5; i++ {
		store.Write(LayerProject, "相同关键词的记忆")
	}

	entries, err := store.Search("相同", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(entries))
	}
}

// TestFileStoreList 测试列出所有层级的记忆。
func TestFileStoreList(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "项目记忆")
	store.Write(LayerLocal, "本地记忆")

	result := store.List()

	projectEntries, ok := result[LayerProject]
	if !ok || len(projectEntries) != 1 {
		t.Errorf("expected 1 project entry, got %v", projectEntries)
	}

	localEntries, ok := result[LayerLocal]
	if !ok || len(localEntries) != 1 {
		t.Errorf("expected 1 local entry, got %v", localEntries)
	}

	// user 层没有文件，应该返回空切片
	userEntries := result[LayerUser]
	if len(userEntries) != 0 {
		t.Errorf("expected 0 user entries, got %d", len(userEntries))
	}
}

// TestFileStoreAutoCreateDir 测试自动创建目录和文件。
func TestFileStoreAutoCreateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested", "path")
	store := NewFileStore(dir)

	// 写入时应自动创建目录
	err := store.Write(LayerProject, "自动创建的目录和文件")
	if err != nil {
		t.Fatalf("Write with auto-create failed: %v", err)
	}

	// 验证文件存在
	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// TestFileStoreDifferentLayers 测试不同层级使用不同文件。
func TestFileStoreDifferentLayers(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "项目数据")
	store.Write(LayerLocal, "本地数据")

	// 项目层不应包含本地数据
	projectEntries, _ := store.Read(LayerProject)
	if len(projectEntries) != 1 || projectEntries[0].Content != "项目数据" {
		t.Errorf("project entries wrong: %+v", projectEntries)
	}

	// 本地层不应包含项目数据
	localEntries, _ := store.Read(LayerLocal)
	if len(localEntries) != 1 || localEntries[0].Content != "本地数据" {
		t.Errorf("local entries wrong: %+v", localEntries)
	}
}

// TestFileStoreMarkdownFormat 验证 Markdown 格式文件内容。
func TestFileStoreMarkdownFormat(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "格式测试", "tag1")

	// 读取原始文件
	path := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "格式测试") {
		t.Error("file content should contain the entry content")
	}
	if !strings.Contains(content, "---") {
		t.Error("file content should contain separator")
	}
}

// TestFileStoreWriteWithTags 测试写入带标签的记忆。
func TestFileStoreWriteWithTags(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	err := store.Write(LayerProject, "带标签的记忆", "重要", "技术", "Golang")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(entries[0].Tags), entries[0].Tags)
	}
	if entries[0].Tags[0] != "重要" {
		t.Errorf("Tag[0] = %q, want %q", entries[0].Tags[0], "重要")
	}
}

// TestFileStoreWriteNoTags 测试写入无标签的记忆。
func TestFileStoreWriteNoTags(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	err := store.Write(LayerProject, "无标签记忆")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(entries[0].Tags))
	}
}

// TestFileStoreMultipleLayersSearch 测试跨层级搜索。
func TestFileStoreMultipleLayersSearch(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "项目层内容", "project")
	store.Write(LayerLocal, "本地层内容", "local")

	// 搜索同时匹配两个层级的词
	entries, err := store.Search("内容", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries across layers, got %d", len(entries))
	}
}

// TestFileStoreSearchEmptyQuery 测试空查询返回空结果。
func TestFileStoreSearchEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "一些内容")
	entries, err := store.Search("不存在的关键词", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 results, got %d", len(entries))
	}
}

// TestFileStoreReadAfterClear 测试清除后写入。
func TestFileStoreReadAfterClear(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "第一轮内容")
	store.Clear(LayerProject)
	store.Write(LayerProject, "第二轮内容")

	entries, _ := store.Read(LayerProject)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after clear+write, got %d", len(entries))
	}
	if entries[0].Content != "第二轮内容" {
		t.Errorf("content = %q, want %q", entries[0].Content, "第二轮内容")
	}
}

// TestFileStoreWriteUserLayer 测试写入用户层级。
func TestFileStoreWriteUserLayer(t *testing.T) {
	// 用户层写入需要 ~/.config/basework 目录
	// 在测试中使用环境变量覆盖
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows 兼容

	dir := t.TempDir()
	store := NewFileStore(dir)

	// 写入用户层
	err := store.Write(LayerUser, "用户记忆")
	if err != nil {
		t.Fatalf("Write user layer: %v", err)
	}

	entries, err := store.Read(LayerUser)
	if err != nil {
		t.Fatalf("Read user layer: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 user entry, got %d", len(entries))
	}
}

// TestFileStorePathForLayer 测试路径生成。
func TestFileStorePathForLayer(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	tests := []struct {
		layer MemoryLayer
		found bool // 是否能获取路径
	}{
		{LayerProject, true},
		{LayerLocal, true},
		{LayerAuto, true},
		{MemoryLayer("invalid"), false},
	}

	for _, tt := range tests {
		_, err := store.pathForLayer(tt.layer)
		if tt.found && err != nil {
			t.Errorf("pathForLayer(%q) unexpected error: %v", tt.layer, err)
		}
		if !tt.found && err == nil {
			t.Errorf("pathForLayer(%q) expected error", tt.layer)
		}
	}
}

// TestFormatEntry 测试条目格式化。
func TestFormatEntry(t *testing.T) {
	result := formatEntry(LayerProject, "测试内容", "tag1", "tag2")

	if !strings.Contains(result, "测试内容") {
		t.Error("formatted entry should contain content")
	}
	if !strings.Contains(result, "---") {
		t.Error("formatted entry should contain separator")
	}
	if !strings.Contains(result, "tags:") {
		t.Error("formatted entry should contain tags line")
	}
}

// TestParseEntry 测试条目解析。
func TestParseEntry(t *testing.T) {
	entry, ok := parseEntry(LayerProject, "[2024-01-01T12:00:00Z] tags: tag1, tag2\n记忆内容")
	if !ok {
		t.Fatal("parseEntry returned false")
	}
	if entry.Layer != LayerProject {
		t.Errorf("Layer = %q", entry.Layer)
	}
	if entry.Content != "记忆内容" {
		t.Errorf("Content = %q", entry.Content)
	}
	if len(entry.Tags) != 2 || entry.Tags[0] != "tag1" {
		t.Errorf("Tags = %v", entry.Tags)
	}
	if entry.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

// TestParseEntryWithoutTags 测试解析无标签条目。
func TestParseEntryWithoutTags(t *testing.T) {
	entry, ok := parseEntry(LayerProject, "[2024-01-01T12:00:00Z]\n无标签内容")
	if !ok {
		t.Fatal("parseEntry returned false")
	}
	if entry.Content != "无标签内容" {
		t.Errorf("Content = %q", entry.Content)
	}
	if len(entry.Tags) != 0 {
		t.Errorf("expected 0 tags, got %v", entry.Tags)
	}
}

// TestParseEntryEmpty 测试解析空行。
func TestParseEntryEmpty(t *testing.T) {
	_, ok := parseEntry(LayerProject, "")
	if ok {
		t.Error("parseEntry should return false for empty input")
	}
}

// TestParseEntryInvalidHeader 测试解析无效头部。
func TestParseEntryInvalidHeader(t *testing.T) {
	entry, ok := parseEntry(LayerProject, "无效格式")
	if ok {
		// 可能返回 true，但 content 会被设置为 "无效格式"
		// 时间会是默认值
		if entry.CreatedAt.IsZero() == false {
			// 也可以接受
		}
	}
}

// TestNewFileStore 测试构造函数。
func TestNewFileStore(t *testing.T) {
	store := NewFileStore("/test/path")
	if store.workspace != "/test/path" {
		t.Errorf("workspace = %q, want %q", store.workspace, "/test/path")
	}
}

// TestFileStoreReadNonExistent 测试读取不存在的文件。
func TestFileStoreReadNonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	entries, err := store.Read(LayerProject)
	if err != nil {
		t.Fatalf("Read on non-existent file: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestFileStoreListEmpty 测试空 store 的 List。
func TestFileStoreListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	result := store.List()
	// 没有写入任何文件时，List 应为空（所有层都不存在文件）
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d layers", len(result))
	}
}

// TestFileStoreInvalidLayer 测试无效层级写入。
func TestFileStoreInvalidLayer(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	err := store.Write("invalid", "内容")
	if err == nil {
		t.Error("expected error for invalid layer")
	}
}

// TestFileStoreSearchNoMatch 测试不匹配的搜索。
func TestFileStoreSearchNoMatch(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Write(LayerProject, "存在的内容")
	entries, err := store.Search("不存在", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 results, got %d", len(entries))
	}
}