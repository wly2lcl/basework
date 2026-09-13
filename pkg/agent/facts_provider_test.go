package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

func TestFactsSummaryProvider_Collect(t *testing.T) {
	base := t.TempDir() // 事实目录
	wsID := session.WorkspaceID(base)
	w := session.NewWorkspaceFacts(wsID)
	w.Add(session.Fact{Kind: session.FactFileModified, Key: session.FileFactKey(session.FactFileModified, "a.go"),
		Path: "a.go", Ref: "plan-1", Source: "edit_files", FirstSeen: "2026-09-12T09:00:00Z"})
	if err := session.SaveWorkspaceFacts(base, w); err != nil {
		t.Fatalf("保存事实: %v", err)
	}

	p := NewFactsSummaryProvider(base, wsID, 0, func(rel string) (string, error) {
		if rel == "a.go" {
			return "h1", nil
		}
		return "", ErrProtectedPath
	})
	if p == nil {
		t.Fatal("provider 不应为 nil")
	}
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !strings.Contains(text, "a.go") || !strings.Contains(text, "edit_files") {
		t.Fatalf("摘要内容不符: %s", text)
	}
	if p.Name() != "facts_summary" {
		t.Fatalf("Name: %q", p.Name())
	}
}

func TestFactsSummaryProvider_ProtectedPathNotRead(t *testing.T) {
	base := t.TempDir()
	wsID := session.WorkspaceID(base)
	w := session.NewWorkspaceFacts(wsID)
	w.Add(session.Fact{Kind: session.FactFileModified, Key: session.FileFactKey(session.FactFileModified, "secret.key"),
		Path: "secret.key", Ref: "plan-2", Source: "edit_files", FirstSeen: "2026-09-12T09:00:00Z"})
	if err := session.SaveWorkspaceFacts(base, w); err != nil {
		t.Fatal(err)
	}

	// 产品层 hash 实现：受保护路径直接拒绝，绝不读盘。
	p := NewFactsSummaryProvider(base, wsID, 0, func(rel string) (string, error) {
		if strings.HasSuffix(rel, ".key") {
			return "", ErrProtectedPath
		}
		return FileHash(filepath.Join(base, rel))
	})
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("受保护路径不应使摘要失败: %v", err)
	}
	if !strings.Contains(text, "[未读取]") {
		t.Fatalf("受保护路径应标注未读取: %s", text)
	}
}

func TestFactsSummaryProvider_FutureVersionYieldsEmpty(t *testing.T) {
	base := t.TempDir()
	wsID := session.WorkspaceID(base)
	// 写一个未来版本的事实文件。
	path := filepath.Join(base, wsID+".json")
	if err := os.WriteFile(path, []byte(`{"version":999,"workspace_id":"`+wsID+`","facts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewFactsSummaryProvider(base, wsID, 0, nil)
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("未来版本应容错为空摘要而非报错: %v", err)
	}
	if text != "" {
		t.Fatalf("未来版本应得空摘要: %q", text)
	}
}

func TestNewFactsSummaryProvider_MissingDeps(t *testing.T) {
	if NewFactsSummaryProvider("", "ws", 0, nil) != nil {
		t.Fatal("缺目录应返回 nil")
	}
	if NewFactsSummaryProvider("/tmp/facts", "", 0, nil) != nil {
		t.Fatal("缺工作区 ID 应返回 nil")
	}
}

func TestFactsSummaryProvider_HashOnlyInsideWorkspace(t *testing.T) {
	// 相对路径里带 .. 的越界路径：wrapHash 直接拒绝，不给 hash 函数机会。
	p := NewFactsSummaryProvider("/tmp/facts", "ws", 0, func(rel string) (string, error) {
		t.Fatal("越界路径不应到达 hash 函数")
		return "", errors.New("unreachable")
	})
	if _, err := p.wrapHash("../escape.txt"); !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("越界路径应拒绝: %v", err)
	}
	if _, err := p.wrapHash("/abs/path"); !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("绝对路径应拒绝: %v", err)
	}
}
