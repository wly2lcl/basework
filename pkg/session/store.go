package session

import (
	"fmt"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// resolveCreateID 返回本次创建应使用的会话 ID。
//
// opts.ID 为空时生成新 ID（原有行为）；非空时校验字符集后原样使用。
// 放在公共位置而不是各实现里各写一遍：三个 Store 实现必须给出同一套 ID 规则，
// 否则「同样的调用在不同后端下是否保留 ID」会随实现漂移。
func resolveCreateID(opts CreateOpts) (string, error) {
	if opts.ID == "" {
		return newID(), nil
	}
	if !safeIDPattern.MatchString(opts.ID) {
		return "", fmt.Errorf("session: 非法的会话 ID: %q（只允许 [a-zA-Z0-9_-]）", opts.ID)
	}
	return opts.ID, nil
}

// Info 表示一个会话的信息摘要。
type Info struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	MessageCount int       `json:"message_count"`
	Usage        llm.Usage `json:"usage"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateOpts 是创建会话时的选项。
type CreateOpts struct {
	Title    string
	Metadata map[string]string
	// ID 指定会话 ID；为空时由实现生成一个新 ID。
	//
	// 存在的理由：迁移必须保留原会话身份。此前没有这个入口，JSONL→SQLite 迁移
	// 只能让实现另发一个新 ID，于是事件里带的原 session_id 找不到对应会话、
	// 逐条外键失败，迁移报「成功」却一条事件都没落库（见 SHIP-002 记录）。
	//
	// 这是**可选**字段：留空即维持「由实现生成 ID」的原有行为，既有调用方不受影响。
	// 非空时必须满足 [a-zA-Z0-9_-]，与 JSONL 的文件名约束一致（避免路径穿越）。
	ID string
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
