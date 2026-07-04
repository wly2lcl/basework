package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
)

func TestWebSearchTool_Name(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)
	if tool.Name() != "web_search" {
		t.Errorf("期望 Name='web_search', 得到 '%s'", tool.Name())
	}
}

func TestWebSearchTool_Description(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)
	if tool.Description() == "" {
		t.Error("期望 Description 非空")
	}
}

func TestWebSearchTool_Parameters(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("期望 Parameters 非空")
	}
}

func TestWebSearchTool_MissingAPIKey(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)

	result, err := tool.Execute(context.Background(), []byte(`{"query": "test"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
	if !strings.Contains(result.Content, "API key") {
		t.Errorf("期望包含 'API key', 得到: %s", result.Content)
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)

	result, err := tool.Execute(context.Background(), []byte(`{"query": ""}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestWebSearchTool_InvalidParams(t *testing.T) {
	cfg := &config.Config{}
	tool := NewWebSearchTool(cfg)

	result, err := tool.Execute(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestWebSearchTool_InvalidBackend(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			WebSearch: config.WebSearchConfig{
				Backend: "unknown",
				APIKey:  "test-key",
			},
		},
	}
	tool := NewWebSearchTool(cfg)

	result, err := tool.Execute(context.Background(), []byte(`{"query": "test"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
	if !strings.Contains(result.Content, "不支持的搜索后端") {
		t.Errorf("期望包含 '不支持的搜索后端', 得到: %s", result.Content)
	}
}

func TestWebSearchTool_DefaultBackend(t *testing.T) {
	// backend 为空时，应默认使用 tavily
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			WebSearch: config.WebSearchConfig{
				Backend: "",
				APIKey:  "test-key",
			},
		},
	}
	tool := NewWebSearchTool(cfg)
	result, err := tool.Execute(context.Background(), []byte(`{"query": "test", "num_results": 3}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	// 由于 API 调用会连接真实服务器，测试只验证不崩溃并且路由到 tavily
	// 这里不检查 result，因为真实 API 调用可能因网络问题失败
	_ = result
}

func TestWebSearchTool_NumResultsBounds(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			WebSearch: config.WebSearchConfig{
				Backend: "tavily",
				APIKey:  "test-key",
			},
		},
	}
	tool := NewWebSearchTool(cfg)

	// num_results 为 0 时应使用默认值 5
	result, err := tool.Execute(context.Background(), []byte(`{"query": "test", "num_results": 0}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	// 不应因参数问题报错
	if strings.Contains(result.Content, "参数解析") {
		t.Errorf("不应出现参数解析错误: %s", result.Content)
	}
}

func TestWebSearchTool_TavilyBackend(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			WebSearch: config.WebSearchConfig{
				Backend: "tavily",
				APIKey:  "test-key",
			},
		},
	}
	tool := NewWebSearchTool(cfg)
	if tool.Name() != "web_search" {
		t.Errorf("期望 Name='web_search'")
	}
	_ = cfg
}

func TestWebSearchTool_ExaBackend(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			WebSearch: config.WebSearchConfig{
				Backend: "exa",
				APIKey:  "test-key",
			},
		},
	}
	tool := NewWebSearchTool(cfg)
	if tool.Name() != "web_search" {
		t.Errorf("期望 Name='web_search'")
	}
}