package llm

import (
	"testing"
)

func TestChatMessage_RoleUser(t *testing.T) {
	msg := ChatMessage{
		Role: RoleUser,
		Content: []ContentPart{
			{Type: ContentTypeText, Text: "你好"},
		},
	}
	if msg.Role != RoleUser {
		t.Errorf("期望 Role=%q, 得到 %q", RoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("期望 Content 长度 1, 得到 %d", len(msg.Content))
	}
	if msg.Content[0].Type != ContentTypeText || msg.Content[0].Text != "你好" {
		t.Errorf("ContentPart 不匹配: %+v", msg.Content[0])
	}
}

func TestChatMessage_RoleAssistantWithToolCalls(t *testing.T) {
	msg := ChatMessage{
		Role: RoleAssistant,
		Content: []ContentPart{
			{Type: ContentTypeText, Text: "我来查询天气"},
		},
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"北京"}`},
		},
	}
	if msg.Role != RoleAssistant {
		t.Errorf("期望 Role=%q", RoleAssistant)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("期望 ToolCalls 长度 1, 得到 %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "get_weather" || tc.ArgsJSON != `{"city":"北京"}` {
		t.Errorf("ToolCall 不匹配: %+v", tc)
	}
}

func TestChatMessage_RoleTool(t *testing.T) {
	msg := ChatMessage{
		Role:       RoleTool,
		ToolCallID: "call_1",
		Content: []ContentPart{
			{Type: ContentTypeText, Text: `{"temperature": 22}`},
		},
	}
	if msg.Role != RoleTool {
		t.Errorf("期望 Role=%q", RoleTool)
	}
	if msg.ToolCallID != "call_1" {
		t.Errorf("期望 ToolCallID=call_1, 得到 %s", msg.ToolCallID)
	}
}

func TestContentPart_Text(t *testing.T) {
	cp := ContentPart{Type: ContentTypeText, Text: "hello"}
	if cp.Type != ContentTypeText || cp.Text != "hello" || cp.ImageURL != "" {
		t.Errorf("文本 ContentPart 不匹配: %+v", cp)
	}
}

func TestContentPart_Image(t *testing.T) {
	cp := ContentPart{
		Type:     ContentTypeImage,
		ImageURL: "https://example.com/img.png",
	}
	if cp.Type != ContentTypeImage {
		t.Errorf("期望 Type=image, 得到 %s", cp.Type)
	}
	if cp.ImageURL != "https://example.com/img.png" {
		t.Errorf("期望 ImageURL 不匹配: %s", cp.ImageURL)
	}
	if cp.Text != "" {
		t.Errorf("图片 ContentPart 不应有 Text")
	}
}

func TestToolCall_Fields(t *testing.T) {
	tc := ToolCall{
		ID:       "tc_1",
		Name:     "search",
		ArgsJSON: `{"q":"test"}`,
	}
	if tc.ID != "tc_1" || tc.Name != "search" || tc.ArgsJSON != `{"q":"test"}` {
		t.Errorf("ToolCall 字段不匹配: %+v", tc)
	}
}

func TestToolResult_Success(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "tc_1",
		Content:    `{"result": "ok"}`,
		IsError:    false,
	}
	if tr.ToolCallID != "tc_1" || tr.Content != `{"result": "ok"}` || tr.IsError {
		t.Errorf("ToolResult 成功应 IsError=false: %+v", tr)
	}
}

func TestToolResult_Error(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "tc_2",
		Content:    "查询失败",
		IsError:    true,
	}
	if tr.ToolCallID != "tc_2" || tr.Content != "查询失败" || !tr.IsError {
		t.Errorf("ToolResult 错误应 IsError=true: %+v", tr)
	}
}