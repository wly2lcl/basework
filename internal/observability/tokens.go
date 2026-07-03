package observability

import "sync"

// TokenTracker 追踪 LLM 调用的 token 使用量，线程安全
type TokenTracker struct {
	mu    sync.Mutex
	stats []tokenRecord
}

// tokenRecord 单次 token 使用记录
type tokenRecord struct {
	model        string
	inputTokens  int
	outputTokens int
}

// NewTokenTracker 创建 token 追踪器
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{}
}

// RecordUsage 记录一次 LLM 调用的 token 使用量
func (tt *TokenTracker) RecordUsage(model string, inputTokens, outputTokens int) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	tt.stats = append(tt.stats, tokenRecord{
		model:        model,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
	})
}

// GetTotalUsage 获取所有模型的累计 token 使用量。
// 返回 (input, output, total)。
func (tt *TokenTracker) GetTotalUsage() (input, output, total int) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	for _, r := range tt.stats {
		input += r.inputTokens
		output += r.outputTokens
	}
	return input, output, input + output
}

// GetUsageByModel 获取指定模型的累计 token 使用量。
// 返回 (input, output, total)。
func (tt *TokenTracker) GetUsageByModel(model string) (input, output, total int) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	for _, r := range tt.stats {
		if r.model == model {
			input += r.inputTokens
			output += r.outputTokens
		}
	}
	return input, output, input + output
}

// Reset 重置所有 token 使用记录
func (tt *TokenTracker) Reset() {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	tt.stats = nil
}