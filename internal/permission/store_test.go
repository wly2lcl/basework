//go:build sqlite

package permission

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore 创建一个测试用的 SQLiteStore。
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_permissions.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建测试 SQLiteStore 失败: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
		os.Remove(dbPath)
	})

	return store
}

func TestStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)

	rule := &StoredRule{
		RuleType: "allow",
		Pattern:  "read_file",
		Scope:    "global",
		Source:   "user",
	}

	err := store.Create(rule)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if rule.ID == "" {
		t.Error("Create 后 ID 不应为空")
	}

	got, err := store.Get(rule.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}

	if got.RuleType != "allow" {
		t.Errorf("RuleType 期望 %q, 得到 %q", "allow", got.RuleType)
	}
	if got.Pattern != "read_file" {
		t.Errorf("Pattern 期望 %q, 得到 %q", "read_file", got.Pattern)
	}
	if got.Scope != "global" {
		t.Errorf("Scope 期望 %q, 得到 %q", "global", got.Scope)
	}
}

func TestStore_Get_不存在(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Get("non-existent-id")
	if err == nil {
		t.Error("获取不存在的规则应返回错误")
	}
}

func TestStore_Update(t *testing.T) {
	store := newTestStore(t)

	rule := &StoredRule{
		RuleType: "allow",
		Pattern:  "read_file",
		Scope:    "global",
	}
	err := store.Create(rule)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	rule.RuleType = "deny"
	rule.Pattern = "write_*"
	err = store.Update(rule)
	if err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	got, err := store.Get(rule.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}

	if got.RuleType != "deny" {
		t.Errorf("更新后 RuleType 期望 %q, 得到 %q", "deny", got.RuleType)
	}
	if got.Pattern != "write_*" {
		t.Errorf("更新后 Pattern 期望 %q, 得到 %q", "write_*", got.Pattern)
	}
}

func TestStore_Delete(t *testing.T) {
	store := newTestStore(t)

	rule := &StoredRule{
		RuleType: "allow",
		Pattern:  "test_tool",
	}
	err := store.Create(rule)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	err = store.Delete(rule.ID)
	if err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	_, err = store.Get(rule.ID)
	if err == nil {
		t.Error("删除后 Get 应返回错误")
	}
}

func TestStore_Delete_不存在(t *testing.T) {
	store := newTestStore(t)

	err := store.Delete("non-existent-id")
	if err == nil {
		t.Error("删除不存在的规则应返回错误")
	}
}

func TestStore_List(t *testing.T) {
	store := newTestStore(t)

	// 空列表
	rules, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("空数据库 List 应返回空切片，得到 %d 条", len(rules))
	}

	// 创建规则
	rulesToCreate := []*StoredRule{
		{RuleType: "allow", Pattern: "read_*", Scope: "global"},
		{RuleType: "deny", Pattern: "write_*", Scope: "global"},
		{RuleType: "ask", Pattern: "execute_*", Scope: "session", SessionID: "sess-1"},
	}
	for _, r := range rulesToCreate {
		if err := store.Create(r); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}

	rules, err = store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rules) != 3 {
		t.Errorf("List 期望 3 条，得到 %d 条", len(rules))
	}
}

func TestStore_FindByPattern(t *testing.T) {
	store := newTestStore(t)

	rules := []*StoredRule{
		{RuleType: "allow", Pattern: "read_file", Scope: "global"},
		{RuleType: "deny", Pattern: "write_*", Scope: "global"},
		{RuleType: "allow", Pattern: "read_file:*.go", Scope: "global"},
	}
	for _, r := range rules {
		if err := store.Create(r); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}

	// 测试匹配工具名
	rule, err := store.FindByPattern("read_file", nil)
	if err != nil {
		t.Fatalf("FindByPattern 失败: %v", err)
	}
	if rule == nil {
		t.Fatal("FindByPattern 期望匹配到规则")
	}
	if rule.RuleType != "allow" {
		t.Errorf("RuleType 期望 %q, 得到 %q", "allow", rule.RuleType)
	}

	// 测试不匹配
	rule, err = store.FindByPattern("unknown_tool", nil)
	if err != nil {
		t.Fatalf("FindByPattern 失败: %v", err)
	}
	if rule != nil {
		t.Errorf("不匹配时 FindByPattern 应返回 nil，得到 %+v", rule)
	}

	// 测试通配符匹配
	rule, err = store.FindByPattern("write_file", nil)
	if err != nil {
		t.Fatalf("FindByPattern 失败: %v", err)
	}
	if rule == nil {
		t.Fatal("通配符模式应匹配")
	}
	if rule.RuleType != "deny" {
		t.Errorf("RuleType 期望 %q, 得到 %q", "deny", rule.RuleType)
	}
}

func TestStore_CRUD事务(t *testing.T) {
	store := newTestStore(t)

	// 批量创建多条规则
	rules := []*StoredRule{
		{RuleType: "allow", Pattern: "tool_1", Scope: "global"},
		{RuleType: "deny", Pattern: "tool_2", Scope: "global"},
		{RuleType: "ask", Pattern: "tool_3", Scope: "session", SessionID: "sess-1"},
	}
	for _, r := range rules {
		if err := store.Create(r); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}

	// 验证全部创建成功
	all, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("期望 3 条规则，得到 %d 条", len(all))
	}

	// 更新第二条规则
	rules[1].RuleType = "allow"
	if err := store.Update(rules[1]); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	// 删除第三条规则
	if err := store.Delete(rules[2].ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 验证最终状态
	all, err = store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("期望 2 条规则，得到 %d 条", len(all))
	}

	// 验证更新后的规则
	updated, err := store.Get(rules[1].ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if updated.RuleType != "allow" {
		t.Errorf("更新后 RuleType 期望 %q, 得到 %q", "allow", updated.RuleType)
	}
}

func TestStore_并发安全(t *testing.T) {
	store := newTestStore(t)

	// 并发创建规则
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			rule := &StoredRule{
				RuleType: "allow",
				Pattern:  "tool",
				Scope:    "global",
			}
			_ = store.Create(rule)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证结果
	all, err := store.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(all) != 10 {
		t.Errorf("期望 10 条规则，得到 %d 条", len(all))
	}
}

func TestStore_时间戳设置(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	rule := &StoredRule{
		RuleType:  "allow",
		Pattern:   "test",
		Scope:     "global",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := store.Create(rule)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	got, err := store.Get(rule.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}

	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt 不应为空")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt 不应为空")
	}
}
