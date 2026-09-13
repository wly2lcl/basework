package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateOptsID_Preserved 校验三个 Store 都能按调用方给定的 ID 创建会话。
//
// 这条能力的来源是 SHIP-002：JSONL→SQLite 迁移必须保留原会话身份，否则事件里带的
// 原 session_id 找不到归属，逐条外键失败，"迁移成功"却零条落库。
func TestCreateOptsID_Preserved(t *testing.T) {
	const want = "fixed-session-id-1"

	t.Run("memory", func(t *testing.T) {
		s := NewMemoryStore()
		info, err := s.Create(CreateOpts{ID: want, Title: "保留 ID"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if info.ID != want {
			t.Fatalf("ID = %q，期望 %q", info.ID, want)
		}
		got, err := s.Get(want)
		if err != nil || got.ID != want {
			t.Fatalf("按原 ID 取不到会话: %v", err)
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		dir := t.TempDir()
		s, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("NewJSONLStore 失败: %v", err)
		}
		info, err := s.Create(CreateOpts{ID: want, Title: "保留 ID"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if info.ID != want {
			t.Fatalf("ID = %q，期望 %q", info.ID, want)
		}
		if _, statErr := os.Stat(filepath.Join(dir, want+".jsonl")); statErr != nil {
			t.Fatalf("文件名未使用给定 ID: %v", statErr)
		}
	})
}

// TestCreateOptsID_RejectsInvalid 非法 ID 必须被拒绝而不是落盘。
// JSONL 后端把 ID 当作文件名，放行 "../x" 就是路径穿越。
func TestCreateOptsID_RejectsInvalid(t *testing.T) {
	bad := []string{"../evil", "a/b", "有空格 的", "tab\tid"}

	t.Run("memory", func(t *testing.T) {
		s := NewMemoryStore()
		for _, id := range bad {
			if _, err := s.Create(CreateOpts{ID: id}); err == nil {
				t.Errorf("MemoryStore 接受了非法 ID %q", id)
			}
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		dir := t.TempDir()
		s, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("NewJSONLStore 失败: %v", err)
		}
		for _, id := range bad {
			if _, err := s.Create(CreateOpts{ID: id}); err == nil {
				t.Errorf("JSONLStore 接受了非法 ID %q", id)
			}
		}
		// 拒绝之后目录里不应留下任何越界文件。
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("读取目录失败: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("非法 ID 被拒绝却留下了 %d 个文件", len(entries))
		}
	})
}

// TestCreateOptsID_DuplicateRejected 指定 ID 重复创建必须失败。
//
// 静默覆盖等于数据丢失：迁移重跑时旧数据会被新会话顶掉而无人察觉。
func TestCreateOptsID_DuplicateRejected(t *testing.T) {
	const id = "dup-session"

	t.Run("memory", func(t *testing.T) {
		s := NewMemoryStore()
		if _, err := s.Create(CreateOpts{ID: id}); err != nil {
			t.Fatalf("首次创建失败: %v", err)
		}
		if _, err := s.Create(CreateOpts{ID: id}); err == nil {
			t.Error("重复 ID 未报错")
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		dir := t.TempDir()
		s, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("NewJSONLStore 失败: %v", err)
		}
		if _, err := s.Create(CreateOpts{ID: id}); err != nil {
			t.Fatalf("首次创建失败: %v", err)
		}
		if _, err := s.Create(CreateOpts{ID: id}); err == nil {
			t.Error("重复 ID 未报错")
		}
	})
}

// TestJSONLList_PropagatesSchemaTooNew 覆盖修复前的静默降级：
// 一个格式版本高于当前的会话文件此前被 List 当作「加载失败」跳过，
// 命令输出是「没有找到会话」——用户看到的是数据消失，而不是版本不兼容。
func TestJSONLList_PropagatesSchemaTooNew(t *testing.T) {
	dir := t.TempDir()
	future := filepath.Join(dir, "future-session.jsonl")
	content := `{"_schema":"basework.session","v":99}` + "\n" +
		`{"id":"e1","session_id":"future-session","type":"prompted","data":{"content":"来自未来"},"seq":1,"created_at":"2026-09-01T00:00:00Z","v":99}` + "\n"
	if err := os.WriteFile(future, []byte(content), 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}

	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	_, err = s.List(ListFilter{})
	if err == nil {
		t.Fatal("版本过高的会话文件被静默跳过，List 未报错")
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("错误 = %v，期望 ErrSchemaTooNew", err)
	}
}

// TestJSONLList_StillSkipsCorruptFile 保证本次修复没有把「跳过」一刀切掉：
// 内容损坏（非版本问题）的文件仍应被跳过，不能让一个坏文件让整次列举失败。
func TestJSONLList_StillSkipsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "corrupt.jsonl"), []byte("这不是 JSON\n"), 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	infos, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("损坏文件不应让列举失败: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("期望跳过损坏文件得到 0 个会话，实际 %d", len(infos))
	}
}
