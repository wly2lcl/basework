package provider

import (
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// TestCapabilitiesFor_VisionIsNotBlanketDeclared 是本任务的核心断言：
// 纯文本模型不得被声明为支持视觉输入，未知模型必须返回 unknown。
func TestCapabilitiesFor_VisionIsNotBlanketDeclared(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     llm.Support
	}{
		{"deepseek 纯文本", "deepseek", "deepseek-chat", llm.SupportUnsupported},
		{"deepseek reasoner 纯文本", "deepseek", "deepseek-reasoner", llm.SupportUnsupported},
		{"groq llama3.3 纯文本", "groq", "llama-3.3-70b-versatile", llm.SupportUnsupported},
		{"together mixtral 纯文本", "together", "mistralai/Mixtral-8x7B-Instruct-v0.1", llm.SupportUnsupported},
		{"openai gpt-3.5 纯文本", "openai", "gpt-3.5-turbo", llm.SupportUnsupported},
		{"openai gpt-4o 有视觉", "openai", "gpt-4o", llm.SupportSupported},
		{"gemini 有视觉", "gemini", "gemini-2.5-flash", llm.SupportSupported},
		{"anthropic 有视觉", "anthropic", "claude-3-5-sonnet-20241022", llm.SupportSupported},
		{"openrouter 上的 claude", "openrouter", "anthropic/claude-3.5-sonnet", llm.SupportSupported},
		{"未知模型 ID", "openai-compat", "my-model", llm.SupportUnknown},
		{"ollama 拉取的模型未知", "ollama", "llama3", llm.SupportUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CapabilitiesFor(tc.provider, tc.model)[llm.CapVision]
			if got.Support != tc.want {
				t.Errorf("%s/%s vision: 期望 %s，得到 %s（%s）",
					tc.provider, tc.model, tc.want, got.Support, got.Note)
			}
			if got.Source != llm.SourceDeclared {
				t.Errorf("vision 结论应标记来源为 declared，得到 %s", got.Source)
			}
		})
	}
}

// TestCapabilitiesFor_UnknownNeverReportedAsSupported 是对上一条的强化：
// 只有 unknown 的模型绝不能被当成支持。
func TestCapabilitiesFor_UnknownNeverReportedAsSupported(t *testing.T) {
	got := CapabilitiesFor("openai-compat", "totally-unknown-model")[llm.CapVision]
	if got.Support == llm.SupportSupported {
		t.Fatalf("未知模型被声明为支持视觉: %+v", got)
	}
	if !got.Support.Known() && got.Support.ShortLabel() != "unknown" {
		t.Fatalf("unknown 的展示标签不正确: %s", got.Support.ShortLabel())
	}
}

// TestCapabilitiesFor_ProtocolCapabilitiesAreDeclared 确认 tools/streaming 由协议层声明。
func TestCapabilitiesFor_ProtocolCapabilitiesAreDeclared(t *testing.T) {
	caps := CapabilitiesFor("deepseek", "deepseek-chat")
	for _, cap := range []llm.Capability{llm.CapTools, llm.CapStreaming} {
		d := caps[cap]
		if d.Support != llm.SupportSupported {
			t.Errorf("%s: 期望 supported，得到 %s", cap, d.Support)
		}
		if d.Source != llm.SourceDeclared {
			t.Errorf("%s: 期望来源 declared，得到 %s", cap, d.Source)
		}
		if d.Note == "" {
			t.Errorf("%s: 协议层声明应说明依据", cap)
		}
	}
	// JSON 模式没有任何声明依据，必须是 unknown 而不是 false 当作结论。
	if got := caps[llm.CapJSON].Support; got != llm.SupportUnknown {
		t.Errorf("json_mode: 期望 unknown，得到 %s", got)
	}
}

// TestRequiresAPIKey 确认“免费”与“需要 key”按 provider 元数据独立判断。
func TestRequiresAPIKey(t *testing.T) {
	cases := map[string]bool{
		"openai":        true,
		"anthropic":     true,
		"gemini":        true,
		"deepseek":      true,
		"openai-compat": true,
		"opencode":      false, // 允许空 key，但是否可用取决于模型是否免费
		"ollama":        false,
		"copilot":       false,
		"unknown-type":  true, // 未知类型走 openai-compat 路径，Create 会要求 key
	}
	for providerType, want := range cases {
		if got := RequiresAPIKey(providerType); got != want {
			t.Errorf("RequiresAPIKey(%q) = %v, 期望 %v", providerType, got, want)
		}
	}
}

// TestAPIKeyEnvVar 确认 CLI 展示的环境变量与运行入口实际读取的一致。
func TestAPIKeyEnvVar(t *testing.T) {
	cases := map[string]string{
		"anthropic": "ANTHROPIC_API_KEY",
		"gemini":    "GOOGLE_API_KEY",
		"opencode":  "OPENCODE_API_KEY",
		"azure":     "AZURE_API_KEY",
		"bedrock":   "AWS_ACCESS_KEY_ID",
		"openai":    "OPENAI_API_KEY",
		"deepseek":  "OPENAI_API_KEY", // 运行入口对未映射类型回落到 OPENAI_API_KEY
		"groq":      "OPENAI_API_KEY",
	}
	for providerType, want := range cases {
		if got := APIKeyEnvVar(providerType); got != want {
			t.Errorf("APIKeyEnvVar(%q) = %q, 期望 %q", providerType, got, want)
		}
	}
}

// TestKnownProvider 确认内置类型判定与元数据表一致。
func TestKnownProvider(t *testing.T) {
	if !KnownProvider("openai") || !KnownProvider("ollama") {
		t.Error("已知类型被判定为未知")
	}
	if KnownProvider("definitely-not-a-provider") {
		t.Error("未知类型被判定为已知")
	}
}

// TestSortedNames 确认 provider 名称输出稳定，避免 map 迭代造成抖动。
func TestSortedNames(t *testing.T) {
	first := SortedNames()
	second := SortedNames()
	if len(first) != len(second) {
		t.Fatalf("长度不一致: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("顺序不稳定: %v vs %v", first, second)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1] > first[i] {
			t.Fatalf("未按字典序排序: %v", first)
		}
	}
}
