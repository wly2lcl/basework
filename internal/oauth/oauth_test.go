package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- PKCE 测试 ----------

func TestGenerateCodeVerifier(t *testing.T) {
	// 验证 verifier 长度为 43（32 字节 base64url 无填充）
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() 返回错误: %v", err)
	}
	if len(verifier) != 43 {
		t.Errorf("verifier 长度应为 43，实际为 %d", len(verifier))
	}

	// 验证两次生成的值不同
	verifier2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("第二次 GenerateCodeVerifier() 返回错误: %v", err)
	}
	if verifier == verifier2 {
		t.Error("两次生成的 verifier 不应相同")
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test-verifier-1234567890abcdefghijklmnop"
	challenge := GenerateCodeChallenge(verifier)

	// 验证 challenge 不为空
	if challenge == "" {
		t.Error("challenge 不应为空")
	}

	// 验证同一 verifier 生成相同的 challenge
	challenge2 := GenerateCodeChallenge(verifier)
	if challenge != challenge2 {
		t.Error("同一 verifier 应生成相同的 challenge")
	}

	// 验证不同 verifier 生成不同的 challenge
	challenge3 := GenerateCodeChallenge("different-verifier")
	if challenge == challenge3 {
		t.Error("不同 verifier 应生成不同的 challenge")
	}

	// 验证 challenge 是有效的 base64url 字符串
	if strings.ContainsAny(challenge, "+/=") {
		t.Error("challenge 包含非 base64url 字符（+/=）")
	}
}

func TestNewPKCEParams(t *testing.T) {
	params, err := NewPKCEParams()
	if err != nil {
		t.Fatalf("NewPKCEParams() 返回错误: %v", err)
	}

	if params.CodeVerifier == "" {
		t.Error("CodeVerifier 不应为空")
	}
	if params.CodeChallenge == "" {
		t.Error("CodeChallenge 不应为空")
	}
	if params.State == "" {
		t.Error("State 不应为空")
	}

	// 验证 challenge 是 verifier 的正确哈希
	expectedChallenge := GenerateCodeChallenge(params.CodeVerifier)
	if params.CodeChallenge != expectedChallenge {
		t.Error("CodeChallenge 与 CodeVerifier 的哈希不匹配")
	}
}

// ---------- Token 测试 ----------

func TestTokenIsExpired(t *testing.T) {
	// 过期 token
	token := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	}
	if !token.IsExpired() {
		t.Error("过期的 token 应返回 true")
	}

	// 未过期 token（30 分钟后）
	token2 := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}
	if token2.IsExpired() {
		t.Error("未过期的 token 应返回 false")
	}

	// 零值时间（应视为过期）
	token3 := &Token{
		AccessToken: "test-token",
	}
	if !token3.IsExpired() {
		t.Error("ExpiresAt 为零值的 token 应返回 true")
	}
}

func TestTokenIsExpiredNearBoundary(t *testing.T) {
	// 30 秒缓冲期内应视为过期
	token := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(15 * time.Second),
	}
	if !token.IsExpired() {
		t.Error("距过期不到 30 秒的 token 应返回 true")
	}

	// 超过 30 秒应视为未过期
	token2 := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(31 * time.Second),
	}
	if token2.IsExpired() {
		t.Error("距过期超过 30 秒的 token 应返回 false")
	}
}

// ---------- CallbackServer 测试 ----------

func TestCallbackServerStartStop(t *testing.T) {
	cs := NewCallbackServer()

	// 启动服务器（port=0 使用随机端口）
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}

	if !strings.HasPrefix(callbackURL, "http://127.0.0.1:") {
		t.Errorf("回调 URL 格式不正确: %s", callbackURL)
	}
	if !strings.HasSuffix(callbackURL, "/callback") {
		t.Errorf("回调 URL 应包含 /callback 路径: %s", callbackURL)
	}

	// 停止服务器
	if err := cs.Stop(); err != nil {
		t.Errorf("Stop() 返回错误: %v", err)
	}

	// 重复停止不应报错
	if err := cs.Stop(); err != nil {
		t.Errorf("重复 Stop() 不应返回错误，但得到: %v", err)
	}
}

