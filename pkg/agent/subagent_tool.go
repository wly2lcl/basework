package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wly2lcl/basework/pkg/tool"
)

type subAgentTool struct {
	runner SubAgentRunner
}

func newSubAgentTool(runner SubAgentRunner) tool.Tool {
	return &subAgentTool{runner: runner}
}

func (t *subAgentTool) Name() string { return "sub_agent" }

func (t *subAgentTool) Description() string {
	return "Delegate a focused task to a sub-agent and return its result."
}

func (t *subAgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": {
				"type": "string",
				"description": "Task description for the sub-agent."
			},
			"prompt": {
				"type": "string",
				"description": "Alternative task prompt for compatibility."
			},
			"task": {
				"type": "string",
				"description": "Alternative task text for compatibility."
			}
		}
	}`)
}

func (t *subAgentTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if t.runner == nil || !t.runner.Enabled() {
		return nil, fmt.Errorf("sub_agent is not enabled")
	}
	var input struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
		Task        string `json:"task"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("parse sub_agent args: %w", err)
		}
	}
	task := firstNonEmpty(input.Description, input.Prompt, input.Task)
	if task == "" {
		return nil, fmt.Errorf("sub_agent requires description, prompt, or task")
	}
	output, err := t.runner.Run(ctx, task)
	if err != nil {
		return &tool.Result{Content: output, IsError: true}, err
	}
	return &tool.Result{Content: output}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ tool.Tool = (*subAgentTool)(nil)
