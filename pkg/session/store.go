package session

import (
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// Info 表示一个会话的信息摘要。
type Info struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	MessageCount int       `json:"message_count"`
	Usage        llm.Usage `json:"usage"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateOpts 是创建会话时的选项。
type CreateOpts struct {
	Title    string
	Metadata map[string]string
}

// ListFilter 是列举会话时的过滤条件。
type ListFilter struct {
	Limit     int
	Offset    int
	AfterTime time.Time
}

// Store 是会话存储接口，定义了事件溯源的核心操作。
type Store interface {
	// AppendEvent 向会话追加一个事件。
	AppendEvent(event Event) error

	// Events 根据过滤条件查询事件。
	Events(filter EventFilter) ([]Event, error)

	// Create 创建一个新的会话，返回会话信息。
	Create(opts CreateOpts) (*Info, error)

	// Get 根据 ID 获取会话信息。
	Get(id string) (*Info, error)

	// List 列举会话，支持过滤和分页。
	List(filter ListFilter) ([]*Info, error)

	// Delete 删除一个会话。
	Delete(id string) error

	// Messages 投影当前会话的所有事件为 LLM 消息列表。
	Messages() ([]llm.ChatMessage, error)
}
