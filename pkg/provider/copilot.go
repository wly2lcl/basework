// Package provider — GitHub Copilot Provider
// 使用 OAuth 设备授权流程进行认证，通过 GitHub API 获取 token
// 端点：https://api.githubcopilot.com/chat/completions

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
	"golang.org/x/oauth2"
)

// 默认 Copilot 模型
const defaultCopilotModel = "gpt-4"

// Copilot 端点（变量而非常量，以便测试覆盖）
var copilotEndpoint = "https://api.githubcopilot.com/chat/completions"

// Copilot 需要的请求头
const (
	copilotEditorVersion       = "vscode/1.85.0"
	copilotEditorPluginVersion = "copilot-chat/0.10.0"
	copilotIntegrationID       = "basework"
)

// CopilotProvider 实现 Provider 接口，封装 GitHub Copilot API
type CopilotProvider struct {
	model    string
	token    *oauth2.Token
	tokenSrc oauth2.TokenSource
	client   *http.Client
	tokenPath string
}

// CopilotConfigOpts 是 Copilot Provider 的配置选项
type CopilotConfigOpts struct {
	Model     string
	TokenPath string
}

// NewCopilotProvider 创建新的 GitHub Copilot Provider
// 需要先通过 OAuth 设备流程认证获取 token
func NewCopilotProvider(cfg CopilotConfigOpts) (*CopilotProvider, error) {
	if cfg.Model == "" {
		cfg.Model = defaultCopilotModel
	}
	if cfg.TokenPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取 home 目录失败: %w", err)
		}
		cfg.TokenPath = filepath.Join(home, ".config", "basework", "copilot_token.json")
	}

	p := &CopilotProvider{
		model:     cfg.Model,
		client:    &http.Client{Timeout: 10 * time.Minute},
		tokenPath: cfg.TokenPath,
	}

	// 尝试从文件加载 token
	if err := p.loadToken(); err != nil {
		// token 不存在或无效，需要认证
		return p, nil
	}

	// 设置 token source
	if p.token != nil {
		p.tokenSrc = oauth2.StaticTokenSource(p.token)
	}

	return p, nil
}

// Name 返回 Provider 名称
func (p *CopilotProvider) Name() string {
	return "copilot"
}

// Token 返回当前 OAuth token
func (p *CopilotProvider) Token() (*oauth2.Token, error) {
	if p.token != nil {
		return p.token, nil
	}
	return nil, fmt.Errorf("未认证，请先运行 Authenticate()")
}

// Authenticate 执行 OAuth 设备授权流程
// 使用 GitHub OAuth 设备代码流程获取 token
// 参考: https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow
func (p *CopilotProvider) Authenticate(ctx context.Context) error {
	// 1. 请求设备代码
	deviceCodeReq := map[string]string{
		"client_id": "Iv1.b507a97f0c0a9cff",
		"scope":     "read:user",
	}
	deviceCodeBody, _ := json.Marshal(deviceCodeReq)

	deviceResp, err := http.Post("https://github.com/login/device/code",
		"application/json", bytes.NewReader(deviceCodeBody))
	if err != nil {
		return fmt.Errorf("请求设备代码失败: %w", err)
	}
	defer deviceResp.Body.Close()

	var deviceCode struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(deviceResp.Body).Decode(&deviceCode); err != nil {
		return fmt.Errorf("解析设备代码响应失败: %w", err)
	}

	fmt.Fprintf(os.Stderr, "请访问 %s 并输入代码: %s\n", deviceCode.VerificationURI, deviceCode.UserCode)
	fmt.Fprint(os.Stderr, "等待认证...\n")

	// 2. 轮询等待用户授权
	interval := deviceCode.Interval
	if interval == 0 {
		interval = 5
	}

	tokenReq := map[string]string{
		"client_id":   "Iv1.b507a97f0c0a9cff",
		"device_code": deviceCode.DeviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		tokenBody, err := json.Marshal(tokenReq)
		if err != nil {
			return fmt.Errorf("序列化 token 请求失败: %w", err)
		}
		tokenResp, err := http.Post("https://github.com/login/oauth/access_token",
			"application/json", bytes.NewReader(tokenBody))
		if err != nil {
			return fmt.Errorf("轮询 token 失败: %w", err)
		}

		var tokenResult struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			Error       string `json:"error"`
			ErrorDesc   string `json:"error_description"`
		}
		if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
			tokenResp.Body.Close()
			return fmt.Errorf("解析 token 响应失败: %w", err)
		}
		tokenResp.Body.Close()

		switch tokenResult.Error {
		case "":
			// 成功
			p.token = &oauth2.Token{
				AccessToken: tokenResult.AccessToken,
				TokenType:   tokenResult.TokenType,
			}
			p.tokenSrc = oauth2.StaticTokenSource(p.token)

			if err := p.saveToken(); err != nil {
				return fmt.Errorf("保存 token 失败: %w", err)
			}

			fmt.Fprintf(os.Stderr, "认证成功！\n")
			return nil

		case "authorization_pending":
			// 继续等待
			continue

		case "slow_down":
			interval += 5
			continue

		case "expired_token", "access_denied":
			return fmt.Errorf("设备授权失败: %s", tokenResult.ErrorDesc)

		default:
			return fmt.Errorf("未知错误: %s - %s", tokenResult.Error, tokenResult.ErrorDesc)
		}
	}
}

