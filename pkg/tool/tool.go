package tool

import (
	"context"
	"encoding/json"
)

// Tool 是工具的统一接口
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema
	Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Result 是工具执行结果
type Result struct {
	Content string
	IsError bool
	Images  []string // base64 或 URL
}