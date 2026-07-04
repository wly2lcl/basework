package tools

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

// ApplyPatchTool 实现 apply_patch 工具，用于应用结构化补丁到文件系统
type ApplyPatchTool struct {
	// WorkDir 是补丁操作的工作目录，为空时使用当前目录
	WorkDir string
}

// NewApplyPatchTool 创建 apply_patch 工具
func NewApplyPatchTool(workDir string) *ApplyPatchTool {
	if workDir == "" {
		workDir = "."
	}
	return &ApplyPatchTool{WorkDir: workDir}
}

// Name 返回工具名称
func (a *ApplyPatchTool) Name() string {
	return "apply_patch"
}

// Description 返回工具描述
func (a *ApplyPatchTool) Description() string {
	return "应用结构化补丁到文件系统，支持添加、删除和更新文件操作"
}

// Parameters 返回工具参数 JSON Schema
func (a *ApplyPatchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {
				"type": "string",
				"description": "结构化补丁文本，格式：*** Begin Patch / *** End Patch，支持 Add/Delete/Update 操作"
			}
		},
		"required": ["patch"]
	}`)
}

// applyPatchParams 工具参数结构
type applyPatchParams struct {
	Patch string `json:"patch"`
}

// patchOperation 补丁操作类型
type patchOperation string

const (
	opAdd    patchOperation = "Add"
	opDelete patchOperation = "Delete"
	opUpdate patchOperation = "Update"
)

// patchFile 解析后的补丁文件操作
type patchFile struct {
	Operation patchOperation
	Path      string
	Content   []string
}

// patchResult 补丁执行结果统计
type patchResult struct {
	Success int
	Failed  int
	Skipped int
	Details []string
}

// Execute 执行 apply_patch 工具
func (a *ApplyPatchTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params applyPatchParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if params.Patch == "" {
		return &tool.Result{
			Content: "参数 'patch' 不能为空",
			IsError: true,
		}, nil
	}

	// 解析补丁
	files, err := parsePatch(params.Patch)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("补丁解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if len(files) == 0 {
		return &tool.Result{
			Content: "补丁中未包含任何文件操作",
			IsError: true,
		}, nil
	}

	// 执行补丁
	result := applyFiles(a.WorkDir, files)

	// 构建报告
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("补丁应用完成：成功 %d，失败 %d，跳过 %d\n", result.Success, result.Failed, result.Skipped))
	if len(result.Details) > 0 {
		buf.WriteString("\n详情:\n")
		for _, d := range result.Details {
			buf.WriteString("  " + d + "\n")
		}
	}

	isError := result.Failed > 0

	return &tool.Result{
		Content: strings.TrimSpace(buf.String()),
		IsError: isError,
	}, nil
}

// parsePatch 解析结构化补丁文本
func parsePatch(patch string) ([]patchFile, error) {
	lines := strings.Split(patch, "\n")

	// 找到开始和结束标记
	startIdx := -1
	endIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "*** Begin Patch" {
			startIdx = i
		}
		if trimmed == "*** End Patch" {
			endIdx = i
			break
		}
	}

	if startIdx == -1 {
		return nil, fmt.Errorf("缺少开始标记 '*** Begin Patch'")
	}
	if endIdx == -1 {
		return nil, fmt.Errorf("缺少结束标记 '*** End Patch'")
	}

	// 解析标记之间的内容
	var files []patchFile
	var current *patchFile

	for i := startIdx + 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 跳过空行
		if trimmed == "" {
			continue
		}

		// 检查是否为操作标记
		if strings.HasPrefix(trimmed, "*** ") {
			// 保存上一个文件操作
			if current != nil {
				files = append(files, *current)
			}

			// 解析新操作
			rest := strings.TrimPrefix(trimmed, "*** ")

			switch {
			case strings.HasPrefix(rest, "Add File: "):
				path := strings.TrimPrefix(rest, "Add File: ")
				if path == "" {
					return nil, fmt.Errorf("第 %d 行：Add File 操作缺少文件路径", i+1)
				}
				current = &patchFile{Operation: opAdd, Path: strings.TrimSpace(path)}

			case strings.HasPrefix(rest, "Delete File: "):
				path := strings.TrimPrefix(rest, "Delete File: ")
				if path == "" {
					return nil, fmt.Errorf("第 %d 行：Delete File 操作缺少文件路径", i+1)
				}
				current = &patchFile{Operation: opDelete, Path: strings.TrimSpace(path)}

			case strings.HasPrefix(rest, "Update File: "):
				path := strings.TrimPrefix(rest, "Update File: ")
				if path == "" {
					return nil, fmt.Errorf("第 %d 行：Update File 操作缺少文件路径", i+1)
				}
				current = &patchFile{Operation: opUpdate, Path: strings.TrimSpace(path)}

			default:
				return nil, fmt.Errorf("第 %d 行：未知操作 '%s'", i+1, rest)
			}

			continue
		}

		// 收集文件内容（以 + 开头的行）
		if current != nil && (current.Operation == opAdd || current.Operation == opUpdate) {
			if strings.HasPrefix(line, "+") {
				content := strings.TrimPrefix(line, "+")
				current.Content = append(current.Content, content)
			}
		}
	}

	// 保存最后一个文件操作
	if current != nil {
		files = append(files, *current)
	}

	return files, nil
}

// applyFiles 执行文件操作
func applyFiles(workDir string, files []patchFile) patchResult {
	var result patchResult

	for _, f := range files {
		fullPath := filepath.Join(workDir, f.Path)

		switch f.Operation {
		case opAdd:
			err := applyAddFile(fullPath, f.Content)
			if err != nil {
				result.Failed++
				result.Details = append(result.Details, fmt.Sprintf("❌ 添加文件失败 %s: %v", f.Path, err))
			} else {
				result.Success++
				result.Details = append(result.Details, fmt.Sprintf("✅ 添加文件 %s", f.Path))
			}

		case opDelete:
			err := applyDeleteFile(fullPath)
			if err != nil {
				result.Failed++
				result.Details = append(result.Details, fmt.Sprintf("❌ 删除文件失败 %s: %v", f.Path, err))
			} else {
				result.Success++
				result.Details = append(result.Details, fmt.Sprintf("✅ 删除文件 %s", f.Path))
			}

		case opUpdate:
			err := applyUpdateFile(fullPath, f.Content)
			if err != nil {
				result.Failed++
				result.Details = append(result.Details, fmt.Sprintf("❌ 更新文件失败 %s: %v", f.Path, err))
			} else {
				result.Success++
				result.Details = append(result.Details, fmt.Sprintf("✅ 更新文件 %s", f.Path))
			}
		}
	}

	return result
}

// applyAddFile 添加新文件
func applyAddFile(path string, content []string) error {
	// 检查文件是否已存在
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("文件已存在")
	}

	// 创建父目录
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录: %w", err)
	}

	// 写入文件
	data := strings.Join(content, "\n")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return fmt.Errorf("写入文件: %w", err)
	}

	return nil
}

// applyDeleteFile 删除文件
func applyDeleteFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在")
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除文件: %w", err)
	}

	return nil
}

// applyUpdateFile 更新文件内容
func applyUpdateFile(path string, content []string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在")
	}

	data := strings.Join(content, "\n")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return fmt.Errorf("写入文件: %w", err)
	}

	return nil
}