func TestCallbackServerStartAlreadyStarted(t *testing.T) {
	cs := NewCallbackServer()
	_, err := cs.Start(0)
	if err != nil {
		t.Fatalf("第一次 Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	// 再次启动应返回错误
	_, err = cs.Start(0)
	if err == nil {
		t.Error("已启动的服务器再次 Start() 应返回错误")
	}
}

func TestCallbackServerWaitForCodeWithoutStart(t *testing.T) {
	cs := NewCallbackServer()
	_, err := cs.WaitForCode(context.Background())
	if err == nil {
		t.Error("未启动的服务器 WaitForCode() 应返回错误")
	}
}

func TestCallbackServerReceiveCode(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	// 预先存储 state
	state := "test-callback-state"
	cs.storeState(state)

	// 发送带 state 的 HTTP 请求模拟回调
	reqURL := callbackURL + "?code=test-auth-code-123&state=" + state
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送回调请求失败: %v", err)
	}
	resp.Body.Close()

	// 等待接收授权码
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	code, err := cs.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("WaitForCode() 返回错误: %v", err)
	}
	if code != "test-auth-code-123" {
		t.Errorf("期望 code=%q，实际 code=%q", "test-auth-code-123", code)
	}
}

func TestCallbackServerReceiveError(t *testing.T) {
	cs := NewCallbackServer()
	callbackURL, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	// 发送带错误的回调请求
	reqURL := callbackURL + "?error=access_denied&error_description=用户拒绝了授权"
	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("发送错误回调请求失败: %v", err)
	}
	resp.Body.Close()

	// 等待接收错误
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cs.WaitForCode(ctx)
	if err == nil {
		t.Fatal("WaitForCode() 应返回错误，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("错误信息应包含 'access_denied'，实际: %v", err)
	}
}

func TestCallbackServerTimeout(t *testing.T) {
	cs := NewCallbackServer()
	_, err := cs.Start(0)
	if err != nil {
		t.Fatalf("Start() 返回错误: %v", err)
	}
	defer cs.Stop()

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err = cs.WaitForCode(ctx)
	if err == nil {
		t.Fatal("已取消的 context 下 WaitForCode() 应返回错误")
	}
	if err != context.Canceled {
		t.Errorf("期望 context.Canceled，实际: %v", err)
	}
}

// ---------- FileStore 测试 ----------

func newTestFileStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore() 返回错误: %v", err)
	}
	return store, dir
}

func TestFileStoreSaveLoad(t *testing.T) {
	store, _ := newTestFileStore(t)

	token := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "read write",
	}

	// 保存 token
	if err := store.Save("test-provider", token); err != nil {
		t.Fatalf("Save() 返回错误: %v", err)
	}

	// 加载 token
	loaded, err := store.Load("test-provider")
	if err != nil {
		t.Fatalf("Load() 返回错误: %v", err)
	}

	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken 不匹配: 期望 %q, 实际 %q", token.AccessToken, loaded.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken 不匹配: 期望 %q, 实际 %q", token.RefreshToken, loaded.RefreshToken)
	}
	if loaded.TokenType != token.TokenType {
		t.Errorf("TokenType 不匹配: 期望 %q, 实际 %q", token.TokenType, loaded.TokenType)
	}
	if loaded.Scope != token.Scope {
		t.Errorf("Scope 不匹配: 期望 %q, 实际 %q", token.Scope, loaded.Scope)
	}

	// ExpiresAt 时间可能略有差异（JSON 精度），检查时间差在合理范围内
	diff := loaded.ExpiresAt.Sub(token.ExpiresAt)
	if diff > time.Second || diff < -time.Second {
		t.Errorf("ExpiresAt 差异过大: 期望 %v, 实际 %v, 差 %v", token.ExpiresAt, loaded.ExpiresAt, diff)
	}
}

func TestFileStoreDelete(t *testing.T) {
	store, dir := newTestFileStore(t)

	// 保存并确认文件存在
	token := &Token{AccessToken: "test", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)}
	if err := store.Save("test-provider", token); err != nil {
		t.Fatalf("Save() 返回错误: %v", err)
	}

	tokenPath := filepath.Join(dir, "test-provider_token.json.enc")
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Fatal("token 文件应存在")
	}

	// 删除 token
	if err := store.Delete("test-provider"); err != nil {
		t.Fatalf("Delete() 返回错误: %v", err)
	}

	// 确认文件已删除
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("token 文件应在删除后不存在")
	}

	// 删除不存在的 provider 不应报错
	if err := store.Delete("non-existent"); err != nil {
		t.Errorf("删除不存在的 provider 不应返回错误: %v", err)
	}
}

