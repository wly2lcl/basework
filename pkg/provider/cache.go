package provider

import (
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
)

// shouldCachePrompt 检查 prompt 缓存是否在配置中启用。
// 如果 cfg 为 nil，默认返回 false（安全默认）。
func shouldCachePrompt(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.PromptCache.Enabled
}

// cacheControlEphemeral 是 cache_control 标记值
var cacheControlEphemeral = map[string]any{
	"type": "ephemeral",
}

// shouldMarkCacheControl 判断给定的内容块类型是否适合添加缓存标记。
// tool_result 和 image 类型的内容块不应标记缓存。
func shouldMarkCacheControl(blockType string) bool {
	return blockType != "tool_result" && blockType != "image" && blockType != "image_url"
}

// markSystemContentForCache 为 Anthropic 格式的 system content blocks 添加缓存标记。
// 将 system 从纯文本转为对象数组，并标记缓存。
func markSystemContentForCache(systemText string) []map[string]any {
	if systemText == "" {
		return nil
	}
	return []map[string]any{
		{
			"type":          "text",
			"text":          systemText,
			"cache_control": cacheControlEphemeral,
		},
	}
}

// markUserContentBlocksForCache 为 Anthropic 格式的用户消息 content blocks 添加缓存标记。
// 标记规则：
//   - 仅标记前 maxBlocks 个符合条件的 content block（text 类型）
//   - tool_result 和 image 类型的 block 不标记
//   - 符合条件的 block 数量不足 maxBlocks 时，全部标记
func markUserContentBlocksForCache(blocks []map[string]any, maxBlocks int) {
	marked := 0
	for i := range blocks {
		if marked >= maxBlocks {
			break
		}
		blockType, _ := blocks[i]["type"].(string)
		if !shouldMarkCacheControl(blockType) {
			continue
		}
		blocks[i]["cache_control"] = cacheControlEphemeral
		marked++
	}
}

// countUserTextBlocks 计算用户消息中可缓存的 text 内容块数量。
// 用于决定需要标记多少个 block。
func countUserTextBlocks(blocks []map[string]any) int {
	count := 0
	for _, b := range blocks {
		blockType, _ := b["type"].(string)
		if shouldMarkCacheControl(blockType) {
			count++
		}
	}
	return count
}

// markAnthropicMessagesForCache 为 Anthropic 消息序列添加缓存标记。
// 标记策略：
//  1. 如果 system 不为空，将其转为带 cache_control 的对象数组
//  2. 第一条 user 消息的前 1-2 个 text content block 标记 cache_control
//
// 返回：systemBlocks（对象数组或 nil）, systemText（原始文本）, messages（标记后的消息）
func markAnthropicMessagesForCache(system string, messages []map[string]any, cacheEnabled bool) ([]map[string]any, string, []map[string]any) {
	if !cacheEnabled {
		return nil, system, messages
	}

	systemBlocks := markSystemContentForCache(system)
	messagesCopy := copyMessages(messages)

	// 为第一条 user 消息的前 2 个 text content block 标记缓存
	for _, msg := range messagesCopy {
		role, _ := msg["role"].(string)
		if role != "user" {
			continue
		}
		contentBlocks, ok := msg["content"].([]map[string]any)
		if !ok {
			// 简单字符串 content，转为 content blocks 再标记
			if contentStr, ok2 := msg["content"].(string); ok2 && contentStr != "" {
				blocks := []map[string]any{
					{"type": "text", "text": contentStr},
				}
				markUserContentBlocksForCache(blocks, 2)
				msg["content"] = blocks
			}
			break
		}
		markUserContentBlocksForCache(contentBlocks, 2)
		break // 只标记第一条 user 消息
	}

	return systemBlocks, system, messagesCopy
}

// copyMessages 深拷贝消息切片以防止修改原始数据
func copyMessages(msgs []map[string]any) []map[string]any {
	if msgs == nil {
		return nil
	}
	result := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		cp := make(map[string]any, len(m))
		for k, v := range m {
			cp[k] = v
		}
		result[i] = cp
	}
	return result
}

// markOpenAIContentForCache 为 OpenAI 格式的用户消息 content parts 添加缓存标记。
// 标记规则：
//   - 仅标记 text 类型的 content part
//   - 标记前 2 个 text parts
//   - image_url 类型的 part 不标记
func markOpenAIContentForCache(parts []llm.ContentPart) []map[string]any {
	blocks := make([]map[string]any, 0, len(parts))
	cacheRemaining := 2

	for _, p := range parts {
		switch p.Type {
		case llm.ContentTypeText:
			block := map[string]any{
				"type": "text",
				"text": p.Text,
			}
			if cacheRemaining > 0 {
				block["cache_control"] = cacheControlEphemeral
				cacheRemaining--
			}
			blocks = append(blocks, block)
		case llm.ContentTypeImage:
			blocks = append(blocks, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": p.ImageURL,
				},
			})
		}
	}
	return blocks
}

// maxUserContentsToCache 是 Gemini 中需要标记缓存的 user content 数量
const maxUserContentsToCache = 2
