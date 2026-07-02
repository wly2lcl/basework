package llm

import "context"

// Capability 表示模型支持的能力
type Capability string

const (
	CapTools     Capability = "tools"
	CapVision    Capability = "vision"
	CapStreaming Capability = "streaming"
	CapJSON      Capability = "json_mode"
)

// Model 是 LLM 提供商的统一抽象
type Model interface {
	ID() string
	Generate(ctx context.Context, req *Request) (*Response, error)
	Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
	Supports(cap Capability) bool
}