// Package tools 实现 basework 的内置工具集合
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/tool"
	"golang.org/x/net/html"
)

// WebFetchTool 实现 web_fetch 工具，用于获取并提取网页内容
type WebFetchTool struct {
	cfg *config.Config
}

// NewWebFetchTool 创建 web_fetch 工具
func NewWebFetchTool(cfg *config.Config) *WebFetchTool {
	return &WebFetchTool{cfg: cfg}
}

// Name 返回工具名称
func (w *WebFetchTool) Name() string {
	return "web_fetch"
}

// Description 返回工具描述
func (w *WebFetchTool) Description() string {
	return "获取指定 URL 的内容并提取文本信息"
}

// Parameters 返回工具参数 JSON Schema
func (w *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "要获取内容的 URL"
			},
			"format": {
				"type": "string",
				"enum": ["text", "markdown", "html"],
				"description": "返回格式：text（纯文本）、markdown（Markdown）、html（原始 HTML）",
				"default": "text"
			}
		},
		"required": ["url"]
	}`)
}

// webFetchParams 工具参数结构
type webFetchParams struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

const (
	maxFetchSize = 100 * 1024 // 100KB
	fetchTimeout = 30 * time.Second
)

// Execute 执行 web_fetch 工具
func (w *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params webFetchParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if params.URL == "" {
		return &tool.Result{
			Content: "参数 'url' 不能为空",
			IsError: true,
		}, nil
	}

	// 默认格式为 text
	if params.Format == "" {
		params.Format = "text"
	}

	// 验证格式
	validFormats := map[string]bool{"text": true, "markdown": true, "html": true}
	if !validFormats[params.Format] {
		return &tool.Result{
			Content: fmt.Sprintf("不支持的格式 '%s'，可选值：text, markdown, html", params.Format),
			IsError: true,
		}, nil
	}

	// 解析 URL 并验证协议
	reqURL, err := url.ParseRequestURI(params.URL)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("无效的 URL: %v", err),
			IsError: true,
		}, nil
	}

	// 协议白名单：只允许 http 和 https
	if reqURL.Scheme != "http" && reqURL.Scheme != "https" {
		return &tool.Result{
			Content: fmt.Sprintf("不支持的协议: %s（仅允许 http 和 https）", reqURL.Scheme),
			IsError: true,
		}, nil
	}

	// 获取允许的内部主机列表
	allowedHosts := getAllowedHosts(w.cfg)

	// 创建带 SSRF 防护的 HTTP 客户端
	client := &http.Client{
		Timeout:   fetchTimeout,
		Transport: newSSRFGuardedTransport(allowedHosts),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("创建请求失败: %v", err),
			IsError: true,
		}, nil
	}

	// 设置 User-Agent
	req.Header.Set("User-Agent", "Basework-WebFetch/1.0")

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("请求失败: %v", err),
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	// 读取响应体，限制大小
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize+1))
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("读取响应失败: %v", err),
			IsError: true,
		}, nil
	}

	// 检查大小限制
	truncated := false
	if len(body) > maxFetchSize {
		body = body[:maxFetchSize]
		truncated = true
	}

	// 检查 HTTP 状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		content := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		if truncated {
			content += "\n\n⚠️ 警告：响应内容已截断（超过 100KB 限制）"
		}
		return &tool.Result{
			Content: content,
			IsError: true,
		}, nil
	}

	// 根据格式处理内容
	var content string
	switch params.Format {
	case "html":
		content = string(body)
	case "text":
		content = htmlToText(string(body))
	case "markdown":
		content = htmlToMarkdown(string(body))
	}

	if truncated {
		content += "\n\n⚠️ 警告：响应内容已截断（超过 100KB 限制）"
	}

	return &tool.Result{
		Content: content,
	}, nil
}

// htmlToText 将 HTML 转换为纯文本
func htmlToText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		// 解析失败时返回原始内容（去除标签）
		return stripHTMLTags(htmlContent)
	}

	var buf bytes.Buffer
	extractText(doc, &buf)
	return strings.TrimSpace(buf.String())
}

// extractText 递归提取 HTML 节点中的文本
func extractText(n *html.Node, buf *bytes.Buffer) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(text)
		}
	}

	// 块级元素后添加换行
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6",
			"li", "tr", "td", "th", "blockquote", "pre":
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, buf)
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6",
			"li", "tr", "td", "th", "blockquote", "pre":
			buf.WriteByte('\n')
		}
	}
}

// htmlToMarkdown 将 HTML 转换为简单的 Markdown
func htmlToMarkdown(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return stripHTMLTags(htmlContent)
	}

	var buf bytes.Buffer
	extractMarkdown(doc, &buf)
	return strings.TrimSpace(buf.String())
}

// extractMarkdown 递归提取 HTML 节点并生成 Markdown
func extractMarkdown(n *html.Node, buf *bytes.Buffer) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			buf.WriteString(text)
		}
		return
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "h1":
			buf.WriteString("\n# ")
		case "h2":
			buf.WriteString("\n## ")
		case "h3":
			buf.WriteString("\n### ")
		case "h4":
			buf.WriteString("\n#### ")
		case "h5":
			buf.WriteString("\n##### ")
		case "h6":
			buf.WriteString("\n###### ")
		case "li":
			buf.WriteString("\n- ")
		case "blockquote":
			buf.WriteString("\n> ")
		case "code", "pre":
			buf.WriteString("`")
		case "strong", "b":
			buf.WriteString("**")
		case "em", "i":
			buf.WriteString("*")
		case "br":
			buf.WriteString("\n")
		case "hr":
			buf.WriteString("\n---\n")
		case "a":
			// 链接单独处理，先收集文本
			var linkText bytes.Buffer
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extractMarkdownForLink(c, &linkText)
			}
			href := getAttr(n, "href")
			if href != "" && linkText.Len() > 0 {
				buf.WriteString("[")
				buf.WriteString(strings.TrimSpace(linkText.String()))
				buf.WriteString("](")
				buf.WriteString(href)
				buf.WriteString(")")
			} else if href != "" {
				buf.WriteString(href)
			} else if linkText.Len() > 0 {
				buf.WriteString(strings.TrimSpace(linkText.String()))
			}
			return // 子节点已处理
		case "img":
			alt := getAttr(n, "alt")
			src := getAttr(n, "src")
			if src != "" {
				buf.WriteString("![")
				buf.WriteString(alt)
				buf.WriteString("](")
				buf.WriteString(src)
				buf.WriteString(")")
			}
			return
		case "p", "div":
			// 段落前加换行
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractMarkdown(c, buf)
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			buf.WriteString("\n")
		case "li":
			buf.WriteString("\n")
		case "p", "div":
			buf.WriteString("\n\n")
		case "blockquote":
			buf.WriteString("\n\n")
		case "code", "pre":
			buf.WriteString("`")
		case "strong", "b":
			buf.WriteString("**")
		case "em", "i":
			buf.WriteString("*")
		}
	}
}

