package agent

import (
	"context"
	"sync"
	"testing"
)

func TestSteering_InjectSteer(t *testing.T) {
	sm := newSteeringManager()

	err := sm.InjectMessage(context.Background(), "steer消息", Steer)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	msgs := sm.drainSteering()
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息, 得到 %d", len(msgs))
	}
	if msgs[0] != "steer消息" {
		t.Errorf("期望消息内容='steer消息', 得到 '%s'", msgs[0])
	}
}

func TestSteering_InjectQueue(t *testing.T) {
	sm := newSteeringManager()

	err := sm.InjectMessage(context.Background(), "queue消息", Queue)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	msgs := sm.drainSteering()
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息, 得到 %d", len(msgs))
	}
	if msgs[0] != "queue消息" {
		t.Errorf("期望消息内容='queue消息', 得到 '%s'", msgs[0])
	}
}

func TestSteering_MultipleMessages(t *testing.T) {
	sm := newSteeringManager()

	_ = sm.InjectMessage(context.Background(), "msg1", Steer)
	_ = sm.InjectMessage(context.Background(), "msg2", Queue)
	_ = sm.InjectMessage(context.Background(), "msg3", Steer)

	msgs := sm.drainSteering()
	if len(msgs) != 3 {
		t.Fatalf("期望 3 条消息, 得到 %d", len(msgs))
	}
}

func TestSteering_EmptyDrain(t *testing.T) {
	sm := newSteeringManager()

	msgs := sm.drainSteering()
	if len(msgs) != 0 {
		t.Errorf("未注入消息时 drainSteering 应返回空切片, 得到 %d", len(msgs))
	}
}

func TestSteering_CancelSequence(t *testing.T) {
	sm := newSteeringManager()

	// 注入第一条消息
	err := sm.InjectMessage(context.Background(), "msg1", Steer)
	if err != nil {
		t.Fatalf("InjectMessage 返回错误: %v", err)
	}

	// cancel 当前 seq
	sm.mu.Lock()
	seq := sm.seqGen
	sm.mu.Unlock()
	sm.Cancel(seq)

	// drain 应该没有消息（被 cancel 了）
	msgs := sm.drainSteering()
	if len(msgs) != 0 {
		t.Errorf("cancel 后 drainSteering 应返回空, 得到 %d 条消息", len(msgs))
	}
}

func TestSteering_ConcurrentInjection(t *testing.T) {
	sm := newSteeringManager()
	var wg sync.WaitGroup

	// 并发注入 20 条消息
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content := string(rune('A' + i))
			mode := Steer
			if i%2 == 0 {
				mode = Queue
			}
			_ = sm.InjectMessage(context.Background(), content, mode)
		}(i)
	}
	wg.Wait()

	msgs := sm.drainSteering()
	// 只测试不会 panic/data race，不验证具体消息数量
	_ = msgs
}

func TestSteering_InjectSteerThenDrainIsolation(t *testing.T) {
	sm := newSteeringManager()

	// Steer 模式应该立即在 drain 中可见
	_ = sm.InjectMessage(context.Background(), "steer1", Steer)
	_ = sm.InjectMessage(context.Background(), "queue1", Queue)

	msgs := sm.drainSteering()
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息, 得到 %d", len(msgs))
	}
}