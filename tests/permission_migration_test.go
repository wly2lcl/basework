//go:build sqlite

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wly2lcl/basework/internal/permission"
)

// TestPermissionMigration 测试完整迁移流程。
func TestPermissionMigration(t *testing.T) {
	// 创建内存缓存并添加规则
	cache := permission.NewCache()
	cache.Set("read_file:path=test.txt", true)
	cache.Set("write_file:path=*.go", false)
	cache.Set("execute_command:", true)

	// 创建 SQLite store
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_migration.db")
	store, err := permission.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}
	defer store.Close()

	// 执行迁移
	migrated, err := permission.MigrateFromMemory(cache, store)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	if migrated != 3 {
		t.Errorf("期望迁移 3 条规则，实际 %d 条", migrated)
	}

	// 验证 store 中的规则
	rules, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}

	if len(rules) != 3 {
		t.Fatalf("期望 3 条规则，得到 %d 条", len(rules))
	}

	// 验证规则来源
	for _, r := range rules {
		if r.Source != "migration" {
			t.Errorf("迁移规则的 Source 应为 'migration', 得到 %q", r.Source)
		}
	}
}

// TestPermissionMigration_空缓存 测试空缓存迁移。
func TestPermissionMigration_空缓存(t *testing.T) {
	cache := permission.NewCache()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_empty.db")
	store, err := permission.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}
	defer store.Close()

	migrated, err := permission.MigrateFromMemory(cache, store)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	if migrated != 0 {
		t.Errorf("空缓存应迁移 0 条，实际 %d 条", migrated)
	}
}

// TestPermissionMigration_Store非空 测试 store 非空时迁移应失败。
func TestPermissionMigration_PreservesCacheScope(t *testing.T) {
	cache := permission.NewCache()
	cache.SetContext(permission.ScopeContext{SessionID: "sess-migrated"})
	cache.Set("read_file:", true)

	dir := t.TempDir()
	store, err := permission.NewSQLiteStore(filepath.Join(dir, "scope.db"))
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}
	defer store.Close()

	if _, err := permission.MigrateFromMemory(cache, store); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if got, err := store.FindByPatternInContext("read_file", nil, permission.ScopeContext{SessionID: "sess-migrated"}); err != nil || got == nil || got.SessionID != "sess-migrated" {
		t.Fatalf("迁移应保留 session 归属，rule=%+v err=%v", got, err)
	}
	if got, err := store.FindByPatternInContext("read_file", nil, permission.ScopeContext{SessionID: "other"}); err != nil || got != nil {
		t.Fatalf("迁移规则不应跨会话命中，rule=%+v err=%v", got, err)
	}
}

func TestPermissionMigration_Store非空(t *testing.T) {
	cache := permission.NewCache()
	cache.Set("tool:", true)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_nonempty.db")
	store, err := permission.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}
	defer store.Close()

	// 先在 store 中创建一条规则
	err = store.Create(&permission.StoredRule{
		RuleType: "allow",
		Pattern:  "existing_tool",
		Scope:    "global",
	})
	if err != nil {
		t.Fatalf("创建初始规则失败: %v", err)
	}

	// 迁移应失败
	_, err = permission.MigrateFromMemory(cache, store)
	if err == nil {
		t.Error("store 非空时应返回错误")
	}
}

// TestPermissionExportImport 测试 export/import 一致性。
func TestPermissionExportImport(t *testing.T) {
	// 创建源 store 并添加规则
	dir := t.TempDir()

	srcPath := filepath.Join(dir, "src.db")
	srcStore, err := permission.NewSQLiteStore(srcPath)
	if err != nil {
		t.Fatalf("创建源 SQLiteStore 失败: %v", err)
	}
	defer srcStore.Close()

	// 添加测试规则
	testRules := []*permission.StoredRule{
		{RuleType: "allow", Pattern: "read_*", Scope: "global", Source: "user"},
		{RuleType: "deny", Pattern: "write_file", Scope: "global", Source: "user"},
		{RuleType: "allow", Pattern: "execute_command", Scope: "session", Source: "user"},
	}
	for _, r := range testRules {
		if err := srcStore.Create(r); err != nil {
			t.Fatalf("创建规则失败: %v", err)
		}
	}

	// 导出为 JSON
	exportData, err := permission.ExportRules(srcStore)
	if err != nil {
		t.Fatalf("ExportRules 失败: %v", err)
	}

	// 验证 JSON 格式
	var exported []permission.StoredRule
	if err := json.Unmarshal(exportData, &exported); err != nil {
		t.Fatalf("解析导出数据失败: %v", err)
	}
	if len(exported) != 3 {
		t.Errorf("期望导出 3 条规则，实际 %d 条", len(exported))
	}

	// 写入临时文件
	exportFile := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(exportFile, exportData, 0644); err != nil {
		t.Fatalf("写入导出文件失败: %v", err)
	}

	// 创建目标 store 并导入
	dstPath := filepath.Join(dir, "dst.db")
	dstStore, err := permission.NewSQLiteStore(dstPath)
	if err != nil {
		t.Fatalf("创建目标 SQLiteStore 失败: %v", err)
	}
	defer dstStore.Close()

	importData, err := os.ReadFile(exportFile)
	if err != nil {
		t.Fatalf("读取导入文件失败: %v", err)
	}

	imported, err := permission.ImportRules(dstStore, importData)
	if err != nil {
		t.Fatalf("ImportRules 失败: %v", err)
	}
	if imported != 3 {
		t.Errorf("期望导入 3 条规则，实际 %d 条", imported)
	}

	// 验证导入结果
	if err := permission.VerifyImport(dstStore, 3); err != nil {
		t.Errorf("VerifyImport 失败: %v", err)
	}

	// 验证规则内容
	dstRules, err := dstStore.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}

	ruleMap := make(map[string]bool)
	for _, r := range dstRules {
		ruleMap[r.Pattern] = r.RuleType == "allow"
	}

	if ruleMap["read_*"] != true {
		t.Error("read_* 规则应允许")
	}
	if ruleMap["write_file"] != false {
		t.Error("write_file 规则应拒绝")
	}
	if ruleMap["execute_command"] != true {
		t.Error("execute_command 规则应允许")
	}
}

// TestPermissionExportImport_空数据 测试导入空数据。
func TestPermissionExportImport_空数据(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_empty_import.db")
	store, err := permission.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}
	defer store.Close()

	_, err = permission.ImportRules(store, []byte("[]"))
	if err != nil {
		t.Errorf("导入空数组应成功，实际: %v", err)
	}

	// 验证
	rules, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("导入空数组后应有 0 条规则，实际 %d 条", len(rules))
	}
}
