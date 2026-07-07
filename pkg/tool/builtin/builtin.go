package builtin

import (
	"github.com/wly2lcl/basework/pkg/tool"
)

// All 返回所有内置工具实例
func All() []tool.Tool {
	return []tool.Tool{
		&BashTool{
			PermissionMode: "default",
		},
		&ReadTool{},
		&WriteTool{},
		&EditTool{},
		&GrepTool{},
		&GlobTool{},
	}
}