func TestFileStoreList(t *testing.T) {
	store, _ := newTestFileStore(t)

	// 初始时应为空
	providers, err := store.List()
	if err != nil {
		t.Fatalf("List() 返回错误: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("初始时 provider 列表应为空，实际: %v", providers)
	}

	// 保存多个 token
	token := &Token{AccessToken: "test", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)}
	for _, p := range []string{"github", "google", "anthropic"} {
		if err := store.Save(p, token); err != nil {
			t.Fatalf("Save(%q) 返回错误: %v", p, err)
		}
	}

	// 列表应包含所有 provider
	providers, err = store.List()
	if err != nil {
		t.Fatalf("List() 返回错误: %v", err)
	}

	expected := map[string]bool{"github": true, "google": true, "anthropic": true}
	if len(providers) != len(expected) {
		t.Errorf("期望 %d 个 provider，实际 %d: %v", len(expected), len(providers), providers)
	}
	for _, p := range providers {
		if !expected[p] {
			t.Errorf("意外的 provider: %s", p)
		}
	}

	// 删除后列表应更新
	if err := store.Delete("google"); err != nil {
		t.Fatalf("Delete() 返回错误: %v", err)
	}
	providers, _ = store.List()
	if len(providers) != 2 {
		t.Errorf("删除后应剩 2 个 provider，实际 %d: %v", len(providers), providers)
	}
}

func TestFileStoreLoadNonExistent(t *testing.T) {
	store, _ := newTestFileStore(t)

	_, err := store.Load("non-existent")
	if err == nil {
		t.Fatal("加载不存在的 provider 应返回错误")
	}
	if !strings.Contains(err.Error(), "未授权") {
		t.Errorf("错误信息应包含 '未授权'，实际: %v", err)
	}
}

func TestFileStoreEncryption(t *testing.T) {
	store, dir := newTestFileStore(t)

	token := &Token{
		AccessToken: "secret-token-value",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	if err := store.Save("encrypted-test", token); err != nil {
		t.Fatalf("Save() 返回错误: %v", err)
	}

	// 读取加密文件，确认内容不是明文 JSON
	tokenPath := filepath.Join(dir, "encrypted-test_token.json.enc")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("读取加密文件失败: %v", err)
	}

	// 尝试解析为 JSON，应失败（加密内容不是有效 JSON）
	var parsed Token
	if json.Unmarshal(data, &parsed) == nil {
		t.Error("加密文件内容不应为有效 JSON")
	}

	// 确认密钥文件存在
	keyPath := filepath.Join(dir, ".encryption_key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("密钥文件应存在")
	}
}

// ---------- ExchangeToken 测试（mock HTTP） ----------

func TestExchangeToken(t *testing.T) {
	// 创建 mock token 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST 方法，实际: %s", r.Method)
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("期望 Content-Type=application/x-www-form-urlencoded，实际: %s", r.Header.Get("Content-Type"))
		}

		// 验证 Accept
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("期望 Accept=application/json，实际: %s", r.Header.Get("Accept"))
		}

		// 验证请求参数
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("期望 grant_type=authorization_code，实际: %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "test-code" {
			t.Errorf("期望 code=test-code，实际: %s", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") != "test-verifier" {
			t.Errorf("期望 code_verifier=test-verifier，实际: %s", r.Form.Get("code_verifier"))
		}
		if r.Form.Get("client_id") != "test-client-id" {
			t.Errorf("期望 client_id=test-client-id，实际: %s", r.Form.Get("client_id"))
		}

		// 返回成功响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "mock-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "mock-refresh-token",
			"scope":         "read write",
		})
	}))
	defer server.Close()

	config := ProviderConfig{
		ClientID:    "test-client-id",
		TokenEndpoint: server.URL,
		RedirectURI: "http://127.0.0.1:9999/callback",
	}

	token, err := ExchangeToken(context.Background(), config, "test-code", "test-verifier")
	if err != nil {
		t.Fatalf("ExchangeToken() 返回错误: %v", err)
	}

	if token.AccessToken != "mock-access-token" {
		t.Errorf("期望 AccessToken=mock-access-token，实际: %s", token.AccessToken)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("期望 TokenType=Bearer，实际: %s", token.TokenType)
	}
	if token.RefreshToken != "mock-refresh-token" {
		t.Errorf("期望 RefreshToken=mock-refresh-token，实际: %s", token.RefreshToken)
	}
	if token.Scope != "read write" {
		t.Errorf("期望 Scope=read write，实际: %s", token.Scope)
	}

	// 验证 expires_at 被正确设置（应在当前时间+3600秒左右）
	expectedExpiry := time.Now().Add(3600 * time.Second)
	diff := token.ExpiresAt.Sub(expectedExpiry)
	if diff > 5*time.Second || diff < -5*time.Second {
		t.Errorf("ExpiresAt 与期望差异过大: 期望约 %v, 实际 %v", expectedExpiry, token.ExpiresAt)
	}
}

