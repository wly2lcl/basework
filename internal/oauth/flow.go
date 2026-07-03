package oauth

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// Flow 是 OAuth 授权流程的编排器，协调 PKCE、回调服务器和 token 交换。
type Flow struct {
	Config ProviderConfig
	Store  Store
	Server *CallbackServer
	Port   int
}

// NewFlow 创建新的 OAuth 流程编排器。
// config: Provider 的 OAuth 配置
// store: token 存储后端
// port: 本地回调服务器端口（0 表示随机端口）
func NewFlow(config ProviderConfig, store Store, port int) *Flow {
	return &Flow{
		Config: config,
		Store:  store,
		Server: NewCallbackServer(),
		Port:   port,
	}
}

// Authorize 执行完整的 OAuth 授权流程：
//  1. 生成 PKCE 参数（code verifier + challenge）
//  2. 启动本地回调服务器
//  3. 构建授权 URL
//  4. 在默认浏览器中打开授权 URL
//  5. 等待授权码回调
//  6. 用授权码交换 access token
//  7. 存储 token
//  8. 返回 token
func (f *Flow) Authorize(ctx context.Context) (*Token, error) {
	// 1. 生成 PKCE 参数
	pkce, err := NewPKCEParams()
	if err != nil {
		return nil, fmt.Errorf("生成 PKCE 参数失败: %w", err)
	}

	// 2. 启动回调服务器
	callbackURL, err := f.Server.Start(f.Port)
	if err != nil {
		return nil, fmt.Errorf("启动回调服务器失败: %w", err)
	}

	// 确保流程结束后停止服务器
	defer func() {
		_ = f.Server.Stop()
	}()

	// 3. 构建授权 URL
	redirectURI := f.Config.RedirectURI
	if redirectURI == "" {
		redirectURI = callbackURL
	}

	authURL := buildAuthorizationURL(f.Config, pkce, redirectURI)
	fmt.Printf("请在浏览器中完成授权:\n%s\n\n", authURL)

	// 4. 在默认浏览器中打开授权 URL
	if err := openBrowser(authURL); err != nil {
		// 打开浏览器失败不是致命错误，用户可以手动复制 URL
		fmt.Printf("无法自动打开浏览器，请手动复制以上链接到浏览器。\n")
	}

	// 5. 等待授权码回调
	code, err := f.Server.WaitForCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("等待授权回调失败: %w", err)
	}

	// 6. 交换 token
	config := f.Config
	config.RedirectURI = redirectURI
	token, err := ExchangeToken(ctx, config, code, pkce.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("交换 access token 失败: %w", err)
	}

	// 7. 存储 token
	if f.Store != nil {
		if err := f.Store.Save("default", token); err != nil {
			return nil, fmt.Errorf("存储 token 失败: %w", err)
		}
	}

	// 8. 返回 token
	return token, nil
}

// buildAuthorizationURL 构建 OAuth 授权 URL，包含 PKCE 参数。
func buildAuthorizationURL(config ProviderConfig, pkce *PKCEParams, redirectURI string) string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {config.ClientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {pkce.CodeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {pkce.State},
	}

	if len(config.Scopes) > 0 {
		params.Set("scope", strings.Join(config.Scopes, " "))
	}

	return config.AuthorizationEndpoint + "?" + params.Encode()
}

// openBrowser 在默认浏览器中打开指定 URL。
// 支持 macOS (open)、Windows (cmd /c start)、Linux (xdg-open)。
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}