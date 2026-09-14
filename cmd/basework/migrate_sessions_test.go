//go:build sqlite

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

// writeSessionFixture 在 dir 下写一个带文件头的 JSONL 会话文件。
func writeSessionFixture(t *testing.T, dir, id string, version int, eventLine string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建会话目录失败: %v", err)
	}
	body := fmt.Sprintf("{\"_schema\":\"basework.session\",\"v\":%d}\n%s\n", version, eventLine)
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatalf("写会话夹具失败: %v", err)
	}
}

// isolateMigrationHome 把 HOME 与 SQLite 目标路径都指到临时目录，隔离真实数据。
// Windows 上必须同时设置 USERPROFILE（见 setTestHome），否则 os.UserHomeDir
// 拿到真实主目录，多个测试会共用同一个 sessions.db 相互污染。
func isolateMigrationHome(t *testing.T) {
	t.Helper()
	setTestHome(t, t.TempDir())
	orig := migrateSQLitePath
	migrateSQLitePath = ""
	t.Cleanup(func() { migrateSQLitePath = orig })
}

// TestRunMigrateSessions_PreservesIDAndData 覆盖 SHIP-002 验证时发现的两个缺陷：
//
//  1. 迁移按实现另发的新 ID 建会话，事件里带的原 session_id 外键对不上，
//     逐条失败被跳过，输出却是「✅ 导入」——报告成功、零条落库；
//  2. 报告行对会话 ID 硬取 `sessionID[:12]`，ID 短于 12 字符时直接 panic。
//
// 这里同时放入 10 字符与 2 字符的 ID：前者触发过 [:12] 越界，后者触发过 [:8] 越界。
func TestRunMigrateSessions_PreservesIDAndData(t *testing.T) {
	isolateMigrationHome(t)

	dir := getSessionDir()
	const longID = "upgrade-v1" // 10 字符：曾触发 [:12] 越界 panic
	const shortID = "ab"        // 2 字符：曾触发 [:8] 越界 panic

	writeSessionFixture(t, dir, longID, 1,
		`{"id":"e-long","session_id":"upgrade-v1","type":"prompted","data":{"content":"升级前写入的内容：请保留我"},"seq":1,"created_at":"2026-09-01T00:00:00Z","v":1}`)
	writeSessionFixture(t, dir, shortID, 1,
		`{"id":"e-short","session_id":"ab","type":"prompted","data":{"content":"短 ID 会话"},"seq":1,"created_at":"2026-09-02T00:00:00Z","v":1}`)

	if err := runMigrateSessions(); err != nil {
		t.Fatalf("迁移应全部成功，实际报错: %v", err)
	}

	dbPath := filepath.Join(resolveSessionDir(), "sessions.db")
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("打开目标库失败: %v", err)
	}
	defer store.Close()

	for _, id := range []string{longID, shortID} {
		events, err := store.Events(session.EventFilter{SessionID: id})
		if err != nil {
			t.Fatalf("读取 %s 的事件失败: %v", id, err)
		}
		if len(events) != 1 {
			t.Errorf("%s 的事件数 = %d，期望 1（迁移必须保留原会话身份）", id, len(events))
		}
	}

	// 内容必须原样保留，而不只是「有 1 条事件」。
	events, err := store.Events(session.EventFilter{SessionID: longID})
	if err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	if len(events) == 0 || !strings.Contains(string(events[0].Data), "请保留我") {
		t.Errorf("事件内容未保留: %+v", events)
	}
}

// TestRunMigrateSessions_RejectsFutureVersion 覆盖第三个缺陷：
// 迁移此前自己按行解析 JSONL，绕过了存储层的版本判定，于是 v99 的文件被
// "成功"导入当前库——未来版本的数据越过「更高版本即拒绝」的契约落进本地存储。
func TestRunMigrateSessions_RejectsFutureVersion(t *testing.T) {
	isolateMigrationHome(t)

	dir := getSessionDir()
	writeSessionFixture(t, dir, "future-v99", 99,
		`{"id":"e-future","session_id":"future-v99","type":"prompted","data":{"content":"来自未来版本"},"seq":1,"created_at":"2026-09-01T00:00:00Z","v":99}`)

	if err := runMigrateSessions(); err == nil {
		t.Fatal("版本过高的会话文件应导致迁移失败，而不是被导入")
	}

	// 拒绝必须发生在写数据之前：不留下部分导入的痕迹。
	dbPath := filepath.Join(resolveSessionDir(), "sessions.db")
	if _, statErr := os.Stat(dbPath); statErr == nil {
		store, err := session.NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("打开目标库失败: %v", err)
		}
		defer store.Close()
		infos, err := store.List(session.ListFilter{})
		if err != nil {
			t.Fatalf("列举失败: %v", err)
		}
		if len(infos) != 0 {
			t.Errorf("拒绝了 v99 却仍写入了 %d 个会话", len(infos))
		}
	}
}
