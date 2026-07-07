package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"golang.org/x/oauth2"
)

// TestCopilot_NewProvider 验证创建 Copilot Provider
func TestCopilot_NewProvider(t *testing.T) {
	p, err := NewCopilotProvider(CopilotConfigOpts{
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}
	if p.Name() != "copilot" {
		t.Errorf("expected name 'copilot', got '%s'", p.Name())
	}
	if p.ID() != "gpt-4" {
		t.Errorf("expected ID 'gpt-4', got '%s'", p.ID())
	}
}

// TestCopilot_DefaultModel 验证默认模型
func TestCopilot_DefaultModel(t *testing.T) {
	p, err := NewCopilotProvider(CopilotConfigOpts{})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}
	if p.ID() != "gpt-4" {
		t.Errorf("expected default model 'gpt-4', got '%s'", p.ID())
	}
}

// TestCopilot_TokenPath 验证默认 token 路径
func TestCopilot_TokenPath(t *testing.T) {
	p, err := NewCopilotProvider(CopilotConfigOpts{})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}
	if p.tokenPath == "" {
		t.Error("expected non-empty token path")
	}
	if !strings.HasSuffix(p.tokenPath, "copilot_token.json") {
		t.Errorf("expected token path to end with 'copilot_token.json', got '%s'", p.tokenPath)
	}
}

// TestCopilot_TokenManagement 验证 token 保存和加载
func TestCopilot_TokenManagement(t *testing.T) {
	dir, err := os.MkdirTemp("", "copilot-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	tokenPath := filepath.Join(dir, "token.json")

	p, err := NewCopilotProvider(CopilotConfigOpts{
		TokenPath: tokenPath,
	})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}

	// 设置 token
	p.token = &oauth2.Token{
		AccessToken: "test-token-123",
		TokenType:   "bearer",
	}

	// 保存
	if err := p.saveToken(); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Fatal("expected token file to exist")
	}

	// 创建新 provider 加载 token
	p2, err := NewCopilotProvider(CopilotConfigOpts{
		TokenPath: tokenPath,
	})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}

	if p2.token == nil {
		t.Fatal("expected token to be loaded from file")
	}
	if p2.token.AccessToken != "test-token-123" {
		t.Errorf("expected access token 'test-token-123', got '%s'", p2.token.AccessToken)
	}
}

// TestCopilot_RequestHeaders 验证请求头
func TestCopilot_RequestHeaders(t *testing.T) {
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","object":"chat.completion","created":123,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	p, err := NewCopilotProvider(CopilotConfigOpts{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("NewCopilotProvider failed: %v", err)
	}
	p.token = &oauth2.Token{AccessToken: "test-token"}

	// 使用 httptest server 替代真实端点
	origEndpoint := copilotEndpoint
	copilotEndpoint = srv.URL
	defer func() { copilotEndpoint = origEndpoint }()

	_, err = p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if capturedHeaders.Get("Editor-Version") == "" {
		t.Error("expected Editor-Version header")
	}
	if capturedHeaders.Get("Editor-Plugin-Version") == "" {
		t.Error("expected Editor-Plugin-Version header")
	}
	if capturedHeaders.Get("Copilot-Integration-Id") != "basework" {
		t.Errorf("expected Copilot-Integration-Id 'basework', got '%s'", capturedHeaders.Get("Copilot-Integration-Id"))
	}
	if capturedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Errorf("expected Authorization header, got '%s'", capturedHeaders.Get("Authorization"))
	}
}

// TestCopilot_Generate 验证非流式请求
func TestCopilot_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		json.Unmarshal(body, &reqMap)
		if reqMap["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %v", reqMap["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123,
			"model": "gpt-4",
			"choices": [{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],
			"usage": {"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`)
	}))
	defer srv.Close()

	p, _ := NewCopilotProvider(CopilotConfigOpts{Model: "gpt-4"})
	p.token = &oauth2.Token{AccessToken: "test-token"}
	origEndpoint := copilotEndpoint
	copilotEndpoint = srv.URL
	defer func() { copilotEndpoint = origEndpoint }()

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "Hello!" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

// TestCopilot_Unauthenticated 验证未认证时出错
func TestCopilot_Unauthenticated(t *testing.T) {
	p, _ := NewCopilotProvider(CopilotConfigOpts{Model: "gpt-4"})
	// 确保 token 为 nil（覆盖环境中有 token 文件的情况）
	p.token = nil
	p.tokenSrc = nil

	_, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for unauthenticated provider")
	}
	if !strings.Contains(err.Error(), "未认证") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

// TestCopilot_Models 验证模型列表
func TestCopilot_Models(t *testing.T) {
	p, _ := NewCopilotProvider(CopilotConfigOpts{})
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
	found := false
	for _, m := range models {
		if m == "gpt-4" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'gpt-4' in models, got %v", models)
	}
}

// TestCopilot_Supports 验证能力支持
func TestCopilot_Supports(t *testing.T) {
	p, _ := NewCopilotProvider(CopilotConfigOpts{})
	if !p.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if !p.Supports(llm.CapStreaming) {
		t.Error("expected streaming support")
	}
}
