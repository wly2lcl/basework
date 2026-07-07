package oauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------- CSRF 保护测试 ----------

func TestCSRFStateMatch(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	state := "test-state-match-123"
	cs.storeState(state)

	// 发送带正确 state 的回调请求
	reqURL := callbackURL + "?code=test-auth-code&state=" + state
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送回调请求失败: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	code, err := cs.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("WaitForCode() 返回错误: %v", err)
	}
	if code != "test-auth-code" {
		t.Errorf("期望 code=%q，实际 code=%q", "test-auth-code", code)
	}
}

func TestCSRFStateMismatch(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	cs.storeState("real-state-456")

	// 使用不同的 state 模拟 CSRF 攻击
	reqURL := callbackURL + "?code=stolen-code&state=fake-state-789"
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送回调请求失败: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cs.WaitForCode(ctx)
	if err == nil {
		t.Fatal("state 不匹配时应返回错误")
	}
	if !strings.Contains(err.Error(), "OAuth state mismatch") {
		t.Errorf("错误信息应包含 'OAuth state mismatch'，实际: %v", err)
	}

	// 验证真实 state 未被消耗（因为不匹配不会删除）
	cs.stateMu.Lock()
	_, exists := cs.stateStore["real-state-456"]
	cs.stateMu.Unlock()
	if !exists {
		t.Error("真实的 state 应仍存在于 store 中")
	}
}

func TestCSRFStateExpiry(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	state := "expired-state"
	// 手动设置已过期的 state（11 分钟前，超过 10 分钟限制）
	cs.stateMu.Lock()
	cs.stateStore[state] = time.Now().Add(-11 * time.Minute)
	cs.stateMu.Unlock()

	reqURL := callbackURL + "?code=test-code&state=" + state
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送回调请求失败: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cs.WaitForCode(ctx)
	if err == nil {
		t.Fatal("state 过期时应返回错误")
	}
	if !strings.Contains(err.Error(), "OAuth state expired") {
		t.Errorf("错误信息应包含 'OAuth state expired'，实际: %v", err)
	}
}

func TestCSRFStateOneTimeUse(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	state := "one-time-state"
	cs.storeState(state)

	// 验证 state 存在于 store 中
	cs.stateMu.Lock()
	_, exists := cs.stateStore[state]
	cs.stateMu.Unlock()
	if !exists {
		t.Fatal("state 应存在于 store 中")
	}

	// 发送首次回调请求（使用正确 state）
	reqURL := callbackURL + "?code=first-code&state=" + state
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送首次回调请求失败: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cs.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("首次 WaitForCode() 返回错误: %v", err)
	}

	// 验证 state 已被删除（一次性使用）
	cs.stateMu.Lock()
	_, exists = cs.stateStore[state]
	cs.stateMu.Unlock()
	if exists {
		t.Error("state 应在使用后被删除")
	}
}

func TestCSRFStateEmpty(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	// 不传入 state 参数
	reqURL := callbackURL + "?code=no-state-code"
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送回调请求失败: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cs.WaitForCode(ctx)
	if err == nil {
		t.Fatal("缺少 state 时应返回错误")
	}
	if !strings.Contains(err.Error(), "OAuth state mismatch") {
		t.Errorf("错误信息应包含 'OAuth state mismatch'，实际: %v", err)
	}
}
