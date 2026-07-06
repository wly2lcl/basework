// Package dialog 测试
package dialog

import (
	"testing"
)

// TestConfirmDialogInit 测试确认对话框初始化
func TestConfirmDialogInit(t *testing.T) {
	d := NewConfirm("标题", "消息", nil, nil)
	cmd := d.Init()
	if cmd != nil {
		t.Fatal("Init() 应返回 nil")
	}
}

// TestConfirmDialogInput 测试输入对话框初始化
func TestInputDialogInit(t *testing.T) {
	d := NewInput("标题", "提示", "", nil, nil)
	cmd := d.Init()
	if cmd != nil {
		t.Fatal("Init() 应返回 nil")
	}
}

// TestSelectDialogInit 测试选择对话框初始化
func TestSelectDialogInit(t *testing.T) {
	d := NewSelect("标题", "消息", []string{"a"}, nil, nil)
	cmd := d.Init()
	if cmd != nil {
		t.Fatal("Init() 应返回 nil")
	}
}