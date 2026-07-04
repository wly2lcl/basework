package tools

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/permission"
)

func TestQuestionTool_Name(t *testing.T) {
	tool := NewQuestionTool(nil)
	if tool.Name() != "question" {
		t.Errorf("期望 Name='question', 得到 '%s'", tool.Name())
	}
}

func TestQuestionTool_Description(t *testing.T) {
	tool := NewQuestionTool(nil)
	if tool.Description() == "" {
		t.Error("期望 Description 非空")
	}
}

func TestQuestionTool_Parameters(t *testing.T) {
	tool := NewQuestionTool(nil)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("期望 Parameters 非空")
	}
}

func TestQuestionTool_Execute_EmptyQuestions(t *testing.T) {
	tool := NewQuestionTool(nil)
	result, err := tool.Execute(context.Background(), []byte(`{"questions": []}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestQuestionTool_Execute_InvalidParams(t *testing.T) {
	tool := NewQuestionTool(nil)
	result, err := tool.Execute(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestQuestionTool_Execute_YoloMode(t *testing.T) {
	// yolo 模式：自动选择第一个选项
	checker := permission.NewChecker(permission.ModeYolo, nil, nil)
	tool := NewQuestionTool(checker)

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "你选什么？", "options": ["选项A", "选项B", "选项C"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "选项A") {
		t.Errorf("yolo 模式应选择第一个选项, 得到: %s", result.Content)
	}
}

func TestQuestionTool_Execute_YoloMode_MultipleQuestions(t *testing.T) {
	checker := permission.NewChecker(permission.ModeYolo, nil, nil)
	tool := NewQuestionTool(checker)

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "问题1", "options": ["A1", "B1"]},
			{"text": "问题2", "options": ["A2", "B2"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "A1") || !strings.Contains(result.Content, "A2") {
		t.Errorf("期望两个问题都选择第一个选项, 得到: %s", result.Content)
	}
}

func TestQuestionTool_Execute_NoOptions(t *testing.T) {
	checker := permission.NewChecker(permission.ModeYolo, nil, nil)
	tool := NewQuestionTool(checker)

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "没有选项的问题", "options": []}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "无选项") {
		t.Errorf("期望包含'无选项'提示, 得到: %s", result.Content)
	}
}

func TestQuestionTool_Execute_InteractiveMode(t *testing.T) {
	// interactive 模式：模拟用户输入
	checker := permission.NewChecker(permission.ModeInteractive, nil, nil)

	// 准备模拟的用户输入
	input := "2\n" // 选择第二个选项

	// 创建临时文件来 mock stdin
	tmpStdin, err := os.CreateTemp("", "stdin")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpStdin.Name())
	tmpStdin.WriteString(input)
	tmpStdin.Seek(0, 0)

	// 创建临时文件来 mock stdout
	tmpStdout, err := os.CreateTemp("", "stdout")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpStdout.Name())

	tool := &QuestionTool{
		checker: checker,
		stdin:   tmpStdin,
		stdout:  tmpStdout,
	}

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "你选什么？", "options": ["选项A", "选项B", "选项C"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "选项B") {
		t.Errorf("期望选择'选项B', 得到: %s", result.Content)
	}

	// 验证 stdout 有提示输出
	stdoutContent, _ := os.ReadFile(tmpStdout.Name())
	if !bytes.Contains(stdoutContent, []byte("你选什么？")) {
		t.Errorf("期望 stdout 输出问题, 得到: %s", string(stdoutContent))
	}
}

func TestQuestionTool_Execute_InteractiveModeDefaultOnEmpty(t *testing.T) {
	checker := permission.NewChecker(permission.ModeInteractive, nil, nil)

	// 空输入，应使用默认第一个选项
	tmpStdin, _ := os.CreateTemp("", "stdin")
	tmpStdin.WriteString("\n")
	tmpStdin.Seek(0, 0)
	defer os.Remove(tmpStdin.Name())

	tmpStdout, _ := os.CreateTemp("", "stdout")
	defer os.Remove(tmpStdout.Name())

	tool := &QuestionTool{
		checker: checker,
		stdin:   tmpStdin,
		stdout:  tmpStdout,
	}

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "默认选择", "options": ["默认A", "选项B"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !strings.Contains(result.Content, "默认A") {
		t.Errorf("空输入应选择第一个选项, 得到: %s", result.Content)
	}
}

func TestQuestionTool_NilChecker(t *testing.T) {
	// checker 为 nil 时，应按 interactive 模式处理
	// 因为没有 stdin 输入，默认选择第一个选项
	tool := NewQuestionTool(nil)

	// 准备 mock stdout（input 为空）
	tmpStdout, err := os.CreateTemp("", "stdout")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpStdout.Name())

	// 因为 nil checker 走 interactive 路径，但没有 stdin 输入
	// 使用 mock stdin 提供输入
	tmpStdin, _ := os.CreateTemp("", "stdin")
	tmpStdin.WriteString("1\n")
	tmpStdin.Seek(0, 0)
	defer os.Remove(tmpStdin.Name())

	tool.stdin = tmpStdin
	tool.stdout = tmpStdout

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "测试问题", "options": ["结果1", "结果2"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !strings.Contains(result.Content, "结果1") {
		t.Errorf("期望'结果1', 得到: %s", result.Content)
	}
}

func TestQuestionTool_DenyAllMode(t *testing.T) {
	// deny-all 模式下，question 工具应仍能运行（它是对用户的，不执行危险操作）
	checker := permission.NewChecker(permission.ModeDenyAll, nil, nil)
	tool := NewQuestionTool(checker)

	result, err := tool.Execute(context.Background(), []byte(`{
		"questions": [
			{"text": "测试", "options": ["是", "否"]}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	// deny-all 模式下 question 应仍能运行（自动选第一个）
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "是") {
		t.Errorf("期望选择'是', 得到: %s", result.Content)
	}
}