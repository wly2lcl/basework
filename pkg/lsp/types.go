// Package lsp 提供 LSP（Language Server Protocol）基础类型和 JSON-RPC 传输层。
package lsp

// Position 表示文本文档中的位置（0 起始的行号和字符偏移）。
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range 表示文本文档中的一个区间。
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location 表示工作区中的一个位置（文件 URI + 区间）。
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Severity 表示诊断消息的严重等级。
type Severity int

const (
	SeverityError       Severity = 1
	SeverityWarning     Severity = 2
	SeverityInformation Severity = 3
	SeverityHint        Severity = 4
)

// Diagnostic 表示 LSP 服务器发出的诊断消息。
type Diagnostic struct {
	Range    Range    `json:"range"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
}

// SymbolKind 对应 LSP SymbolKind 枚举值。
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)

// SymbolInfo 表示工作区中的一个符号。
type SymbolInfo struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Location Location   `json:"location"`
}