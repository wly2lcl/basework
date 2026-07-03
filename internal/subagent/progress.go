package subagent

import (
	"fmt"
	"sync"
	"time"
)

// ProgressReporter 进度报告器接口
type ProgressReporter interface {
	// OnStart 子代理任务开始时调用
	OnStart(task *Task)
	// OnProgress 子代理任务进度更新时调用
	OnProgress(task *Task, progress string)
	// OnComplete 子代理任务完成时调用
	OnComplete(task *Task, result *Result)
}

// ProgressEvent 进度事件
type ProgressEvent struct {
	// TaskID 任务 ID
	TaskID string `json:"task_id"`
	// Type 事件类型：start / progress / complete
	Type string `json:"type"`
	// Description 任务描述
	Description string `json:"description"`
	// Progress 进度信息（仅 progress 类型）
	Progress string `json:"progress,omitempty"`
	// Result 执行结果（仅 complete 类型）
	Result *Result `json:"result,omitempty"`
	// Timestamp 事件时间戳
	Timestamp time.Time `json:"timestamp"`
}

// NopProgressReporter 空实现进度报告器
type NopProgressReporter struct{}

func (n *NopProgressReporter) OnStart(task *Task)    {}
func (n *NopProgressReporter) OnProgress(task *Task, progress string) {}
func (n *NopProgressReporter) OnComplete(task *Task, result *Result) {}

// CallbackProgressReporter 回调式进度报告器
type CallbackProgressReporter struct {
	mu            sync.RWMutex
	onStartFn     func(event ProgressEvent)
	onProgressFn  func(event ProgressEvent)
	onCompleteFn  func(event ProgressEvent)
}

// NewCallbackProgressReporter 创建回调式进度报告器
func NewCallbackProgressReporter() *CallbackProgressReporter {
	return &CallbackProgressReporter{}
}

// SetOnStart 设置任务开始回调
func (c *CallbackProgressReporter) SetOnStart(fn func(event ProgressEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStartFn = fn
}

// SetOnProgress 设置进度更新回调
func (c *CallbackProgressReporter) SetOnProgress(fn func(event ProgressEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onProgressFn = fn
}

// SetOnComplete 设置任务完成回调
func (c *CallbackProgressReporter) SetOnComplete(fn func(event ProgressEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onCompleteFn = fn
}

// OnStart 触发任务开始事件
func (c *CallbackProgressReporter) OnStart(task *Task) {
	event := ProgressEvent{
		TaskID:      task.ID,
		Type:        "subagent.start",
		Description: task.Description,
		Timestamp:   time.Now(),
	}
	c.mu.RLock()
	fn := c.onStartFn
	c.mu.RUnlock()
	if fn != nil {
		fn(event)
	}
}

// OnProgress 触发进度更新事件
func (c *CallbackProgressReporter) OnProgress(task *Task, progress string) {
	event := ProgressEvent{
		TaskID:      task.ID,
		Type:        "subagent.progress",
		Description: task.Description,
		Progress:    progress,
		Timestamp:   time.Now(),
	}
	c.mu.RLock()
	fn := c.onProgressFn
	c.mu.RUnlock()
	if fn != nil {
		fn(event)
	}
}

// OnComplete 触发任务完成事件
func (c *CallbackProgressReporter) OnComplete(task *Task, result *Result) {
	event := ProgressEvent{
		TaskID:      task.ID,
		Type:        "subagent.complete",
		Description: task.Description,
		Result:      result,
		Timestamp:   time.Now(),
	}
	c.mu.RLock()
	fn := c.onCompleteFn
	c.mu.RUnlock()
	if fn != nil {
		fn(event)
	}

	// 打印结果到控制台
	if result != nil {
		if result.Success {
			fmt.Printf("[子代理 %s] 任务完成: %s\n", task.ID[:8], task.Description)
		} else {
			fmt.Printf("[子代理 %s] 任务失败: %s - %s\n", task.ID[:8], task.Description, result.Error)
		}
	}
}