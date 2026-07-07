package provider

import (
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ============================================================
// Tests for markSystemContentForCache
// ============================================================

func TestMarkSystemContentForCache_Empty(t *testing.T) {
	result := markSystemContentForCache("")
	if result != nil {
		t.Errorf("expected nil for empty system, got %+v", result)
	}
}

func TestMarkSystemContentForCache_NonEmpty(t *testing.T) {
	result := markSystemContentForCache("You are a helpful assistant.")
	if len(result) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result))
	}
	if result[0]["type"] != "text" {
		t.Errorf("expected type 'text', got '%v'", result[0]["type"])
	}
	if result[0]["text"] != "You are a helpful assistant." {
		t.Errorf("expected text, got '%v'", result[0]["text"])
	}
	cc, ok := result[0]["cache_control"].(map[string]any)
	if !ok {
		t.Fatal("expected cache_control field")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("expected cache_control type 'ephemeral', got '%v'", cc["type"])
	}
}

// ============================================================
// Tests for shouldMarkCacheControl
// ============================================================

func TestShouldMarkCacheControl(t *testing.T) {
	tests := []struct {
		blockType string
		want      bool
	}{
		{"text", true},
		{"image", false},
		{"tool_result", false},
		{"image_url", false},
		{"tool_use", true},
	}
	for _, tt := range tests {
		got := shouldMarkCacheControl(tt.blockType)
		if got != tt.want {
			t.Errorf("shouldMarkCacheControl(%q) = %v, want %v", tt.blockType, got, tt.want)
		}
	}
}

// ============================================================
// Tests for markUserContentBlocksForCache
// ============================================================

func TestMarkUserContentBlocksForCache_Empty(t *testing.T) {
	blocks := []map[string]any{}
	markUserContentBlocksForCache(blocks, 2)
	if len(blocks) != 0 {
		t.Errorf("expected empty blocks, got %d", len(blocks))
	}
}

func TestMarkUserContentBlocksForCache_AllText(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "Hello"},
		{"type": "text", "text": "World"},
		{"type": "text", "text": "Third"},
	}
	markUserContentBlocksForCache(blocks, 2)

	// First 2 should have cache_control
	if blocks[0]["cache_control"] == nil {
		t.Error("expected block[0] to have cache_control")
	}
	if blocks[1]["cache_control"] == nil {
		t.Error("expected block[1] to have cache_control")
	}
	// Third should NOT have cache_control
	if blocks[2]["cache_control"] != nil {
		t.Error("expected block[2] to NOT have cache_control")
	}
}

func TestMarkUserContentBlocksForCache_SkipNonText(t *testing.T) {
	blocks := []map[string]any{
		{"type": "image", "source": map[string]any{"type": "base64"}},
		{"type": "text", "text": "Describe this"},
	}
	markUserContentBlocksForCache(blocks, 2)

	// Image should not have cache_control
	if blocks[0]["cache_control"] != nil {
		t.Error("expected image block to NOT have cache_control")
	}
	// Text should have cache_control
	if blocks[1]["cache_control"] == nil {
		t.Error("expected text block to have cache_control")
	}
}

func TestMarkUserContentBlocksForCache_ToolResult(t *testing.T) {
	blocks := []map[string]any{
		{"type": "tool_result", "content": "result"},
		{"type": "text", "text": "Follow up"},
	}
	markUserContentBlocksForCache(blocks, 2)

	if blocks[0]["cache_control"] != nil {
		t.Error("expected tool_result to NOT have cache_control")
	}
	if blocks[1]["cache_control"] == nil {
		t.Error("expected text block to have cache_control")
	}
}

func TestMarkUserContentBlocksForCache_MaxBlocksLimit(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "A"},
		{"type": "text", "text": "B"},
		{"type": "text", "text": "C"},
	}
	// Only mark 1 block
	markUserContentBlocksForCache(blocks, 1)

	if blocks[0]["cache_control"] == nil {
		t.Error("expected block[0] to have cache_control")
	}
	if blocks[1]["cache_control"] != nil {
		t.Error("expected block[1] to NOT have cache_control (max=1)")
	}
	if blocks[2]["cache_control"] != nil {
		t.Error("expected block[2] to NOT have cache_control (max=1)")
	}
}

// ============================================================
// Tests for countUserTextBlocks
// ============================================================

func TestCountUserTextBlocks(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "A"},
		{"type": "image", "source": map[string]any{}},
		{"type": "tool_result", "content": "result"},
		{"type": "text", "text": "B"},
	}
	count := countUserTextBlocks(blocks)
	if count != 2 {
		t.Errorf("expected 2 text blocks, got %d", count)
	}
}

// ============================================================
// Tests for markAnthropicMessagesForCache
// ============================================================