// buildCopilotHeaders 构建 Copilot 请求头
func (p *CopilotProvider) buildCopilotHeaders() map[string]string {
	headers := map[string]string{
		"Editor-Version":         copilotEditorVersion,
		"Editor-Plugin-Version":  copilotEditorPluginVersion,
		"Copilot-Integration-Id": copilotIntegrationID,
	}
	if p.token != nil && p.token.AccessToken != "" {
		headers["Authorization"] = "Bearer " + p.token.AccessToken
	}
	return headers
}

// refreshTokenIfNeeded checks if the current token is expired and refreshes it if possible.
func (p *CopilotProvider) refreshTokenIfNeeded(ctx context.Context) error {
	if p.token == nil {
		return nil
	}
	if !p.token.Valid() {
		if p.tokenSrc == nil {
			return fmt.Errorf("token expired and no token source available")
		}
		newToken, err := p.tokenSrc.Token()
		if err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
		p.token = newToken
		// Update the static token source for future refreshes
		p.tokenSrc = oauth2.StaticTokenSource(p.token)
		// Persist the refreshed token
		if err := p.saveToken(); err != nil {
			return fmt.Errorf("token refresh: save failed: %w", err)
		}
	}
	return nil
}

// Chat 发送非流式聊天请求
func (p *CopilotProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	if p.token == nil {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: "Copilot 未认证，请先调用 Authenticate()",
		}
	}

	// 自动刷新过期 token
	if err := p.refreshTokenIfNeeded(ctx); err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeAuth,
			Message:    fmt.Sprintf("token refresh failed: %v", err),
			StatusCode: 0,
		}
	}

	body := buildOpenAIRequest(req, false, p.model, false)
	headers := p.buildCopilotHeaders()

	respBody, statusCode, err := jsonRequest(ctx, p.client, copilotEndpoint, headers, body)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if statusCode != 200 {
		return nil, mapHTTPError(statusCode, respBody, "copilot")
	}

	return fromOpenAIResponse(respBody)
}

// ChatStream 发送流式聊天请求
func (p *CopilotProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	if p.token == nil {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: "Copilot 未认证，请先调用 Authenticate()",
		}
	}

	// 自动刷新过期 token
	if err := p.refreshTokenIfNeeded(ctx); err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeAuth,
			Message:    fmt.Sprintf("token refresh failed: %v", err),
			StatusCode: 0,
		}
	}

	body := buildOpenAIRequest(req, true, p.model, false)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range p.buildCopilotHeaders() {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "copilot")
	}

	return parseOpenAIStream(ctx, resp), nil
}

// Models 返回支持的模型列表
func (p *CopilotProvider) Models() []string {
	return []string{"gpt-4", "gpt-4o", "claude-3.5-sonnet", "gemini-2.0-flash"}
}

// ID 返回模型标识（实现 llm.Model）
func (p *CopilotProvider) ID() string {
	return p.model
}

// Supports 检查能力支持（实现 llm.Model）
func (p *CopilotProvider) Supports(cap llm.Capability) bool {
	switch cap {
	case llm.CapTools, llm.CapStreaming:
		return true
	default:
		return false
	}
}

// Generate 非流式生成（实现 llm.Model）
func (p *CopilotProvider) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.Chat(ctx, req)
}

// Stream 流式生成（实现 llm.Model）
func (p *CopilotProvider) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.ChatStream(ctx, req)
}

// saveToken 保存 token 到文件
func (p *CopilotProvider) saveToken() error {
	if p.token == nil {
		return nil
	}

	dir := filepath.Dir(p.tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.Marshal(p.token)
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}

	if err := os.WriteFile(p.tokenPath, data, 0600); err != nil {
		return fmt.Errorf("写入 token 文件失败: %w", err)
	}

	return nil
}

// loadToken 从文件加载 token
func (p *CopilotProvider) loadToken() error {
	data, err := os.ReadFile(p.tokenPath)
	if err != nil {
		return err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	p.token = &token
	return nil
}

// newCopilot 是 factory.Create 使用的构造函数（返回 llm.Model）
func newCopilot(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	cfg := CopilotConfigOpts{Model: modelID}

	if opts != nil {
		if v, ok := opts["token_path"].(string); ok {
			cfg.TokenPath = v
		}
	}

	return NewCopilotProvider(cfg)
}

// 编译时验证
var _ ModelProvider = (*CopilotProvider)(nil)