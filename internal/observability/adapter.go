package observability

import (
	"time"

	"github.com/wly2lcl/basework/pkg/agent"
)

// EventBusAdapter 将 *EventBus 适配为 agent.EventPublisher 接口。
type EventBusAdapter struct {
	inner *EventBus
}

// NewEventBusAdapter 创建适配器。
func NewEventBusAdapter(eb *EventBus) *EventBusAdapter {
	return &EventBusAdapter{inner: eb}
}

// PublishEvent 实现 agent.EventPublisher。
func (a *EventBusAdapter) PublishEvent(eventType string, data map[string]interface{}) {
	a.inner.Publish(Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
}

// Ensure EventBusAdapter implements agent.EventPublisher.
var _ agent.EventPublisher = (*EventBusAdapter)(nil)