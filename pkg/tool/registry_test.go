package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestRegistry_Register_Success(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&EchoTool{})
	if err != nil {
		t.Fatalf("注册应成功，得到错误: %v", err)
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})
	err := r.Register(&EchoTool{})
	if err == nil {
		t.Fatal("重复注册应返回 error")
	}
}

func TestRegistry_Get_Exists(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})
	tool := r.Get("echo")
	if tool == nil {
		t.Fatal("Get 应返回已注册的工具")
	}
	if tool.Name() != "echo" {
		t.Fatalf("工具名应为 echo，得到 %q", tool.Name())
	}
}

func TestRegistry_Get_NotExists(t *testing.T) {
	r := NewRegistry()
	tool := r.Get("nonexistent")
	if tool != nil {
		t.Fatal("Get 不存在的工具应返回 nil")
	}
}

func TestRegistry_Settle_Success(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})

	call := llm.ToolCall{
		ID:       "call_1",
		Name:     "echo",
		ArgsJSON: `{"message":"hello"}`,
	}

	result, err := r.Settle(context.Background(), call)
	if err != nil {
		t.Fatalf("Settle 应成功，得到错误: %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("结果应为 hello，得到 %q", result.Content)
	}
}

func TestRegistry_Settle_NotExists(t *testing.T) {
	r := NewRegistry()

	call := llm.ToolCall{
		ID:       "call_1",
		Name:     "nonexistent",
		ArgsJSON: `{}`,
	}

	_, err := r.Settle(context.Background(), call)
	if err == nil {
		t.Fatal("Settle 不存在的工具应返回 error")
	}
}

func TestRegistry_Settle_Disabled(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})
	r.Disable("echo")

	call := llm.ToolCall{
		ID:       "call_1",
		Name:     "echo",
		ArgsJSON: `{"message":"hello"}`,
	}

	_, err := r.Settle(context.Background(), call)
	if err == nil {
		t.Fatal("Settle 已禁用的工具应返回 error")
	}
}

func TestRegistry_Clone_Isolation(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})

	clone := r.Clone()

	// 修改原始 registry
	_ = r.Register(&AddTool{})
	r.Disable("echo")

	// clone 不应受影响
	if clone.Get("add") != nil {
		t.Fatal("clone 不应包含 add 工具")
	}

	// 验证 clone 中的 echo 仍可用
	call := llm.ToolCall{
		ID:       "call_1",
		Name:     "echo",
		ArgsJSON: `{"message":"test"}`,
	}
	result, err := clone.Settle(context.Background(), call)
	if err != nil {
		t.Fatalf("clone 中 echo 应可用，得到错误: %v", err)
	}
	if result.Content != "test" {
		t.Fatalf("结果应为 test，得到 %q", result.Content)
	}
}

func TestRegistry_Materialize_ExcludesDisabled(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})
	_ = r.Register(&AddTool{})

	defs := r.Materialize()
	if len(defs) != 2 {
		t.Fatalf("Materialize 应返回 2 个定义，得到 %d", len(defs))
	}

	r.Disable("echo")
	defs = r.Materialize()
	if len(defs) != 1 {
		t.Fatalf("禁用后 Materialize 应返回 1 个定义，得到 %d", len(defs))
	}
	if defs[0].Name != "add" {
		t.Fatalf("剩余工具应为 add，得到 %q", defs[0].Name)
	}
}

func TestRegistry_Disable_Materialize(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&EchoTool{})
	r.Disable("echo")

	defs := r.Materialize()
	for _, d := range defs {
		if d.Name == "echo" {
			t.Fatal("Materialize 不应包含已禁用的工具")
		}
	}
}

func TestEchoTool_Execute(t *testing.T) {
	tool := &EchoTool{}

	t.Run("正常执行", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"message":"hello world"}`))
		if err != nil {
			t.Fatalf("Execute 应成功: %v", err)
		}
		if result.Content != "hello world" {
			t.Fatalf("期望 hello world，得到 %q", result.Content)
		}
		if result.IsError {
			t.Fatal("不应标记为错误")
		}
	})

	t.Run("参数解析失败", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`invalid`))
		if err != nil {
			t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
		}
		if !result.IsError {
			t.Fatal("应标记为错误")
		}
	})
}

func TestAddTool_Execute(t *testing.T) {
	tool := &AddTool{}

	t.Run("正常相加", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"a":1,"b":2}`))
		if err != nil {
			t.Fatalf("Execute 应成功: %v", err)
		}
		if result.Content != "3" {
			t.Fatalf("期望 3，得到 %q", result.Content)
		}
	})

	t.Run("浮点数相加", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"a":1.5,"b":2.5}`))
		if err != nil {
			t.Fatalf("Execute 应成功: %v", err)
		}
		if result.Content != "4" {
			t.Fatalf("期望 4，得到 %q", result.Content)
		}
	})

	t.Run("参数解析失败", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), json.RawMessage(`invalid`))
		if err != nil {
			t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
		}
		if !result.IsError {
			t.Fatal("应标记为错误")
		}
	})
}