func TestExchangeTokenErrorResponse(t *testing.T) {
	// mock 返回错误的 token 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_grant",
			"error_description": "授权码已过期",
		})
	}))
	defer server.Close()

	config := ProviderConfig{
		ClientID:    "test-client-id",
		TokenEndpoint: server.URL,
		RedirectURI: "http://127.0.0.1:9999/callback",
	}

	_, err := ExchangeToken(context.Background(), config, "expired-code", "test-verifier")
	if err == nil {
		t.Fatal("错误响应时 ExchangeToken() 应返回错误")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("错误信息应包含状态码 400，实际: %v", err)
	}
}

// ---------- TokenSource 测试（mock Store） ----------

type mockStore struct {
	tokens map[string]*Token
}

func (m *mockStore) Save(provider string, token *Token) error {
	m.tokens[provider] = token
	return nil
}

func (m *mockStore) Load(provider string) (*Token, error) {
	t, ok := m.tokens[provider]
	if !ok {
		return nil, os.ErrNotExist
	}
	return t, nil
}

func (m *mockStore) Delete(provider string) error {
	delete(m.tokens, provider)
	return nil
}

func (m *mockStore) List() ([]string, error) {
	var providers []string
	for k := range m.tokens {
		providers = append(providers, k)
	}
	return providers, nil
}

func newMockStore() *mockStore {
	return &mockStore{tokens: make(map[string]*Token)}
}

func TestTokenSourceValidToken(t *testing.T) {
	store := newMockStore()
	store.Save("test", &Token{
		AccessToken: "valid-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})

	source := NewTokenSource(ProviderConfig{}, store, "test")
	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() 返回错误: %v", err)
	}
	if token.AccessToken != "valid-token" {
		t.Errorf("期望 AccessToken=valid-token，实际: %s", token.AccessToken)
	}
}

func TestTokenSourceExpiredNoRefresh(t *testing.T) {
	store := newMockStore()
	store.Save("test", &Token{
		AccessToken: "expired-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
		// 无 RefreshToken
	})

	source := NewTokenSource(ProviderConfig{}, store, "test")
	_, err := source.Token(context.Background())
	if err == nil {
		t.Fatal("过期且无 refresh token 时应返回错误")
	}
if !strings.Contains(err.Error(), "重新运行授权") {
			t.Errorf("错误信息应包含 '重新运行授权'，实际: %v", err)
	}
}

func TestTokenSourceRefreshSuccess(t *testing.T) {
	// mock token 刷新服务器
	refreshCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCount++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("期望 grant_type=refresh_token，实际: %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "original-refresh-token" {
			t.Errorf("期望 refresh_token=original-refresh-token，实际: %s", r.Form.Get("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "refreshed-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new-refresh-token",
			"scope":         "read",
		})
	}))
	defer server.Close()

	store := newMockStore()
	store.Save("test", &Token{
		AccessToken:  "old-token",
		RefreshToken: "original-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	})

	source := NewTokenSource(ProviderConfig{TokenEndpoint: server.URL}, store, "test")
	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() 返回错误: %v", err)
	}

	if token.AccessToken != "refreshed-access-token" {
		t.Errorf("期望 AccessToken=refreshed-access-token，实际: %s", token.AccessToken)
	}
	if token.RefreshToken != "new-refresh-token" {
		t.Errorf("期望 RefreshToken=new-refresh-token，实际: %s", token.RefreshToken)
	}
	if token.Scope != "read" {
		t.Errorf("期望 Scope=read，实际: %s", token.Scope)
	}

	// 验证 store 已更新
	stored, _ := store.Load("test")
	if stored.AccessToken != "refreshed-access-token" {
		t.Error("store 中的 token 应已被刷新")
	}

	if refreshCount != 1 {
		t.Errorf("刷新请求应只调用 1 次，实际: %d", refreshCount)
	}
}

