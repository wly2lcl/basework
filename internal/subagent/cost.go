package subagent

import (
	"fmt"
	"sync"
)

// costRecord 单次成本记录
type costRecord struct {
	taskID      string
	tokenUsage  TokenUsage
	cost        float64
}

// CostTracker 成本追踪器，跟踪子代理的 token 使用量和成本
type CostTracker struct {
	mu      sync.RWMutex
	records map[string][]costRecord // taskID -> cost records
	totalCost float64
}

// NewCostTracker 创建新的成本追踪器
func NewCostTracker() *CostTracker {
	return &CostTracker{
		records:   make(map[string][]costRecord),
		totalCost: 0,
	}
}

// TrackUsage 记录 token 使用量和成本
func (ct *CostTracker) TrackUsage(taskID string, usage TokenUsage, cost float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	record := costRecord{
		taskID:     taskID,
		tokenUsage: usage,
		cost:       cost,
	}
	ct.records[taskID] = append(ct.records[taskID], record)
	ct.totalCost += cost
}

// GetTaskCost 获取指定任务的累积成本
func (ct *CostTracker) GetTaskCost(taskID string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	records, ok := ct.records[taskID]
	if !ok {
		return 0
	}

	var total float64
	for _, r := range records {
		total += r.cost
	}
	return total
}

// GetTaskUsage 获取指定任务的累计 token 使用量
func (ct *CostTracker) GetTaskUsage(taskID string) TokenUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	records, ok := ct.records[taskID]
	if !ok {
		return TokenUsage{}
	}

	var total TokenUsage
	for _, r := range records {
		total.InputTokens += r.tokenUsage.InputTokens
		total.OutputTokens += r.tokenUsage.OutputTokens
		total.TotalTokens += r.tokenUsage.TotalTokens
	}
	return total
}

// GetTotalCost 获取所有任务的累计成本
func (ct *CostTracker) GetTotalCost() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCost
}

// GetTotalUsage 获取所有任务的累计 token 使用量
func (ct *CostTracker) GetTotalUsage() TokenUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var total TokenUsage
	for _, records := range ct.records {
		for _, r := range records {
			total.InputTokens += r.tokenUsage.InputTokens
			total.OutputTokens += r.tokenUsage.OutputTokens
			total.TotalTokens += r.tokenUsage.TotalTokens
		}
	}
	return total
}

// IsOverLimit 检查指定任务是否超过成本限制
func (ct *CostTracker) IsOverLimit(taskID string, limit float64) bool {
	return ct.GetTaskCost(taskID) >= limit
}

// Reset 重置所有成本记录
func (ct *CostTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.records = make(map[string][]costRecord)
	ct.totalCost = 0
}

// Summary 返回成本摘要信息
func (ct *CostTracker) Summary() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	usage := TokenUsage{}
	for _, records := range ct.records {
		for _, r := range records {
			usage.InputTokens += r.tokenUsage.InputTokens
			usage.OutputTokens += r.tokenUsage.OutputTokens
			usage.TotalTokens += r.tokenUsage.TotalTokens
		}
	}

	return fmt.Sprintf("总成本: $%.6f, 总 token: %d (输入: %d, 输出: %d)",
		ct.totalCost, usage.TotalTokens, usage.InputTokens, usage.OutputTokens)
}