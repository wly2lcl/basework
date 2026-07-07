package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// EchoTool 返回输入参数的 echo
type EchoTool struct{}

func (e *EchoTool) Name() string { return "echo" }

func (e *EchoTool) Description() string { return "返回输入的消息内容" }

func (e *EchoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}}}`)
}

func (e *EchoTool) Execute(_ context.Context, args json.RawMessage) (*Result, error) {
	var params struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}
	return &Result{Content: params.Message}, nil
}

// AddTool 两个数相加
type AddTool struct{}

func (a *AddTool) Name() string { return "add" }

func (a *AddTool) Description() string { return "计算两个数的和" }

func (a *AddTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}}}`)
}

func (a *AddTool) Execute(_ context.Context, args json.RawMessage) (*Result, error) {
	var params struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}
	sum := params.A + params.B
	return &Result{Content: fmt.Sprintf("%v", sum)}, nil
}
