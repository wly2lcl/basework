package permission

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeAt 越界安全取值（仅测试用）。
func safeAt(in []string, i int) string {
	if i < 0 || i >= len(in) {
		return ""
	}
	return in[i]
}

// TestApprovalBroker_RequestRespond 允许路径：Respond 按 ID 命中，
// Request 返回 DecisionApproved。
func TestApprovalBroker_RequestRespond(t *testing.T) {
	b := NewApprovalBroker()
	t.Cleanup(b.Close)

	var mu sync.Mutex
	var gotID string
	b.SetNotifier(func(req ApprovalRequest) {
		mu.Lock()
		gotID = req.ID
		mu.Unlock()
	})

	done := make(chan Decision, 1)
	go func() {
		dec, err := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"}, time.Second)
		if err != nil {
			t.Errorf("Request: %v", err)
		}
		done <- dec
	}()

	var id string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		id = gotID
		mu.Unlock()
		if id != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if id == "" {
		t.Fatal("UI 未收到通知")
	}
	if !b.Respond(id, DecisionApproved) {
		t.Fatal("Respond 应命中 pending")
	}
	select {
	case dec := <-done:
		if dec != DecisionApproved {
			t.Fatalf("应返回 approved，得到 %s", dec)
		}
	case <-time.After(time.Second):
		t.Fatal("Request 未返回")
	}
}

// TestApprovalBroker_TimeoutIsDenial 无人应答 → 超时，且不等于放行。
func TestApprovalBroker_TimeoutIsDenial(t *testing.T) {
	b := NewApprovalBroker()
	t.Cleanup(b.Close)

	dec, err := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"}, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if dec != DecisionTimeout || dec.Approved() {
		t.Fatalf("超时应为 DecisionTimeout 且不放行: %s", dec)
	}
}

// TestApprovalBroker_StaleApprovalDoesNotLeak 验收条件「批准不会错误应用
// 到下一请求」：请求 1 超时后，对它的迟到批准必须被忽略；请求 2 拿到
// 新 ID，不因旧批准而放行。
func TestApprovalBroker_StaleApprovalDoesNotLeak(t *testing.T) {
	b := NewApprovalBroker()
	t.Cleanup(b.Close)

	var mu sync.Mutex
	var ids []string
	b.SetNotifier(func(req ApprovalRequest) {
		mu.Lock()
		ids = append(ids, req.ID)
		mu.Unlock()
	})

	// 请求 1 超时。
	dec1, _ := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"}, 20*time.Millisecond)
	if dec1 != DecisionTimeout {
		t.Fatalf("第一个请求应超时: %s", dec1)
	}
	mu.Lock()
	staleID := ids[0]
	mu.Unlock()

	// 迟到批准：应答不命中。
	if b.Respond(staleID, DecisionApproved) {
		t.Fatal("过期请求的批准应被忽略")
	}

	// 请求 2：新 ID，仍然要等应答；不受旧批准影响。
	done := make(chan Decision, 1)
	go func() {
		dec, _ := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"}, time.Second)
		done <- dec
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(ids)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	idCount, firstID, secondID := len(ids), safeAt(ids, 0), safeAt(ids, 1)
	mu.Unlock()
	if idCount < 2 || secondID == firstID {
		t.Fatalf("第二个请求应有新 ID: %v", ids)
	}
	// 不应答它，验证它不会因 stale 批准而提前放行。
	select {
	case dec := <-done:
		t.Fatalf("未应答却返回: %s", dec)
	case <-time.After(50 * time.Millisecond):
	}
	b.Respond(secondID, DecisionDenied)
	if dec := <-done; dec != DecisionDenied {
		t.Fatalf("第二个请求应按自己的应答: %s", dec)
	}
}

// TestApprovalBroker_CloseCancelsPending Close 使 pending 全部以 Closed 结局。
func TestApprovalBroker_CloseCancelsPending(t *testing.T) {
	b := NewApprovalBroker()

	done := make(chan Decision, 1)
	go func() {
		dec, _ := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"}, time.Second)
		done <- dec
	}()
	time.Sleep(10 * time.Millisecond)
	b.Close()
	select {
	case dec := <-done:
		if dec != DecisionClosed {
			t.Fatalf("关闭应产生 DecisionClosed: %s", dec)
		}
	case <-time.After(time.Second):
		t.Fatal("Close 后 Request 未返回")
	}
	// 关闭后的新请求直接失败。
	if _, err := b.Request(context.Background(), ApprovalRequest{}, time.Second); err == nil {
		t.Fatal("关闭后的 Request 应返回错误")
	}
}

// TestApprovalPromptFunc PromptFunc 适配：允许通过、超时拒绝、不缓存。
func TestApprovalPromptFunc(t *testing.T) {
	b := NewApprovalBroker()
	t.Cleanup(b.Close)
	prompt := ApprovalPromptFunc(b, 30*time.Millisecond)

	// 无人应答 → 拒绝（超时）。
	allow, cache, err := prompt(context.Background(), "bash", map[string]interface{}{"command": "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if allow || cache {
		t.Fatalf("超时应拒绝且不缓存: allow=%v cache=%v", allow, cache)
	}

	// 有人允许 → 放行。
	var mu sync.Mutex
	var lastID string
	b.SetNotifier(func(req ApprovalRequest) {
		mu.Lock()
		lastID = req.ID
		mu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		var id string
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			id = lastID
			mu.Unlock()
			if id != "" {
				break
			}
			time.Sleep(time.Millisecond)
		}
		b.Respond(id, DecisionApproved)
		close(done)
	}()
	allow, cache, err = prompt(context.Background(), "bash", map[string]interface{}{"command": "ls"})
	<-done
	if err != nil || !allow || cache {
		t.Fatalf("允许路径: allow=%v cache=%v err=%v", allow, cache, err)
	}
}

// TestBuildApprovalRequest 信息提取：危险命令风险依据、路径收集、diff 截断。
func TestBuildApprovalRequest(t *testing.T) {
	req := BuildApprovalRequest("bash", map[string]interface{}{
		"command": "rm -rf /tmp/x && cat /etc/passwd",
	}, nil)
	if req.Purpose == "" || req.RiskReason == "" {
		t.Fatalf("应有目的与风险依据: %+v", req)
	}
	if !strings.Contains(req.RiskReason, "删除文件") {
		t.Fatalf("风险依据应含删除文件: %q", req.RiskReason)
	}

	req2 := BuildApprovalRequest("edit_files", map[string]interface{}{
		"path":  "/repo/a.go",
		"edits": []map[string]string{{"old": "x", "new": "y"}},
	}, func(_ string, _ map[string]interface{}) string { return "命中敏感路径" })
	found := false
	for _, p := range req2.Paths {
		if p == "/repo/a.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提取到路径: %v", req2.Paths)
	}
	if !strings.Contains(req2.RiskReason, "敏感路径") {
		t.Fatalf("风险依据应合并自定义来源: %q", req2.RiskReason)
	}
	if len(req2.Diff) > maxApprovalDiffBytes+64 {
		t.Fatalf("diff 应截断: %d", len(req2.Diff))
	}
}
