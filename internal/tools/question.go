package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Question 表示单个问题
type Question struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
}

// QuestionTool 实现 question 工具，用于向用户提出问题并获取选择
type QuestionTool struct {
	checker *permission.Checker
	stdin   *os.File // 用于测试时 mock stdin
	stdout  *os.File // 用于测试时 mock stdout
}

// NewQuestionTool 创建 question 工具
func NewQuestionTool(checker *permission.Checker) *QuestionTool {
	return &QuestionTool{
		checker: checker,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
	}
}

// Name 返回工具名称
func (q *QuestionTool) Name() string {
	return "question"
}

// Description 返回工具描述
func (q *QuestionTool) Description() string {
	return "向用户提出问题并提供选项供选择"
}

// Parameters 返回工具参数 JSON Schema
func (q *QuestionTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"text": { "type": "string", "description": "问题文本" },
						"options": {
							"type": "array",
							"items": { "type": "string" },
							"description": "选项列表"
						}
					},
					"required": ["text", "options"]
				},
				"description": "问题列表"
			}
		},
		"required": ["questions"]
	}`)
}

// questionParams 工具参数结构
type questionParams struct {
	Questions []Question `json:"questions"`
}

// Execute 执行 question 工具
func (q *QuestionTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params questionParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if len(params.Questions) == 0 {
		return &tool.Result{
			Content: "问题列表为空",
			IsError: true,
		}, nil
	}

	// 根据权限模式处理
	var responses []string

	for _, qItem := range params.Questions {
		if len(qItem.Options) == 0 {
			responses = append(responses, fmt.Sprintf("%s: (无选项)", qItem.Text))
			continue
		}

		var response string

		if q.checker != nil && q.checker.Mode == permission.ModeYolo {
			// yolo 模式：自动选择第一个选项
			response = qItem.Options[0]
		} else {
			// interactive 模式：通过 stdin/stdout 交互
			response = q.promptUser(qItem)
			if response == "" {
				// 无输入时默认第一项
				response = qItem.Options[0]
			}
		}

		responses = append(responses, fmt.Sprintf("%s: %s", qItem.Text, response))
	}

	// 构建结果
	var buf bytes.Buffer
	buf.WriteString("问答结果:\n\n")
	for _, r := range responses {
		buf.WriteString(r + "\n")
	}

	return &tool.Result{
		Content: strings.TrimSpace(buf.String()),
	}, nil
}

// promptUser 通过标准输入输出向用户提问
func (q *QuestionTool) promptUser(question Question) string {
	reader := bufio.NewReader(q.stdin)

	// 输出问题
	fmt.Fprintf(q.stdout, "\n❓ %s\n", question.Text)
	fmt.Fprintf(q.stdout, "请选择（输入数字 1-%d）:\n", len(question.Options))
	for i, opt := range question.Options {
		fmt.Fprintf(q.stdout, "  %d. %s\n", i+1, opt)
	}
	fmt.Fprint(q.stdout, "> ")

	// 读取用户输入
	input, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	// 尝试解析数字
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil {
		if idx >= 1 && idx <= len(question.Options) {
			return question.Options[idx-1]
		}
	}

	// 尝试匹配选项文本
	for _, opt := range question.Options {
		if strings.EqualFold(input, opt) {
			return opt
		}
	}

	return question.Options[0]
}
