//go:build memory

package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Engine 是记忆引擎，管理记忆的压缩、检索和组装。
type Engine struct {
	store     Store
	fts       *FTSIndex
	maxTokens int
}

// NewEngine 创建一个新的记忆引擎。
// store 用于持久化记忆，fts 用于全文搜索，maxTokens 是压缩时的 token 上限。
func NewEngine(store Store, fts *FTSIndex, maxTokens int) *Engine {
	return &Engine{
		store:     store,
		fts:       fts,
		maxTokens: maxTokens,
	}
}

// Compress 压缩对话历史。
// 如果消息总 token 数超过 maxTokens，将较早的消息压缩为记忆。
func (e *Engine) Compress(ctx context.Context, messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
	if len(messages) <= 1 {
		return messages, nil
	}

	// 粗略估算 token 数：按 4 字符 ≈ 1 token
	totalTokens := estimateTokens(messages)
	if totalTokens <= e.maxTokens {
		return messages, nil
	}

	// 需要压缩：将最早的消息（除最后一条外）压缩为记忆
	// 保留最后一条消息（通常是最近的用户消息或助手回复）
	keepCount := 1
	if len(messages) > keepCount {
		compressCount := len(messages) - keepCount

		// 将较早的消息合并为记忆
		var parts []string
		for i := 0; i < compressCount; i++ {
			msg := messages[i]
			var text string
			for _, part := range msg.Content {
				text += part.Text
			}
			if text != "" {
				parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, text))
			}
		}

		if len(parts) > 0 {
			summary := strings.Join(parts, "\n")

			// 存储为自动记忆
			if err := e.store.Write(LayerAuto, summary, "compressed", "auto"); err != nil {
				return nil, fmt.Errorf("store compressed memory: %w", err)
			}

			// 索引到 FTS（使用内容前缀作为 ID，与 entriesByIDs 一致）
			if e.fts != nil {
				if err := e.fts.Index(entryID(summary), summary); err != nil {
					return nil, fmt.Errorf("index compressed memory: %w", err)
				}
			}
		}

		// 返回未被压缩的消息
		return messages[compressCount:], nil
	}

	return messages, nil
}

// Retrieve 检索与当前对话相关的记忆。
func (e *Engine) Retrieve(ctx context.Context, messages []llm.ChatMessage, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 10
	}

	// 从最后一条消息中提取查询文本
	query := extractQuery(messages)
	if query == "" {
		// 没有查询文本时，返回最近的记忆
		allEntries := e.store.List()
		var recent []Entry
		for _, entries := range allEntries {
			recent = append(recent, entries...)
		}
		if len(recent) > limit {
			recent = recent[:limit]
		}
		return recent, nil
	}

	// 优先使用 FTS 搜索
	if e.fts != nil {
		ids, err := e.fts.Search(query, limit)
		if err == nil && len(ids) > 0 {
			return e.entriesByIDs(ids, limit)
		}
	}

	// 回退到 Store 的简单搜索
	return e.store.Search(query, limit)
}

// Assemble 将系统提示和检索到的记忆组装到消息列表中。
// 插入一条系统消息（包含记忆上下文）在消息列表最前面。
func (e *Engine) Assemble(systemPrompt string, memories []Entry, messages []llm.ChatMessage) []llm.ChatMessage {
	if len(memories) == 0 {
		return messages
	}

	// 构建记忆上下文文本
	var contextLines []string
	contextLines = append(contextLines, "以下是相关的历史记忆：")
	contextLines = append(contextLines, "")
	for _, m := range memories {
		timeStr := m.CreatedAt.Format("2006-01-02 15:04")
		line := fmt.Sprintf("- [%s] %s", timeStr, m.Content)
		if len(m.Tags) > 0 {
			line += fmt.Sprintf(" (%s)", strings.Join(m.Tags, ", "))
		}
		contextLines = append(contextLines, line)
	}

	memoryContext := strings.Join(contextLines, "\n")

	// 构建系统消息：系统提示 + 记忆上下文
	var fullContent string
	if systemPrompt != "" {
		fullContent = systemPrompt + "\n\n" + memoryContext
	} else {
		fullContent = memoryContext
	}

	sysMsg := llm.ChatMessage{
		Role: llm.RoleSystem,
		Content: []llm.ContentPart{
			{Type: llm.ContentTypeText, Text: fullContent},
		},
	}

	result := make([]llm.ChatMessage, 0, len(messages)+1)
	result = append(result, sysMsg)
	result = append(result, messages...)
	return result
}

// ---------------------------------------------------------------------------
// 内部辅助函数
// ---------------------------------------------------------------------------

// estimateTokens 粗略估算消息的 token 数量。
func estimateTokens(messages []llm.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			// 粗略估算：每 4 个字符 ≈ 1 token
			total += len(part.Text) / 4
		}
	}
	// 每条消息增加 4 个 token 的开销（角色标记等）
	total += len(messages) * 4
	return total
}

// extractQuery 从消息列表中提取搜索查询文本。
func extractQuery(messages []llm.ChatMessage) string {
	// 从最后一条用户消息中提取
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			var text string
			for _, part := range messages[i].Content {
				text += part.Text
			}
			// 取前 200 个字符作为查询
			if len(text) > 200 {
				text = text[:200]
			}
			return text
		}
	}
	return ""
}

// entriesByIDs 根据 ID（content 的前 50 字符）从 store 中查找 Entry。
func (e *Engine) entriesByIDs(ids []string, limit int) ([]Entry, error) {
	allEntries := e.store.List()
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	var result []Entry
	for _, entries := range allEntries {
		for _, entry := range entries {
			// 用 content 的前 50 字符作为 ID 匹配
			entryID := entryID(entry.Content)
			if idSet[entryID] {
				result = append(result, entry)
				if len(result) >= limit {
					return result, nil
				}
			}
		}
	}
	return result, nil
}

// entryID 从 content 生成简短的 ID。
func entryID(content string) string {
	if len(content) > 50 {
		return content[:50]
	}
	return content
}
