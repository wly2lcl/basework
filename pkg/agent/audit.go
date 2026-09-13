package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// AuditMode 决定「请求审计记录写不进去」时这一轮怎么办。
//
// 背景：`request.built` 事件是「模型看到的请求 == 日志记录的请求」这个不变量的
// 唯一凭证。历史实现把它当尽力而为的事——写入失败只打一行日志就继续发请求，
// 于是日志会缺条目，而缺条目和「这次请求没有被记录」在事后无法区分。
// 严格模式把这种缺失变成一次明确的失败，代价是对话本身也会失败。
type AuditMode string

const (
	// AuditModeCompatible 是默认值：审计写入失败只记日志并继续发送请求。
	//
	// 保留这个默认值是为了不破坏既有嵌入调用——它们没有为审计失败准备错误路径，
	// 直接改成严格模式会让它们的对话无故中断。
	AuditModeCompatible AuditMode = "compatible"

	// AuditModeStrict 是严格模式：审计写入失败时，在本轮**发出任何 Provider 调用
	// 之前**中断。它的代价是可用性，换来的是「日志里没有 request.built 就一定
	// 没有发出过请求」这条可以依赖的性质。
	AuditModeStrict AuditMode = "strict"
)

// Valid 报告该模式是否是已定义取值。
func (m AuditMode) Valid() bool {
	return m == AuditModeCompatible || m == AuditModeStrict
}

// Normalize 把空值归一化为默认模式，未知取值原样返回以便调用方报错。
func (m AuditMode) Normalize() AuditMode {
	if m == "" {
		return AuditModeCompatible
	}
	return m
}

// AuditPolicyProvider 是 TurnD 的**可选**扩展：提供审计策略。
//
// 用可选接口而不是给 TurnD 加方法，是为了保持既有嵌入实现可用：没有实现它的
// TurnD 按 AuditModeCompatible 工作，与历史行为一致。
type AuditPolicyProvider interface {
	AuditMode() AuditMode
}

// 审计失败的阶段。放在错误里，便于定位是「写不进去」还是「重建对不上」。
const (
	// AuditStageEncode 表示事件数据编码失败。
	AuditStageEncode = "encode"
	// AuditStagePersist 表示事件落盘失败。
	AuditStagePersist = "persist"
	// AuditStageRebuild 表示按 Seq 重建请求并核对指纹失败。
	AuditStageRebuild = "rebuild"
)

// AuditError 是一次审计失败的完整描述。
//
// Source 记录失败来源（事件类型或片段名），Stage 记录发生阶段。严格模式下它会
// 直接阻断本轮 Provider 调用，错误里必须带上足够信息让人知道该去修什么。
type AuditError struct {
	SessionID string
	Stage     string
	Source    string
	Err       error
}

func (e *AuditError) Error() string {
	return fmt.Sprintf("agent: 请求审计失败（阶段=%s 来源=%s session=%s）: %v",
		e.Stage, e.Source, e.SessionID, e.Err)
}

func (e *AuditError) Unwrap() error { return e.Err }

// IsAuditError 报告错误链中是否包含审计失败。
func IsAuditError(err error) bool {
	var ae *AuditError
	return errors.As(err, &ae)
}

// AuditBoundaries 说明请求指纹覆盖到哪里、到哪里为止。
//
// 这不是给文档看的注释，而是把「边界」写成代码里可查的常量：指纹的语义没变，
// 但使用它的人必须知道哪一段差异**本来就不该**被重建出来，否则会把正常的
// hook 改写当成漂移去修。
type AuditBoundary struct {
	// Name 是边界名称。
	Name string
	// Covered 表示是否被 request.built 的指纹覆盖。
	Covered bool
	// Reproducible 表示能否仅凭事件日志重建出当时的内容。
	Reproducible bool
	// Note 说明原因。
	Note string
}

