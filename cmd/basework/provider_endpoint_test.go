package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
)

// 本文件是 CFG-004 的验收测试。
//
// 关键点在「证明请求真的打到配置的端点」：改动前配置里的 base_url 没有任何路径
// 进入请求，请求静默落到内置默认端点，只在最后表现为一个误导性的鉴权错误。
// 因此这里不看配置解析结果，而是起一个本地假端点，看它是否真的收到了请求。

// isolatedConfig 返回一份隔离了宿主环境的配置快照。
//
// HOME 指向临时目录，避免测试写到真实会话目录；BASEWORK_* 与 key 环境变量清空，
// 让「来源」判定只取决于本测试给出的内容。
func isolatedConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASEWORK_PROVIDER", "")
	t.Setenv("BASEWORK_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.json")).Get()
	// 重试会让「不可达端点」类用例白等几秒退避；这里测的是端点选择，不是重试。
	cfg.Retry.Enabled = false
	return cfg
}

// fakeChatEndpoint 起一个只回一句「ok」的 OpenAI 兼容流式端点。
// 返回的端点会记录收到的请求路径与 Authorization 头。
func fakeChatEndpoint(t *testing.T) (*httptest.Server, *int64, *[]string) {
	t.Helper()
	var hits int64
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		paths = append(paths, r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer sk-cfg004-test" {
			t.Errorf("Authorization 头未带上配置的 key: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &paths
}

// TestRuntimeAgent_UsesConfiguredBaseURL 配置里的自定义端点必须真的被请求到。
//
// 这是 CFG-004 第一条验收：请求打到该端点而不是默认端点。
func TestRuntimeAgent_UsesConfiguredBaseURL(t *testing.T) {
	cfg := isolatedConfig(t)
	srv, hits, paths := fakeChatEndpoint(t)

	cfg.Provider = "openai"
	cfg.Model = "gpt-4o-mini"
	cfg.Providers = map[string]config.ProviderEndpoint{
		"openai": {BaseURL: srv.URL, APIKey: "sk-cfg004-test"},
	}

	rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err != nil {
		t.Fatalf("构造运行时失败: %v", err)
	}
	svc := rt.Service()
	if svc == nil {
		t.Fatal("运行服务为空")
	}
	defer svc.Close()

	run, err := svc.Start(context.Background(), "只回复 ok")
	if err != nil {
		t.Fatalf("启动运行失败: %v", err)
	}
	if run.Err != nil {
		t.Fatalf("运行出错: %v", run.Err)
	}
	if atomic.LoadInt64(hits) == 0 {
		t.Fatal("配置的 base_url 未生效：本地假端点没有收到任何请求（请求落到了默认端点）")
	}
	if len(*paths) == 0 || !strings.HasSuffix((*paths)[0], "/chat/completions") {
		t.Fatalf("请求路径不符合 OpenAI 兼容约定: %v", *paths)
	}
}

// TestRuntimeAgent_UnreachableEndpointFailsLoudly 端点不可达时必须明确失败，
// 且错误指向配置的端点——不许静默退回默认端点（那会变成误导性的 401）。
func TestRuntimeAgent_UnreachableEndpointFailsLoudly(t *testing.T) {
	cfg := isolatedConfig(t)

	// 占一个端口再立刻释放，得到一个「本机必然连不上」的地址。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听临时端口失败: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg.Provider = "openai"
	cfg.Model = "gpt-4o-mini"
	cfg.Providers = map[string]config.ProviderEndpoint{
		"openai": {BaseURL: "http://" + addr + "/v1", APIKey: "sk-cfg004-test"},
	}

	rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err != nil {
		t.Fatalf("构造运行时失败（端点形态合法，不该在这一步失败）: %v", err)
	}
	svc := rt.Service()
	defer svc.Close()

	run, startErr := svc.Start(context.Background(), "只回复 ok")
	msg := ""
	if startErr != nil {
		msg = startErr.Error()
	}
	if run != nil && run.Err != nil {
		msg += " " + run.Err.Error()
	}
	if strings.TrimSpace(msg) == "" {
		t.Fatal("不可达端点竟被当成成功：既没有启动错误也没有运行错误")
	}
	if !strings.Contains(msg, addr) {
		t.Errorf("错误信息应指向配置的端点 %s（证明没有静默退回默认端点）: %s", addr, msg)
	}
	if strings.Contains(msg, "api.openai.com") {
		t.Errorf("错误信息里出现了默认端点，说明请求被打到了默认端点: %s", msg)
	}
}

// TestRuntimeAgent_MissingKeyForCustomEndpoint 配了自定义端点却没有 key 时，
// 必须在构造阶段就明确失败，而不是静默换用别的来源继续。
func TestRuntimeAgent_MissingKeyForCustomEndpoint(t *testing.T) {
	cfg := isolatedConfig(t)
	cfg.Provider = "openai"
	cfg.Model = "gpt-4o-mini"
	cfg.Providers = map[string]config.ProviderEndpoint{
		// 只有端点，没有 key；环境变量也已在 isolatedConfig 里清空。
		"openai": {BaseURL: "https://gw.invalid.example.com/v1"},
	}

	_, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err == nil {
		t.Fatal("配置了自定义端点但缺 key，应启动失败而不是静默继续")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("错误信息应说明缺 key: %v", err)
	}
}

// TestRuntimeAgent_OpenAICompatWithoutBaseURLFails 未提供 base_url 的
// openai-compat 没有默认端点可用，必须报错（既有行为，回归防线）。
//
// 这里刻意给了一个 key：否则先报的是「缺 key」，掩盖了本用例要证的
// 「没有默认端点可回落」这条性质。两条都属于「明确报错而非静默继续」。
func TestRuntimeAgent_OpenAICompatWithoutBaseURLFails(t *testing.T) {
	cfg := isolatedConfig(t)
	cfg.Provider = "openai-compat"
	cfg.Model = "some-model"
	t.Setenv("OPENAI_API_KEY", "sk-cfg004-test")

	if _, err := newRuntimeAgent(cfg, runtimeAgentOptions{}); err == nil {
		t.Fatal("openai-compat 缺少 base_url 应报错")
	} else if !strings.Contains(err.Error(), "BaseURL") {
		t.Errorf("错误信息应指向缺失的 BaseURL: %v", err)
	}
}

// TestRuntimeAgent_MalformedEnvBaseURLFailsFast 环境变量给出的端点写错时，
// 启动阶段就报错，并且错误里指明是环境变量来源。
func TestRuntimeAgent_MalformedEnvBaseURLFailsFast(t *testing.T) {
	cfg := isolatedConfig(t)
	cfg.Provider = "openai"
	cfg.Model = "gpt-4o-mini"
	t.Setenv("BASEWORK_BASE_URL", "127.0.0.1:11434") // 漏了 scheme

	_, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err == nil {
		t.Fatal("形态非法的 BASEWORK_BASE_URL 应导致启动失败")
	}
	if !strings.Contains(err.Error(), "BASEWORK_BASE_URL") {
		t.Errorf("错误信息应指明来源是环境变量: %v", err)
	}
}

// TestRuntimeAgent_EnvBaseURLOverridesConfig 环境变量端点覆盖配置文件端点。
func TestRuntimeAgent_EnvBaseURLOverridesConfig(t *testing.T) {
	cfg := isolatedConfig(t)
	srv, hits, _ := fakeChatEndpoint(t)

	cfg.Provider = "openai"
	cfg.Model = "gpt-4o-mini"
	cfg.Providers = map[string]config.ProviderEndpoint{
		"openai": {BaseURL: "http://127.0.0.1:1/v1", APIKey: "sk-cfg004-test"},
	}
	t.Setenv("BASEWORK_BASE_URL", srv.URL)

	rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err != nil {
		t.Fatalf("构造运行时失败: %v", err)
	}
	svc := rt.Service()
	defer svc.Close()

	run, err := svc.Start(context.Background(), "只回复 ok")
	if err != nil {
		t.Fatalf("启动运行失败: %v", err)
	}
	if run.Err != nil {
		t.Fatalf("运行出错（说明用的是配置文件里那个不可达端点，而非环境变量端点）: %v", run.Err)
	}
	if atomic.LoadInt64(hits) == 0 {
		t.Fatal("环境变量端点没有收到请求")
	}
}

// TestProviderAPIKey_PrefersProviderEndpoint 配置文件里的 provider 级 key
// 优先于 provider 专属块与环境变量。
func TestProviderAPIKey_PrefersProviderEndpoint(t *testing.T) {
	cfg := config.NewStore("").Get()
	cfg.Providers = map[string]config.ProviderEndpoint{
		"opencode": {APIKey: "sk-from-providers-map"},
	}
	cfg.OpenCode.APIKey = "sk-from-opencode-block"
	t.Setenv("OPENCODE_API_KEY", "sk-from-env")

	if got := providerAPIKey(cfg, "opencode"); got != "sk-from-providers-map" {
		t.Errorf("provider 级 key 应优先: got %q", got)
	}
}

// TestProviderAPIKey_FallsBackToEnv 未配置 provider 级 key 时保持旧行为：
// provider 专属块优先，其次环境变量。
func TestProviderAPIKey_FallsBackToEnv(t *testing.T) {
	cfg := config.NewStore("").Get()
	cfg.OpenCode.APIKey = "sk-from-opencode-block"
	t.Setenv("OPENCODE_API_KEY", "sk-from-env")
	if got := providerAPIKey(cfg, "opencode"); got != "sk-from-opencode-block" {
		t.Errorf("无 provider 级 key 时应沿用 provider 专属块: got %q", got)
	}

	cfg.OpenCode.APIKey = ""
	if got := providerAPIKey(cfg, "opencode"); got != "sk-from-env" {
		t.Errorf("都没有时应回落到环境变量: got %q", got)
	}
}

// TestBuildProviderOptions_OllamaEndpointPrecedence 同时写了 ollama.endpoint 与
// providers.ollama.base_url 时，以 provider 级 base_url 为准（ADR 0007）：
// 不能再向 provider 下发旧的 endpoint，否则 explain 与实际会分叉。
func TestBuildProviderOptions_OllamaEndpointPrecedence(t *testing.T) {
	cfg := config.NewStore("").Get()
	cfg.Ollama.Endpoint = "http://legacy.example.com:11434"

	// 未配置 CFG-004 端点 → 保留旧行为。
	opts := buildProviderOptions(cfg, "ollama", config.Endpoint{Source: config.EndpointSourceDefault})
	if opts["endpoint"] != "http://legacy.example.com:11434" {
		t.Errorf("未配置自定义端点时应沿用 ollama.endpoint: %v", opts["endpoint"])
	}

	// 配置了 CFG-004 端点 → 不下发旧 endpoint，由 BaseURL 生效。
	opts = buildProviderOptions(cfg, "ollama", config.Endpoint{
		BaseURL: "http://new.example.com:11434", Source: config.EndpointSourceConfig,
	})
	if _, ok := opts["endpoint"]; ok {
		t.Errorf("provider 级 base_url 存在时不应再下发旧 endpoint: %v", opts)
	}
}

// TestConfigExplain_ShowsCustomEndpointAndRedactsKey explain 必须展示生效端点
// （URL 不是秘密，排查需要），同时绝不显示 key。
func TestConfigExplain_ShowsCustomEndpointAndRedactsKey(t *testing.T) {
	const secret = "sk-cfg004-explain-4b81"
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	raw, err := json.Marshal(map[string]any{
		"provider": "openai",
		"model":    "agnes-2.5-flash",
		"providers": map[string]any{
			"openai": map[string]any{
				"base_url": "https://gw.example.com/v1",
				"api_key":  secret,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("写配置: %v", err)
	}

	t.Setenv("BASEWORK_PROVIDER", "")
	t.Setenv("BASEWORK_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")

	store, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	var out bytes.Buffer
	if err := renderConfigExplain(&out, cfgPath, store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	text := out.String()

	if strings.Contains(text, secret) {
		t.Errorf("API key 泄漏到 explain 输出:\n%s", text)
	}
	if !strings.Contains(text, "https://gw.example.com/v1") {
		t.Errorf("应展示生效 base_url:\n%s", text)
	}
	if !strings.Contains(text, "来源: 配置文件 providers.openai.base_url") {
		t.Errorf("应说明端点来源:\n%s", text)
	}
	if !strings.Contains(text, "已设置（来源：配置文件）") {
		t.Errorf("providers.openai.api_key 应显示为配置文件来源:\n%s", text)
	}
	if !strings.Contains(text, config.RedactionMarker) {
		t.Errorf("应包含脱敏占位符:\n%s", text)
	}
}

// TestConfigExplain_EnvBaseURLWins explain 里的端点来源必须与 runtime 同口径：
// BASEWORK_BASE_URL 覆盖配置文件值。
func TestConfigExplain_EnvBaseURLWins(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	raw, err := json.Marshal(map[string]any{
		"provider":  "openai",
		"providers": map[string]any{"openai": map[string]any{"base_url": "https://from-config.example.com/v1"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("写配置: %v", err)
	}

	t.Setenv("BASEWORK_PROVIDER", "")
	t.Setenv("BASEWORK_BASE_URL", "https://from-env.example.com/v1")
	t.Setenv("OPENAI_API_KEY", "")

	store, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	var out bytes.Buffer
	if err := renderConfigExplain(&out, cfgPath, store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	text := out.String()

	if !strings.Contains(text, "https://from-env.example.com/v1") {
		t.Errorf("环境变量端点应生效:\n%s", text)
	}
	if strings.Contains(text, "生效 base_url: https://from-config.example.com/v1") {
		t.Errorf("配置文件端点不应被展示为生效值:\n%s", text)
	}
	if !strings.Contains(text, "来源: 环境变量 BASEWORK_BASE_URL") {
		t.Errorf("应标明来源为环境变量:\n%s", text)
	}
}

// TestConfigExplain_UnconfiguredEndpointUnchanged 未配置自定义端点时，
// explain 必须明确说明「使用内置默认端点」，且旧配置行为不变。
func TestConfigExplain_UnconfiguredEndpointUnchanged(t *testing.T) {
	t.Setenv("BASEWORK_PROVIDER", "")
	t.Setenv("BASEWORK_BASE_URL", "")
	dir := t.TempDir()

	store := config.NewStore(filepath.Join(dir, "config.json"))
	var out bytes.Buffer
	if err := renderConfigExplain(&out, filepath.Join(dir, "config.json"), store.Get()); err != nil {
		t.Fatalf("renderConfigExplain: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "未配置 → provider") || !strings.Contains(text, "使用内置默认端点") {
		t.Errorf("应说明使用内置默认端点:\n%s", text)
	}
}
