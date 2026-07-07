// Package provider — Amazon Bedrock Provider
// 使用原生 HTTP + AWS SigV4 签名实现（不依赖 AWS SDK）
// 通过 AWS Converse API 调用模型

package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Bedrock 默认区域
const defaultBedrockRegion = "us-east-1"

// Bedrock 模型 ID 映射（简化名 → AWS 模型 ID）
var bedrockModelIDs = map[string]string{
	"claude-3-5-sonnet": "anthropic.claude-3-5-sonnet-20241022-v2:0",
	"claude-3-opus":     "anthropic.claude-3-opus-20240229-v1:0",
	"claude-3-sonnet":   "anthropic.claude-3-sonnet-20240229-v1:0",
	"claude-3-haiku":    "anthropic.claude-3-haiku-20240307-v1:0",
	"claude-4-sonnet":   "anthropic.claude-3-5-sonnet-20241022-v2:0",
	"llama-3-1-70b":     "meta.llama3-1-70b-instruct-v1:0",
	"llama-3-1-8b":      "meta.llama3-1-8b-instruct-v1:0",
	"mistral-large":     "mistral.mistral-large-2402-v1:0",
}

// BedrockProvider 实现 Provider 接口，封装 Amazon Bedrock Converse API
type BedrockProvider struct {
	accessKey        string
	secretKey        string
	region           string
	modelID          string
	awsModelID       string
	client           *http.Client
	bedrockURL       string // 非流式端点 URL（可被测试覆盖）
	bedrockStreamURL string // 流式端点 URL（可被测试覆盖）
}

// BedrockConfigOpts 是 Bedrock Provider 的配置选项
type BedrockConfigOpts struct {
	AccessKey string
	SecretKey string
	Region    string
	Model     string // 简化模型名或完整 AWS 模型 ID
}

// NewBedrockProvider 创建新的 Amazon Bedrock Provider
func NewBedrockProvider(cfg BedrockConfigOpts) (*BedrockProvider, error) {
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("Bedrock access key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("Bedrock secret key is required")
	}
	if cfg.Region == "" {
		cfg.Region = defaultBedrockRegion
	}
	if cfg.Model == "" {
		cfg.Model = "claude-3-5-sonnet"
	}

	// 查找 AWS 模型 ID
	awsModelID, ok := bedrockModelIDs[cfg.Model]
	if !ok {
		awsModelID = cfg.Model
	}

	return &BedrockProvider{
		accessKey:        cfg.AccessKey,
		secretKey:        cfg.SecretKey,
		region:           cfg.Region,
		modelID:          cfg.Model,
		awsModelID:       awsModelID,
		client:           &http.Client{Timeout: 10 * time.Minute},
		bedrockURL:       fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", cfg.Region, awsModelID),
		bedrockStreamURL: fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse-stream", cfg.Region, awsModelID),
	}, nil
}

// Name 返回 Provider 名称
func (p *BedrockProvider) Name() string {
	return "bedrock"
}

// sigV4Sign 对请求进行 AWS SigV4 签名
// 参考: https://docs.aws.amazon.com/general/latest/gr/sigv4_signing.html
func (p *BedrockProvider) sigV4Sign(req *http.Request, body []byte) error {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")

	// 计算 payload hash
	payloadHash := sha256Hex(body)

	// 规范请求
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-date:%s\n", req.Host, amzDate)
	signedHeaders := "host;x-amz-date"

	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// 签名字符串
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/bedrock/aws4_request", date, p.region)
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 派生签名密钥
	signingKey := p.signingKey(date)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// 设置 Authorization 头
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, p.accessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("x-amz-date", amzDate)

	return nil
}

