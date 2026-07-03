package lsp

import (
	"context"
	"encoding/json"

	"github.com/wly2lcl/basework/pkg/tool"
)

// ---------------------------------------------------------------------------
// 工具内部参数类型
// ---------------------------------------------------------------------------

// positionArgs 是包含文件路径和光标位置的参数。
type positionArgs struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

// fileArgs 是仅包含文件路径的参数。
type fileArgs struct {
	File string `json:"file"`
}

// queryArgs 是包含搜索关键词的参数。
type queryArgs struct {
	Query string `json:"query"`
}

// diagnosticsResult 是 lsp_diagnostics 工具返回的结构，包含诊断列表和版本号。
type diagnosticsResult struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Version     uint64       `json:"version"`
}

// ---------------------------------------------------------------------------
// 工具：lsp_definition
// ---------------------------------------------------------------------------

type definitionTool struct {
	manager *Manager
}

func (t *definitionTool) Name() string {
	return "lsp_definition"
}

func (t *definitionTool) Description() string {
	return "跳转到定义位置。返回目标符号的定义位置列表。"
}

func (t *definitionTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file":       {"type": "string", "description": "文件路径"},
			"line":       {"type": "integer", "description": "行号（0 起始）"},
			"character":  {"type": "integer", "description": "列号（0 起始）"}
		},
		"required": ["file", "line", "character"]
	}`)
}

func (t *definitionTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: "[]"}, nil
	}
	var params positionArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	locations, err := t.manager.Definition(ctx, params.File, Position{Line: params.Line, Character: params.Character})
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	if locations == nil {
		locations = []Location{}
	}
	data, _ := json.Marshal(locations)
	return &tool.Result{Content: string(data)}, nil
}

// ---------------------------------------------------------------------------
// 工具：lsp_references
// ---------------------------------------------------------------------------

type referencesTool struct {
	manager *Manager
}

func (t *referencesTool) Name() string {
	return "lsp_references"
}

func (t *referencesTool) Description() string {
	return "查找所有引用位置。返回目标符号的所有引用位置列表。"
}

func (t *referencesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file":       {"type": "string", "description": "文件路径"},
			"line":       {"type": "integer", "description": "行号（0 起始）"},
			"character":  {"type": "integer", "description": "列号（0 起始）"}
		},
		"required": ["file", "line", "character"]
	}`)
}

func (t *referencesTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: "[]"}, nil
	}
	var params positionArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	locations, err := t.manager.References(ctx, params.File, Position{Line: params.Line, Character: params.Character})
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	if locations == nil {
		locations = []Location{}
	}
	data, _ := json.Marshal(locations)
	return &tool.Result{Content: string(data)}, nil
}

// ---------------------------------------------------------------------------
// 工具：lsp_hover
// ---------------------------------------------------------------------------

type hoverTool struct {
	manager *Manager
}

func (t *hoverTool) Name() string {
	return "lsp_hover"
}

func (t *hoverTool) Description() string {
	return "获取悬停信息。返回目标位置的类型信息、文档注释等。"
}

func (t *hoverTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file":       {"type": "string", "description": "文件路径"},
			"line":       {"type": "integer", "description": "行号（0 起始）"},
			"character":  {"type": "integer", "description": "列号（0 起始）"}
		},
		"required": ["file", "line", "character"]
	}`)
}

func (t *hoverTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: ""}, nil
	}
	var params positionArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	text, err := t.manager.Hover(ctx, params.File, Position{Line: params.Line, Character: params.Character})
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	return &tool.Result{Content: text}, nil
}

// ---------------------------------------------------------------------------
// 工具：lsp_diagnostics
// ---------------------------------------------------------------------------

type diagnosticsTool struct {
	manager *Manager
}

func (t *diagnosticsTool) Name() string {
	return "lsp_diagnostics"
}

func (t *diagnosticsTool) Description() string {
	return "获取文件诊断信息。返回指定文件的错误、警告等诊断列表。"
}

func (t *diagnosticsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file": {"type": "string", "description": "文件路径"}
		},
		"required": ["file"]
	}`)
}

func (t *diagnosticsTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: `{"diagnostics":[],"version":0}`}, nil
	}
	var params fileArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	diags, version := t.manager.Diagnostics(params.File)
	result := diagnosticsResult{
		Diagnostics: diags,
		Version:     version,
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []Diagnostic{}
	}
	data, _ := json.Marshal(result)
	return &tool.Result{Content: string(data)}, nil
}

// ---------------------------------------------------------------------------
// 工具：lsp_document_symbols
// ---------------------------------------------------------------------------

type documentSymbolsTool struct {
	manager *Manager
}

func (t *documentSymbolsTool) Name() string {
	return "lsp_document_symbols"
}

func (t *documentSymbolsTool) Description() string {
	return "获取文档符号列表。返回文件中的函数、类型、变量等符号信息。"
}

func (t *documentSymbolsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file": {"type": "string", "description": "文件路径"}
		},
		"required": ["file"]
	}`)
}

func (t *documentSymbolsTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: "[]"}, nil
	}
	var params fileArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	symbols, err := t.manager.DocumentSymbols(ctx, params.File)
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	if symbols == nil {
		symbols = []SymbolInfo{}
	}
	data, _ := json.Marshal(symbols)
	return &tool.Result{Content: string(data)}, nil
}

// ---------------------------------------------------------------------------
// 工具：lsp_workspace_symbols
// ---------------------------------------------------------------------------

type workspaceSymbolsTool struct {
	manager *Manager
}

func (t *workspaceSymbolsTool) Name() string {
	return "lsp_workspace_symbols"
}

func (t *workspaceSymbolsTool) Description() string {
	return "搜索工作区符号。根据查询字符串匹配工作区中的函数、类型、变量等符号。"
}

func (t *workspaceSymbolsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "搜索关键词"}
		},
		"required": ["query"]
	}`)
}

func (t *workspaceSymbolsTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.manager == nil {
		return &tool.Result{Content: "[]"}, nil
	}
	var params queryArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	symbols, err := t.manager.WorkspaceSymbols(ctx, params.Query)
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}
	if symbols == nil {
		symbols = []SymbolInfo{}
	}
	data, _ := json.Marshal(symbols)
	return &tool.Result{Content: string(data)}, nil
}

// ---------------------------------------------------------------------------
// 工厂函数
// ---------------------------------------------------------------------------

// Tools 返回所有 LSP 工具。如果 manager 为 nil，返回仍然可用的降级工具。
func Tools(manager *Manager) []tool.Tool {
	return []tool.Tool{
		&definitionTool{manager: manager},
		&referencesTool{manager: manager},
		&hoverTool{manager: manager},
		&diagnosticsTool{manager: manager},
		&documentSymbolsTool{manager: manager},
		&workspaceSymbolsTool{manager: manager},
	}
}