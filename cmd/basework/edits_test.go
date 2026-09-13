package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimetools "github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// TestEditFactsSurviveSessionReload 走通「编辑 → 事件落盘 → 重开会话存储（模拟
// 重启）→ 审阅仍可用」的端到端链路。
//
// 验收口径：会话恢复后变更记录仍可用，且不会重复提交。重启后计划注册表清空，
// 同一编辑再次 preview 会因 old 文本已不存在而失败——从机制上排除重复写入。
func TestEditFactsSurviveSessionReload(t *testing.T) {
	storeDir := t.TempDir()
	workRoot := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(workRoot); err == nil {
		workRoot = resolved
	}
	if err := os.WriteFile(filepath.Join(workRoot, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("准备文件: %v", err)
	}

	store, err := session.NewJSONLStore(storeDir)
	if err != nil {
		t.Fatalf("打开会话存储: %v", err)
	}
	info, err := store.Create(session.CreateOpts{Title: "edit-e2e"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sessionID := info.ID
	sink := &runtimeEditEventSink{store: store, sessionID: func() string { return sessionID }}

	editTool := runtimetools.NewEditFilesTool(workRoot)
	editTool.Sink = sink

	// 1. preview → commit。
	preview := callEditTool(t, editTool, map[string]interface{}{
		"action": "preview",
		"edits": []interface{}{
			map[string]interface{}{"path": "a.txt", "old": "world", "new": "Go"},
		},
	})
	if preview.IsError {
		t.Fatalf("预览失败: %s", preview.Content)
	}
	planID := extractPlanID(t, preview.Content)
	commit := callEditTool(t, editTool, map[string]interface{}{
		"action":  "commit",
		"plan_id": planID,
		"verify":  "cat a.txt",
	})
	if commit.IsError {
		t.Fatalf("提交失败: %s", commit.Content)
	}

	// 2. 重开存储：模拟进程重启后读同一份持久化事件。
	reopened, err := session.NewJSONLStore(storeDir)
	if err != nil {
		t.Fatalf("重开会话存储: %v", err)
	}
	facts, err := loadEditEvents(reopened, sessionID, 100)
	if err != nil {
		t.Fatalf("读取编辑事件: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("期望 2 条编辑事实（预览+提交），得到 %d", len(facts))
	}
	if facts[0].Phase != "preview" || facts[1].Phase != "committed" {
		t.Fatalf("阶段不符: %s, %s", facts[0].Phase, facts[1].Phase)
	}
	if facts[1].Verify != "cat a.txt" {
		t.Fatalf("验证命令未持久化: %+v", facts[1])
	}
	if len(facts[1].Files) != 1 || facts[1].Files[0].State != "written" || facts[1].Files[0].Path != "a.txt" {
		t.Fatalf("提交事实的文件结局不符: %+v", facts[1].Files)
	}

	// 3. 重启后同一编辑不能重复提交：old 文本已不在文件里，重新 preview 直接失败。
	afterRestart := runtimetools.NewEditFilesTool(workRoot)
	afterRestart.Sink = &runtimeEditEventSink{store: reopened, sessionID: func() string { return sessionID }}
	replay := callEditTool(t, afterRestart, map[string]interface{}{
		"action": "preview",
		"edits": []interface{}{
			map[string]interface{}{"path": "a.txt", "old": "world", "new": "Go"},
		},
	})
	if !replay.IsError {
		t.Fatalf("重启后重放同一编辑应失败（old 文本已不存在）: %s", replay.Content)
	}
	if !strings.Contains(replay.Content, "未找到待替换文本") {
		t.Fatalf("报错应说明替换文本不存在: %s", replay.Content)
	}
}

// callEditTool 调用工具并把参数序列化成 JSON。
func callEditTool(t *testing.T, tool *runtimetools.EditFilesTool, params map[string]interface{}) *tool.Result {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("序列化参数: %v", err)
	}
	res, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	return res
}

func extractPlanID(t *testing.T, content string) string {
	t.Helper()
	marker := "计划 plan-"
	idx := strings.Index(content, marker)
	if idx < 0 {
		t.Fatalf("结果里没有 plan_id: %s", content)
	}
	rest := content[idx+len(marker):]
	if len(rest) < 12 {
		t.Fatalf("plan_id 截断: %s", content)
	}
	return "plan-" + rest[:12]
}
