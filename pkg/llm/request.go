package llm

// Request 是 LLM 请求
type Request struct {
	Messages    []ChatMessage
	Tools       []ToolDefinition
	MaxTokens   int
	Temperature *float64
	Stop        []string
	Extra       map[string]any // 模型特定选项（透传）
}

// Response 是 LLM 完整响应
type Response struct {
	Message      ChatMessage
	Usage        Usage
	FinishReason string
}

// Usage 记录 token 用量
type Usage struct {
	PromptTokens             int
	CompletionTokens         int
	TotalTokens              int
	CacheCreationInputTokens int // Prompt 缓存创建时的输入 token 数
	CacheReadInputTokens     int // Prompt 缓存命中时的读取 token 数
}
