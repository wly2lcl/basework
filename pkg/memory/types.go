// Package memory 提供分层记忆系统，支持用户层、项目层、本地层和自动层。
package memory

import "time"

// MemoryLayer 表示记忆的层级。
type MemoryLayer string

const (
	// LayerUser 表示用户级记忆，存储在 ~/.config/basework/AGENTS.md。
	LayerUser MemoryLayer = "user"
	// LayerProject 表示项目级记忆，存储在 {workspace}/AGENTS.md。
	LayerProject MemoryLayer = "project"
	// LayerLocal 表示本地级记忆，存储在 {workspace}/.basework/AGENTS.md。
	LayerLocal MemoryLayer = "local"
	// LayerAuto 表示自动记忆，由系统自动管理，存储在 {workspace}/.basework/auto-memory.md。
	LayerAuto MemoryLayer = "auto"
)

// Entry 表示一条记忆记录。
type Entry struct {
	// Layer 是记忆所属的层级。
	Layer MemoryLayer `json:"layer"`
	// Content 是记忆的文本内容。
	Content string `json:"content"`
	// Tags 是记忆的标签列表。
	Tags []string `json:"tags,omitempty"`
	// CreatedAt 是记忆的创建时间。
	CreatedAt time.Time `json:"created_at"`
}