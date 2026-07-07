package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/config"
)

// newTestWebFetch 创建允许 127.0.0.1 的 WebFetchTool（供 httptest 使用）
func newTestWebFetch() *WebFetchTool {
	return NewWebFetchTool(&config.Config{
		Tools: config.ToolsConfig{
			WebFetch: config.WebFetchConfig{
				AllowedInternalHosts: []string{"127.0.0.1"},
			},
		},
	})
}

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool(nil)
	if tool.Name() != "web_fetch" {
		t.Errorf("期望 Name='web_fetch', 得到 '%s'", tool.Name())
	}
}

func TestWebFetchTool_Description(t *testing.T) {
	tool := NewWebFetchTool(nil)
	if tool.Description() == "" {
		t.Error("期望 Description 非空")
	}
}

func TestWebFetchTool_Parameters(t *testing.T) {
	tool := NewWebFetchTool(nil)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("期望 Parameters 非空")
	}
}

func TestWebFetchTool_Execute_MissingURL(t *testing.T) {
	tool := NewWebFetchTool(nil)
	result, err := tool.Execute(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
	if !strings.Contains(result.Content, "不能为空") {
		t.Errorf("期望包含'不能为空', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_InvalidFormat(t *testing.T) {
	tool := NewWebFetchTool(nil)
	result, err := tool.Execute(context.Background(), []byte(`{"url": "http://example.com", "format": "xml"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
	if !strings.Contains(result.Content, "不支持的格式") {
		t.Errorf("期望包含'不支持的格式', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_InvalidParams(t *testing.T) {
	tool := NewWebFetchTool(nil)
	result, err := tool.Execute(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestWebFetchTool_Execute_SuccessText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><h1>Hello World</h1><p>This is a test page.</p></body></html>`))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`", "format": "text"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Hello World") {
		t.Errorf("期望包含 'Hello World', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "This is a test page") {
		t.Errorf("期望包含 'This is a test page', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_SuccessMarkdown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><h1>Title</h1><p>Some <strong>bold</strong> text.</p></body></html>`))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`", "format": "markdown"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Title") {
		t.Errorf("期望包含 'Title', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_SuccessHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><h1>Raw HTML</h1></body></html>`))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`", "format": "html"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "<h1>Raw HTML</h1>") {
		t.Errorf("期望包含 '<h1>Raw HTML</h1>', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
	if !strings.Contains(result.Content, "404") {
		t.Errorf("期望包含 '404', 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_SizeLimit(t *testing.T) {
	// 生成超过 100KB 的内容
	largeContent := strings.Repeat("A", 120*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeContent))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`", "format": "text"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "截断") {
		t.Errorf("期望包含截断警告, 得到: %s", result.Content)
	}
}

func TestWebFetchTool_Execute_Timeout(t *testing.T) {
	// 使用带超时的 context 来模拟超时场景
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 使用慢响应服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(ctx, []byte(`{"url": "`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple text",
			input:    "<html><body><p>Hello World</p></body></html>",
			expected: "Hello World",
		},
		{
			name:     "multiple paragraphs",
			input:    "<html><body><p>First</p><p>Second</p></body></html>",
			expected: "First\n\n Second",
		},
		{
			name:     "nested elements",
			input:    "<div><h1>Title</h1><p>Content</p></div>",
			expected: "Title\n\n Content",
		},
		{
			name:     "invalid html",
			input:    "<p>Broken <span>html</p>",
			expected: "Broken html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := htmlToText(tt.input)
			if result != tt.expected {
				t.Errorf("期望 '%s', 得到 '%s'", tt.expected, result)
			}
		})
	}
}

func TestWebFetchTool_Execute_DefaultFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><p>Default format</p></body></html>`))
	}))
	defer ts.Close()

	tool := newTestWebFetch()
	result, err := tool.Execute(context.Background(), []byte(`{"url": "`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Default format") {
		t.Errorf("期望包含 'Default format', 得到: %s", result.Content)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<b>Bold</b>", "Bold"},
		{"<a href='x'>Link</a>", "Link"},
		{"Text<br/>More", "TextMore"},
	}
	for _, tt := range tests {
		result := stripHTMLTags(tt.input)
		if result != tt.expected {
			t.Errorf("stripHTMLTags(%q) = %q, 期望 %q", tt.input, result, tt.expected)
		}
	}
}
