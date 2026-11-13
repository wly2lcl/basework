// 四包外部嵌入验收：只用 pkg 公共 API 组合 jobs、edits、permission、observability。
//
// 断言：
//   - 两个 jobs.Manager / 两个权限实例并行运行，workspace 与规则互不覆盖；
//   - job 可取消、可读回输出，跨实例查询返回 ErrNotFound；
//   - 编辑预览不写盘，提交落盘，撤销按提交前内容恢复；
//   - deny-all 拒绝、yolo 允许，敏感路径被 strict 拒绝；
//   - 事件异步投递不阻塞发布者，成本与 token 统计可读；
//   - 工具通过 builtin.AllWithRuntime 注入实例配置，全程不调用 builtin.Set*。
//
// 退出码 0 = 全部断言通过；非 0 = 失败（消息写 stderr）。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/edits"
	"github.com/wly2lcl/basework/pkg/jobs"
	"github.com/wly2lcl/basework/pkg/observability"
	"github.com/wly2lcl/basework/pkg/permission"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "嵌入组件验收失败: "+format+"\n", args...)
	os.Exit(1)
}

// pathCheckerAdapter 把 pkg/permission.PathChecker 桥接为 builtin.PathChecker。
type pathCheckerAdapter struct{ inner *permission.PathChecker }

func (p pathCheckerAdapter) CheckPath(path string) (bool, string) {
	return p.inner.CheckPath(path)
}

// eventPublisherAdapter 把 observability.EventBus 桥接为 builtin.EventPublisher。
type eventPublisherAdapter struct{ inner *observability.EventBus }

func (e eventPublisherAdapter) PublishEvent(eventType string, data map[string]interface{}) {
	e.inner.Publish(observability.Event{Type: eventType, Timestamp: time.Now().UTC(), Data: data})
}