func TestTokenSourceRefreshPreservesRefreshToken(t *testing.T) {
	// 新 token 响应中没有 refresh_token 时，应沿用旧的
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "refreshed-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			// 没有 refresh_token
		})
	}))
	defer server.Close()

	store := newMockStore()
	store.Save("test", &Token{
		AccessToken:  "old-token",
		RefreshToken: "original-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	})

	source := NewTokenSource(ProviderConfig{TokenEndpoint: server.URL}, store, "test")
	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() 返回错误: %v", err)
	}

	if token.RefreshToken != "original-refresh-token" {
		t.Errorf("应沿用旧的 refresh token，期望 %q，实际 %q", "original-refresh-token", token.RefreshToken)
	}
}

func TestTokenSourceNoStore(t *testing.T) {
	source := &TokenSource{store: nil, name: "test"}
	_, err := source.Token(context.Background())
	if err == nil {
		t.Fatal("无 store 时应返回错误")
	}
}

// ---------- buildAuthorizationURL 测试 ----------

func TestBuildAuthorizationURL(t *testing.T) {
	config := ProviderConfig{
		AuthorizationEndpoint: "https://provider.com/oauth/authorize",
		ClientID:              "test-client",
		Scopes:                []string{"openid", "profile"},
		RedirectURI:           "http://127.0.0.1:8080/callback",
	}

	pkce := &PKCEParams{
		CodeVerifier:  "test-verifier-abcdefghijklmnopqrstuvwxyz123456",
		CodeChallenge: "test-challenge-abcdefgh",
		State:         "test-state-123",
	}

	url := buildAuthorizationURL(config, pkce, config.RedirectURI)

	if !strings.Contains(url, "response_type=code") {
		t.Error("URL 应包含 response_type=code")
	}
	if !strings.Contains(url, "client_id=test-client") {
		t.Error("URL 应包含 client_id=test-client")
	}
	if !strings.Contains(url, "redirect_uri=http") {
		t.Error("URL 应包含 redirect_uri")
	}
	if !strings.Contains(url, "code_challenge=test-challenge-abcdefgh") {
		t.Error("URL 应包含 code_challenge")
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("URL 应包含 code_challenge_method=S256")
	}
	if !strings.Contains(url, "state=test-state-123") {
		t.Error("URL 应包含 state")
	}
	if !strings.Contains(url, "scope=openid+profile") {
		t.Error("URL 应包含 scope")
	}
}

func TestBuildAuthorizationURLNoScopes(t *testing.T) {
	config := ProviderConfig{
		AuthorizationEndpoint: "https://provider.com/oauth/authorize",
		ClientID:              "test-client",
		RedirectURI:           "http://127.0.0.1:8080/callback",
	}

	pkce := &PKCEParams{
		CodeVerifier:  "verifier",
		CodeChallenge: "challenge",
		State:         "state",
	}

	url := buildAuthorizationURL(config, pkce, config.RedirectURI)

	if strings.Contains(url, "scope=") {
		t.Error("无 scope 时 URL 不应包含 scope 参数")
	}
}

// ---------- DefaultConfig 测试 ----------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("默认 OAuth 应为禁用")
	}
	if cfg.StorageBackend != "file" {
		t.Errorf("默认存储后端应为 file，实际: %s", cfg.StorageBackend)
	}
	if cfg.CallbackPort != 8080 {
		t.Errorf("默认回调端口应为 8080，实际: %d", cfg.CallbackPort)
	}
	if cfg.Providers == nil {
		t.Error("默认 Providers map 不应为 nil")
	}
}