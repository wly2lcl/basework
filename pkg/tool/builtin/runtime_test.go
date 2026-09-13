package builtin

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// blockingChecker 全部拒绝（带标记），用于验证实例注入的检查器被真正使用。
type taggedChecker struct{ tag string }

func (t taggedChecker) CheckPath(path string) (bool, string) {
	return false, "blocked-by-" + t.tag
}

// nilChecker 全部允许。
type allowChecker struct{}

func (allowChecker) CheckPath(string) (bool, string) { return true, "" }

// recordingBus 记录事件名，用于验证实例级 bus 被使用。
// PublishEvent 会在超时定时器 goroutine 里被调用，必须加锁。
type recordingBus struct {
	mu     sync.Mutex
	events []string
}

func (b *recordingBus) PublishEvent(eventType string, _ map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, eventType)
}

func (b *recordingBus) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.events...)
}

// TestRuntime_InstanceCheckerWinsOverGlobal 实例注入的检查器优先生效，
// 且不影响依赖全局的另一实例（CFG-003 核心：一个实例的配置不影响另一个）。
func TestRuntime_InstanceCheckerWinsOverGlobal(t *testing.T) {
	// 全局检查器放行一切。
	old := globalPathChecker
	globalPathChecker = allowChecker{}
	defer func() { globalPathChecker = old }()

	// 实例 A 注入拒绝检查器；实例 B 不注入（回落全局）。
	instanceA := &BashTool{Runtime: &Runtime{PathChecker: taggedChecker{tag: "A"}}}
	instanceB := &BashTool{}

	call := func(bt *BashTool) *toolResultAlias {
		res, err := bt.Execute(context.Background(), []byte(`{"command":"cat /etc/hostname"}`))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return &toolResultAlias{content: res.Content, isError: res.IsError}
	}

	resA := call(instanceA)
	if !resA.isError || !strings.Contains(resA.content, "blocked-by-A") {
		t.Fatalf("实例 A 应使用自己的检查器: %+v", resA)
	}
	resB := call(instanceB)
	if resB.isError && strings.Contains(resB.content, "访问被拒绝") {
		t.Fatalf("实例 B 应回落全局检查器并放行: %+v", resB)
	}
	// 注意：命令本身可能执行失败（文件不存在），关键是不被实例 A 的检查器拦截。

	// 全局换成拒绝，实例 A 仍用自己的——互不影响。
	globalPathChecker = taggedChecker{tag: "global"}
	resA = call(instanceA)
	if !strings.Contains(resA.content, "blocked-by-A") {
		t.Fatalf("全局变化不应影响实例 A: %+v", resA)
	}
	resB = call(instanceB)
	if !resB.isError || !strings.Contains(resB.content, "blocked-by-global") {
		t.Fatalf("实例 B 应感知全局变化: %+v", resB)
	}
}

type toolResultAlias struct {
	content string
	isError bool
}

// TestRuntime_InstanceTimeoutWinsOverGlobal 实例超时配置优先于全局。
func TestRuntime_InstanceTimeoutWinsOverGlobal(t *testing.T) {
	old := globalTimeoutConfig
	globalTimeoutConfig = TimeoutConfig{DefaultTimeout: 99}
	defer func() { globalTimeoutConfig = old }()

	rt := &Runtime{Timeout: &TimeoutConfig{DefaultTimeout: 7}}
	if got := rt.resolveTimeout().GetTimeout("read"); got != 7*time.Second {
		t.Fatalf("实例超时应为 7s，得到 %v", got)
	}
	if got := getTimeoutConfig().GetTimeout("read"); got != 99*time.Second {
		t.Fatalf("全局超时应保持 99s，得到 %v", got)
	}
	// nil Runtime 回落全局。
	var nilRT *Runtime
	if got := nilRT.resolveTimeout().GetTimeout("read"); got != 99*time.Second {
		t.Fatalf("nil Runtime 应回落全局: %v", got)
	}
}

// TestWithTimeoutBus_InstanceIsolation 两个实例的 bus 互不影响。
func TestWithTimeoutBus_InstanceIsolation(t *testing.T) {
	busA := &recordingBus{}
	busB := &recordingBus{}

	// A 设置极短超时，等它触发；B 不设置超时。
	ctxA, cancelA := WithTimeoutBus(context.Background(), "toolA", 5*time.Millisecond, busA)
	defer cancelA()
	ctxB, cancelB := WithTimeoutBus(context.Background(), "toolB", 0, busB)
	defer cancelB()
	_ = ctxA
	_ = ctxB

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(busA.snapshot()) > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	eventsA := busA.snapshot()
	if len(eventsA) == 0 || eventsA[0] != EventToolTimeout {
		t.Fatalf("实例 A 的 bus 应收到超时事件: %v", eventsA)
	}
	if eventsB := busB.snapshot(); len(eventsB) != 0 {
		t.Fatalf("实例 B 的 bus 不应收到事件: %v", eventsB)
	}
}

// TestAllWithRuntime_InjectsEveryInstance AllWithRuntime 注入到每个支持实例的
// 工具上，且 All() 保持不带实例配置（旧入口兼容）。
func TestAllWithRuntime_InjectsEveryInstance(t *testing.T) {
	rt := &Runtime{PathChecker: taggedChecker{tag: "injected"}}
	for _, tool := range AllWithRuntime(rt) {
		switch typed := tool.(type) {
		case *BashTool:
			if typed.Runtime != rt {
				t.Fatalf("bash 未注入实例 Runtime")
			}
		case *ReadTool:
			if typed.Runtime != rt {
				t.Fatalf("read 未注入实例 Runtime")
			}
		case *WriteTool:
			if typed.Runtime != rt {
				t.Fatalf("write 未注入实例 Runtime")
			}
		case *EditTool:
			if typed.Runtime != rt {
				t.Fatalf("edit 未注入实例 Runtime")
			}
		}
	}
	// nil Runtime 与 All() 等价：不 panic，行为回落全局。
	for _, tool := range AllWithRuntime(nil) {
		_ = tool
	}

	// 行为验证：AllWithRuntime 注入的检查器在 Execute 路径上生效。
	tools := AllWithRuntime(&Runtime{PathChecker: taggedChecker{tag: "all"}})
	var bash *BashTool
	for _, tool := range tools {
		if b, ok := tool.(*BashTool); ok {
			bash = b
		}
	}
	if bash == nil {
		t.Fatalf("AllWithRuntime 未包含 bash")
	}
	res, err := bash.Execute(context.Background(), []byte(`{"command":"cat /etc/hostname"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "blocked-by-all") {
		t.Fatalf("AllWithRuntime 注入的检查器应在执行路径生效: %+v", res)
	}
}