func main() {
	root, err := os.MkdirTemp("", "basework-embed-components-")
	if err != nil {
		fail("建临时根: %v", err)
	}
	// 统一成解析软链后的路径，避免 /var → /private/var 让工作区相对路径失真。
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	defer os.RemoveAll(root)

	wsA, wsB := filepath.Join(root, "ws-a"), filepath.Join(root, "ws-b")
	for _, dir := range []string{wsA, wsB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("建工作区 %s: %v", dir, err)
		}
	}

	// 1) 两个 jobs.Manager：不同 workspace、不同 ID 前缀、不同输出目录。
	mgrA := jobs.New(jobs.Options{WorkspaceID: "ws-a", IDPrefix: "a-", Output: jobs.OutputOptions{Dir: filepath.Join(wsA, ".out")}})
	mgrB := jobs.New(jobs.Options{WorkspaceID: "ws-b", IDPrefix: "b-", Output: jobs.OutputOptions{Dir: filepath.Join(wsB, ".out")}})
	defer mgrA.Close()
	defer mgrB.Close()

	var wg sync.WaitGroup
	var jobA, jobB *jobs.Job
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		jobA, err = mgrA.Start("session-1", "sleep until canceled", func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return -1, ctx.Err()
		})
		if err != nil {
			fail("ws-a 启动 job: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		jobB, err = mgrB.StartWithOutput("session-1", "echo", func(_ context.Context, stdout, _ io.Writer) (int, error) {
			_, _ = fmt.Fprintln(stdout, "hello from ws-b")
			return 0, nil
		})
		if err != nil {
			fail("ws-b 启动 job: %v", err)
		}
	}()
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgrA.Cancel("session-1", jobA.ID); err != nil {
		fail("取消 ws-a job: %v", err)
	}
	doneA, err := mgrA.Await(ctx, "session-1", jobA.ID)
	if err != nil {
		fail("等待 ws-a 终态: %v", err)
	}
	if doneA.State != jobs.StateCanceled {
		fail("ws-a job 终态应为 canceled，实际 %s", doneA.State)
	}

	doneB, err := mgrB.Await(ctx, "session-1", jobB.ID)
	if err != nil {
		fail("等待 ws-b 终态: %v", err)
	}
	if doneB.State != jobs.StateSucceeded {
		fail("ws-b job 终态应为 succeeded，实际 %s", doneB.State)
	}
	page, err := mgrB.ReadOutput("session-1", jobB.ID, jobs.StreamStdout, 0, 1024)
	if err != nil {
		fail("读 ws-b 输出: %v", err)
	}
	if page.Text != "hello from ws-b\n" {
		fail("ws-b 输出不符: %q", page.Text)
	}

	// 实例隔离：同一 owner 也不能跨 Manager 看到对方的 job。
	if _, err := mgrA.Get("session-1", jobB.ID); !errors.Is(err, jobs.ErrNotFound) {
		fail("ws-a 不应看到 ws-b 的 job，实际错误 %v", err)
	}
	if _, err := mgrB.Get("session-1", jobA.ID); !errors.Is(err, jobs.ErrNotFound) {
		fail("ws-b 不应看到 ws-a 的 job，实际错误 %v", err)
	}
	if len(mgrA.List("session-1")) != 1 || len(mgrB.List("session-1")) != 1 {
		fail("两个 Manager 的列表长度应各为 1")
	}

	// 2) 编辑：预览不写盘 → 提交 → 撤销。
	file := filepath.Join(wsA, "notes.txt")
	if err := os.WriteFile(file, []byte("alpha\nbeta\n"), 0o644); err != nil {
		fail("写初始文件: %v", err)
	}
	planner, err := edits.NewPlanner(edits.Options{Root: wsA})
	if err != nil {
		fail("建 Planner: %v", err)
	}
	op, err := planner.Preview("notes.txt", "alpha", "ALPHA")
	if err != nil {
		fail("编辑预览: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "alpha\nbeta\n" {
		fail("预览不应写盘，实际 %q", got)
	}
	batch, err := planner.Prepare(op)
	if err != nil {
		fail("编辑准备: %v", err)
	}
	result, err := planner.Commit(batch, edits.CommitOptions{})
	if err != nil {
		fail("编辑提交: %v", err)
	}
	if !result.OK() {
		fail("编辑提交未全部成功: %s", result.Summary())
	}
	if got, _ := os.ReadFile(file); string(got) != "ALPHA\nbeta\n" {
		fail("提交后内容不符: %q", got)
	}
	if _, err := result.Rollback(edits.CommitOptions{}); err != nil {
		fail("编辑撤销: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "alpha\nbeta\n" {
		fail("撤销后内容不符: %q", got)
	}

	// 3) 权限：两个 Checker 模式互不覆盖；路径保护与权限模式独立。
	yolo := permission.NewChecker(permission.ModeYolo, nil, nil)
	deny := permission.NewChecker(permission.ModeDenyAll, nil, nil)
	args := map[string]interface{}{"command": "ls"}
	if ok, err := yolo.Check(ctx, "bash", args); err != nil || !ok {
		fail("yolo 应允许 bash: ok=%v err=%v", ok, err)
	}
	if ok, err := deny.Check(ctx, "bash", args); err != nil || ok {
		fail("deny-all 应拒绝 bash: ok=%v err=%v", ok, err)
	}
	paths := permission.NewPathChecker(nil, nil, permission.ProtectionStrict)
	if allowed, _ := paths.CheckPath(file); !allowed {
		fail("普通工作区文件应被允许: %s", file)
	}
	if allowed, reason := paths.CheckPath(filepath.Join(wsA, ".git", "config")); allowed {
		fail("敏感路径应被 strict 拒绝: %s", reason)
	}

	// 4) 观测：事件异步投递不阻塞发布者；成本与 token 统计可读。
	bus := observability.NewEventBus()
	received := make(chan observability.Event, 8)
	bus.Subscribe(observability.EventToolStart, func(event observability.Event) {
		time.Sleep(20 * time.Millisecond) // 慢订阅者不应拖慢发布者
		received <- event
	})
	start := time.Now()
	const published = 16
	for i := 0; i < published; i++ {
		bus.Publish(observability.NewEvent(observability.EventToolStart, map[string]interface{}{"tool": "bash", "n": i}))
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		fail("事件发布被订阅者阻塞: %v", elapsed)
	}
	deadline := time.After(2 * time.Second)
	for i := 0; i < published; i++ {
		select {
		case <-received:
		case <-deadline:
			fail("只收到 %d/%d 个事件", i, published)
		}
	}

	costTracker := observability.NewCostTracker()
	costTracker.RecordCall("gpt-4o", 1000, 500, 0.0123)
	if got := costTracker.GetTotalCost(); got < 0.012 || got > 0.013 {
		fail("成本累计不符: %v", got)
	}
	tokenTracker := observability.NewTokenTracker()
	tokenTracker.RecordUsage("gpt-4o", 1000, 500)
	if in, out, total := tokenTracker.GetTotalUsage(); in != 1000 || out != 500 || total != 1500 {
		fail("token 统计不符: in=%d out=%d total=%d", in, out, total)
	}

	// 5) 工具实例注入走 builtin.Runtime，不使用 builtin.Set* 全局。
	runtimeTools := builtin.AllWithRuntime(&builtin.Runtime{
		PathChecker: pathCheckerAdapter{inner: paths},
		EventBus:    eventPublisherAdapter{inner: bus},
	})
	if len(runtimeTools) == 0 {
		fail("builtin.AllWithRuntime 返回空工具集")
	}

	logger := observability.NewLogger(observability.LogLevelInfo, io.Discard)
	logger.Info("embed components ok", map[string]interface{}{"workspaces": 2})

	fmt.Printf("嵌入组件验收通过: jobs=%s/%s 编辑=%s 事件=%d 成本=%.4f token=%d 工具=%d\n",
		jobA.ID, jobB.ID, op.RelPath, published, costTracker.GetTotalCost(), 1500, len(runtimeTools))
}