// extractMarkdownForLink 为链接提取文本（不递归处理链接本身）
func extractMarkdownForLink(n *html.Node, buf *bytes.Buffer) {
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractMarkdownForLink(c, buf)
	}
}

// getAttr 获取 HTML 节点的属性值
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// getAllowedHosts 从配置中提取允许的内部主机列表
func getAllowedHosts(cfg *config.Config) []string {
	if cfg != nil {
		return cfg.Tools.WebFetch.AllowedInternalHosts
	}
	return nil
}

// newSSRFGuardedTransport 创建带有 SSRF 防护的 HTTP Transport
func newSSRFGuardedTransport(allowedHosts []string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			// 检查是否在白名单中
			for _, allowed := range allowedHosts {
				if host == allowed {
					return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
				}
			}
			// 解析 DNS 并检查 IP
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isBlockedIP(ip.IP) {
					return nil, fmt.Errorf("SSRF 已被阻止: %s 解析到被阻止的地址 %s", host, ip.IP)
				}
			}
			return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
		},
	}
}

// isBlockedIP 检查 IP 是否为内网/回环/链路本地地址
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// stripHTMLTags 使用简单方法去除 HTML 标签
func stripHTMLTags(s string) string {
	var buf bytes.Buffer
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}
	return strings.TrimSpace(buf.String())
}