package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 测试用的 mock Provider
type mockProvider struct {
	name   string
	models []string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return &llm.Response{Message: llm.ChatMessage{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "mock"}}}}, nil
}
func (m *mockProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (m *mockProvider) Models() []string { return m.models }

// TestRegistry_Register 验证注册 Provider
func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "test", models: []string{"model-1"}}
	r.Register(p)

	if got := r.Get("test"); got == nil {
		t.Fatal("expected to find registered provider")
	}
}

// TestRegistry_Get 验证获取 Provider
func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "my-provider", models: []string{"m1"}}
	r.Register(p)

	got := r.Get("my-provider")
	if got == nil {
		t.Fatal("expected to find provider")
	}
	if got.Name() != "my-provider" {
		t.Errorf("expected name 'my-provider', got '%s'", got.Name())
	}

	// 不存在的 provider
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent provider")
	}
}

// TestRegistry_List 验证列出所有 Provider
func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{name: "a"})
	r.Register(&mockProvider{name: "b"})
	r.Register(&mockProvider{name: "c"})

	names := r.List()
	if len(names) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(names))
	}

	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	if !seen["a"] || !seen["b"] || !seen["c"] {
		t.Errorf("expected all providers in list, got %v", names)
	}
}

// TestRegistry_GetByModel 验证按模型名查找 Provider
func TestRegistry_GetByModel(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{name: "provider-a", models: []string{"model-1", "model-2"}})
	r.Register(&mockProvider{name: "provider-b", models: []string{"model-3"}})

	// 查找存在的模型
	p := r.GetByModel("model-2")
	if p == nil {
		t.Fatal("expected to find provider for model-2")
	}
	if p.Name() != "provider-a" {
		t.Errorf("expected 'provider-a', got '%s'", p.Name())
	}

	// 查找不存在的模型
	if r.GetByModel("nonexistent") != nil {
		t.Error("expected nil for nonexistent model")
	}
}

// TestRegistry_GetByModel_Multiple 验证多个 Provider 有相同模型时返回其中一个（顺序不确定）
func TestRegistry_GetByModel_Multiple(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{name: "a", models: []string{"shared-model"}})
	r.Register(&mockProvider{name: "b", models: []string{"shared-model"}})

	p := r.GetByModel("shared-model")
	if p == nil {
		t.Fatal("expected to find provider")
	}
	// 只要返回的是 a 或 b 就算正确
	if p.Name() != "a" && p.Name() != "b" {
		t.Errorf("expected 'a' or 'b', got '%s'", p.Name())
	}
}

// TestRegistry_MustRegister 验证 MustRegister 在冲突时 panic
func TestRegistry_MustRegister(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{name: "duplicate"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate registration")
		}
	}()

	r.MustRegister(&mockProvider{name: "duplicate"})
}

// TestRegistry_ConcurrentAccess 验证并发读写安全
func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	// 并发注册和读取
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(i int) {
			name := fmt.Sprintf("provider-%d", i)
			r.Register(&mockProvider{name: name, models: []string{name}})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		go func(i int) {
			r.List()
			r.Get(fmt.Sprintf("provider-%d", i))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	names := r.List()
	if len(names) != 10 {
		t.Errorf("expected 10 providers, got %d", len(names))
	}
}

// TestRegistry_DefaultRegistry 验证默认注册中心
func TestRegistry_DefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	// 默认注册中心应该为空（需要配置）
	names := r.List()
	_ = names
}
