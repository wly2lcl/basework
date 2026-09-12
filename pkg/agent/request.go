package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// 请求来源的 Kind 取值。
const (
	// SourceSystemPrompt 表示会话级 system prompt。
	SourceSystemPrompt = "system_prompt"
	// SourceSteering 表示运行中注入的 steering 消息。
	SourceSteering = "steering"
	// SourceHistory 表示由事件投影出的对话历史。
	SourceHistory = "history"
)

// BuildRequestMessages 从事件日志重建「将要发给 provider 的消息列表」。
//
// 这是「模型可见即已记录」（model-visible means logged）不变量的落点：
// 请求必须是事件日志的**纯函数**。任何绕过事件日志进入请求的片段——例如直接读
// 配置注入 system prompt，或从内存队列 Drain() 之后即丢的 steering 消息——都会让
// 下面这个等式失效，而且不会有任何报错：
//
//	BuildRequestMessages(events) == 实际发给 provider 的 messages
//
// 唯一的例外是 hook 改写了请求：hook 是调用方代码，不在日志管辖范围内。
//
// 组装顺序与历史行为保持一致：
//
//	[system prompt] + [steering...] + [投影出的历史消息...]
//
// system prompt 与 steering 之所以不交给 ProjectMessages 产出，见 projection.go
// 中「请求级上下文事件」一段的说明（Seq 单调性与压缩会吃掉 system prompt）。
func BuildRequestMessages(events []session.Event) []llm.ChatMessage {
	history := session.ProjectMessages(events)

	var msgs []llm.ChatMessage
	if prompt, _ := latestSystemPrompt(events); prompt != "" {
		msgs = append(msgs, llm.ChatMessage{
			Role:    llm.RoleSystem,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: prompt}},
		})
	}
	steering, _ := steeringMessages(events)
	msgs = append(msgs, steeringToChatMessages(steering)...)

	return append(msgs, history...)
}

// latestSystemPrompt 返回最后一条 system.prompt_set 事件的内容与其 Seq。
// 没有该事件时返回空串——此时请求里就不应该有 system 消息。
func latestSystemPrompt(events []session.Event) (string, int64) {
	var content string
	var seq int64
	for _, e := range events {
		if e.Type != session.EventSystemPromptSet {
			continue
		}
		var d session.SystemPromptSetData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			continue
		}
		content, seq = d.Content, e.Seq
	}
	return content, seq
}

// steeringMessages 返回日志里全部 steering 消息（按写入顺序）与最后一个 Seq。
//
// 返回的是**全部**而非最后一轮：steering 一旦注入，模型在那一轮就看得到，它便
// 已经是对话上下文的一部分。只回填最后一轮会让历史出现断层——"模型当时看得到、
// 现在重建不出来"，正是本不变量要消除的情况。
func steeringMessages(events []session.Event) ([]string, int64) {
	var msgs []string
	var seq int64
	for _, e := range events {
		if e.Type != session.EventSteered {
			continue
		}
		var d session.SteeredData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			continue
		}
		msgs = append(msgs, d.Messages...)
		seq = e.Seq
	}
	return msgs, seq
}

// describeRequestSources 描述请求各片段的来源，写入 EventRequestBuilt 备查。
func describeRequestSources(events []session.Event) []session.RequestSource {
	var out []session.RequestSource

	if prompt, promptSeq := latestSystemPrompt(events); prompt != "" {
		out = append(out, session.RequestSource{
			Kind:  SourceSystemPrompt,
			Count: 1,
			Seq:   promptSeq,
			Hash:  contentHash(prompt),
		})
	}
	if steering, steeringSeq := steeringMessages(events); len(steering) > 0 {
		out = append(out, session.RequestSource{
			Kind:  SourceSteering,
			Count: len(steering),
			Seq:   steeringSeq,
			Hash:  contentHash(strings.Join(steering, "\n")),
		})
	}
	if history := session.ProjectMessages(events); len(history) > 0 {
		out = append(out, session.RequestSource{
			Kind:  SourceHistory,
			Count: len(history),
		})
	}

	return out
}

