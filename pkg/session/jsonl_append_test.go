package session

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLStore_AppendPreservesExistingBytes(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Create(CreateOpts{Title: "incremental"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted,
		Data: rawJSON(t, PromptedData{Content: "first"})}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, info.ID+".jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{SessionID: info.ID, Type: EventTextEnded}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Fatal("增量追加不应重写已有 JSONL 字节")
	}
}

func TestJSONLStore_AppendRefreshesExternalWriter(t *testing.T) {
	dir := t.TempDir()
	first, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := first.Create(CreateOpts{Title: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Events(EventFilter{SessionID: info.ID}); err != nil {
		t.Fatal(err)
	}

	second, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted,
		Data: rawJSON(t, PromptedData{Content: "from second"})}); err != nil {
		t.Fatal(err)
	}
	if err := first.AppendEvent(Event{SessionID: info.ID, Type: EventTextEnded}); err != nil {
		t.Fatal(err)
	}

	events, err := first.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("跨实例追加后事件/序号错误: %+v", events)
	}
}

func TestJSONLStore_CrossInstanceConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	seed, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := seed.Create(CreateOpts{Title: "cross-process"})
	if err != nil {
		t.Fatal(err)
	}
	stores := make([]*JSONLStore, 2)
	for i := range stores {
		stores[i], err = NewJSONLStore(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	const perStore = 20
	errs := make(chan error, len(stores)*perStore)
	for i, store := range stores {
		go func(i int, store *JSONLStore) {
			for j := 0; j < perStore; j++ {
				errs <- store.AppendEvent(Event{SessionID: info.ID, Type: EventTextDelta,
					Data: rawJSON(t, TextDeltaData{Delta: "parallel"})})
			}
		}(i, store)
	}
	for i := 0; i < len(stores)*perStore; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("跨实例并发追加失败: %v", err)
		}
	}
	check, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := check.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(stores)*perStore {
		t.Fatalf("跨实例并发追加事件数 = %d，期望 %d", len(events), len(stores)*perStore)
	}
	for i, event := range events {
		if event.Seq != int64(i+1) {
			t.Fatalf("跨实例并发追加 Seq[%d] = %d，期望 %d", i, event.Seq, i+1)
		}
	}
}

func TestJSONLStore_RecoversIncompleteFinalLine(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Create(CreateOpts{Title: "tail"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted,
		Data: rawJSON(t, PromptedData{Content: "kept"})}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, info.ID+".jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"id":"crash-tail"`); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := reopened.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 1 {
		t.Fatalf("不完整尾部不应丢失前一条事件: %+v", events)
	}
	if err := reopened.AppendEvent(Event{SessionID: info.ID, Type: EventTextEnded}); err != nil {
		t.Fatal(err)
	}
	events, err = reopened.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Seq != 2 {
		t.Fatalf("修剪残尾后追加结果错误: %+v", events)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("crash-tail")) {
		t.Fatal("崩溃残尾应在下一次追加前被修剪")
	}
}

func TestJSONLStore_MiddleCorruptionStillErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jsonl")
	if err := os.WriteFile(path, []byte("{\"_schema\":\"basework.session\",\"v\":2}\nnot-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Events(EventFilter{SessionID: "broken"}); err == nil {
		t.Fatal("带换行的中间损坏必须显式报错")
	}
}

func TestJSONLStore_AppendEncodingFailureDoesNotMutateCache(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Create(CreateOpts{Title: "encode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: []byte("{")}); err == nil {
		t.Fatal("非法事件数据应编码失败")
	}
	events, err := store.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("编码失败不应污染缓存: %+v", events)
	}
	if err := store.AppendEvent(Event{SessionID: info.ID, Type: EventTextEnded}); err != nil {
		t.Fatal(err)
	}
	events, err = store.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 1 {
		t.Fatalf("编码失败后 Seq 不应跳号: %+v", events)
	}
}
