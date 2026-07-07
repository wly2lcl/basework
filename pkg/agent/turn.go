package agent

import (
	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// TurnD 是 turn 执行所需的依赖接口，Pipeline 通过此接口获取所有运行时依赖
type TurnD interface {
	SystemPrompt() string
	MaxSteps() int
	Model() llm.Model
	Session() session.Store
	SessionID() string
	History() ([]llm.ChatMessage, error)
	Hooks() *hook.Chain
	ToolRegistry() *tool.Registry
	Callback() Callback
}

// Instance 是 agent 实例属性接口
type Instance interface {
	ID() string
	Model() llm.Model
	TurnD() TurnD
}
