package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wly2lcl/basework/pkg/tool"
)

// EditTool 精确字符串替换
type EditTool struct{}

func (e *EditTool) Name() string { return "edit" }

func (e *EditTool) Description() string { return "在文件中进行精确字符串替换" }

func (e *EditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"old": {"type": "string", "description": "Text to find"},
			"new": {"type": "string", "description": "Replacement text"}
		},
		"required": ["path", "old", "new"]
	}`)
}

func (e *EditTool) Execute(_ context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	if params.Old == "" {
		return &tool.Result{Content: "old 文本不能为空", IsError: true}, nil
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("读取文件失败: %v", err),
			IsError: true,
		}, nil
	}

	content := string(data)
	count := strings.Count(content, params.Old)

	switch {
	case count == 0:
		return &tool.Result{
			Content: fmt.Sprintf("未找到匹配的文本: %q", params.Old),
			IsError: true,
		}, nil
	case count > 1:
		return &tool.Result{
			Content: fmt.Sprintf("找到 %d 处匹配，请提供更多上下文以唯一匹配", count),
			IsError: true,
		}, nil
	default:
		newContent := strings.Replace(content, params.Old, params.New, 1)
		if err := os.WriteFile(params.Path, []byte(newContent), 0644); err != nil {
			return &tool.Result{
				Content: fmt.Sprintf("写入文件失败: %v", err),
				IsError: true,
			}, nil
		}
		return &tool.Result{Content: fmt.Sprintf("替换成功：%q → %q", params.Old, params.New)}, nil
	}
}