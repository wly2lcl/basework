package llm

import "strings"

// Support 表示一个能力结论的三态。
//
// 历史实现用 map[Capability]bool 表达能力，于是「查不到」和「明确为 false」得到
// 同一个值 false。调用方无法区分“这个模型确实不支持”与“没人声明过”。把未声明
// 当结论使用，就是“未知伪装为支持”的另一种形式（在需要能力时被误判为可用，或在
// 需要报错时被误判为已知限制）。这里把两者分开。
type Support uint8

const (
	// SupportUnknown 表示尚无可靠结论：既没有声明，也没有实测。
	// 调用方**不得**把 Unknown 当作 Supported 使用。
	SupportUnknown Support = iota
	// SupportUnsupported 表示已明确不支持（有声明或实测依据）。
	SupportUnsupported
	// SupportSupported 表示已确认支持（有声明或实测依据）。
	SupportSupported
)

// String 返回稳定的英文标识，用于日志与 CLI 输出。
func (s Support) String() string {
	switch s {
	case SupportSupported:
		return "supported"
	case SupportUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// Known 表示该能力是否有明确结论（支持或不支持）。
func (s Support) Known() bool { return s != SupportUnknown }

// ShortLabel 返回 CLI 表格用的短标签。
func (s Support) ShortLabel() string {
	switch s {
	case SupportSupported:
		return "yes"
	case SupportUnsupported:
		return "no"
	default:
		return "unknown"
	}
}

// CapabilitySource 表示能力结论的来源，用于区分“静态声明”与“实测”。
type CapabilitySource string

const (
	// SourceDeclared 表示来自 provider/模型实现的静态声明。
	SourceDeclared CapabilitySource = "declared"
	// SourceConfigured 表示来自用户配置的显式声明。
	SourceConfigured CapabilitySource = "configured"
	// SourceProbed 表示来自真实请求的实测结论。
	SourceProbed CapabilitySource = "probed"
)

// CapabilityDetail 描述单个能力的结论、来源与可选说明。
type CapabilityDetail struct {
	Support Support
	Source  CapabilitySource
	// Note 用于补充结论依据，例如家族匹配规则或实测方式。可为空。
	Note string
}

// CapabilityReporter 是可选接口。实现了它的 Model 可以报告带来源的三态能力。
//
// 未实现该接口的 Model 会退回 Supports()，此时 false 只能解释为“未声明”，
// 即 SupportUnknown。见 DescribeCapability。
type CapabilityReporter interface {
	Capability(cap Capability) CapabilityDetail
}

// DescribeCapability 统一读取一个模型的能力结论。
//
// 对未实现 CapabilityReporter 的模型：Supports() 为 true 记为已声明支持；
// 为 false 记为 Unknown 而非 Unsupported —— bool 的 false 不构成“不支持”的证据。
func DescribeCapability(m Model, cap Capability) CapabilityDetail {
	if m == nil {
		return CapabilityDetail{Support: SupportUnknown, Source: SourceDeclared, Note: "模型为空"}
	}
	if r, ok := m.(CapabilityReporter); ok {
		return r.Capability(cap)
	}
	if m.Supports(cap) {
		return CapabilityDetail{Support: SupportSupported, Source: SourceDeclared}
	}
	return CapabilityDetail{
		Support: SupportUnknown,
		Source:  SourceDeclared,
		Note:    "该模型未声明此能力；Supports() 返回 false 不代表已验证不支持",
	}
}

// RequiredCapability 描述一次运行对某项能力的硬性要求。
type RequiredCapability struct {
	Cap    Capability
	Reason string
}

// MissingCapabilityError 表示运行所需能力被明确判定为不支持或未知。
type MissingCapabilityError struct {
	ModelID    string
	Cap        Capability
	Detail     CapabilityDetail
	Reason     string
	Mitigation []string
}

func (e *MissingCapabilityError) Error() string {
	var b strings.Builder
	b.WriteString("模型 ")
	b.WriteString(e.ModelID)
	b.WriteString(" 缺少所需能力 ")
	b.WriteString(string(e.Cap))
	b.WriteString("（结论: ")
	b.WriteString(e.Detail.Support.String())
	b.WriteString(", 来源: ")
	b.WriteString(string(e.Detail.Source))
	b.WriteString("）")
	if e.Reason != "" {
		b.WriteString("：")
		b.WriteString(e.Reason)
	}
	if e.Detail.Note != "" {
		b.WriteString("；")
		b.WriteString(e.Detail.Note)
	}
	if len(e.Mitigation) > 0 {
		b.WriteString("。可选做法：")
		b.WriteString(strings.Join(e.Mitigation, "；"))
	}
	return b.String()
}

// CheckRequiredCapabilities 校验一组硬性能力要求。
//
// 与历史行为的关键差别：Unknown 不再被当作“可以继续”。如果某项能力没有明确
// 结论，这里会返回错误，并由调用方给出用户可选替代，而不是静默降级
// （静默禁用工具或偷偷换模型都是被禁止的通过方式）。
func CheckRequiredCapabilities(m Model, reqs []RequiredCapability) error {
	for _, req := range reqs {
		detail := DescribeCapability(m, req.Cap)
		if detail.Support == SupportSupported {
			continue
		}
		modelID := ""
		if m != nil {
			modelID = m.ID()
		}
		return &MissingCapabilityError{
			ModelID: modelID,
			Cap:     req.Cap,
			Detail:  detail,
			Reason:  req.Reason,
		}
	}
	return nil
}
