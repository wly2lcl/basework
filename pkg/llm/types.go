package llm

// Role 表示对话中消息的角色
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType 表示消息内容的类型
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// ContentPart 支持多模态内容，可以是纯文本或图片
type ContentPart struct {
	Type         ContentType
	Text         string
	ImageURL     string
	CacheControl *CacheControl // Prompt 缓存控制标记，可选
}

// ChatMessage 是 LLM 对话的基本单元
type ChatMessage struct {
	Role       Role
	Content    []ContentPart
	Name       string     // 可选，工具名或助手名
	ToolCalls  []ToolCall // 助手请求的工具调用
	ToolCallID string     // 工具响应关联的 ID
}
