package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/wly2lcl/basework/pkg/tool"
)

// ReadTool 读取文件内容
type ReadTool struct{}

func (r *ReadTool) Name() string { return "read" }

func (r *ReadTool) Description() string { return "读取文件内容，支持 offset 和 limit 参数" }

func (r *ReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"offset": {"type": "integer", "description": "Start line (1-indexed, default 1)"},
			"limit": {"type": "integer", "description": "Max lines to read (default all)"}
		},
		"required": ["path"]
	}`)
}

func (r *ReadTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	// 工具级超时控制
	timeout := getTimeoutConfig().GetTimeout("read")
	ctx, cancel := WithTimeout(ctx, "read", timeout)
	defer cancel()
	_ = ctx // 为未来 context-aware 操作预留

	// 敏感路径检查
	if pc := getPathChecker(); pc != nil {
		if allowed, reason := pc.CheckPath(params.Path); !allowed {
			return &tool.Result{Content: fmt.Sprintf("访问被拒绝: %s (%s)", params.Path, reason), IsError: true}, nil
		}
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("读取文件失败: %v", err),
			IsError: true,
		}, nil
	}

	lines := bytes.Split(data, []byte("\n"))
	// 去掉最后一行如果为空（文件末尾换行符）
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	offset := params.Offset
	if offset <= 0 {
		offset = 1
	}

	limit := params.Limit
	if limit <= 0 {
		limit = len(lines)
	}

	start := offset - 1
	if start >= len(lines) {
		return &tool.Result{Content: "", IsError: false}, nil
	}

	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}

	var buf bytes.Buffer
	for i := start; i < end; i++ {
		buf.WriteString(fmt.Sprintf("%d: %s\n", i+1, string(lines[i])))
	}

	return &tool.Result{Content: buf.String()}, nil
}