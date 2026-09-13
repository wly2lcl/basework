package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// 本文件集中处理「自定义模型端点」的配置载体与解析规则（CFG-004）。
//
// 为什么单独成文件：端点解析必须只有一份实现。runtime（provider.Create）与
// `config explain` 都要给出「生效端点」，两处各写一套就会重演 CFG-001 要消除的
// 信任裂缝——explain 说 A、请求实际打到 B。
//
// 设计取舍与优先级链见 docs/adr/0007-custom-provider-endpoint.md。

// ProviderEndpoint 是单个 provider 的自定义端点与凭据（CFG-004）。
//
// 两个字段都可选：
//   - BaseURL 为空 → 该 provider 使用内置默认端点（与本项引入前行为一致）。
//   - APIKey 为空 → 回落到该 provider 的环境变量链。
type ProviderEndpoint struct {
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
}

// ProviderEndpointFor 返回指定 provider 的自定义端点配置，未配置时返回零值。
//
// providerType 必须传「生效 provider」（EffectiveProviderName 的结果）：传配置文件
// 里的 provider 字段，在 BASEWORK_PROVIDER 覆盖时会读到另一份配置块。
func (c *Config) ProviderEndpointFor(providerType string) ProviderEndpoint {
	if c == nil || len(c.Providers) == 0 {
		return ProviderEndpoint{}
	}
	return c.Providers[providerType]
}

// EffectiveProviderName 返回生效的 provider 名称：
// 环境变量 BASEWORK_PROVIDER 优先于配置文件 provider 字段。
func (c *Config) EffectiveProviderName(envProvider string) string {
	if v := strings.TrimSpace(envProvider); v != "" {
		return v
	}
	if c == nil {
		return ""
	}
	return c.Provider
}

// EndpointSource 表示生效端点取值来自哪里。
//
// 单独给出来源而不是只给 URL，是因为「这个 URL 从哪来」在排查时与 URL 本身同等
// 重要：同一个 URL 来自环境变量还是配置文件，决定了该去改哪里。
type EndpointSource string

const (
	// EndpointSourceDefault 未配置自定义端点，使用内置默认端点。
	EndpointSourceDefault EndpointSource = "default"
	// EndpointSourceConfig 来自配置文件 providers.<name>.base_url。
	EndpointSourceConfig EndpointSource = "config"
	// EndpointSourceEnv 来自环境变量 BASEWORK_BASE_URL。
	EndpointSourceEnv EndpointSource = "env"
)

// Endpoint 是解析后的生效端点。
type Endpoint struct {
	BaseURL string
	Source  EndpointSource
}

// IsCustom 表示存在用户显式指定的端点（而非内置默认端点）。
func (e Endpoint) IsCustom() bool { return e.BaseURL != "" }

// ResolveEndpoint 解析生效的自定义端点。
//
// envBaseURL 由调用方注入（BASEWORK_BASE_URL 的取值），因此本函数是纯函数：
// `config explain` 可以用同一份输入复现 runtime 的结果，测试也不必污染进程环境。
//
// 优先级（ADR 0007）：环境变量 > 配置文件 providers.<生效 provider>.base_url。
// 两者都为空时返回 EndpointSourceDefault，表示沿用内置默认端点。
func (c *Config) ResolveEndpoint(providerType, envBaseURL string) Endpoint {
	if v := strings.TrimSpace(envBaseURL); v != "" {
		return Endpoint{BaseURL: v, Source: EndpointSourceEnv}
	}
	if v := strings.TrimSpace(c.ProviderEndpointFor(providerType).BaseURL); v != "" {
		return Endpoint{BaseURL: v, Source: EndpointSourceConfig}
	}
	return Endpoint{Source: EndpointSourceDefault}
}

// ValidateBaseURL 校验自定义端点 URL 的形态。
//
// 只做形态检查，不发任何网络请求：配置加载与启动都不能依赖远端可达。
// 拦的是「写了但写错」这类静默失效——例如漏了 scheme 的 "127.0.0.1:11434"
// 会被 HTTP 客户端当成相对路径处理，报出的错误与本意完全无关。
//
// label 用于在错误里指明来源，例如 "providers.openai.base_url" 或
// "BASEWORK_BASE_URL（环境变量）"。空值合法：表示不覆盖默认端点。
func ValidateBaseURL(label, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s 不是合法 URL: %q", label, raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s 必须以 http:// 或 https:// 开头: %q", label, raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%s 缺少主机名: %q", label, raw)
	}
	return nil
}

// DisplayURL 返回可安全打印的端点 URL。
//
// URL 本身不是秘密，排查时需要看到它，所以不做整体脱敏；但
// `https://user:pass@host` 这类写法会把凭据带进 explain 输出，因此去掉 userinfo。
func DisplayURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.User == nil {
		return trimmed
	}
	u.User = nil
	return u.String()
}

// validateProviderEndpoints 校验配置文件里所有自定义端点的 URL 形态。
//
// 按名字排序后校验，保证报错顺序稳定（map 迭代顺序随机，会让同一份坏配置
// 每次报不同的字段）。
func (c *Config) validateProviderEndpoints() error {
	if c == nil || len(c.Providers) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ValidateBaseURL("providers."+name+".base_url", c.Providers[name].BaseURL); err != nil {
			return err
		}
	}
	return nil
}
