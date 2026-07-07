package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/tool"
)

// WebSearchTool 实现 web_search 工具，用于搜索网络信息
type WebSearchTool struct {
	cfg      *config.Config
	backends map[string]SearchBackend
}

// NewWebSearchTool 创建 web_search 工具
func NewWebSearchTool(cfg *config.Config) *WebSearchTool {
	return NewWebSearchToolWithBackends(cfg, defaultSearchBackends())
}

// SearchBackend 是 web_search 后端适配器接口。
type SearchBackend interface {
	Search(ctx context.Context, apiKey, query string, numResults int) ([]SearchResult, error)
}

// SearchBackendFunc 将函数适配为 SearchBackend，便于测试和扩展。
type SearchBackendFunc func(ctx context.Context, apiKey, query string, numResults int) ([]SearchResult, error)

// Search 调用函数后端。
func (f SearchBackendFunc) Search(ctx context.Context, apiKey, query string, numResults int) ([]SearchResult, error) {
	return f(ctx, apiKey, query, numResults)
}

// NewWebSearchToolWithBackends 创建可注入后端的 web_search 工具。
func NewWebSearchToolWithBackends(cfg *config.Config, backends map[string]SearchBackend) *WebSearchTool {
	if backends == nil {
		backends = defaultSearchBackends()
	}
	return &WebSearchTool{cfg: cfg, backends: backends}
}

func defaultSearchBackends() map[string]SearchBackend {
	return map[string]SearchBackend{
		"tavily": SearchBackendFunc(searchTavily),
		"exa":    SearchBackendFunc(searchExa),
	}
}

// Name 返回工具名称
func (w *WebSearchTool) Name() string {
	return "web_search"
}

// Description 返回工具描述
func (w *WebSearchTool) Description() string {
	return "搜索网络信息，支持 Tavily 和 Exa 搜索后端"
}

// Parameters 返回工具参数 JSON Schema
func (w *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "搜索查询词"
			},
			"num_results": {
				"type": "integer",
				"description": "返回结果数量",
				"default": 5,
				"minimum": 1,
				"maximum": 20
			}
		},
		"required": ["query"]
	}`)
}

// webSearchParams 工具参数结构
type webSearchParams struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
}

// SearchResult 搜索结果的统一结构
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
}

const searchTimeout = 30 * time.Second

// Execute 执行 web_search 工具
func (w *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params webSearchParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if params.Query == "" {
		return &tool.Result{
			Content: "参数 'query' 不能为空",
			IsError: true,
		}, nil
	}

	if params.NumResults <= 0 {
		params.NumResults = 5
	}
	if params.NumResults > 20 {
		params.NumResults = 20
	}

	// 获取配置
	webSearchCfg := w.webSearchConfig()
	backend := webSearchCfg.Backend
	if backend == "" {
		backend = "tavily"
	}

	apiKey := webSearchCfg.APIKey
	if apiKey == "" {
		return &tool.Result{
			Content: "web_search 工具的 API key 未配置，请设置 config.tools.web_search.api_key",
			IsError: true,
		}, nil
	}

	searcher, ok := w.backends[backend]
	if !ok {
		return &tool.Result{
			Content: fmt.Sprintf("不支持的搜索后端 '%s'，可选值：tavily, exa", backend),
			IsError: true,
		}, nil
	}

	results, err := searcher.Search(ctx, apiKey, params.Query, params.NumResults)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("搜索失败: %v", err),
			IsError: true,
		}, nil
	}

	if len(results) == 0 {
		return &tool.Result{
			Content: fmt.Sprintf("未找到与 \"%s\" 相关的结果", params.Query),
		}, nil
	}

	// 渲染结果
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("搜索结果（%s）:\n\n", params.Query))
	for i, r := range results {
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		buf.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Summary != "" {
			buf.WriteString(fmt.Sprintf("   摘要: %s\n", r.Summary))
		}
		buf.WriteString("\n")
	}

	return &tool.Result{
		Content: strings.TrimSpace(buf.String()),
	}, nil
}

func (w *WebSearchTool) webSearchConfig() config.WebSearchConfig {
	if w == nil || w.cfg == nil {
		return config.WebSearchConfig{}
	}
	return w.cfg.Tools.WebSearch
}

// tavilyRequest Tavily API 请求结构
type tavilyRequest struct {
	APIKey     string `json:"api_key"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// tavilyResponse Tavily API 响应结构
type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

func searchTavily(ctx context.Context, apiKey, query string, numResults int) ([]SearchResult, error) {
	reqBody := tavilyRequest{
		APIKey:     apiKey,
		Query:      query,
		MaxResults: numResults,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: searchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tavily API 返回状态 %d: %s", resp.StatusCode, string(respBody))
	}

	var tavilyResp tavilyResponse
	if err := json.Unmarshal(respBody, &tavilyResp); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}

	results := make([]SearchResult, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Summary: r.Content,
		})
	}

	return results, nil
}

// exaRequest Exa API 请求结构
type exaRequest struct {
	Query string `json:"query"`
	Num   int    `json:"num"`
}

// exaResponse Exa API 响应结构
type exaResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
}

func searchExa(ctx context.Context, apiKey, query string, numResults int) ([]SearchResult, error) {
	reqBody := exaRequest{
		Query: query,
		Num:   numResults,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.exa.ai/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: searchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Exa API 返回状态 %d: %s", resp.StatusCode, string(respBody))
	}

	var exaResp exaResponse
	if err := json.Unmarshal(respBody, &exaResp); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}

	results := make([]SearchResult, 0, len(exaResp.Results))
	for _, r := range exaResp.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Summary: r.Summary,
		})
	}

	return results, nil
}