// RequestFingerprint 计算一次请求内容的稳定指纹（messages + tools）。
//
// 稳定性来自三点：
//   - 载荷是结构体（encoding/json 的字段顺序固定），不含 map，因此同一份内容在
//     任何机器、任何次运行上都得到同一个值；
//   - nil 切片先归一化为空切片——json.Marshal 把 nil 切片写成 `null`、空切片写成
//     `[]`，若不一视同仁，「没有工具」会因为切片是不是 nil 而得到两个指纹；
//   - 不做任何取整或截断。
func RequestFingerprint(messages []llm.ChatMessage, tools []llm.ToolDefinition) string {
	if messages == nil {
		messages = []llm.ChatMessage{}
	}
	if tools == nil {
		tools = []llm.ToolDefinition{}
	}

	payload := struct {
		Messages []llm.ChatMessage    `json:"messages"`
		Tools    []llm.ToolDefinition `json:"tools"`
	}{Messages: messages, Tools: tools}

	b, err := json.Marshal(payload)
	if err != nil {
		// 消息中不含无法序列化的类型（全是字符串与切片），这里实际不可达。
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// contentHash 计算字符串内容指纹，用于记录 system prompt / steering 的内容摘要。
func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// CheckRequestInvariant 校验「实际请求 == 事件日志重建的请求」。
//
// 用于测试与诊断。差异会带出首个不一致的消息位置与原因，便于定位漂移来源：
//
//   - system prompt 不一致 → 有人绕过事件日志改了 prompt；
//   - 中间多出 system 消息 → steering 没有落盘，或落盘后又被别的路径注入一次；
//   - 长度不一致 → 请求里有内容完全没进日志（例如 hook 之外的隐式注入）。
//
// 注意：安装改写了请求的 hook 后本校验必然失败，这是预期行为——hook 不在日志管辖内。
func CheckRequestInvariant(events []session.Event, actual []llm.ChatMessage) error {
	want := BuildRequestMessages(events)
	if len(want) != len(actual) {
		return fmt.Errorf("agent: 请求与事件日志不一致：日志重建 %d 条消息，实际发出 %d 条",
			len(want), len(actual))
	}
	for i := range want {
		if reason := diffMessage(want[i], actual[i]); reason != "" {
			return fmt.Errorf("agent: 第 %d 条消息与事件日志不一致：%s", i, reason)
		}
	}
	return nil
}

// diffMessage 返回两条消息的差异描述，一致时返回空串。
func diffMessage(a, b llm.ChatMessage) string {
	if reflect.DeepEqual(a, b) {
		return ""
	}
	if a.Role != b.Role {
		return "role 不一致"
	}
	if a.ToolCallID != b.ToolCallID {
		return "tool_call_id 不一致"
	}
	if a.Name != b.Name {
		return "name 不一致"
	}
	if len(a.Content) != len(b.Content) {
		return fmt.Sprintf("content 分段数量不一致（日志 %d，实际 %d）", len(a.Content), len(b.Content))
	}
	if (a.Content == nil) != (b.Content == nil) {
		return "content 切片的 nil/empty 状态不一致"
	}
	for i := range a.Content {
		want, got := a.Content[i], b.Content[i]
		if want.Type != got.Type {
			return fmt.Sprintf("content[%d].type 不一致", i)
		}
		if want.Text != got.Text {
			return fmt.Sprintf("content[%d].text 不一致", i)
		}
		if want.ImageURL != got.ImageURL {
			return fmt.Sprintf("content[%d].image_url 不一致", i)
		}
		if (want.CacheControl == nil) != (got.CacheControl == nil) {
			return fmt.Sprintf("content[%d].cache_control presence 不一致", i)
		}
		if want.CacheControl != nil && got.CacheControl != nil && want.CacheControl.Type != got.CacheControl.Type {
			return fmt.Sprintf("content[%d].cache_control.type 不一致", i)
		}
		if !reflect.DeepEqual(want.CacheControl, got.CacheControl) {
			return fmt.Sprintf("content[%d].cache_control 字段不一致", i)
		}
	}
	if len(a.ToolCalls) != len(b.ToolCalls) {
		return fmt.Sprintf("tool_calls 数量不一致（日志 %d，实际 %d）", len(a.ToolCalls), len(b.ToolCalls))
	}
	if (a.ToolCalls == nil) != (b.ToolCalls == nil) {
		return "tool_calls 切片的 nil/empty 状态不一致"
	}
	for i := range a.ToolCalls {
		want, got := a.ToolCalls[i], b.ToolCalls[i]
		if want.ID != got.ID {
			return fmt.Sprintf("tool_calls[%d].id 不一致", i)
		}
		if want.Name != got.Name {
			return fmt.Sprintf("tool_calls[%d].name 不一致", i)
		}
		if want.ArgsJSON != got.ArgsJSON {
			return fmt.Sprintf("tool_calls[%d].args_json 不一致", i)
		}
		if !reflect.DeepEqual(want, got) {
			return fmt.Sprintf("tool_calls[%d] 存在未分类字段差异", i)
		}
	}
	// DeepEqual above detects additions to ChatMessage and nested structs too; this
	// fallback keeps those future differences visible without printing payload data.
	return "消息存在未分类字段差异"
}

// messageText 拼接消息里所有文本片段。
func messageText(msg llm.ChatMessage) string {
	var b strings.Builder
	for _, part := range msg.Content {
		if part.Type == llm.ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
