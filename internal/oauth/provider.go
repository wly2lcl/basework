package oauth

import (
	"context"
	"fmt"
)

// OAuthProvider 是支持 OAuth 认证的 Provider 接口。
// 实现了此接口的 LLM Provider 可以使用 OAuth 流程进行认证。
type OAuthProvider interface {
	// GetOAuthConfig 返回 Provider 的 OAuth 配置。
	GetOAuthConfig() *ProviderConfig

	// GetToken 返回当前有效的 token。
	GetToken() (*Token, error)
}

// TokenSource 提供有效的 access token，并在过期时自动刷新。
type TokenSource struct {
	config ProviderConfig
	store  Store
	name   string // provider 名称，用于存储/加载 token
}

// NewTokenSource 创建新的 token 源。
func NewTokenSource(config ProviderConfig, store Store, name string) *TokenSource {
	return &TokenSource{
		config: config,
		store:  store,
		name:   name,
	}
}

// Token 返回有效的 access token。
//
// 如果当前 token 过期且有 refresh token，会自动执行刷新流程：
//  1. 使用 refresh token 请求新的 access token
//  2. 保留新的 refresh token（如果新响应中没有提供则沿用旧 refresh token）
//  3. 保存更新后的 token
//
// 如果刷新失败或无 refresh token，返回错误提示重新授权。
func (ts *TokenSource) Token(ctx context.Context) (*Token, error) {
	if ts.store == nil {
		return nil, fmt.Errorf("token 存储未配置")
	}

	token, err := ts.store.Load(ts.name)
	if err != nil {
		return nil, fmt.Errorf("加载 token 失败: %w", err)
	}

	if !token.IsExpired() {
		return token, nil
	}

	// Token 过期，尝试刷新
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("token 已过期且无 refresh token，请重新运行授权")
	}

	newToken, err := RefreshToken(ctx, ts.config, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("刷新 token 失败: %w，请重新运行授权", err)
	}

	// 如果新响应中没有 refresh token，沿用旧的
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}

	// 保存新 token
	if err := ts.store.Save(ts.name, newToken); err != nil {
		return nil, fmt.Errorf("保存刷新后的 token 失败: %w", err)
	}

	return newToken, nil
}