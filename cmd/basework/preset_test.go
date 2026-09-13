package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

// fakeTool 只为实现 tool.Tool 接口，让过滤/校验函数可以脱离真实工具组合测试。
type fakeTool struct{ name string }

func (f fakeTool) Name() string { return f.name }

func (fakeTool) Description() string { return "fake" }
func (fakeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{}`)
}
func (fakeTool) Execute(context.Context, json.RawMessage) (*tool.Result, error) {
	return &tool.Result{}, nil
}

func TestFilterToolsByAllowlist(t *testing.T) {
	tools := []tool.Tool{
		fakeTool{"read"}, fakeTool{"bash"}, fakeTool{"lsp_definition"},
	}

	if got := filterToolsByAllowlist(tools, nil); len(got) != 3 {
		t.Fatalf("空白名单应原样返回全部工具: %d", len(got))
	}

	got := filterToolsByAllowlist(tools, []string{"read", "lsp_*"})
	if len(got) != 2 || got[0].Name() != "read" || got[1].Name() != "lsp_definition" {
		t.Fatalf("过滤结果不符: %v", names(got))
	}
}

func TestEnsureReadonlyTools(t *testing.T) {
	// 只读集合内的工具全部通过
	ok := []tool.Tool{fakeTool{"read"}, fakeTool{"grep"}, fakeTool{"lsp_diagnostics"}}
	if err := ensureReadonlyTools(ok); err != nil {
		t.Fatalf("只读工具不应报泄漏: %v", err)
	}

	// 混进执行类工具必须失败
	leaky := []tool.Tool{fakeTool{"read"}, fakeTool{"bash"}}
	err := ensureReadonlyTools(leaky)
	if err == nil {
		t.Fatal("bash 混入只读预设必须报错")
	}
	if !strings.Contains(err.Error(), "bash") {
		t.Errorf("错误信息应点名泄漏工具: %v", err)
	}
}

// TestReadonlyPresetRealBuiltinSet 用真实 builtin 工具组合验证白名单过滤
// 不泄漏写/执行能力——fakeTool 只测逻辑，真实组合防「工具改名」漂移。
func TestReadonlyPresetRealBuiltinSet(t *testing.T) {
	all := builtin.All()
	if len(all) == 0 {
		t.Fatal("builtin.All() 不应返回空")
	}
	filtered := filterToolsByAllowlist(all, config.ReadonlyToolset())
	if len(filtered) == 0 {
		t.Fatal("只读白名单过滤后不应为空（工具名可能整体改名，需同步 readonlyToolset）")
	}
	for _, tl := range filtered {
		if !config.ToolAllowed(config.ReadonlyToolset(), tl.Name()) {
			t.Errorf("过滤后仍有工具不在只读集合: %q", tl.Name())
		}
	}
	// bash/write/edit 必须被滤掉
	for _, banned := range []string{"bash", "write", "edit"} {
		for _, tl := range filtered {
			if tl.Name() == banned {
				t.Errorf("写/执行工具 %q 泄漏进只读工具表", banned)
			}
		}
	}
}

func names(tools []tool.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tl := range tools {
		out = append(out, tl.Name())
	}
	return out
}
