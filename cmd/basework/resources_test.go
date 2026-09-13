package main

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestResourceScope_ClosesInReverseOrder 释放顺序必须是创建的逆序：
// 后创建的资源可能依赖先创建的，先关依赖方会留下悬空引用。
func TestResourceScope_ClosesInReverseOrder(t *testing.T) {
	scope := newRuntimeResourceScope()
	var order []string
	var mu sync.Mutex

	record := func(name string, err error) func() error {
		return func() error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return err
		}
	}

	scope.Register("first", record("first", nil))
	scope.Register("second", record("second", nil))
	scope.Register("third", record("third", nil))

	if err := scope.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "third" || order[1] != "second" || order[2] != "first" {
		t.Fatalf("释放顺序应为逆序 third→second→first，得到 %v", order)
	}
}

// TestResourceScope_IdempotentClose 重复 Close 安全：第二次是空操作。
// 场景对应「agent 正常关闭（插件触发）+ 装配失败路径的 defer」双重释放。
func TestResourceScope_IdempotentClose(t *testing.T) {
	scope := newRuntimeResourceScope()
	var calls int
	scope.Register("only", func() error {
		calls++
		return nil
	})

	if err := scope.Close(); err != nil {
		t.Fatalf("第一次 Close: %v", err)
	}
	if err := scope.Close(); err != nil {
		t.Fatalf("第二次 Close 不应报错: %v", err)
	}
	if calls != 1 {
		t.Fatalf("释放动作只应执行一次，执行了 %d 次", calls)
	}
}

// TestResourceScope_FailureDoesNotSkipRest 单个资源释放失败不阻断其余释放：
// 跳过剩下的清理等于把泄漏从一个资源扩大到全部剩余资源。
func TestResourceScope_FailureDoesNotSkipRest(t *testing.T) {
	scope := newRuntimeResourceScope()
	var released []string
	boom := errors.New("boom")

	scope.Register("a", func() error { released = append(released, "a"); return nil })
	scope.Register("b", func() error { released = append(released, "b"); return boom })
	scope.Register("c", func() error { released = append(released, "c"); return nil })

	err := scope.Close()
	if !errors.Is(err, boom) {
		t.Fatalf("Close 应返回第一个错误: %v", err)
	}
	// 逆序 c→b→a，全部执行。
	if len(released) != 3 {
		t.Fatalf("全部资源都应被尝试释放，得到 %v", released)
	}
}

// TestResourceScope_MultiInstanceIsolation 两个 scope 互不影响：
// 关掉一个实例的全部资源，另一个实例的资源保持存活。
func TestResourceScope_MultiInstanceIsolation(t *testing.T) {
	scopeA := newRuntimeResourceScope()
	scopeB := newRuntimeResourceScope()

	var aClosed, bClosed bool
	scopeA.Register("a", func() error { aClosed = true; return nil })
	scopeB.Register("b", func() error { bClosed = true; return nil })

	if err := scopeA.Close(); err != nil {
		t.Fatalf("scopeA Close: %v", err)
	}
	if !aClosed || bClosed {
		t.Fatalf("关闭 scopeA 不应影响 scopeB: a=%v b=%v", aClosed, bClosed)
	}

	if err := scopeB.Close(); err != nil {
		t.Fatalf("scopeB Close: %v", err)
	}
	if !bClosed {
		t.Fatalf("scopeB 应能独立关闭")
	}
}

// TestResourcePlugin_ShutdownThroughAgent 释放插件把 agent 的 Shutdown 转成
// scope.Close；插件 Shutdown 两次等价于重复 Close（幂等）。
func TestResourcePlugin_ShutdownThroughAgent(t *testing.T) {
	scope := newRuntimeResourceScope()
	var calls int
	scope.Register("r", func() error { calls++; return nil })

	plugin := newRuntimeResourcePlugin(scope)
	if plugin.Name() != "runtime-resources" {
		t.Fatalf("插件名不符: %s", plugin.Name())
	}
	if err := plugin.Initialize(context.Background(), nil); err != nil {
		t.Fatalf("Initialize 应为空操作: %v", err)
	}
	if err := plugin.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := plugin.Shutdown(context.Background()); err != nil {
		t.Fatalf("重复 Shutdown 不应报错: %v", err)
	}
	if calls != 1 {
		t.Fatalf("资源只应释放一次，实际 %d 次", calls)
	}
}

// TestResourceScope_RegisterAfterCloseRejected scope 关闭后注册会被拒绝并告警，
// 防止「创建于关闭之后、永远无人释放」的静默泄漏。
func TestResourceScope_RegisterAfterCloseRejected(t *testing.T) {
	scope := newRuntimeResourceScope()
	if err := scope.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	var called bool
	scope.Register("late", func() error { called = true; return nil })
	if err := scope.Close(); err != nil {
		t.Fatalf("再次 Close: %v", err)
	}
	if called {
		t.Fatalf("关闭后注册的资源不应被释放（它从未被登记）")
	}
}
