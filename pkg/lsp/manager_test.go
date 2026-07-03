package lsp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Manager 测试
// ---------------------------------------------------------------------------

// TestNewManagerDefault 测试 NewManager 使用默认配置。
func TestNewManagerDefault(t *testing.T) {
	m := NewManager(Config{})
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.config.Servers) == 0 {
		t.Error("expected default servers to be set")
	}
	if _, ok := m.config.Servers["go"]; !ok {
		t.Error("expected 'go' in default servers")
	}
	if _, ok := m.config.Servers["typescript"]; !ok {
		t.Error("expected 'typescript' in default servers")
	}
}

// TestNewManagerCustom 测试 NewManager 使用自定义配置。
func TestNewManagerCustom(t *testing.T) {
	customServers := map[string]ServerConfig{
		"go": {Command: "gopls", Args: []string{"-logfile", "/tmp/gopls.log"}},
		"rust": {Command: "rust-analyzer"},
	}
	m := NewManager(Config{Servers: customServers})
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.config.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(m.config.Servers))
	}
	if m.config.Servers["go"].Command != "gopls" {
		t.Errorf("expected gopls, got %q", m.config.Servers["go"].Command)
	}
	if len(m.config.Servers["go"].Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(m.config.Servers["go"].Args))
	}
}

// TestManagerRouting 测试路由：.go 文件路由到 go 客户端，.ts 文件路由到 typescript 客户端。
func TestManagerRouting(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{
			"go":         {Command: "gopls"},
			"typescript": {Command: "typescript-language-server"},
		},
	})
	m.Start(context.Background(), t.TempDir())

	// 直接注入 Ready 状态的客户端
	m.mu.Lock()
	goClient := &Client{
		name:      "go",
		openFiles: make(map[string]bool),
	}
	goClient.state.Store(int32(StateReady))
	tsClient := &Client{
		name:      "typescript",
		openFiles: make(map[string]bool),
	}
	tsClient.state.Store(int32(StateReady))
	m.clients["go"] = goClient
	m.clients["typescript"] = tsClient
	m.mu.Unlock()

	ctx := context.Background()

	// .go 文件应返回 go 客户端
	got := m.getClient(ctx, "test.go")
	if got != goClient {
		t.Error("expected go client for .go file")
	}

	// .ts 文件应返回 typescript 客户端
	got = m.getClient(ctx, "test.ts")
	if got != tsClient {
		t.Error("expected typescript client for .ts file")
	}

	// .tsx 文件也应返回 typescript 客户端
	got = m.getClient(ctx, "test.tsx")
	if got != tsClient {
		t.Error("expected typescript client for .tsx file")
	}
}

// TestManagerUnknownExtension 测试未知文件扩展名返回空结果。
func TestManagerUnknownExtension(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{
			"go": {Command: "gopls"},
		},
	})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()

	// 未知扩展名 → 所有方法返回空结果
	locs, err := m.Definition(ctx, "test.xyz", Position{Line: 0, Character: 0})
	if err != nil || len(locs) != 0 {
		t.Errorf("expected empty Definition, got %v, %v", locs, err)
	}

	refs, err := m.References(ctx, "test.xyz", Position{Line: 0, Character: 0})
	if err != nil || len(refs) != 0 {
		t.Errorf("expected empty References, got %v, %v", refs, err)
	}

	hover, err := m.Hover(ctx, "test.xyz", Position{Line: 0, Character: 0})
	if err != nil || hover != "" {
		t.Errorf("expected empty Hover, got %q, %v", hover, err)
	}

	diags, version := m.Diagnostics("test.xyz")
	if len(diags) != 0 || version != 0 {
		t.Errorf("expected empty Diagnostics, got %v, %d", diags, version)
	}

	syms, err := m.DocumentSymbols(ctx, "test.xyz")
	if err != nil || len(syms) != 0 {
		t.Errorf("expected empty DocumentSymbols, got %v, %v", syms, err)
	}
}

