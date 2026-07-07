package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wly2lcl/basework/pkg/tool"
)

// GlobTool 文件模式匹配
type GlobTool struct{}

func (g *GlobTool) Name() string { return "glob" }

func (g *GlobTool) Description() string { return "使用 glob 模式匹配文件路径" }

func (g *GlobTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string"},
			"path": {"type": "string", "description": "Base directory"}
		},
		"required": ["pattern", "path"]
	}`)
}

func (g *GlobTool) Execute(_ context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	var buf bytes.Buffer
	matchCount := 0
	const maxResults = 200

	err := filepath.WalkDir(params.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无法访问的目录
		}

		// 跳过隐藏目录
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// 匹配文件名
		matched, err := filepath.Match(params.Pattern, d.Name())
		if err != nil {
			return nil
		}
		if matched {
			buf.WriteString(path + "\n")
			matchCount++
			if matchCount >= maxResults {
				buf.WriteString(fmt.Sprintf("... (结果超过 %d 条，已截断)", maxResults))
				return filepath.SkipAll
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
