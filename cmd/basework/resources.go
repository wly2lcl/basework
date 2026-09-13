package main

import (
	"context"
	"log"
	"sync"

	"github.com/wly2lcl/basework/pkg/agent"
)

// 本文件是运行时资源的归属与释放（CFG-003）。
//
// 一次 newRuntimeAgent 会创建多种资源：权限持久化、后台任务管理器、LSP、MCP。
// 它们的共同问题是「创建方分散、释放方不明」：
//   - agent.New 失败时，此前创建的资源没有任何人负责释放（泄漏）；
//   - agent 正常运行时，释放由 cleanup 插件承担；
//   - 释放顺序必须是创建的逆序——后创建的资源可能依赖先创建的（例如还在跑的
//     后台命令可能依赖 LSP/MCP 提供的工具），先关依赖方会留下悬空引用。
//
// runtimeResourceScope 是这三个问题的统一答案：创建时 Register（带名字，方便
// 日志定位），Close 逆序执行、幂等、逐个执行完再返回错误清单。归属只有一种：
// scope 被创建后，要么交给 cleanup 插件（agent 正常关闭时释放），要么在装配
// 失败路径上由 defer 释放——一个 scope 同时被两处释放是安全的（幂等）。

// namedCleanup 是带名字的释放动作。名字只用于日志，让「哪个资源关失败了」
// 可以直接定位，而不用对着裸错误猜。
type namedCleanup struct {
	name string
	fn   func() error
}

// runtimeResourceScope 归集并逆序释放一组运行时资源。并发安全。
type runtimeResourceScope struct {
	mu       sync.Mutex
	closed   bool
	cleanups []namedCleanup
}

func newRuntimeResourceScope() *runtimeResourceScope {
	return &runtimeResourceScope{}
}

// Register 登记一个资源的释放动作。注册顺序即创建顺序，Close 按逆序执行。
// scope 已关闭后再 Register 是调用方 BUG：资源创建在 Close 之后发生，
// 没有人能再释放它。这里选择拒绝并记日志，而不是默默吞掉。
func (s *runtimeResourceScope) Register(name string, fn func() error) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		log.Printf("[runtime] warning: 资源 %s 在 scope 关闭后才注册，将无人释放", name)
		return
	}
	s.cleanups = append(s.cleanups, namedCleanup{name: name, fn: fn})
}

// Close 逆序释放全部资源。
//
// 幂等：第二次调用直接返回 nil（资源已释放，不能也不需要再释放）。
// 单个资源释放失败不阻断其余资源的释放——失败时跳过剩下的清理等于把泄漏
// 从「一个资源」扩大到「全部剩余资源」；这里选择全部执行完，返回第一个错误。
func (s *runtimeResourceScope) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	pending := s.cleanups
	s.cleanups = nil
	s.mu.Unlock()

	var firstErr error
	for i := len(pending) - 1; i >= 0; i-- {
		if err := pending[i].fn(); err != nil && firstErr == nil {
			firstErr = err
			log.Printf("[runtime] warning: 资源 %s 释放失败: %v", pending[i].name, err)
		}
	}
	return firstErr
}

// newRuntimeResourcePlugin 把 scope 交给 agent 生命周期：
// agent.Close → Shutdown → scope.Close（逆序、幂等）。
// Initialize 是空操作：资源的创建发生在 agent.New 之前，插件只接管释放。
func newRuntimeResourcePlugin(scope *runtimeResourceScope) agent.Plugin {
	if scope == nil {
		return nil
	}
	return &runtimeResourcePlugin{scope: scope}
}

type runtimeResourcePlugin struct {
	scope *runtimeResourceScope
}

func (p *runtimeResourcePlugin) Name() string { return "runtime-resources" }

func (p *runtimeResourcePlugin) Initialize(context.Context, agent.Agent) error { return nil }

func (p *runtimeResourcePlugin) Shutdown(context.Context) error { return p.scope.Close() }

// runtimeAgentInitFail 把 agent.New 之前创建的资源在失败路径上全部释放。
// 用法（newRuntimeAgent）：
//
//	scope := newRuntimeResourceScope()
//	defer scope.CloseIfNotOwnedBy(&owned)   // 或显式 owned 标记
//	...各资源 scope.Register...
//	agt, err := agent.New(...)
//	if err != nil { return nil, err }       // defer 释放全部
//	owned = true                            // 成功后由插件接管
type ownedMarker struct{ owned bool }

func (o *ownedMarker) markOwned() { o.owned = true }

func (o *ownedMarker) releaseIfNotOwned(scope *runtimeResourceScope) func() {
	return func() {
		if !o.owned {
			if err := scope.Close(); err != nil {
				log.Printf("[runtime] warning: 运行时装配失败后的资源释放出错: %v", err)
			}
		}
	}
}
