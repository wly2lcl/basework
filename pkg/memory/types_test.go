package memory

import (
	"encoding/json"
	"testing"
	"time"
)

// TestMemoryLayerConstants 验证 MemoryLayer 常量定义正确。
func TestMemoryLayerConstants(t *testing.T) {
	tests := []struct {
		layer MemoryLayer
		want  string
	}{
		{LayerUser, "user"},
		{LayerProject, "project"},
		{LayerLocal, "local"},
		{LayerAuto, "auto"},
	}

	for _, tt := range tests {
		if string(tt.layer) != tt.want {
			t.Errorf("MemoryLayer(%s) = %q, want %q", tt.layer, tt.layer, tt.want)
		}
	}
}

// TestEntryCreation 验证 Entry 结构体的创建和字段访问。
func TestEntryCreation(t *testing.T) {
	now := time.Now()
	entry := Entry{
		Layer:     LayerUser,
		Content:   "这是一条测试记忆",
		Tags:      []string{"test", "demo"},
		CreatedAt: now,
	}

	if entry.Layer != LayerUser {
		t.Errorf("Layer = %q, want %q", entry.Layer, LayerUser)
	}
	if entry.Content != "这是一条测试记忆" {
		t.Errorf("Content = %q, want %q", entry.Content, "这是一条测试记忆")
	}
	if len(entry.Tags) != 2 || entry.Tags[0] != "test" || entry.Tags[1] != "demo" {
		t.Errorf("Tags = %v, want [test demo]", entry.Tags)
	}
	if !entry.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
}

// TestEntryEmptyTags 验证 Tags 为 nil 时的行为。
func TestEntryEmptyTags(t *testing.T) {
	entry := Entry{
		Layer:   LayerProject,
		Content: "no tags",
	}

	if entry.Tags != nil {
		t.Error("Tags should be nil when not set")
	}
}

// TestEntryZeroTime 验证零值时间的 Entry。
func TestEntryZeroTime(t *testing.T) {
	entry := Entry{
		Layer:   LayerAuto,
		Content: "zero time",
	}

	if !entry.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero value")
	}
}

// TestMemoryLayerString 验证 MemoryLayer 的字符串表示。
func TestMemoryLayerString(t *testing.T) {
	if string(LayerUser) != "user" {
		t.Errorf("LayerUser = %q", LayerUser)
	}
	if string(LayerProject) != "project" {
		t.Errorf("LayerProject = %q", LayerProject)
	}
	if string(LayerLocal) != "local" {
		t.Errorf("LayerLocal = %q", LayerLocal)
	}
	if string(LayerAuto) != "auto" {
		t.Errorf("LayerAuto = %q", LayerAuto)
	}
}

// TestEntryJSON 验证 Entry 的 JSON 序列化和反序列化。
func TestEntryJSON(t *testing.T) {
	now := time.Now()
	entry := Entry{
		Layer:     LayerUser,
		Content:   "json test",
		Tags:      []string{"a", "b"},
		CreatedAt: now,
	}

	data, err := entry.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded Entry
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if decoded.Layer != entry.Layer {
		t.Errorf("Layer mismatch")
	}
	if decoded.Content != entry.Content {
		t.Errorf("Content mismatch")
	}
	if len(decoded.Tags) != 2 {
		t.Errorf("Tags length mismatch")
	}
}

// Helper: implement MarshalJSON/UnmarshalJSON for Entry so JSON tests work
func (e Entry) MarshalJSON() ([]byte, error) {
	type alias Entry
	return json.Marshal(alias(e))
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	type alias Entry
	return json.Unmarshal(data, (*alias)(e))
}
