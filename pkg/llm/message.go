package llm

// CacheControl 表示内容块的缓存控制标记。
// 用于支持 Prompt Caching 功能。
type CacheControl struct {
	Type string `json:"type"` // 缓存类型，如 "ephemeral"
}

// CacheControlEphemeral 是 short-lived 缓存类型的常量值
const CacheControlEphemeral = "ephemeral"