// signingKey 派生 AWS SigV4 签名密钥
func (p *BedrockProvider) signingKey(date string) []byte {
	kSecret := []byte("AWS4" + p.secretKey)
	kDate := hmacSHA256(kSecret, []byte(date))
	kRegion := hmacSHA256(kDate, []byte(p.region))
	kService := hmacSHA256(kRegion, []byte("bedrock"))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// sha256Hex 计算 SHA-256 并返回 hex 字符串
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// hmacSHA256 计算 HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// bedrockMessage 表示 Bedrock Converse API 的消息
type bedrockMessage struct {
	Role    string         `json:"role"`
	Content []bedrockBlock `json:"content"`
}

type bedrockBlock struct {
	Text *string `json:"text,omitempty"`
}

// bedrockRequest 是 Bedrock Converse API 请求体
type bedrockRequest struct {
	Messages        []bedrockMessage        `json:"messages"`
	System          []bedrockSystemBlock    `json:"system,omitempty"`
	InferenceConfig *bedrockInferenceConfig `json:"inferenceConfig,omitempty"`
}

type bedrockSystemBlock struct {
	Text string `json:"text"`
}

type bedrockInferenceConfig struct {
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// bedrockResponse 是 Bedrock Converse API 响应体
type bedrockResponse struct {
	Output     *bedrockOutput `json:"output"`
	StopReason string         `json:"stopReason"`
	Usage      *bedrockUsage  `json:"usage"`
}

type bedrockOutput struct {
	Message bedrockMessage `json:"message"`
}

type bedrockUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// toBedrockMessages 转换内部消息格式为 Bedrock 格式
func toBedrockMessages(msgs []llm.ChatMessage) ([]bedrockMessage, []bedrockSystemBlock) {
	var system []bedrockSystemBlock
	var messages []bedrockMessage

	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			for _, part := range msg.Content {
				if part.Text != "" {
					system = append(system, bedrockSystemBlock{Text: part.Text})
				}
			}
		case llm.RoleUser:
			var blocks []bedrockBlock
			for _, part := range msg.Content {
				if part.Text != "" {
					t := part.Text
					blocks = append(blocks, bedrockBlock{Text: &t})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, bedrockMessage{Role: "user", Content: blocks})
			}
		case llm.RoleAssistant:
			var blocks []bedrockBlock
			for _, part := range msg.Content {
				if part.Text != "" {
					t := part.Text
					blocks = append(blocks, bedrockBlock{Text: &t})
				}
			}
			if len(blocks) == 0 {
				t := ""
				blocks = append(blocks, bedrockBlock{Text: &t})
			}
			messages = append(messages, bedrockMessage{Role: "assistant", Content: blocks})
		case llm.RoleTool:
			// Bedrock Converse API 通过 conversation role 处理工具结果
			var blocks []bedrockBlock
			for _, part := range msg.Content {
				if part.Text != "" {
					t := part.Text
					blocks = append(blocks, bedrockBlock{Text: &t})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, bedrockMessage{Role: "user", Content: blocks})
			}
		}
	}

	return messages, system
}

// buildBedrockRequest 构造 Bedrock Converse API 请求体
func buildBedrockRequest(modelID string, req *llm.Request, messages []bedrockMessage, system []bedrockSystemBlock) bedrockRequest {
	br := bedrockRequest{
		Messages: messages,
	}

	if len(system) > 0 {
		br.System = system
	}

	if req.MaxTokens > 0 || req.Temperature != nil || len(req.Stop) > 0 {
		ic := &bedrockInferenceConfig{}
		if req.MaxTokens > 0 {
			ic.MaxTokens = req.MaxTokens
		}
		if req.Temperature != nil {
			ic.Temperature = req.Temperature
		}
		if len(req.Stop) > 0 {
			ic.StopSequences = req.Stop
		}
		br.InferenceConfig = ic
	}

	return br
}

// Chat 发送非流式聊天请求到 Bedrock Converse API
func (p *BedrockProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	messages, system := toBedrockMessages(req.Messages)
	body := buildBedrockRequest(p.awsModelID, req, messages, system)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.bedrockURL
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// 应用 SigV4 签名
	if err := p.sigV4Sign(httpReq, jsonBody); err != nil {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: fmt.Sprintf("签名失败: %v", err),
		}
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("read response: %v", err),
			StatusCode: 0,
		}
	}

	if resp.StatusCode != 200 {
		return nil, mapHTTPError(resp.StatusCode, respBody, "bedrock")
	}

	return fromBedrockResponse(respBody)
}

// ChatStream 发送流式请求（Bedrock Converse Stream API）
func (p *BedrockProvider) ChatStream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	messages, system := toBedrockMessages(req.Messages)
	body := buildBedrockRequest(p.awsModelID, req, messages, system)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 流式端点
	url := p.bedrockStreamURL
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("create request failed: %v", err),
			StatusCode: 0,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	if err := p.sigV4Sign(httpReq, jsonBody); err != nil {
		return nil, &llm.Error{
			Type:    llm.ErrorTypeAuth,
			Message: fmt.Sprintf("签名失败: %v", err),
		}
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Type:       llm.ErrorTypeNetwork,
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
		}
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, bodyBytes, "bedrock")
	}

	return parseBedrockStream(ctx, resp), nil
}

// fromBedrockResponse 解析 Bedrock Converse API 响应
func fromBedrockResponse(body []byte) (*llm.Response, error) {
	var resp bedrockResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse bedrock response: %w", err)
	}

	if resp.Output == nil {
		return nil, fmt.Errorf("bedrock response: empty output")
	}

	msg := resp.Output.Message
	llmMsg := llm.ChatMessage{
		Role: llm.RoleAssistant,
	}

	for _, block := range msg.Content {
		if block.Text != nil && *block.Text != "" {
			llmMsg.Content = append(llmMsg.Content, llm.ContentPart{
				Type: llm.ContentTypeText,
				Text: *block.Text,
			})
		}
	}

	result := &llm.Response{
		Message:      llmMsg,
		FinishReason: mapBedrockFinishReason(resp.StopReason),
	}

	if resp.Usage != nil {
		result.Usage = llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return result, nil
}