// TestManagerEmptyResults 测试没有客户端时所有查询方法返回空。
func TestManagerEmptyResults(t *testing.T) {
	// 使用空配置，没有服务器配置
	m := NewManager(Config{Servers: map[string]ServerConfig{}})
	m.Start(context.Background(), t.TempDir())

	// 创建一个临时 .go 文件用于测试
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main"), 0644)

	ctx := context.Background()

	// 所有方法返回空（没有客户端可路由）
	locs, err := m.Definition(ctx, goFile, Position{Line: 0, Character: 0})
	if err != nil || len(locs) != 0 {
		t.Errorf("expected empty Definition, got %v, %v", locs, err)
	}

	refs, err := m.References(ctx, goFile, Position{Line: 0, Character: 0})
	if err != nil || len(refs) != 0 {
		t.Errorf("expected empty References, got %v, %v", refs, err)
	}

	hover, err := m.Hover(ctx, goFile, Position{Line: 0, Character: 0})
	if err != nil || hover != "" {
		t.Errorf("expected empty Hover, got %q, %v", hover, err)
	}

	diags, version := m.Diagnostics(goFile)
	if len(diags) != 0 || version != 0 {
		t.Errorf("expected empty Diagnostics, got %v, %d", diags, version)
	}

	syms, err := m.DocumentSymbols(ctx, goFile)
	if err != nil || len(syms) != 0 {
		t.Errorf("expected empty DocumentSymbols, got %v, %v", syms, err)
	}

	syms, err = m.WorkspaceSymbols(ctx, "test")
	if err != nil || len(syms) != 0 {
		t.Errorf("expected empty WorkspaceSymbols, got %v, %v", syms, err)
	}
}

// TestManagerStop 测试 Stop 不 panic。
func TestManagerStop(t *testing.T) {
	// 空 Manager（无客户端）
	m := NewManager(Config{})
	m.Start(context.Background(), t.TempDir())
	err := m.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 有客户端但不就绪
	m = NewManager(Config{Servers: map[string]ServerConfig{"go": {Command: "gopls"}}})
	m.Start(context.Background(), t.TempDir())
	err = m.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 多次 Stop 不 panic
	err = m.Stop()
	if err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// TestManagerConcurrent 测试并发访问安全。
func TestManagerConcurrent(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{
			"go":         {Command: "gopls"},
			"typescript": {Command: "typescript-language-server"},
			"python":     {Command: "pyright-langserver"},
		},
	})
	m.Start(context.Background(), t.TempDir())

	// 注入一些 Ready 客户端
	m.mu.Lock()
	langs := []string{"go", "typescript", "python"}
	for _, lang := range langs {
		c := &Client{
			name:      lang,
			openFiles: make(map[string]bool),
		}
		c.state.Store(int32(StateReady))
		m.clients[lang] = c
	}
	m.mu.Unlock()

	ctx := context.Background()
	files := []string{"a.go", "b.ts", "c.py", "d.xyz", "e.go"}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			file := files[idx%len(files)]
			// 并发调用 getClient（路由逻辑）和 Diagnostics（只读缓存）
			_ = m.getClient(ctx, file)
			_, _ = m.Diagnostics(file)
		}(i)
	}
	wg.Wait()

	// 并发 Stop
	var wg2 sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			_ = m.Stop()
		}()
	}
	wg2.Wait()
}

// TestManagerGetClientNilForUnknown 测试 getClient 对未知扩展名返回 nil。
func TestManagerGetClientNilForUnknown(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{
			"go": {Command: "gopls"},
		},
	})
	m.Start(context.Background(), t.TempDir())
	m.mu.Lock()
	goClient := &Client{
		name:      "go",
		openFiles: make(map[string]bool),
	}
	goClient.state.Store(int32(StateReady))
	m.clients["go"] = goClient
	m.mu.Unlock()

	ctx := context.Background()

	// 已知扩展名应有客户端
	got := m.getClient(ctx, "file.go")
	if got == nil {
		t.Error("expected client for .go file")
	}

	// 未知扩展名应返回 nil
	got = m.getClient(ctx, "file.unknown")
	if got != nil {
		t.Error("expected nil for unknown extension")
	}

	// 无扩展名应返回 nil
	got = m.getClient(ctx, "Makefile")
	if got != nil {
		t.Error("expected nil for file without extension")
	}
}

// TestManagerGetClientNotReady 测试客户端未就绪时 getClient 返回 nil。
func TestManagerGetClientNotReady(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{
			"go": {Command: "gopls"},
		},
	})
	m.Start(context.Background(), t.TempDir())

	// 注入未就绪的客户端
	m.mu.Lock()
	notReadyClient := &Client{name: "go"}
	notReadyClient.state.Store(int32(StateError))
	m.clients["go"] = notReadyClient
	m.mu.Unlock()

	ctx := context.Background()
	got := m.getClient(ctx, "file.go")
	if got != nil {
		t.Error("expected nil for non-ready client")
	}
}

// TestManagerWorkspaceSymbolsEmpty 测试 WorkspaceSymbols 无客户端时返回空。
func TestManagerWorkspaceSymbolsEmpty(t *testing.T) {
	m := NewManager(Config{Servers: map[string]ServerConfig{}})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()
	syms, err := m.WorkspaceSymbols(ctx, "test")
	if err != nil || len(syms) != 0 {
		t.Errorf("expected empty WorkspaceSymbols, got %v, %v", syms, err)
	}
}