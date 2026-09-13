// Package session 提供事件溯源会话层，用于记录和管理 LLM 对话事件。
package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// EventType 表示事件的类型。
type EventType string

const (
	// 用户输入事件
	EventPrompted EventType = "prompted"
	// 文本流式输出开始
	EventTextStarted EventType = "text.started"
	// 文本流式输出增量
	EventTextDelta EventType = "text.delta"
	// 文本流式输出结束
	EventTextEnded EventType = "text.ended"
	// 工具被调用
	EventToolCalled EventType = "tool.called"
	// 工具执行成功
	EventToolSuccess EventType = "tool.success"
	// 工具执行失败
	EventToolFailed EventType = "tool.failed"
	// 对话轮次开始
	EventTurnStarted EventType = "turn.started"
	// 对话轮次结束
	EventTurnEnded EventType = "turn.ended"
	// 对话轮次失败
	EventTurnFailed EventType = "turn.failed"
	// 事件压缩
	EventCompacted EventType = "compacted"
	// Agent 切换
	EventAgentSwitched EventType = "agent.switched"
	// 会话级 system prompt 被设定或更新
	EventSystemPromptSet EventType = "system.prompt_set"
	// 运行中注入的 steering 消息
	EventSteered EventType = "steered"
	// 一次真实发给 provider 的请求组装完成（只留指纹，不投影为消息）
	EventRequestBuilt EventType = "request.built"
	// 一次文件编辑流程步骤的可追溯事实（预览/提交/撤销/失败）
	EventFileEdited EventType = "file.edited"
)

// Event 表示一个溯源事件。
type Event struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Type      EventType       `json:"type"`
	Data      json.RawMessage `json:"data"`
	Seq       int64           `json:"seq"`
	CreatedAt time.Time       `json:"created_at"`
	// SchemaVersion 是该事件写入时所用的格式版本。
	//
	// 零值表示 v0——历史数据没有这个字段。用 omitempty 而非总是写出，
	// 是为了让 v0 数据的字节不因引入本字段而改变；读取端把 0 视为 v0，
	// 并按 migrate.go 的相邻迁移链升级到 SchemaVersion。
	SchemaVersion int `json:"v,omitempty"`
}

// PromptedData 表示用户提示数据。
type PromptedData struct {
	Content string `json:"content"`
}

// TextDeltaData 表示文本增量数据。
type TextDeltaData struct {
	Delta string `json:"delta"`
}

// ToolCalledData 表示工具调用数据。
type ToolCalledData struct {
	ToolCall llm.ToolCall `json:"tool_call"`
}

// ToolSuccessData 表示工具执行成功数据。
type ToolSuccessData struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ToolFailedData 表示工具执行失败数据。
type ToolFailedData struct {
	ToolCallID string `json:"tool_call_id"`
	Error      string `json:"error"`
}

// TurnStartedData 表示轮次开始数据。
type TurnStartedData struct {
	Step int `json:"step"`
}

// TurnEndedData 表示轮次结束数据（含 token 用量）。
type TurnEndedData struct {
	Usage llm.Usage `json:"usage"`
}

// CompactedData 表示事件压缩数据。
type CompactedData struct {
	Summary      string `json:"summary"`
	TruncatedSeq int64  `json:"truncated_seq"`
	KeepFrom     int    `json:"keep_from"`
	// Snapshot 是压缩后要作为完整历史重放的消息序列。
	// nil 表示旧格式事件，继续按 Summary/TruncatedSeq/KeepFrom 投影；
	// 非 nil 的空切片表示压缩结果有意清空历史。
	// 不使用 omitempty，以保留 []（显式清空）与 null/缺失（旧事件）的区别。
	Snapshot []llm.ChatMessage `json:"snapshot"`
}