// mapBedrockFinishReason 映射 Bedrock finish 原因
func mapBedrockFinishReason(reason string) string {
	switch reason {
	case "end_turn", "stop":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "content_filtered":
		return "content_filter"
	default:
		return reason
	}
}

// parseBedrockStream 解析 Bedrock Converse Stream 响应
func parseBedrockStream(ctx context.Context, resp *http.Response) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		accumulatedText := ""

		for {
			var event struct {
				Type string `json:"type"`
				// ContentBlockStart
				ContentBlockIndex int `json:"contentBlockIndex,omitempty"`
				Start             *struct {
					Text string `json:"text,omitempty"`
				} `json:"start,omitempty"`
				// ContentBlockDelta
				Delta *struct {
					Text string `json:"text,omitempty"`
				} `json:"delta,omitempty"`
				// MessageStop
				StopReason string `json:"stopReason,omitempty"`
				// Metadata
				Usage *struct {
					InputTokens  int `json:"inputTokens"`
					OutputTokens int `json:"outputTokens"`
					TotalTokens  int `json:"totalTokens"`
				} `json:"usage,omitempty"`
			}

			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					ch <- llm.StreamEvent{Type: llm.StreamEventDone}
					return
				}
				if ctx.Err() != nil {
					return
				}
				ch <- llm.StreamEvent{Error: err}
				return
			}

			switch event.Type {
			case "contentBlockStart":
				if event.Start != nil && event.Start.Text != "" {
					accumulatedText = event.Start.Text
					ch <- llm.StreamEvent{
						Type:  llm.StreamEventText,
						Delta: event.Start.Text,
					}
				}
			case "contentBlockDelta":
				if event.Delta != nil && event.Delta.Text != "" {
					delta := event.Delta.Text
					if len(accumulatedText) > 0 && strings.HasPrefix(event.Delta.Text, accumulatedText) {
						delta = strings.TrimPrefix(event.Delta.Text, accumulatedText)
					}
					accumulatedText = event.Delta.Text
					if delta != "" {
						ch <- llm.StreamEvent{
							Type:  llm.StreamEventText,
							Delta: delta,
						}
					}
				}
			case "messageStop":
				ch <- llm.StreamEvent{Type: llm.StreamEventDone}
				return
			case "metadata":
				if event.Usage != nil {
					usage := &llm.Usage{
						PromptTokens:     event.Usage.InputTokens,
						CompletionTokens: event.Usage.OutputTokens,
						TotalTokens:      event.Usage.TotalTokens,
					}
					ch <- llm.StreamEvent{
						Type:  llm.StreamEventUsage,
						Usage: usage,
					}
				}
			}
		}
	}()

	return ch
}

// Models 返回支持的模型列表
func (p *BedrockProvider) Models() []string {
	var models []string
	for k := range bedrockModelIDs {
		models = append(models, k)
	}
	sort.Strings(models)
	return models
}

// ID 返回模型标识（实现 llm.Model）
func (p *BedrockProvider) ID() string {
	return p.modelID
}

// Supports 检查能力支持（实现 llm.Model）
func (p *BedrockProvider) Supports(cap llm.Capability) bool {
	switch cap {
	case llm.CapTools, llm.CapStreaming:
		return true
	default:
		return false
	}
}

// Generate 非流式生成（实现 llm.Model）
func (p *BedrockProvider) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return p.Chat(ctx, req)
}

// Stream 流式生成（实现 llm.Model）
func (p *BedrockProvider) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return p.ChatStream(ctx, req)
}

// newBedrock 是 factory.Create 使用的构造函数（返回 llm.Model）
func newBedrock(baseURL, apiKey, modelID string, opts map[string]any) (llm.Model, error) {
	cfg := BedrockConfigOpts{Model: modelID}

	if opts != nil {
		if v, ok := opts["access_key"].(string); ok {
			cfg.AccessKey = v
		}
		if v, ok := opts["secret_key"].(string); ok {
			cfg.SecretKey = v
		}
		if v, ok := opts["region"].(string); ok {
			cfg.Region = v
		}
	}

	// API key 作 access key fallback
	if cfg.AccessKey == "" {
		cfg.AccessKey = apiKey
	}

	return NewBedrockProvider(cfg)
}

// 编译时验证
var _ ModelProvider = (*BedrockProvider)(nil)
