package observability

import (
	"fmt"
	"os"
)

// Subscriber 是可观测事件订阅者接口。
// 实现此接口的类型可以订阅事件总线并处理事件。
type Subscriber interface {
	// Handle 处理一个事件。返回 error 表示处理失败。
	Handle(event Event) error
	// EventTypes 返回该订阅者感兴趣的事件类型列表
	EventTypes() []string
}

// LogSubscriber 是日志订阅者，将事件写入日志记录器
type LogSubscriber struct {
	logger *Logger
}

// NewLogSubscriber 创建日志订阅者
func NewLogSubscriber(logger *Logger) *LogSubscriber {
	return &LogSubscriber{logger: logger}
}

// Handle 将事件写入日志
func (ls *LogSubscriber) Handle(event Event) error {
	fields := map[string]interface{}{
		"event_type": event.Type,
	}
	for k, v := range event.Data {
		fields[k] = v
	}
	ls.logger.Info("事件："+event.Type, fields)
	return nil
}

// EventTypes 返回日志订阅者关心的事件类型（所有类型）
func (ls *LogSubscriber) EventTypes() []string {
	return []string{
		EventAgentStart,
		EventAgentEnd,
		EventAgentError,
		EventToolStart,
		EventToolEnd,
		EventToolError,
		EventLLMCallStart,
		EventLLMCallEnd,
	}
}

// MetricsSubscriber 是指标订阅者，更新成本和 token 追踪
type MetricsSubscriber struct {
	costTracker  *CostTracker
	tokenTracker *TokenTracker
}

// NewMetricsSubscriber 创建指标订阅者
func NewMetricsSubscriber(costTracker *CostTracker, tokenTracker *TokenTracker) *MetricsSubscriber {
	return &MetricsSubscriber{
		costTracker:  costTracker,
		tokenTracker: tokenTracker,
	}
}

// Handle 处理事件，更新成本和 token 追踪
func (ms *MetricsSubscriber) Handle(event Event) error {
	switch event.Type {
	case EventLLMCallEnd:
		// 从事件数据中提取模型、token 使用量和成本
		model, _ := event.Data["model"].(string)
		inputTokens, _ := event.Data["input_tokens"].(int)
		outputTokens, _ := event.Data["output_tokens"].(int)
		cost, _ := event.Data["cost"].(float64)

		if model != "" {
			ms.tokenTracker.RecordUsage(model, inputTokens, outputTokens)
			if cost > 0 {
				ms.costTracker.RecordCall(model, inputTokens, outputTokens, cost)
			} else {
				// 如果没有显式成本，根据 model 定价计算
				calculatedCost := ms.costTracker.GetCallCost(model, inputTokens, outputTokens)
				ms.costTracker.RecordCall(model, inputTokens, outputTokens, calculatedCost)
			}
		}
	}
	return nil
}

// EventTypes 返回指标订阅者关心的事件类型
func (ms *MetricsSubscriber) EventTypes() []string {
	return []string{
		EventLLMCallEnd,
	}
}

// defaultLogger 创建默认的日志记录器（stdout, info 级别）
func defaultLogger() *Logger {
	return NewLogger(LogLevelInfo, os.Stdout)
}

// SetupSubscribers 根据配置初始化并返回订阅者列表。
// 返回的订阅者已经与事件总线关联。
func SetupSubscribers(config Config, bus *EventBus, logger *Logger, costTracker *CostTracker, tokenTracker *TokenTracker) {
	if !config.Enabled {
		return
	}

	// 配置日志级别
	level := ParseLogLevel(config.LogLevel)
	logger.SetLevel(level)

	// 配置日志输出
	if config.LogOutput == "stderr" {
		logger.SetOutput(os.Stderr)
	} else if config.LogOutput != "" && config.LogOutput != "stdout" {
		f, err := os.OpenFile(config.LogOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Warn("无法打开日志文件，使用默认输出", map[string]interface{}{
				"path":  config.LogOutput,
				"error": err.Error(),
			})
		} else {
			logger.SetOutput(f)
		}
	}

	// 创建并注册日志订阅者
	if logger != nil {
		logSub := NewLogSubscriber(logger)
		for _, et := range logSub.EventTypes() {
			eventType := et
			bus.Subscribe(eventType, func(event Event) {
				if err := logSub.Handle(event); err != nil {
					fmt.Fprintf(os.Stderr, "日志订阅者处理事件失败: %v\n", err)
				}
			})
		}
	}

	// 创建并注册指标订阅者
	if costTracker != nil && tokenTracker != nil {
		metricsSub := NewMetricsSubscriber(costTracker, tokenTracker)
		for _, et := range metricsSub.EventTypes() {
			eventType := et
			bus.Subscribe(eventType, func(event Event) {
				if err := metricsSub.Handle(event); err != nil {
					fmt.Fprintf(os.Stderr, "指标订阅者处理事件失败: %v\n", err)
				}
			})
		}
	}
}
