package config

import (
	"encoding/json"
	"strings"
)

// 本文件提供配置的脱敏视图（CFG-001）。
//
// 目标只有一个：解释配置时绝不把秘密写到 stdout/stderr。规则是「宁可多脱、
// 不可漏脱」——漏掉一个 API key 的代价远大于多隐藏一个普通字符串。但也做了
// 明确的边界：token_path 这类"引用秘密的位置"不是秘密本身，过度脱敏会让
// 解释输出失去排查价值。

// RedactionMarker 是脱敏后字符串的统一占位。它只表达"这里有一个值且已隐藏"，
// 不保留长度、前缀等任何可猜测信息。
const RedactionMarker = "«已设置，已脱敏»"

// SensitiveKey 判断配置字段名是否属于敏感字段。大小写不敏感。
//
// 判定规则（对 snake_case / camelCase 的 json key 同样适用）：
//   - 含 secret / password / passwd / api_key / apikey / private_key / credential
//   - 等于 key 或以 _key 结尾（access_key、secret_key、api_key……）
//   - 等于 token 或以 _token 结尾（access_token、auth_token……）；
//     token_path 这类"路径"不算
//   - 等于 authorization 或以 _authorization 结尾
func SensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	if lower == "" {
		return false
	}
	for _, frag := range []string{"secret", "password", "passwd", "api_key", "apikey", "private_key", "credential"} {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	if lower == "key" || strings.HasSuffix(lower, "_key") {
		return true
	}
	if lower == "token" || strings.HasSuffix(lower, "_token") {
		return true
	}
	if lower == "authorization" || strings.HasSuffix(lower, "_authorization") {
		return true
	}
	return false
}

// RedactedView 返回配置的脱敏视图。
//
// 实现是「序列化为通用 map → 递归走 key → 敏感的字符串值替换为 RedactionMarker」。
// 走通用 map 而不是逐字段写死的原因：Config 里有 MCPConfigs 这类 map[string]interface{}
// 的任意嵌套用户数据，里面完全可能出现 token 字段；靠字段清单维护脱敏必然漏。
//
// 返回的 map 是全新构造的，与原配置对象不共享任何可变结构。
func RedactedView(cfg *Config) (map[string]any, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return redactValue(tree).(map[string]any), nil
}

// redactValue 递归脱敏。map 按 key 判定，slice 逐项递归，
// 其他类型原样返回（数字、布尔不是秘密；字符串只在 key 敏感时替换）。
func redactValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			if SensitiveKey(k) {
				if s, ok := val.(string); ok && s != "" {
					out[k] = RedactionMarker
					continue
				}
				// 非字符串（数字/对象）或空值：空值原样保留（"未设置"是有用信息），
				// 对象里可能有嵌套敏感字段，继续递归而不是整块抹掉。
				if _, isMap := val.(map[string]any); isMap {
					out[k] = redactValue(val)
					continue
				}
				if _, isSlice := val.([]any); isSlice {
					out[k] = redactValue(val)
					continue
				}
				out[k] = val
				continue
			}
			out[k] = redactValue(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}
