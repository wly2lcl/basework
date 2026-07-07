package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Store 接口
// ---------------------------------------------------------------------------

// Store 是记忆存储的抽象接口。
type Store interface {
	// Write 写入一条记忆。
	Write(layer MemoryLayer, content string, tags ...string) error

	// Read 读取指定层级的全部记忆。
	Read(layer MemoryLayer) ([]Entry, error)

	// Search 在所有层级中搜索匹配查询的记忆。
	Search(query string, limit int) ([]Entry, error)

	// Clear 清空指定层级的全部记忆。
	Clear(layer MemoryLayer) error

	// List 返回所有层级的全部记忆。
	List() map[MemoryLayer][]Entry
}

// ---------------------------------------------------------------------------
// FileStore — 基于文件的 Markdown 格式存储
// ---------------------------------------------------------------------------

// FileStore 将记忆以 Markdown 格式存储到文件中。
// 每个 MemoryLayer 映射到不同的文件路径。
type FileStore struct {
	workspace string // 项目工作区根路径
}

// NewFileStore 创建一个新的 FileStore。
// workspace 是项目根目录，用于解析项目级和本地级文件路径。
func NewFileStore(workspace string) *FileStore {
	return &FileStore{workspace: workspace}
}

// Write 将一条记忆写入对应层级的文件。
// 如果文件不存在，自动创建目录和文件。
func (fs *FileStore) Write(layer MemoryLayer, content string, tags ...string) error {
	path, err := fs.pathForLayer(layer)
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// 构建条目文本
	entry := formatEntry(layer, content, tags...)

	// 追加到文件
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

// Read 读取指定层级的全部记忆。
func (fs *FileStore) Read(layer MemoryLayer) ([]Entry, error) {
	path, err := fs.pathForLayer(layer)
	if err != nil {
		return nil, err
	}

	entries, err := fs.readEntriesFromFile(path, layer)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	return entries, nil
}

// Search 在所有记忆文件中搜索匹配查询文本的记忆。
// 使用简单的子串匹配（不区分大小写）。
func (fs *FileStore) Search(query string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 10
	}

	query = strings.ToLower(query)
	var results []Entry

	layers := []MemoryLayer{LayerUser, LayerProject, LayerLocal, LayerAuto}
	for _, layer := range layers {
		path, err := fs.pathForLayer(layer)
		if err != nil {
			continue
		}

		entries, err := fs.readEntriesFromFile(path, layer)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Content), query) {
				results = append(results, e)
			}
		}
	}

	// 按时间从新到旧排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Clear 清空指定层级的文件。
func (fs *FileStore) Clear(layer MemoryLayer) error {
	path, err := fs.pathForLayer(layer)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

// List 返回所有层级的全部记忆。
func (fs *FileStore) List() map[MemoryLayer][]Entry {
	result := make(map[MemoryLayer][]Entry)

	layers := []MemoryLayer{LayerUser, LayerProject, LayerLocal, LayerAuto}
	for _, layer := range layers {
		path, err := fs.pathForLayer(layer)
		if err != nil {
			continue
		}

		entries, err := fs.readEntriesFromFile(path, layer)
		if err != nil {
			continue
		}
		result[layer] = entries
	}
	return result
}

// ---------------------------------------------------------------------------
// 文件路径解析
// ---------------------------------------------------------------------------

// pathForLayer 返回指定层级对应的文件路径。
func (fs *FileStore) pathForLayer(layer MemoryLayer) (string, error) {
	switch layer {
	case LayerUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		return filepath.Join(home, ".config", "basework", "AGENTS.md"), nil

	case LayerProject:
		if fs.workspace == "" {
			return "", fmt.Errorf("workspace not set for project layer")
		}
		return filepath.Join(fs.workspace, "AGENTS.md"), nil

	case LayerLocal:
		if fs.workspace == "" {
			return "", fmt.Errorf("workspace not set for local layer")
		}
		return filepath.Join(fs.workspace, ".basework", "AGENTS.md"), nil

	case LayerAuto:
		if fs.workspace == "" {
			return "", fmt.Errorf("workspace not set for auto layer")
		}
		return filepath.Join(fs.workspace, ".basework", "auto-memory.md"), nil

	default:
		return "", fmt.Errorf("unknown memory layer: %s", layer)
	}
}

// ---------------------------------------------------------------------------
// Markdown 格式的读写
// ---------------------------------------------------------------------------

// entrySeparator 是 Markdown 文件中记忆条目之间的分隔符。
const entrySeparator = "\n---\n"

// formatEntry 将记忆格式化为 Markdown 文本。
func formatEntry(layer MemoryLayer, content string, tags ...string) string {
	var b strings.Builder

	// 时间戳行
	b.WriteString(fmt.Sprintf("[%s]", time.Now().Format(time.RFC3339)))

	// 标签行
	if len(tags) > 0 {
		b.WriteString(" tags: ")
		b.WriteString(strings.Join(tags, ", "))
	}
	b.WriteString("\n")

	// 内容
	b.WriteString(content)
	b.WriteString("\n")

	// 分隔符
	b.WriteString(entrySeparator)

	return b.String()
}

// parseEntry 解析一条记忆条目的文本内容。
// 格式:
//
//	[timestamp] tags: tag1, tag2
//	content...
func parseEntry(layer MemoryLayer, text string) (Entry, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Entry{}, false
	}

	lines := strings.SplitN(text, "\n", 2)
	if len(lines) < 2 {
		return Entry{}, false
	}

	header := strings.TrimSpace(lines[0])
	content := strings.TrimSpace(lines[1])

	// 解析头部: [timestamp] tags: tag1, tag2
	var timestamp time.Time
	var tags []string

	if strings.HasPrefix(header, "[") {
		closeBracket := strings.Index(header, "]")
		if closeBracket > 0 {
			tsStr := header[1:closeBracket]
			timestamp, _ = time.Parse(time.RFC3339, tsStr)

			// 解析标签
			rest := strings.TrimSpace(header[closeBracket+1:])
			if strings.HasPrefix(rest, "tags:") {
				tagStr := strings.TrimSpace(rest[5:])
				if tagStr != "" {
					for _, tag := range strings.Split(tagStr, ",") {
						t := strings.TrimSpace(tag)
						if t != "" {
							tags = append(tags, t)
						}
					}
				}
			}
		}
	}

	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return Entry{
		Layer:     layer,
		Content:   content,
		Tags:      tags,
		CreatedAt: timestamp,
	}, true
}

// readEntriesFromFile 从文件中读取所有记忆条目。
func (fs *FileStore) readEntriesFromFile(path string, layer MemoryLayer) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := string(data)
	// 按分隔符分割
	rawEntries := strings.Split(text, entrySeparator)

	var entries []Entry
	for _, raw := range rawEntries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		if entry, ok := parseEntry(layer, raw); ok {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}