// AuditBoundaries 返回请求审计的完整边界清单。
func AuditBoundaries() []AuditBoundary {
	return []AuditBoundary{
		{
			Name:         "事件日志投影出的 messages",
			Covered:      true,
			Reproducible: true,
			Note:         "system prompt、steering、历史消息都来自事件日志，可按 Seq 精确重建",
		},
		{
			Name:         "hook 改写后的 messages",
			Covered:      true,
			Reproducible: false,
			Note: "指纹在 RunBeforeLLM 之后计算，因此改写结果被记入哈希；" +
				"但 hook 是调用方代码、不在日志里，所以只能证明“请求与日志不同”，无法证明“如何不同”",
		},
		{
			Name:         "工具定义快照",
			Covered:      true,
			Reproducible: false,
			Note: "工具定义来自注册表而非事件日志，日志只留 ToolCount 与它们对哈希的贡献；" +
				"核对时必须传入当时实际使用的同一份工具定义",
		},
		{
			Name:         "Provider 传输层变换",
			Covered:      false,
			Reproducible: false,
			Note:         "指纹算在交给 llm.Request 的那一刻；此后各家适配器做的协议改写（缓存前缀、工具 schema 变形等）不在覆盖范围内",
		},
		{
			Name:         "助手回复与工具结果",
			Covered:      false,
			Reproducible: false,
			Note:         "发生在请求之后。核对时必须把日志截回请求那一刻，不能用最终日志比对",
		},
	}
}

// RebuildRequestAt 把事件日志截回指定 Seq（含）并重建请求消息。
//
// 关键点是「截回那一刻」。请求发出之后还会有 assistant 回复、工具结果、压缩等
// 事件继续落盘，用最终日志重建会得到另一份消息列表——那不是在核对当时发生了什么，
// 而是在核对现在是什么样。
func RebuildRequestAt(events []session.Event, seq int64) []llm.ChatMessage {
	truncated := make([]session.Event, 0, len(events))
	for _, e := range events {
		if e.Seq <= seq {
			truncated = append(truncated, e)
		}
	}
	return BuildRequestMessages(truncated)
}

// VerifyRequestBuilt 核对某条 request.built 事件记录的指纹与按它自己的 Seq
// 重建出来的请求是否一致。
//
// tools 必须传入当时实际使用的工具定义——工具定义不在日志里（见 AuditBoundaries），
// 不传就只比对消息条数，无法发现工具定义漂移。
//
// 装了改写请求的 hook 时本函数必然报错，这是设计如此，不是 bug：hook 不在日志管辖内。
func VerifyRequestBuilt(events []session.Event, builtSeq int64, tools []llm.ToolDefinition) error {
	built, err := findRequestBuilt(events, builtSeq)
	if err != nil {
		return err
	}

	messages := RebuildRequestAt(events, builtSeq)
	if len(messages) != built.MsgCount {
		return &AuditError{
			Stage:  AuditStageRebuild,
			Source: string(session.EventRequestBuilt),
			Err: fmt.Errorf("消息条数不一致：日志记录 %d 条，按 Seq=%d 重建得到 %d 条",
				built.MsgCount, builtSeq, len(messages)),
		}
	}
	if tools != nil && len(tools) != built.ToolCount {
		return &AuditError{
			Stage:  AuditStageRebuild,
			Source: string(session.EventRequestBuilt),
			Err: fmt.Errorf("工具定义条数不一致：日志记录 %d 个，实际提供 %d 个",
				built.ToolCount, len(tools)),
		}
	}

	got := RequestFingerprint(messages, tools)
	if got != built.Hash {
		return &AuditError{
			Stage:  AuditStageRebuild,
			Source: string(session.EventRequestBuilt),
			Err: fmt.Errorf("请求指纹不一致：日志记录 %s，按 Seq=%d 重建得到 %s",
				shortHash(built.Hash), builtSeq, shortHash(got)),
		}
	}
	return nil
}

// findRequestBuilt 取出指定 Seq 的 request.built 事件并解码。
func findRequestBuilt(events []session.Event, seq int64) (*session.RequestBuiltData, error) {
	for _, e := range events {
		if e.Seq != seq || e.Type != session.EventRequestBuilt {
			continue
		}
		var d session.RequestBuiltData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return nil, &AuditError{
				Stage:  AuditStageRebuild,
				Source: string(session.EventRequestBuilt),
				Err:    fmt.Errorf("解码 Seq=%d 的 request.built 失败: %w", seq, err),
			}
		}
		return &d, nil
	}
	return nil, &AuditError{
		Stage:  AuditStageRebuild,
		Source: string(session.EventRequestBuilt),
		Err:    fmt.Errorf("日志中没有 Seq=%d 的 request.built 事件", seq),
	}
}

// RequestBuiltSeqs 按写入顺序返回全部 request.built 事件的 Seq。
func RequestBuiltSeqs(events []session.Event) []int64 {
	var seqs []int64
	for _, e := range events {
		if e.Type == session.EventRequestBuilt {
			seqs = append(seqs, e.Seq)
		}
	}
	return seqs
}

// shortHash 截短哈希用于错误信息，避免刷屏；空值单独显示。
func shortHash(h string) string {
	if h == "" {
		return "(空)"
	}
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "…"
}
