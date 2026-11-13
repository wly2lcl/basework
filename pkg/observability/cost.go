package observability

import (
	"math"
	"sync"
)

// modelPricing 常见模型的定价信息（单位：美元/百万 token）
var modelPricing = map[string]struct {
	inputPrice  float64 // 输入价格（每百万 token）
	outputPrice float64 // 输出价格（每百万 token）
}{
	"gpt-4o":            {inputPrice: 2.50, outputPrice: 10.00},
	"gpt-4o-mini":       {inputPrice: 0.15, outputPrice: 0.60},
	"gpt-4-turbo":       {inputPrice: 10.00, outputPrice: 30.00},
	"gpt-4":             {inputPrice: 30.00, outputPrice: 60.00},
	"gpt-3.5-turbo":     {inputPrice: 0.50, outputPrice: 1.50},
	"claude-3-5-sonnet": {inputPrice: 3.00, outputPrice: 15.00},
	"claude-3-opus":     {inputPrice: 15.00, outputPrice: 75.00},
	"claude-3-haiku":    {inputPrice: 0.25, outputPrice: 1.25},
	"claude-3-sonnet":   {inputPrice: 3.00, outputPrice: 15.00},
	"deepseek-v2":       {inputPrice: 0.14, outputPrice: 0.28},
	"deepseek-v3":       {inputPrice: 0.27, outputPrice: 1.10},
	"gemini-1.5-pro":    {inputPrice: 3.50, outputPrice: 10.50},
	"gemini-1.5-flash":  {inputPrice: 0.075, outputPrice: 0.30},
	"mistral-large":     {inputPrice: 2.00, outputPrice: 6.00},
	"llama-3-70b":       {inputPrice: 0.65, outputPrice: 2.75},
}

// CostTracker 追踪 LLM 调用的成本，线程安全
type CostTracker struct {
	mu    sync.Mutex
	calls []costRecord
}

// costRecord 单次调用成本记录
type costRecord struct {
	model        string
	inputTokens  int
	outputTokens int
	cost         float64
}

// NewCostTracker 创建成本追踪器
func NewCostTracker() *CostTracker {
	return &CostTracker{}
}

// RecordCall 记录一次 LLM 调用的成本。
// 如果 model 不在已知定价列表中，使用默认定价（$1.0/百万 token）。
func (ct *CostTracker) RecordCall(model string, inputTokens, outputTokens int, cost float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.calls = append(ct.calls, costRecord{
		model:        model,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
		cost:         cost,
	})
}

// GetCallCost 计算单次调用成本，使用内置模型定价。
// 如果 model 不在已知定价列表中，使用默认定价。
func (ct *CostTracker) GetCallCost(model string, inputTokens, outputTokens int) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		// 默认定价：$1.0/百万 token
		pricing = struct {
			inputPrice  float64
			outputPrice float64
		}{inputPrice: 1.0, outputPrice: 1.0}
	}

	inputCost := float64(inputTokens) / 1_000_000 * pricing.inputPrice
	outputCost := float64(outputTokens) / 1_000_000 * pricing.outputPrice

	return math.Round((inputCost+outputCost)*10000) / 10000
}

// GetTotalCost 获取所有模型的总成本
func (ct *CostTracker) GetTotalCost() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	var total float64
	for _, r := range ct.calls {
		total += r.cost
	}
	return math.Round(total*10000) / 10000
}

// GetCostByModel 获取指定模型的累计成本
func (ct *CostTracker) GetCostByModel(model string) float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	var total float64
	for _, r := range ct.calls {
		if r.model == model {
			total += r.cost
		}
	}
	return math.Round(total*10000) / 10000
}

// Reset 重置所有成本记录
func (ct *CostTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.calls = nil
}
