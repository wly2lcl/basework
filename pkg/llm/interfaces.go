package llm

// EventBus 可观测性事件总线接口，用于替代 internal/observability 的依赖。
type EventBus interface {
	PublishEvent(eventType string, data map[string]interface{})
}

// TokenSource OAuth token 源接口，用于替代 internal/oauth.TokenSource 的依赖。
type TokenSource interface {
	// SetTokenSource 将 token 源注入到底层 HTTP 客户端。
	SetTokenSource(ts interface{}) error
}