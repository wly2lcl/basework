package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceID_IsolatedAndStable(t *testing.T) {
	a := WorkspaceID("/tmp/ws-a")
	b := WorkspaceID("/tmp/ws-b")
	if a == b {
		t.Fatal("不同工作区必须得到不同 ID")
	}
	if a != WorkspaceID("/tmp/ws-a") {
		t.Fatal("同一工作区的 ID 必须稳定")
	}
	// 软链与真实路径是同一工作区
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("无法创建软链: %v", err)
	}
	if WorkspaceID(real) != WorkspaceID(link) {
		t.Fatal("软链与目标路径应是同一工作区")
	}
}

func TestWorkspaceFacts_Dedup(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	first := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	f := Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "a.go"), Path: "a.go", Ref: "plan-1", FirstSeen: first.Format(time.RFC3339)}

	if !w.Add(f) {
		t.Fatal("首次添加应为新事实")
	}
	// 相同事实再次投影：不新增、不刷新 FirstSeen
	again := f
	again.FirstSeen = time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if w.Add(again) {
		t.Fatal("相同 Key 不应重复累计")
	}
	if w.Count() != 1 {
		t.Fatalf("去重后应只有 1 条，得到 %d", w.Count())
	}
	if got := w.List()[0].FirstSeen; got != f.FirstSeen {
		t.Fatalf("FirstSeen 应保留首次时间 %s，得到 %s", f.FirstSeen, got)
	}
}

func TestWorkspaceFacts_RejectsFactWithoutKey(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	if w.Add(Fact{Kind: FactFileRead, Path: "a.go"}) {
		t.Fatal("无 Key 的事实应被拒绝")
	}
}

func TestWorkspaceFacts_ListSorted(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "b.go"), Path: "b.go"})
	w.Add(Fact{Kind: FactFileRead, Key: FileFactKey(FactFileRead, "a.go"), Path: "a.go"})
	w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "a.go"), Path: "a.go"})
	got := w.List()
	if len(got) != 3 {
		t.Fatalf("应 3 条，得到 %d", len(got))
	}
	if got[0].Key != "file_modified:a.go" || got[1].Key != "file_modified:b.go" || got[2].Kind != FactFileRead {
		t.Fatalf("排序不符: %+v", got)
	}
}

func TestSaveLoadFacts_Roundtrip(t *testing.T) {
	base := t.TempDir()
	w := NewWorkspaceFacts(WorkspaceID(base))
	w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "x/y.go"), Path: "x/y.go", Ref: "plan-abc", Source: "edit_files"})
	w.Add(Fact{Kind: FactCommand, Key: CommandFactKey("job-1"), Ref: "job-1", Source: "job"})

	if err := SaveWorkspaceFacts(base, w); err != nil {
		t.Fatalf("保存: %v", err)
	}
	loaded, err := LoadWorkspaceFacts(base, w.WorkspaceID)
	if err != nil {
		t.Fatalf("加载: %v", err)
	}
	if loaded.Count() != 2 {
		t.Fatalf("往返后应 2 条，得到 %d", loaded.Count())
	}
	byKey := map[string]Fact{}
	for _, f := range loaded.List() {
		byKey[f.Key] = f
	}
	file, ok := byKey[FileFactKey(FactFileModified, "x/y.go")]
	if !ok || file.Path != "x/y.go" || file.Ref != "plan-abc" || file.Source != "edit_files" {
		t.Fatalf("文件事实字段往返丢失: %+v", file)
	}
	cmd, ok := byKey[CommandFactKey("job-1")]
	if !ok || cmd.Ref != "job-1" || cmd.Source != "job" {
		t.Fatalf("命令事实字段往返丢失: %+v", cmd)
	}
}

func TestLoadFacts_MissingFileIsEmpty(t *testing.T) {
	base := t.TempDir()
	w, err := LoadWorkspaceFacts(base, "nonexistent-ws")
	if err != nil {
		t.Fatalf("文件缺失应返回空集合而非错误: %v", err)
	}
	if w.Count() != 0 || w.WorkspaceID != "nonexistent-ws" {
		t.Fatalf("应得到空集合: %+v", w)
	}
}