// SystemPromptSetData 记录会话级 system prompt 的设定。
//
// system prompt 是请求的一部分，此前只存在于配置里、不入日志，于是「模型看到的
// 上下文」与「日志记录的历史」之间存在一段无法核对的差。落成事件后，请求可以
// 由日志完整重建（见 agent.BuildRequestMessages）。
type SystemPromptSetData struct {
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

// SteeredData 记录一次 steering 注入。
//
// steering 消息此前在 Drain() 之后即被丢弃：模型在那一轮看得到，日志里却没有，
// 事后无法解释「模型为什么那么回答」。落成事件后，它成为历史的一部分。
type SteeredData struct {
	Messages []string `json:"messages"`
}

// RequestSource 描述最终请求里一个片段的来源，用于事后核对请求是怎么拼出来的。
type RequestSource struct {
	// Kind 取值：system_prompt / steering / history（见 agent 包的同名常量）。
	Kind string `json:"kind"`
	// Count 是该来源贡献的消息条数。
	Count int `json:"count,omitempty"`
	// Seq 是来源事件的序号（history 这类聚合片段为 0）。
	Seq int64 `json:"seq,omitempty"`
	// Hash 是片段内容的指纹，用于比对内容是否被改写。
	Hash string `json:"hash,omitempty"`
}

// RequestBuiltData 是「本次真正发给 provider 的请求」的指纹。
//
// 只记指纹不记全文：全文可以从事件日志重建，重复存一份徒增体积。
// 事后核对时用同样的算法重算 Hash，不一致即说明请求与日志发生了漂移。
type RequestBuiltData struct {
	MsgCount  int             `json:"msg_count"`
	ToolCount int             `json:"tool_count"`
	Hash      string          `json:"hash"`
	Sources   []RequestSource `json:"sources,omitempty"`
}

// FileEditRecord 是 file.edited 事件中单个文件的结局。
type FileEditRecord struct {
	// Path 是工作区相对路径。
	Path string `json:"path"`
	// Op 是操作种类（如 replace）。预留扩展，不参与判定。
	Op string `json:"op,omitempty"`
	// State 是该文件的结局。预览阶段为 preview；提交阶段为
	// written / conflict / failed / not_attempted；撤销阶段为
	// reverted / skipped_modified / failed / not_written。
	State string `json:"state"`
	// Err 是失败或冲突的原因，成功时为空。
	Err string `json:"error,omitempty"`
}

// FileEditedData 记录一次编辑流程步骤的可追溯事实（EDIT-003）。
//
// 编辑是"批准后写盘"的多步流程，工具调用的成功/失败事件只回答"调用成没成"，
// 不回答"哪个文件变成了什么"。本事件把 plan/commit/undo 对应到可追溯记录：
// 每个阶段一条、逐文件结局一条不落，会话恢复后仍可用于定位修改。
type FileEditedData struct {
	// Phase 是流程阶段：preview（只读预览）、committed（提交全部成功）、
	// failed（提交未全部成功或整体失败）、rolled_back（已撤销）。
	Phase string `json:"phase"`
	// PlanID 是计划标识，稳定派生自操作内容（路径 + 基线哈希 + 新内容），
	// 同一份计划重新预览得到同一个 ID，可用于跨事件串联与防重复提交。
	PlanID string `json:"plan_id"`
	// Files 是逐文件结局清单，与内部提交结果的口径一致。
	Files []FileEditRecord `json:"files"`
	// Verify 是提交方声明的验证命令（如 "go test ./..."），原样记录、
	// 本包不执行。用于把"改了什么"和"怎么验证"关联起来。
	Verify string `json:"verify,omitempty"`
	// Summary 是人类可读的结果摘要。
	Summary string `json:"summary,omitempty"`
	// WorkspaceID 是产生本次编辑的工作区标识（CTX-001）。旧事件没有该
	// 字段：读取方按「未归属」处理，不默认归给当前工作区。可选字段，
	// 属兼容变更，不递增 SchemaVersion。
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// EncodeData 将事件数据编码为 json.RawMessage。
// 支持的 data 类型：*PromptedData, *TextDeltaData, *ToolCalledData, *ToolSuccessData,
// *ToolFailedData, *TurnStartedData, *TurnEndedData, *CompactedData,
// *SystemPromptSetData, *SteeredData, *RequestBuiltData, *FileEditedData。
func EncodeData(data interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("session: 编码事件数据失败: %w", err)
	}
	return json.RawMessage(b), nil
}

// DecodeData 根据 Event.Type 解码 Event.Data 为对应的结构化数据。
func DecodeData(event Event) (interface{}, error) {
	if len(event.Data) == 0 {
		return nil, nil
	}
	switch event.Type {
	case EventPrompted:
		var d PromptedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 PromptedData 失败: %w", err)
		}
		return &d, nil
	case EventTextDelta:
		var d TextDeltaData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TextDeltaData 失败: %w", err)
		}
		return &d, nil
	case EventToolCalled:
		var d ToolCalledData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolCalledData 失败: %w", err)
		}
		return &d, nil
	case EventToolSuccess:
		var d ToolSuccessData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolSuccessData 失败: %w", err)
		}
		return &d, nil
	case EventToolFailed:
		var d ToolFailedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolFailedData 失败: %w", err)
		}
		return &d, nil
	case EventTurnStarted:
		var d TurnStartedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TurnStartedData 失败: %w", err)
		}
		return &d, nil
	case EventTurnEnded:
		var d TurnEndedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TurnEndedData 失败: %w", err)
		}
		return &d, nil
	case EventCompacted:
		var d CompactedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 CompactedData 失败: %w", err)
		}
		return &d, nil
	case EventSystemPromptSet:
		var d SystemPromptSetData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 SystemPromptSetData 失败: %w", err)
		}
		return &d, nil
	case EventSteered:
		var d SteeredData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 SteeredData 失败: %w", err)
		}
		return &d, nil
	case EventRequestBuilt:
		var d RequestBuiltData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 RequestBuiltData 失败: %w", err)
		}
		return &d, nil
	case EventFileEdited:
		var d FileEditedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 FileEditedData 失败: %w", err)
		}
		return &d, nil
	default:
		return nil, fmt.Errorf("session: 不支持的事件类型: %s", event.Type)
	}
}

// EventFilter 用于过滤事件查询。
type EventFilter struct {
	SessionID string      `json:"session_id,omitempty"`
	Types     []EventType `json:"types,omitempty"`
	AfterSeq  int64       `json:"after_seq,omitempty"`
	Limit     int         `json:"limit,omitempty"`
}
