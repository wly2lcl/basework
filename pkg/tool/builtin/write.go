package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wly2lcl/basework/pkg/tool"
)

// WriteTool 写入文件内容
type WriteTool struct {
	// Runtime 是实例级运行时注入。nil 时回落包级全局，行为与旧版本一致。
	Runtime *Runtime
}

func (w *WriteTool) Name() string { return "write" }

func (w *WriteTool) Description() string { return "写入文件内容，自动创建父目录" }

func (w *WriteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"},
			"content": {"type": "string"}
		},
		"required": ["path", "content"]
	}`)
}

func (w *WriteTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}, nil
	}

	// 工具级超时控制（实例注入优先，回落全局）
	timeout := w.Runtime.resolveTimeout().GetTimeout("write")
	ctx, cancel := WithTimeoutBus(ctx, "write", timeout, w.Runtime.resolveEventBus())
	defer cancel()
	_ = ctx // 为未来 context-aware 操作预留

	// 敏感路径检查（实例注入优先，回落全局）
	if pc := w.Runtime.resolvePathChecker(); pc != nil {
		if allowed, reason := pc.CheckPath(params.Path); !allowed {
			return &tool.Result{Content: fmt.Sprintf("访问被拒绝: %s (%s)", params.Path, reason), IsError: true}, nil
		}
	}

	// 自动创建父目录
	dir := filepath.Dir(params.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("创建目录失败: %v", err),
			IsError: true,
		}, nil
	}

	// 原子写入：写 .tmp 文件 → os.Rename
	tmpPath := params.Path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(params.Content), 0644); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("写入临时文件失败: %v", err),
			IsError: true,
		}, nil
	}

	if err := os.Rename(tmpPath, params.Path); err != nil {
		// 清理临时文件
		os.Remove(tmpPath)
		return &tool.Result{
			Content: fmt.Sprintf("重命名文件失败: %v", err),
			IsError: true,
		}, nil
	}

	return &tool.Result{Content: fmt.Sprintf("成功写入 %s (%d 字节)", params.Path, len(params.Content))}, nil
}
