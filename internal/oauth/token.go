package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProviderConfig 是 OAuth Provider 的配置，包含端点地址和客户端信息。
type ProviderConfig struct {
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret,omitempty"`
	Scopes                []string `json:"scopes"`
	RedirectURI           string   `json:"redirect_uri,omitempty"`
}

// Token 表示 OAuth 2.0 的 token 响应，包含 access token 和可选的 refresh token。
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// IsExpired 检查 access token 是否已过期。
// 为防边界情况，过期时间前 30 秒即视为过期。
func (t *Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

// tokenResponse 是 OAuth token 端点返回的标准 JSON 结构。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// ExchangeToken 使用授权码向 token 端点交换 access token。
func ExchangeToken(ctx context.Context, config ProviderConfig, code, codeVerifier string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {config.RedirectURI},
		"client_id":     {config.ClientID},
		"code_verifier": {codeVerifier},
	}
	if config.ClientSecret != "" {
		data.Set("client_secret", config.ClientSecret)
	}

	return doTokenRequest(ctx, config.TokenEndpoint, data)
}

// RefreshToken 使用 refresh token 刷新 access token。
func RefreshToken(ctx context.Context, config ProviderConfig, refreshToken string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {config.ClientID},
	}
	if config.ClientSecret != "" {
		data.Set("client_secret", config.ClientSecret)
	}

	return doTokenRequest(ctx, config.TokenEndpoint, data)
}

// doTokenRequest 执行 token 端点请求，解析响应并返回 Token。
func doTokenRequest(ctx context.Context, tokenEndpoint string, data url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建 token 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("执行 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 token 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 请求返回 %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("解析 token 响应失败: %w", err)
	}

	expiresAt := time.Now().Add(1 * time.Hour) // 默认 1 小时
	if tr.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}

	token := &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        tr.Scope,
	}

	return token, nil
}
