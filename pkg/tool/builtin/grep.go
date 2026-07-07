package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wly2lcl/basework/pkg/tool"
)

// GrepTool 在文件内容中进行正则搜索
type GrepTool struct{}

func (g *GrepTool) Name() string { return "grep" }

func (g *GrepTool) Description() string { return "在文件中搜索匹配正则表达式的行" }

func (g *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string"},
			"path": {"type": "string", "description": "Directory to search in"},
			"include": {"type": "string", "description": "File pattern (e.g. *.go)"}
		},
		"required": ["pattern", "path"]
	}`)
}

func (g *GrepTool) Execute(_ context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("正则表达式编译失败: %v", err),
			IsError: true,
		}, nil
	}

	var buf bytes.Buffer
	matchCount := 0
	const maxResults = 100

	err = filepath.WalkDir(params.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无法访问的目录
		}

		// 跳过目录
		if d.IsDir() {
			// 跳过隐藏目录和 vendor
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// 应用 include 过滤
		if params.Include != "" {
			matched, err := filepath.Match(params.Include, d.Name())
			if err != nil || !matched {
				return nil
			}
		}

		// 跳过隐藏文件
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		// 读取文件内容
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				buf.WriteString(fmt.Sprintf("%s:%d: %s\n", path, i+1, line))
				matchCount++
				if matchCount >= maxResults {
					buf.WriteString(fmt.Sprintf("... (结果超过 %d 条，已截断)", maxResults))
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("搜索失败: %v", err),
			IsError: true,
		}, nil
	}

	return &tool.Result{Content: buf.String()}, nil
}
