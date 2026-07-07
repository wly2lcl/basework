package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// testPlugin 测试用插件实现
type testPlugin struct {
	name      string
	initFn    func(ctx context.Context, agent Agent) error
	shutdown  func(ctx context.Context) error
	initOrder int
}

func (p *testPlugin) Name() string { return p.name }
func (p *testPlugin) Initialize(ctx context.Context, agent Agent) error {
	if p.initFn != nil {
		return p.initFn(ctx, agent)
	}
	return nil
}
func (p *testPlugin) Shutdown(ctx context.Context) error {
	if p.shutdown != nil {
		return p.shutdown(ctx)
	}
	return nil
}

func TestPlugin_OrderedInit(t *testing.T) {
	var mu sync.Mutex
	var order []string

	p1 := &testPlugin{
		name: "plugin1",
		initFn: func(_ context.Context, _ Agent) error {
			mu.Lock()
			order = append(order, "plugin1")
			mu.Unlock()
			return nil
		},
		shutdown: func(_ context.Context) error {
			mu.Lock()
			order = append(order, "shutdown1")
			mu.Unlock()
			return nil
		},
	}
	p2 := &testPlugin{
		name: "plugin2",
		initFn: func(_ context.Context, _ Agent) error {
			mu.Lock()
			order = append(order, "plugin2")
			mu.Unlock()
			return nil
		},
		shutdown: func(_ context.Context) error {
			mu.Lock()
			order = append(order, "shutdown2")
			mu.Unlock()
			return nil
		},
	}

	model := &mockModel{}
	a, err := New(WithModel(model), WithPlugin(p1, p2))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	mu.Lock()
	initOrder := make([]string, len(order))
	copy(initOrder, order)
	mu.Unlock()

	// 验证初始化顺序：plugin1 -> plugin2
	if len(initOrder) != 2 || initOrder[0] != "plugin1" || initOrder[1] != "plugin2" {
		t.Errorf("期望初始化顺序 [plugin1, plugin2], 得到 %v", initOrder)
	}

	// 关闭 agent，验证逆序 shutdown
	_ = a.Close()

	mu.Lock()
	fullOrder := make([]string, len(order))
	copy(fullOrder, order)
	mu.Unlock()

	// 全顺序: plugin1 init, plugin2 init, plugin2 shutdown, plugin1 shutdown
	expectedOrder := []string{"plugin1", "plugin2", "shutdown2", "shutdown1"}
	if len(fullOrder) != 4 {
		t.Fatalf("期望 4 个调用, 得到 %d: %v", len(fullOrder), fullOrder)
	}
	for i, name := range expectedOrder {
		if fullOrder[i] != name {
			t.Errorf("位置 %d: 期望 %s, 得到 %s", i, name, fullOrder[i])
		}
	}
}

func TestPlugin_InitFailureRollback(t *testing.T) {
	var mu sync.Mutex
	var shutdownCalled []string

	p1 := &testPlugin{
		name: "plugin1",
		initFn: func(_ context.Context, _ Agent) error {
			return nil
		},
		shutdown: func(_ context.Context) error {
			mu.Lock()
			shutdownCalled = append(shutdownCalled, "plugin1")
			mu.Unlock()
			return nil
		},
	}
	p2 := &testPlugin{
		name: "plugin2",
		initFn: func(_ context.Context, _ Agent) error {
			return errors.New("plugin2 初始化失败")
		},
		shutdown: func(_ context.Context) error {
			mu.Lock()
			shutdownCalled = append(shutdownCalled, "plugin2")
			mu.Unlock()
			return nil
		},
	}

	model := &mockModel{}
	_, err := New(WithModel(model), WithPlugin(p1, p2))
	if err == nil {
		t.Fatal("期望 plugin2 初始化失败返回错误")
	}

	// 验证已初始化的 plugin1 被 shutdown
	mu.Lock()
	called := make([]string, len(shutdownCalled))
	copy(called, shutdownCalled)
	mu.Unlock()

	if len(called) != 1 || called[0] != "plugin1" {
		t.Errorf("期望已初始化的 plugin1 被 shutdown, 得到 %v", called)
	}
}

func TestPlugin_CloseShutdown(t *testing.T) {
	var mu sync.Mutex
	var shutdownCalls []string

	p1 := &testPlugin{
		name: "plugin_a",
		shutdown: func(_ context.Context) error {
			mu.Lock()
			shutdownCalls = append(shutdownCalls, "plugin_a")
			mu.Unlock()
			return nil
		},
	}
	p2 := &testPlugin{
		name: "plugin_b",
		shutdown: func(_ context.Context) error {
			mu.Lock()
			shutdownCalls = append(shutdownCalls, "plugin_b")
			mu.Unlock()
			return nil
		},
	}

	model := &mockModel{}
	a, err := New(WithModel(model), WithPlugin(p1, p2))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	_ = a.Close()

	mu.Lock()
	calls := make([]string, len(shutdownCalls))
	copy(calls, shutdownCalls)
	mu.Unlock()

	// 验证逆序 shutdown
	if len(calls) != 2 || calls[0] != "plugin_b" || calls[1] != "plugin_a" {
		t.Errorf("期望逆序 shutdown [plugin_b, plugin_a], 得到 %v", calls)
	}
}
