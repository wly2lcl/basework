package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
	"golang.org/x/oauth2"
)

// TestHTTPRetry_429RetryThenSuccess 验证 429 后重试最终成功
func TestHTTPRetry_429RetryThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := doWithRetry(ctx, client, req, 3, nil)
	if err != nil {
		t.Fatalf("doWithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", attempts)
	}
}

// TestHTTPRetry_500RetryThenSuccess 验证 500 后重试最终成功
func TestHTTPRetry_500RetryThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"server error"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := doWithRetry(ctx, client, req, 3, nil)
	if err != nil {
		t.Fatalf("doWithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", attempts)
	}
}

// TestHTTPRetry_RespectsRetryAfter 验证 Retry-After 头被正确使用
func TestHTTPRetry_RespectsRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	start := time.Now()
	resp, err := doWithRetry(ctx, client, req, 3, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("doWithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	// Should have waited at least 1 second for Retry-After
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected wait at least 1s for Retry-After, took %v", elapsed)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestHTTPRetry_GivesUpAfterMaxAttempts 验证超过最大重试次数后放弃，返回最后一个响应
func TestHTTPRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"service unavailable"}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := doWithRetry(ctx, client, req, 2, nil)
	if err != nil {
		t.Fatalf("doWithRetry should return last response, not error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
	if attempts != 3 { // initial + 2 retries
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

// TestHTTPRetry_ContextCancellation 验证上下文取消会停止重试
func TestHTTPRetry_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	_, err = doWithRetry(ctx, client, req, 3, nil)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// TestHTTPRetry_jsonRequest 验证 jsonRequest 集成重试逻辑
func TestHTTPRetry_jsonRequest(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	client := newHTTPClient(5 * time.Second)
	ctx := context.Background()

	body, status, err := jsonRequest(ctx, client, srv.URL, nil, map[string]string{"test": "data"})
	if err != nil {
		t.Fatalf("jsonRequest failed: %v", err)
	}
	if status != 200 {
		t.Errorf("expected 200, got %d", status)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("unexpected body: %s", body)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// TestCopilot_TokenRefreshExpired 验证过期 token 自动刷新
func TestCopilot_TokenRefreshExpired(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		t.Logf("request body: %s", body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id":"1","object":"chat.completion","created":123,"model":"gpt-4",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`)
	}))
	defer srv.Close()

	p, _ := NewCopilotProvider(CopilotConfigOpts{Model: "gpt-4"})

	// 设置一个已过期的 token
	p.token = &oauth2.Token{
		AccessToken: "expired-token",
		TokenType:   "bearer",
		Expiry:      time.Now().Add(-1 * time.Hour),
	}

	// 设置一个自定义的 TokenSource，在调用 Token() 时返回新 token
	p.tokenSrc = &mockTokenSource{
		token: &oauth2.Token{
			AccessToken: "refreshed-token",
			TokenType:   "bearer",
			Expiry:      time.Now().Add(1 * time.Hour),
		},
		onToken: func() { refreshCalled = true },
	}

	origEndpoint := copilotEndpoint
	copilotEndpoint = srv.URL
	defer func() { copilotEndpoint = origEndpoint }()

	_, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if !refreshCalled {
		t.Error("expected token refresh to be called")
	}
	if p.token.AccessToken != "refreshed-token" {
		t.Errorf("expected refreshed token, got %s", p.token.AccessToken)
	}
}

// TestCopilot_TokenValidDoesNotRefresh 验证有效的 token 不会触发刷新
func TestCopilot_TokenValidDoesNotRefresh(t *testing.T) {
	refreshCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id":"1","object":"chat.completion","created":123,"model":"gpt-4",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`)
	}))
	defer srv.Close()

	p, _ := NewCopilotProvider(CopilotConfigOpts{Model: "gpt-4"})

	// 设置一个有效的 token（未过期）
	p.token = &oauth2.Token{
		AccessToken: "valid-token",
		TokenType:   "bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}
	p.tokenSrc = &mockTokenSource{
		token: &oauth2.Token{
			AccessToken: "refreshed-token",
			TokenType:   "bearer",
		},
		onToken: func() { refreshCalled = true },
	}

	origEndpoint := copilotEndpoint
	copilotEndpoint = srv.URL
	defer func() { copilotEndpoint = origEndpoint }()

	_, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if refreshCalled {
		t.Error("unexpected token refresh for valid token")
	}
	if p.token.AccessToken != "valid-token" {
		t.Errorf("expected original token, got %s", p.token.AccessToken)
	}
}

// mockTokenSource implements oauth2.TokenSource for testing
type mockTokenSource struct {
	token   *oauth2.Token
	onToken func()
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.onToken != nil {
		m.onToken()
	}
	return m.token, nil
}
