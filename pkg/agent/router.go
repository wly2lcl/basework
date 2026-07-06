package agent

// RouteForProvider 根据 Provider 名称返回对应的内置模板名称。
// 用于在 Agent 初始化时选择适合该 Provider 的系统提示模板。
//
// 映射规则:
//
//	"anthropic" → "anthropic"（Claude 系列优化）
//	"openai"    → "openai"（GPT 系列优化）
//	"gemini"    → "gemini"（Gemini 系列优化）
//	其他        → "default"（通用模板）
func RouteForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "gemini":
		return "gemini"
	case "opencode":
		return "openai" // OpenCode Zen 使用 OpenAI 兼容协议
	case "bedrock":
		return "anthropic" // Bedrock 常用 Claude 模型
	default:
		return "default"
	}
}