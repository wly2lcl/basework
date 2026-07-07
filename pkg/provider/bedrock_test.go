package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestBedrock_NewProvider 验证创建 Bedrock Provider
func TestBedrock_NewProvider(t *testing.T) {
	p, err := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test-access",
		SecretKey: "test-secret",
		Region:    "us-west-2",
		Model:     "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("NewBedrockProvider failed: %v", err)
	}
	if p.Name() != "bedrock" {
		t.Errorf("expected name 'bedrock', got '%s'", p.Name())
	}
	if p.ID() != "claude-3-5-sonnet" {
		t.Errorf("expected ID 'claude-3-5-sonnet', got '%s'", p.ID())
	}
	if p.awsModelID != "anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Errorf("unexpected aws model ID: %s", p.awsModelID)
	}
}

// TestBedrock_NewProvider_Validation 验证必填字段
func TestBedrock_NewProvider_Validation(t *testing.T) {
	_, err := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "",
		SecretKey: "secret",
	})
	if err == nil {
		t.Error("expected error for empty access key")
	}

	_, err = NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "access",
		SecretKey: "",
	})
	if err == nil {
		t.Error("expected error for empty secret key")
	}
}

// TestBedrock_DefaultRegion 验证默认区域
func TestBedrock_DefaultRegion(t *testing.T) {
	p, err := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test",
		SecretKey: "test",
	})
	if err != nil {
		t.Fatalf("NewBedrockProvider failed: %v", err)
	}
	if p.region != "us-east-1" {
		t.Errorf("expected default region 'us-east-1', got '%s'", p.region)
	}
}

// TestBedrock_DefaultModel 验证默认模型
func TestBedrock_DefaultModel(t *testing.T) {
	p, err := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test",
		SecretKey: "test",
	})
	if err != nil {
		t.Fatalf("NewBedrockProvider failed: %v", err)
	}
	if p.modelID != "claude-3-5-sonnet" {
		t.Errorf("expected default model 'claude-3-5-sonnet', got '%s'", p.modelID)
	}
}

// TestBedrock_ModelIDMapping 验证模型 ID 映射
func TestBedrock_ModelIDMapping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-3-5-sonnet", "anthropic.claude-3-5-sonnet-20241022-v2:0"},
		{"claude-3-opus", "anthropic.claude-3-opus-20240229-v1:0"},
		{"llama-3-1-70b", "meta.llama3-1-70b-instruct-v1:0"},
		{"custom-model", "custom-model"}, // 未映射的保留原样
	}
	for _, tt := range tests {
		p, err := NewBedrockProvider(BedrockConfigOpts{
			AccessKey: "test",
			SecretKey: "test",
			Model:     tt.input,
		})
		if err != nil {
			t.Fatalf("NewBedrockProvider(%s) failed: %v", tt.input, err)
		}
		if p.awsModelID != tt.want {
			t.Errorf("model %q: expected awsID %q, got %q", tt.input, tt.want, p.awsModelID)
		}
	}
}

// TestBedrock_Models 验证模型列表
func TestBedrock_Models(t *testing.T) {
	p, _ := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test",
		SecretKey: "test",
	})
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
	found := false
	for _, m := range models {
		if m == "claude-3-5-sonnet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'claude-3-5-sonnet' in models, got %v", models)
	}
}

// TestBedrock_SigV4Signing 验证 SigV4 签名头的存在
func TestBedrock_SigV4Signing(t *testing.T) {
	var capturedAuth, capturedDate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedDate = r.Header.Get("x-amz-date")

		// 读取并丢弃请求体
		io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"output": {
				"message": {
					"role": "assistant",
					"content": [{"text": "Hello from Bedrock!"}]
				}
			},
			"stopReason": "end_turn",
			"usage": {"inputTokens": 10, "outputTokens": 5, "totalTokens": 15}
		}`)
	}))
	defer srv.Close()

	p, err := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "AKID123",
		SecretKey: "secret123",
		Region:    "us-east-1",
		Model:     "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("NewBedrockProvider failed: %v", err)
	}

	// 覆盖端点到测试服务器
	p.client = srv.Client()
	p.bedrockURL = srv.URL

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	// 验证签名头
	if capturedAuth == "" {
		t.Error("expected Authorization header with SigV4 signature")
	}
	if !strings.Contains(capturedAuth, "AWS4-HMAC-SHA256") {
		t.Errorf("expected AWS4-HMAC-SHA256 in auth header, got '%s'", capturedAuth)
	}
	if !strings.Contains(capturedAuth, "AKID123") {
		t.Errorf("expected access key in auth header, got '%s'", capturedAuth)
	}
	if capturedDate == "" {
		t.Error("expected x-amz-date header")
	}

	// 验证响应内容
	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "Hello from Bedrock!" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

// TestBedrock_Chat 验证请求构造
func TestBedrock_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]any
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}

		// 验证消息格式
		msgs, ok := reqBody["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Error("expected messages in request")
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got '%s'", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{
			"output": {
				"message": {
					"role": "assistant",
					"content": [{"text": "Response"}]
				}
			},
			"stopReason": "end_turn"
		}`)
	}))
	defer srv.Close()

	p, _ := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test",
		SecretKey: "test",
		Model:     "claude-3-5-sonnet",
	})
	p.client = srv.Client()
	p.bedrockURL = srv.URL
	defer func() { p.bedrockURL = "" }()

	resp, err := p.Chat(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "You are helpful."}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "Hello"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if len(resp.Message.Content) == 0 || resp.Message.Content[0].Text != "Response" {
		t.Errorf("unexpected content: %+v", resp.Message.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
}

// TestBedrock_Supports 验证能力支持
func TestBedrock_Supports(t *testing.T) {
	p, _ := NewBedrockProvider(BedrockConfigOpts{
		AccessKey: "test",
		SecretKey: "test",
	})
	if !p.Supports(llm.CapTools) {
		t.Error("expected tools support")
	}
	if !p.Supports(llm.CapStreaming) {
		t.Error("expected streaming support")
	}
	if p.Supports(llm.CapVision) {
		t.Error("did not expect vision support")
	}
}
