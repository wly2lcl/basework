package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeModel 只实现 llm.Model，不实现 CapabilityReporter，用于验证回退语义。
type fakeModel struct {
	id       string
	supports map[Capability]bool
}

func (f *fakeModel) ID() string { return f.id }
func (f *fakeModel) Generate(_ context.Context, _ *Request) (*Response, error) {
	return &Response{}, nil
}
func (f *fakeModel) Stream(_ context.Context, _ *Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}
func (f *fakeModel) Supports(cap Capability) bool { return f.supports[cap] }

// TestDescribeCapability_FalseMeansUnknownNotUnsupported 是本任务的核心断言：
// bool 的 false 不能升级成“已确认不支持”，只能表达“未声明”。
func TestDescribeCapability_FalseMeansUnknown(t *testing.T) {
	m := &fakeModel{id: "m", supports: map[Capability]bool{CapTools: true}}

	if got := DescribeCapability(m, CapTools); got.Support != SupportSupported {
		t.Errorf("CapTools: 期望 Supported，得到 %s", got.Support)
	}
	if got := DescribeCapability(m, CapTools); got.Source != SourceDeclared {
		t.Errorf("CapTools: 期望来源 declared，得到 %s", got.Source)
	}

	vision := DescribeCapability(m, CapVision)
	if vision.Support != SupportUnknown {
		t.Errorf("CapVision: false 必须解释为 Unknown，得到 %s", vision.Support)
	}
	if vision.Support == SupportSupported {
		t.Error("未知能力被当成了支持")
	}
	if vision.Note == "" {
		t.Error("期望 unknown 附带说明")
	}
}

// TestDescribeCapability_NilModel 确认空模型不会 panic 也不会被当作支持。
func TestDescribeCapability_NilModel(t *testing.T) {
	if got := DescribeCapability(nil, CapTools); got.Support != SupportUnknown {
		t.Errorf("nil 模型: 期望 Unknown，得到 %s", got.Support)
	}
}

// TestCheckRequiredCapabilities_UnknownBlocks 确认未知能力不会被放行。
func TestCheckRequiredCapabilities_UnknownBlocks(t *testing.T) {
	m := &fakeModel{id: "m", supports: map[Capability]bool{}}
	err := CheckRequiredCapabilities(m, []RequiredCapability{{Cap: CapTools, Reason: "循环需要工具"}})
	if err == nil {
		t.Fatal("未知能力必须阻断，不能静默继续")
	}
	var missing *MissingCapabilityError
	if !errors.As(err, &missing) {
		t.Fatalf("期望 MissingCapabilityError，得到 %T", err)
	}
	if missing.Cap != CapTools || missing.Detail.Support != SupportUnknown {
		t.Errorf("错误内容不正确: cap=%s support=%s", missing.Cap, missing.Detail.Support)
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("错误信息应包含结论来源，得到: %v", err)
	}
}

// TestCheckRequiredCapabilities_UnsupportedBlocks 确认明确不支持同样阻断。
func TestCheckRequiredCapabilities_UnsupportedBlocks(t *testing.T) {
	m := &reporterModel{id: "m", details: map[Capability]CapabilityDetail{
		CapTools: {Support: SupportUnsupported, Source: SourceDeclared, Note: "该模型无工具调用"},
	}}
	err := CheckRequiredCapabilities(m, []RequiredCapability{{Cap: CapTools, Reason: "循环需要工具"}})
	if err == nil {
		t.Fatal("明确不支持必须阻断")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("错误信息应包含 unsupported，得到: %v", err)
	}
}

// TestCheckRequiredCapabilities_SupportedPasses 确认有依据的支持可以放行。
func TestCheckRequiredCapabilities_SupportedPasses(t *testing.T) {
	m := &reporterModel{id: "m", details: map[Capability]CapabilityDetail{
		CapTools:     {Support: SupportSupported, Source: SourceDeclared},
		CapStreaming: {Support: SupportSupported, Source: SourceDeclared},
	}}
	err := CheckRequiredCapabilities(m, []RequiredCapability{
		{Cap: CapTools, Reason: "工具"},
		{Cap: CapStreaming, Reason: "流式"},
	})
	if err != nil {
		t.Fatalf("应通过，得到 %v", err)
	}
}

// TestMissingCapabilityError_MentionsSourceAndMitigation 确认错误文本给出依据与可选做法。
func TestMissingCapabilityError_MentionsSourceAndMitigation(t *testing.T) {
	err := &MissingCapabilityError{
		ModelID:    "my-model",
		Cap:        CapVision,
		Detail:     CapabilityDetail{Support: SupportUnsupported, Source: SourceDeclared, Note: "纯文本家族"},
		Reason:     "请求包含图片",
		Mitigation: []string{"改用支持视觉的模型"},
	}
	msg := err.Error()
	for _, want := range []string{"my-model", "vision", "unsupported", "declared", "纯文本家族", "改用支持视觉的模型"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q: %s", want, msg)
		}
	}
}

// TestSupport_ShortLabel 确认 CLI 标签稳定且三态可区分。
func TestSupport_ShortLabel(t *testing.T) {
	cases := map[Support]string{
		SupportSupported:   "yes",
		SupportUnsupported: "no",
		SupportUnknown:     "unknown",
	}
	for s, want := range cases {
		if got := s.ShortLabel(); got != want {
			t.Errorf("ShortLabel(%s) = %q, 期望 %q", s, got, want)
		}
	}
	if SupportSupported.Known() == false || SupportUnknown.Known() == true {
		t.Error("Known() 语义不正确")
	}
}

// reporterModel 实现 CapabilityReporter，用于测试自定义结论。
type reporterModel struct {
	id      string
	details map[Capability]CapabilityDetail
}

func (r *reporterModel) ID() string { return r.id }
func (r *reporterModel) Generate(_ context.Context, _ *Request) (*Response, error) {
	return &Response{}, nil
}
func (r *reporterModel) Stream(_ context.Context, _ *Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}
func (r *reporterModel) Supports(cap Capability) bool {
	return r.details[cap].Support == SupportSupported
}
func (r *reporterModel) Capability(cap Capability) CapabilityDetail {
	if d, ok := r.details[cap]; ok {
		return d
	}
	return CapabilityDetail{Support: SupportUnknown, Source: SourceDeclared}
}
