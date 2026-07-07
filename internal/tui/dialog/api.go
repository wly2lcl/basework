// Package dialog 提供对话框系统
package dialog

// API 提供命令式快捷对话框创建函数

// Confirm 创建一个确认对话框并返回
func Confirm(title, message string, onConfirm func()) *ConfirmDialog {
	return NewConfirm(title, message, onConfirm, nil)
}

// ConfirmWithCancel 创建带取消回调的确认对话框
func ConfirmWithCancel(title, message string, onConfirm func(), onCancel func()) *ConfirmDialog {
	return NewConfirm(title, message, onConfirm, onCancel)
}

// Input 创建输入对话框
func Input(title, message, defaultValue string, onSubmit func(string)) *InputDialog {
	return NewInput(title, message, defaultValue, onSubmit, nil)
}

// InputWithCancel 创建带取消回调的输入对话框
func InputWithCancel(title, message, defaultValue string, onSubmit func(string), onCancel func()) *InputDialog {
	return NewInput(title, message, defaultValue, onSubmit, onCancel)
}

// Select 创建选择对话框
func Select(title, message string, items []string, onSelect func(string)) *SelectDialog {
	return NewSelect(title, message, items, onSelect, nil)
}

// SelectWithCancel 创建带取消回调的选择对话框
func SelectWithCancel(title, message string, items []string, onSelect func(string), onCancel func()) *SelectDialog {
	return NewSelect(title, message, items, onSelect, onCancel)
}
