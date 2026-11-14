package provider

import (
	"sort"
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 本文件集中回答两个问题：某个 provider/model 声称拥有哪些能力，以及这种声称的
// 依据是什么。原则是**只声明能站得住的结论**，其余一律返回 Unknown，由调用方决定
// 是否继续，而不是替调用方假设“大概支持”。

// protocolCapability 表示能力是协议适配层保证的（tools/streaming），
// 还是取决于具体模型（vision/json_mode）。
type protocolCapability struct {
	cap  llm.Capability
	note string
}

// protocolCapabilities 是走 OpenAI/Anthropic/Gemini 适配层时由代码保证的能力。
// 注意这里声明的是“请求构造与流式解析已实现”，模型侧是否响应仍是模型的责任。
var protocolCapabilities = []protocolCapability{
	{llm.CapTools, "协议适配层已实现工具调用请求与解析；模型是否响应取决于该模型"},
	{llm.CapStreaming, "协议适配层已实现 SSE 流式解析"},
}

// visionRule 是一条视觉能力家族规则。
//
// 规则必须按“更具体优先”的顺序排列并由上到下取第一个命中项，例如 o1-mini 必须在
// o1 之前，grok-2-vision 必须在 grok-2 之前。两列分开写会让顺序失去意义。
type visionRule struct {
	marker  string
	support llm.Support
	note    string
}

var visionRules = []visionRule{
	// 更具体的例外先匹配
	{"grok-2-vision", llm.SupportSupported, "视觉变体"},
	{"o1-mini", llm.SupportUnsupported, "纯文本推理模型"},
	{"gpt-3.5", llm.SupportUnsupported, "纯文本"},
	{"deepseek", llm.SupportUnsupported, "纯文本"},
	{"mixtral", llm.SupportUnsupported, "纯文本"},
	{"llama-3.1", llm.SupportUnsupported, "纯文本"},
	{"llama-3.3", llm.SupportUnsupported, "纯文本"},
	{"grok-2", llm.SupportUnsupported, "纯文本"},
	{"grok-3", llm.SupportUnsupported, "纯文本"},
	{"mistral-small", llm.SupportUnsupported, "纯文本"},
	{"mistral-medium", llm.SupportUnsupported, "纯文本"},
	{"mistral-large", llm.SupportUnsupported, "纯文本"},

	// 更宽泛的视觉家族
	{"gpt-4o", llm.SupportSupported, "多模态"},
	{"gpt-4-turbo", llm.SupportSupported, "多模态"},
	{"gpt-4.1", llm.SupportSupported, "多模态"},
	{"gpt-5", llm.SupportSupported, "多模态"},
	{"o1", llm.SupportSupported, "多模态推理模型"},
	{"o3", llm.SupportSupported, "多模态推理模型"},
	{"claude", llm.SupportSupported, "多模态"},
	{"gemini", llm.SupportSupported, "多模态"},
	{"llama-4", llm.SupportSupported, "多模态"},
	{"pixtral", llm.SupportSupported, "多模态"},
	{"qwen-vl", llm.SupportSupported, "多模态"},
	{"qwen2-vl", llm.SupportSupported, "多模态"},
	{"-vl-", llm.SupportSupported, "多模态命名约定"},
	{"vision", llm.SupportSupported, "多模态命名约定"},
}

// capabilitySet 是一个带来源的静态能力表，实现 llm.CapabilityReporter。
type capabilitySet struct {
	providerType string
	modelID      string
	withVision   bool
}

// Capability 返回单个能力的三态结论。
func (c *capabilitySet) Capability(cap llm.Capability) llm.CapabilityDetail {
	for _, pc := range protocolCapabilities {
		if pc.cap == cap {
			return llm.CapabilityDetail{
				Support: llm.SupportSupported,
				Source:  llm.SourceDeclared,
				Note:    pc.note,
			}
		}
	}
	if cap == llm.CapVision {
		return c.vision()
	}
	return llm.CapabilityDetail{
		Support: llm.SupportUnknown,
		Source:  llm.SourceDeclared,
		Note:    "该 provider 未声明此能力",
	}
}

// vision 按模型家族推断视觉能力；无法判断时返回 Unknown 而不是 false。
func (c *capabilitySet) vision() llm.CapabilityDetail {
	if !c.withVision {
		return llm.CapabilityDetail{
			Support: llm.SupportUnknown,
			Source:  llm.SourceDeclared,
			Note:    "该模型未在能力表中声明视觉输入",
		}
	}
	id := strings.ToLower(c.modelID)
	for _, rule := range visionRules {
		if strings.Contains(id, rule.marker) {
			return llm.CapabilityDetail{
				Support: rule.support,
				Source:  llm.SourceDeclared,
				Note:    "按模型家族匹配 " + rule.marker + "（" + rule.note + "）",
			}
		}
	}
	return llm.CapabilityDetail{
		Support: llm.SupportUnknown,
		Source:  llm.SourceDeclared,
		Note:    "模型 ID 未匹配已知家族，视觉能力未知",
	}
}

// newCapabilitySet 为指定 provider/model 构造能力表。
//
// withVision=false 用于那些整体不走多模态入口的 provider（例如 bedrock 当前未实现
// 图片传输），此时视觉能力返回 Unknown 而不是假装支持。
func newCapabilitySet(providerType, modelID string) *capabilitySet {
	return &capabilitySet{
		providerType: providerType,
		modelID:      modelID,
		withVision:   true,
	}
}

// CapabilitiesFor 返回指定 provider/model 的能力结论，键为能力名。
// 供 CLI（model list / model info）与运行时入口共用，保证展示与实际一致。
func CapabilitiesFor(providerType, modelID string) map[llm.Capability]llm.CapabilityDetail {
	set := newCapabilitySet(providerType, modelID)
	out := make(map[llm.Capability]llm.CapabilityDetail, 4)
	for _, cap := range []llm.Capability{llm.CapTools, llm.CapStreaming, llm.CapVision, llm.CapJSON} {
		out[cap] = set.Capability(cap)
	}
	return out
}

// CapabilityOrder 返回展示能力时的稳定顺序，避免 map 迭代导致的输出抖动。
func CapabilityOrder() []llm.Capability {
	return []llm.Capability{llm.CapTools, llm.CapStreaming, llm.CapVision, llm.CapJSON}
}

// KnownProvider 判断 providerType 是否为内置已知类型。
func KnownProvider(providerType string) bool {
	_, ok := protocols[providerType]
	return ok
}

// RequiresAPIKey 表示该 provider 在没有 API key 时无法工作。
// 与 Create 的校验共用同一张表，避免 CLI 展示与运行时行为不一致。
func RequiresAPIKey(providerType string) bool {
	meta, ok := protocols[providerType]
	if !ok {
		// 未知类型走 openai-compat 路径，Create 会要求 baseURL 与 key。
		return true
	}
	return !meta.allowEmptyKey
}

// APIKeyEnvVar 返回该 provider 实际读取的环境变量名。
// 必须与 cmd/basework 的 lookupAPIKey 保持一致；未单独映射的类型回落到 OPENAI_API_KEY。
func APIKeyEnvVar(providerType string) string {
	switch providerType {
	case "agnes-responses":
		return "AGNES_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GOOGLE_API_KEY"
	case "opencode":
		return "OPENCODE_API_KEY"
	case "azure":
		return "AZURE_API_KEY"
	case "bedrock":
		return "AWS_ACCESS_KEY_ID"
	default:
		return "OPENAI_API_KEY"
	}
}

// SortedNames 返回排序后的已知 provider 名称，用于稳定输出。
func SortedNames() []string {
	names := make([]string, 0, len(protocols))
	for name := range protocols {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