func TestMarkAnthropicMessagesForCache_Disabled(t *testing.T) {
	system := "You are helpful."
	messages := []map[string]any{
		{"role": "user", "content": "Hi"},
	}

	systemBlocks, systemText, msgs := markAnthropicMessagesForCache(system, messages, false)

	if systemBlocks != nil {
		t.Error("expected nil systemBlocks when cache disabled")
	}
	if systemText != system {
		t.Errorf("expected systemText '%s', got '%s'", system, systemText)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

func TestMarkAnthropicMessagesForCache_SystemOnly(t *testing.T) {
	system := "You are helpful."
	messages := []map[string]any{
		{"role": "user", "content": "Hi"},
	}

	systemBlocks, systemText, msgs := markAnthropicMessagesForCache(system, messages, true)

	if len(systemBlocks) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(systemBlocks))
	}
	cc, ok := systemBlocks[0]["cache_control"].(map[string]any)
	if !ok {
		t.Fatal("expected cache_control on system block")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("expected ephemeral cache control, got %v", cc["type"])
	}
	if systemText != system {
		t.Errorf("expected systemText preserved, got '%s'", systemText)
	}
	_ = msgs
}

func TestMarkAnthropicMessagesForCache_SimpleStringContent(t *testing.T) {
	system := ""
	messages := []map[string]any{
		{"role": "system", "content": "Be helpful."},
		{"role": "user", "content": "Hello"},
	}

	_, _, msgs := markAnthropicMessagesForCache(system, messages, true)

	// Should convert string content to blocks with cache_control
	for _, msg := range msgs {
		if role, _ := msg["role"].(string); role == "user" {
			blocks, ok := msg["content"].([]map[string]any)
			if !ok {
				t.Fatal("expected content to be []map[string]any")
			}
			if len(blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(blocks))
			}
			if blocks[0]["cache_control"] == nil {
				t.Error("expected cache_control on user text block")
			}
		}
	}
}

func TestMarkAnthropicMessagesForCache_OnlyFirstUserMessage(t *testing.T) {
	system := ""
	messages := []map[string]any{
		{"role": "user", "content": []map[string]any{
			{"type": "text", "text": "First message"},
		}},
		{"role": "assistant", "content": "Response"},
		{"role": "user", "content": []map[string]any{
			{"type": "text", "text": "Second message"},
		}},
	}

	_, _, msgs := markAnthropicMessagesForCache(system, messages, true)

	// First user message should have cache_control
	firstContent := msgs[0]["content"].([]map[string]any)
	if firstContent[0]["cache_control"] == nil {
		t.Error("expected first user message to have cache_control")
	}

	// Second user message (index 2) should NOT have cache_control
	secondContent := msgs[2]["content"].([]map[string]any)
	if secondContent[0]["cache_control"] != nil {
		t.Error("expected second user message to NOT have cache_control")
	}
}

// ============================================================
// Tests for markOpenAIContentForCache
// ============================================================

func TestMarkOpenAIContentForCache_TextOnly(t *testing.T) {
	parts := []llm.ContentPart{
		{Type: llm.ContentTypeText, Text: "Hello"},
		{Type: llm.ContentTypeText, Text: "World"},
		{Type: llm.ContentTypeText, Text: "Third"},
	}

	blocks := markOpenAIContentForCache(parts)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	// First 2 should have cache_control
	if blocks[0]["cache_control"] == nil {
		t.Error("expected block[0] to have cache_control")
	}
	if blocks[1]["cache_control"] == nil {
		t.Error("expected block[1] to have cache_control")
	}
	// Third should NOT have cache_control
	if blocks[2]["cache_control"] != nil {
		t.Error("expected block[2] to NOT have cache_control")
	}
}

func TestMarkOpenAIContentForCache_WithImage(t *testing.T) {
	parts := []llm.ContentPart{
		{Type: llm.ContentTypeText, Text: "Describe"},
		{Type: llm.ContentTypeImage, ImageURL: "https://example.com/img.jpg"},
	}

	blocks := markOpenAIContentForCache(parts)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	// Text block should have cache_control
	if blocks[0]["cache_control"] == nil {
		t.Error("expected text block to have cache_control")
	}
	// Image block should NOT have cache_control
	if blocks[1]["cache_control"] != nil {
		t.Error("expected image block to NOT have cache_control")
	}
	if blocks[1]["type"] != "image_url" {
		t.Errorf("expected image_url type, got '%v'", blocks[1]["type"])
	}
}

// ============================================================
// Tests for shouldCachePrompt (with config)
// ============================================================

func TestShouldCachePrompt_Enabled(t *testing.T) {
	// Test with a minimal mock approach - just test the edge cases
	// shouldCachePrompt is used internally; test via markAnthropicMessagesForCache

	// When cache is disabled, systemBlocks should be nil
	sysBlocks, _, _ := markAnthropicMessagesForCache("test", []map[string]any{}, false)
	if sysBlocks != nil {
		t.Error("expected nil system blocks when cache disabled")
	}

	// When cache is enabled, systemBlocks should be non-nil
	sysBlocks, _, _ = markAnthropicMessagesForCache("test", []map[string]any{}, true)
	if sysBlocks == nil {
		t.Error("expected non-nil system blocks when cache enabled")
	}
}
