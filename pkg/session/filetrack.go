package session

import (
	"encoding/json"
	"sync"
)

// FileOpType 表示文件操作类型。
type FileOpType string

const (
	FileRead  FileOpType = "read"
	FileWrite FileOpType = "write"
	FileEdit  FileOpType = "edit"
)

// FileRecord 记录一次文件操作。
type FileRecord struct {
	Path string     `json:"path"`
	Op   FileOpType `json:"op"`
}

// FileTracker 追踪会话中访问或修改的文件。
// 线程安全，使用 map 去重。
type FileTracker struct {
	mu         sync.Mutex
	readFiles  map[string]bool
	writeFiles map[string]bool
	editFiles  map[string]bool
	allFiles   map[string]bool
}

// NewFileTracker 创建一个新的 FileTracker。
func NewFileTracker() *FileTracker {
	return &FileTracker{
		readFiles:  make(map[string]bool),
		writeFiles: make(map[string]bool),
		editFiles:  make(map[string]bool),
		allFiles:   make(map[string]bool),
	}
}

// TrackRead 记录一次文件读取操作。
func (ft *FileTracker) TrackRead(path string) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.readFiles[path] = true
	ft.allFiles[path] = true
}

// TrackWrite 记录一次文件写入操作。
func (ft *FileTracker) TrackWrite(path string) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.writeFiles[path] = true
	ft.allFiles[path] = true
}

// TrackEdit 记录一次文件编辑操作。
func (ft *FileTracker) TrackEdit(path string) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.editFiles[path] = true
	ft.allFiles[path] = true
}

// GetTrackedFiles 返回所有被追踪的文件路径（已去重）。
func (ft *FileTracker) GetTrackedFiles() []string {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	files := make([]string, 0, len(ft.allFiles))
	for path := range ft.allFiles {
		files = append(files, path)
	}
	return files
}

// GetReadFiles 返回被读取的文件路径列表。
func (ft *FileTracker) GetReadFiles() []string {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	files := make([]string, 0, len(ft.readFiles))
	for path := range ft.readFiles {
		files = append(files, path)
	}
	return files
}

// GetModifiedFiles 返回被修改（写入或编辑）的文件路径列表。
func (ft *FileTracker) GetModifiedFiles() []string {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	modified := make(map[string]bool)
	for path := range ft.writeFiles {
		modified[path] = true
	}
	for path := range ft.editFiles {
		modified[path] = true
	}

	files := make([]string, 0, len(modified))
	for path := range modified {
		files = append(files, path)
	}
	return files
}

// MarshalJSON 实现 json.Marshaler 接口，用于持久化。
func (ft *FileTracker) MarshalJSON() ([]byte, error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	records := make([]FileRecord, 0, len(ft.allFiles))
	for path := range ft.readFiles {
		records = append(records, FileRecord{Path: path, Op: FileRead})
	}
	for path := range ft.writeFiles {
		records = append(records, FileRecord{Path: path, Op: FileWrite})
	}
	for path := range ft.editFiles {
		records = append(records, FileRecord{Path: path, Op: FileEdit})
	}

	return json.Marshal(records)
}

// UnmarshalJSON 实现 json.Unmarshaler 接口，用于从持久化数据恢复。
func (ft *FileTracker) UnmarshalJSON(data []byte) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	var records []FileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}

	ft.readFiles = make(map[string]bool)
	ft.writeFiles = make(map[string]bool)
	ft.editFiles = make(map[string]bool)
	ft.allFiles = make(map[string]bool)

	for _, r := range records {
		ft.allFiles[r.Path] = true
		switch r.Op {
		case FileRead:
			ft.readFiles[r.Path] = true
		case FileWrite:
			ft.writeFiles[r.Path] = true
		case FileEdit:
			ft.editFiles[r.Path] = true
		}
	}

	return nil
}
