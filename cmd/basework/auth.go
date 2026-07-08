// Package main 是 basework CLI 的入口点
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/oauth"
	"github.com/wly2lcl/basework/pkg/config"
)

// authCmd 表示 auth 子命令
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "管理 OAuth 认证",
	Long: `管理 OAuth 认证，包括登录、查看状态和登出。
支持 OAuth 2.0 PKCE 流程进行安全认证。`,
}

// authLoginCmd 表示 auth login 子命令
var authLoginCmd = &cobra.Command{
	Use:   "login <provider>",
	Short: "为指定 provider 启动 OAuth 授权流程",
	Long: `启动 OAuth 授权流程，在浏览器中打开授权页面，
完成授权后将获取并存储 access token。
支持的 provider 需在配置文件中定义 OAuth 配置。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthLogin(cmd.Context(), args[0])
	},
}

// authStatusCmd 表示 auth status 子命令
var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示所有 provider 的认证状态",
	Long:  `列出所有已配置和已认证的 provider，显示其认证状态和 token 过期信息。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthStatus()
	},
}

// authLogoutCmd 表示 auth logout 子命令
var authLogoutCmd = &cobra.Command{
	Use:   "logout <provider>",
	Short: "清除指定 provider 的认证信息",
	Long:  `删除指定 provider 的 access token，解除认证状态。`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthLogout(args[0])
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

// getOAuthConfig 加载配置并返回 OAuth 配置和 token 存储
func getOAuthConfig() (*oauth.Config, oauth.Store, error) {
	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.Discover()
		if err != nil {
			return nil, nil, fmt.Errorf("查找配置文件失败: %w", err)
		}
	}

	store, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("加载配置失败: %w", err)
	}

	cfg := store.Get()

	// 构建 OAuth 内部配置
	oauthCfg := oauth.DefaultConfig()
	oauthCfg.Enabled = cfg.OAuth.Enabled
	oauthCfg.StorageBackend = cfg.OAuth.StorageBackend
	oauthCfg.CallbackPort = cfg.OAuth.CallbackPort
	oauthCfg.Providers = make(map[string]oauth.ProviderConfig, len(cfg.OAuth.Providers))
	for name, provider := range cfg.OAuth.Providers {
		oauthCfg.Providers[name] = oauth.ProviderConfig{
			AuthorizationEndpoint: provider.AuthorizationEndpoint,
			TokenEndpoint:         provider.TokenEndpoint,
			ClientID:              provider.ClientID,
			ClientSecret:          provider.ClientSecret,
			Scopes:                append([]string(nil), provider.Scopes...),
			RedirectURI:           provider.RedirectURI,
		}
	}

	// 创建 token 存储
	switch oauthCfg.StorageBackend {
	case "", "file":
		tokenDir := getTokenDir()
		fileStore, err := oauth.NewFileStore(tokenDir)
		if err != nil {
			return nil, nil, fmt.Errorf("创建 token 存储失败: %w", err)
		}
		return &oauthCfg, fileStore, nil
	case "keychain":
		return nil, nil, fmt.Errorf("oauth.storage_backend=keychain 尚未实现，请先使用 storage_backend=file")
	default:
		return nil, nil, fmt.Errorf("不支持的 OAuth 存储后端: %s", oauthCfg.StorageBackend)
	}
}

// getTokenDir 返回 token 存储目录
func getTokenDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "basework", "tokens")
	}
	return filepath.Join(home, ".local", "share", "basework", "tokens")
}

// runAuthLogin 执行 OAuth 登录流程
func runAuthLogin(ctx context.Context, provider string) error {
	oauthCfg, fileStore, err := getOAuthConfig()
	if err != nil {
		return err
	}

	if !oauthCfg.Enabled {
		return fmt.Errorf("OAuth 认证未启用，请在配置文件中设置 oauth.enabled=true")
	}

	// 查找 provider 的 OAuth 配置
	pCfg, ok := oauthCfg.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %q 未配置 OAuth，请在配置文件中添加 oauth.providers.%s", provider, provider)
	}

	// 创建 OAuth 流程
	flow := oauth.NewFlow(pCfg, fileStore, oauthCfg.CallbackPort)

	// 执行授权
	token, err := flow.Authorize(ctx)
	if err != nil {
		return fmt.Errorf("OAuth 授权失败: %w", err)
	}

	fmt.Printf("✅ provider %q 授权成功！\n", provider)
	fmt.Printf("   Token 类型: %s\n", token.TokenType)
	fmt.Printf("   过期时间: %s\n", token.ExpiresAt.Format(time.RFC3339))
	if token.Scope != "" {
		fmt.Printf("   授权范围: %s\n", token.Scope)
	}
	return nil
}

// runAuthStatus 显示所有 provider 的认证状态
func runAuthStatus() error {
	oauthCfg, fileStore, err := getOAuthConfig()
	if err != nil {
		return err
	}

	if !oauthCfg.Enabled {
		fmt.Println("OAuth 认证未启用（配置文件中 oauth.enabled=false）")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Provider\t状态\tToken 过期时间\t范围")
	fmt.Fprintln(w, "--------\t----\t----------------\t----")

	// 从配置文件读取所有已配置的 provider
	configuredProviders := make(map[string]bool)
	for name := range oauthCfg.Providers {
		configuredProviders[name] = true
	}

	// 从 token 存储中读取所有已认证的 provider
	authenticatedProviders, err := fileStore.List()
	if err != nil {
		return fmt.Errorf("读取 token 存储失败: %w", err)
	}

	// 显示所有已知 provider 的状态
	seen := make(map[string]bool)

	// 先显示已认证的 provider
	for _, name := range authenticatedProviders {
		token, loadErr := fileStore.Load(name)
		if loadErr != nil {
			fmt.Fprintf(w, "%s\t⚠️ 加载失败\t-\t-\n", name)
		} else {
			status := "✅ 已认证"
			if token.IsExpired() {
				status = "⚠️ 已过期"
			}
			expiry := token.ExpiresAt.Format(time.RFC3339)
			if token.ExpiresAt.IsZero() {
				expiry = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, status, expiry, token.Scope)
		}
		seen[name] = true
		delete(configuredProviders, name)
	}

	// 再显示已配置但未认证的 provider
	for name := range configuredProviders {
		fmt.Fprintf(w, "%s\t❌ 未认证\t-\t-\n", name)
		seen[name] = true
	}

	if len(seen) == 0 {
		fmt.Fprintln(w, "(无)\t-\t-\t-")
	}

	w.Flush()

	// 显示提示信息
	if len(authenticatedProviders) == 0 && len(configuredProviders) == 0 {
		fmt.Println("\n提示: 使用 `basework auth login <provider>` 进行 OAuth 授权")
		fmt.Println("       或在配置文件中添加 oauth.providers 配置")
	}

	return nil
}

// runAuthLogout 清除指定 provider 的认证信息
func runAuthLogout(provider string) error {
	_, fileStore, err := getOAuthConfig()
	if err != nil {
		return err
	}

	// 检查 token 是否存在
	_, loadErr := fileStore.Load(provider)
	if loadErr != nil {
		return fmt.Errorf("provider %q 未认证或 token 不存在", provider)
	}

	if err := fileStore.Delete(provider); err != nil {
		return fmt.Errorf("清除 %q 的认证信息失败: %w", provider, err)
	}

	fmt.Printf("✅ 已清除 provider %q 的认证信息\n", provider)
	return nil
}
