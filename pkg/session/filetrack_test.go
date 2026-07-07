package session

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestFileTracker_TrackRead(t *testing.T) {
	ft := NewFileTracker()
	ft.TrackRead("/path/to/file.go")

	files := ft.GetReadFiles()
	if len(files) != 1 {
		t.Fatalf("期望 1 个读取文件，得到 %d", len(files))
	}
	if files[0] != "/path/to/file.go" {
		t.Errorf("文件路径 = %q, 期望 %q", files[0], "/path/to/file.go")
	}

	tracked := ft.GetTrackedFiles()
	if len(tracked) != 1 {
		t.Fatalf("期望 1 个追踪文件，得到 %d", len(tracked))
	}
}

func TestFileTracker_TrackWrite(t *testing.T) {
	ft := NewFileTracker()
	ft.TrackWrite("/path/to/output.txt")

	modified := ft.GetModifiedFiles()
	if len(modified) != 1 {
		t.Fatalf("期望 1 个修改文件，得到 %d", len(modified))
	}
	if modified[0] != "/path/to/output.txt" {
		t.Errorf("文件路径 = %q, 期望 %q", modified[0], "/path/to/output.txt")
	}
}

func TestFileTracker_TrackEdit(t *testing.T) {
	ft := NewFileTracker()
	ft.TrackEdit("/path/to/edit.go")

	modified := ft.GetModifiedFiles()
	if len(modified) != 1 {
		t.Fatalf("期望 1 个修改文件（编辑），得到 %d", len(modified))
	}
}

func TestFileTracker_Dedup(t *testing.T) {
	ft := NewFileTracker()

	// 重复追踪同一个文件
	ft.TrackRead("/path/to/file.go")
	ft.TrackRead("/path/to/file.go")
	ft.TrackWrite("/path/to/file.go")

	files := ft.GetTrackedFiles()
	if len(files) != 1 {
		t.Fatalf("去重后期望 1 个文件，得到 %d: %v", len(files), files)
	}

	readFiles := ft.GetReadFiles()
	if len(readFiles) != 1 {
		t.Fatalf("期望 1 个读取文件，得到 %d", len(readFiles))
	}
}

func TestFileTracker_GetModifiedFiles(t *testing.T) {
	ft := NewFileTracker()
	ft.TrackRead("/path/to/read.txt")
	ft.TrackWrite("/path/to/write.txt")
	ft.TrackEdit("/path/to/edit.txt")

	modified := ft.GetModifiedFiles()
	if len(modified) != 2 {
		t.Fatalf("期望 2 个修改文件，得到 %d", len(modified))
	}

	tracked := ft.GetTrackedFiles()
	if len(tracked) != 3 {
		t.Fatalf("期望 3 个追踪文件，得到 %d", len(tracked))
	}
}

func TestFileTracker_JSONSerialization(t *testing.T) {
	ft := NewFileTracker()
	ft.TrackRead("/a.go")
	ft.TrackWrite("/b.go")
	ft.TrackEdit("/c.go")

	data, err := json.Marshal(ft)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	// 反序列化回新的 FileTracker
	ft2 := NewFileTracker()
	if err := json.Unmarshal(data, ft2); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	tracked := ft2.GetTrackedFiles()
	if len(tracked) != 3 {
		t.Fatalf("反序列化后期望 3 个文件，得到 %d", len(tracked))
	}

	readFiles := ft2.GetReadFiles()
	if len(readFiles) != 1 {
		t.Fatalf("反序列化后期望 1 个读取文件，得到 %d", len(readFiles))
	}

	modified := ft2.GetModifiedFiles()
	if len(modified) != 2 {
		t.Fatalf("反序列化后期望 2 个修改文件，得到 %d", len(modified))
	}
}

func TestFileTracker_Empty(t *testing.T) {
	ft := NewFileTracker()

	tracked := ft.GetTrackedFiles()
	if len(tracked) != 0 {
		t.Fatalf("新 FileTracker 期望 0 个文件，得到 %d", len(tracked))
	}

	readFiles := ft.GetReadFiles()
	if len(readFiles) != 0 {
		t.Fatalf("新 FileTracker 期望 0 个读取文件，得到 %d", len(readFiles))
	}

	modified := ft.GetModifiedFiles()
	if len(modified) != 0 {
		t.Fatalf("新 FileTracker 期望 0 个修改文件，得到 %d", len(modified))
	}
}

func TestFileTracker_ConcurrentSafety(t *testing.T) {
	ft := NewFileTracker()
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := "/path/to/file"
			switch i % 3 {
			case 0:
				ft.TrackRead(path)
			case 1:
				ft.TrackWrite(path)
			case 2:
				ft.TrackEdit(path)
			}
		}(i)
	}
	wg.Wait()

	// 虽然并发调用了 100 次，但去重后应只有 1 个文件
	tracked := ft.GetTrackedFiles()
	if len(tracked) != 1 {
		t.Fatalf("并发去重后期望 1 个文件，得到 %d", len(tracked))
	}
}
