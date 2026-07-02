package hook

import (
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestPermissionAllow 测试 Allow 规则
func TestPermissionAllow(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "write", Resource: "*", Effect: Allow},
	}, nil)

	call := llm.ToolCall{Name: "write", ID: "1"}
	result, err := hook.BeforeTool(call)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil call")
	}
	if result.Name != "write" {
		t.Errorf("expected name 'write', got '%s'", result.Name)
	}
}

// TestPermissionDeny 测试 Deny 规则
func TestPermissionDeny(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "bash", Resource: "*", Effect: Deny},
	}, nil)

	_, err := hook.BeforeTool(llm.ToolCall{Name: "bash"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestPermissionAskAllowed 测试 Ask 规则 + onAsk 返回 true
func TestPermissionAskAllowed(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "rm", Resource: "*", Effect: Ask},
	}, func(action, resource string) (bool, error) {
		return true, nil
	})

	call := llm.ToolCall{Name: "rm", ID: "1"}
	result, err := hook.BeforeTool(call)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil call")
	}
}

// TestPermissionAskDenied 测试 Ask 规则 + onAsk 返回 false
func TestPermissionAskDenied(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "rm", Resource: "*", Effect: Ask},
	}, func(action, resource string) (bool, error) {
		return false, nil
	})

	_, err := hook.BeforeTool(llm.ToolCall{Name: "rm"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestPermissionWildcard 测试通配符匹配 "tool:*"
func TestPermissionWildcard(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "tool:*", Resource: "*", Effect: Deny},
	}, nil)

	// "tool:write" 应匹配 "tool:*"
	_, err := hook.BeforeTool(llm.ToolCall{Name: "tool:write"})
	if err == nil {
		t.Error("expected deny for 'tool:write'")
	}

	// "tool:bash" 也应匹配
	_, err = hook.BeforeTool(llm.ToolCall{Name: "tool:bash"})
	if err == nil {
		t.Error("expected deny for 'tool:bash'")
	}
}

// TestPermissionDefaultAllow 测试无匹配规则时默认 Allow
func TestPermissionDefaultAllow(t *testing.T) {
	hook := NewPermissionHook([]Rule{
		{Action: "write", Resource: "*", Effect: Deny},
	}, nil)

	// "read" 没有匹配规则，应默认 Allow
	call := llm.ToolCall{Name: "read", ID: "1"}
	result, err := hook.BeforeTool(call)
	if err != nil {
		t.Fatalf("expected no error for non-matching action, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil call")
	}
}