func TestLoadFacts_RejectsUnknownFutureVersion(t *testing.T) {
	base := t.TempDir()
	id := WorkspaceID(base)
	path := factsPath(base, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	env := factsEnvelope{Version: FactsSchemaVersion + 1, WorkspaceID: id, Facts: nil}
	data, _ := json.Marshal(env)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadWorkspaceFacts(base, id)
	if !errors.Is(err, ErrFactsTooNew) {
		t.Fatalf("未知未来版本必须拒绝，得到: %v", err)
	}
}

func TestLoadFacts_RejectsForeignWorkspace(t *testing.T) {
	base := t.TempDir()
	owner := WorkspaceID(base)
	w := NewWorkspaceFacts(owner)
	w.Add(Fact{Kind: FactFileRead, Key: FileFactKey(FactFileRead, "a.go"), Path: "a.go"})
	if err := SaveWorkspaceFacts(base, w); err != nil {
		t.Fatal(err)
	}
	// 模拟「别人的事实文件出现在我的读取路径下」：复制到他人 ID 的文件名。
	src := factsPath(base, owner)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(factsPath(base, "someone-else"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspaceFacts(base, "someone-else"); err == nil {
		t.Fatal("读取他人工作区的事实文件必须报错")
	}
}

func TestLoadFacts_V0LegacyArrayReadable(t *testing.T) {
	base := t.TempDir()
	id := WorkspaceID(base)
	path := factsPath(base, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// v0：无信封裸数组，且没有 Key 字段（Key 是 v1 引入的去重键）
	legacy := `[
		{"kind":"file_modified","path":"old.go","ref":"plan-old","first_seen":"2026-01-01T00:00:00Z"},
		{"kind":"command","ref":"job-legacy","first_seen":"2026-01-01T00:00:00Z"}
	]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWorkspaceFacts(base, id)
	if err != nil {
		t.Fatalf("v0 数据必须可读: %v", err)
	}
	if w.Count() != 2 {
		t.Fatalf("应恢复 2 条，得到 %d", w.Count())
	}
	// Key 按当前规则重建，去重语义与 v1 一致
	if w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "old.go"), Path: "old.go"}) {
		t.Fatal("v0 恢复的事实应能被 v1 键去重")
	}
}

func TestFoldFileEdited_IdempotentProjection(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	data := FileEditedData{
		Phase:  "committed",
		PlanID: "plan-1",
		Files: []FileEditRecord{
			{Path: "a.go", State: "written"},
			{Path: "b.go", State: "conflict"}, // 冲突不是事实
		},
	}
	at := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	FoldFileEdited(w, data, at)
	FoldFileEdited(w, data, at) // 重复投影
	FoldFileEdited(w, data, at.Add(time.Hour))

	if w.Count() != 1 {
		t.Fatalf("重复投影 + 冲突文件不得新增事实，应 1 条，得到 %d", w.Count())
	}
	f := w.List()[0]
	if f.Kind != FactFileModified || f.Path != "a.go" || f.Ref != "plan-1" {
		t.Fatalf("折叠结果不符: %+v", f)
	}

	// preview 不产生事实
	w2 := NewWorkspaceFacts("ws2")
	FoldFileEdited(w2, FileEditedData{Phase: "preview", PlanID: "plan-2",
		Files: []FileEditRecord{{Path: "c.go", State: "preview"}}}, at)
	if w2.Count() != 0 {
		t.Fatalf("预览不构成事实，得到 %d", w2.Count())
	}

	// rolled_back 产生 file_reverted
	w3 := NewWorkspaceFacts("ws3")
	FoldFileEdited(w3, FileEditedData{Phase: "rolled_back", PlanID: "plan-1",
		Files: []FileEditRecord{{Path: "a.go", State: "reverted"}}}, at)
	if w3.Count() != 1 || w3.List()[0].Kind != FactFileReverted {
		t.Fatalf("撤销应产生 file_reverted: %+v", w3.List())
	}
}

func TestFoldFileEdited_PartialFailureKeepsWrittenFact(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	FoldFileEdited(w, FileEditedData{Phase: "failed", PlanID: "partial", Files: []FileEditRecord{
		{Path: "a.go", State: "written"}, {Path: "b.go", State: "conflict"},
	}}, time.Now())
	if w.Count() != 1 || w.List()[0].Path != "a.go" {
		t.Fatalf("部分提交的已写文件必须保留事实: %+v", w.List())
	}
}
