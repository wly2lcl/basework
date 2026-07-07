package session

import (
	"sync"
	"testing"
)

func TestQueue_EnqueueDequeue(t *testing.T) {
	q := NewQueue(100)

	// 入队
	msg1 := Message{ID: "1", Content: "消息1"}
	msg2 := Message{ID: "2", Content: "消息2"}

	if ok := q.Enqueue(msg1); !ok {
		t.Fatal("Enqueue 应成功")
	}
	if ok := q.Enqueue(msg2); !ok {
		t.Fatal("Enqueue 应成功")
	}

	if q.Len() != 2 {
		t.Fatalf("Len = %d, 期望 2", q.Len())
	}

	// FIFO 出队
	got1 := q.Dequeue()
	if got1.ID != "1" || got1.Content != "消息1" {
		t.Errorf("Dequeue 得到 %+v, 期望 {ID:1 Content:消息1}", got1)
	}

	got2 := q.Dequeue()
	if got2.ID != "2" || got2.Content != "消息2" {
		t.Errorf("Dequeue 得到 %+v, 期望 {ID:2 Content:消息2}", got2)
	}

	if !q.IsEmpty() {
		t.Fatal("队列应为空")
	}
}

func TestQueue_EmptyDequeue(t *testing.T) {
	q := NewQueue(10)
	msg := q.Dequeue()
	if msg.ID != "" || msg.Content != "" {
		t.Errorf("空队列 Dequeue 应返回空 Message，得到 %+v", msg)
	}
}

func TestQueue_Cancel(t *testing.T) {
	q := NewQueue(10)
	q.Enqueue(Message{ID: "1", Content: "第一"})
	q.Enqueue(Message{ID: "2", Content: "第二"})
	q.Enqueue(Message{ID: "3", Content: "第三"})

	// 取消中间的消息
	if ok := q.Cancel("2"); !ok {
		t.Fatal("Cancel 应返回 true")
	}

	if q.Len() != 2 {
		t.Fatalf("取消后期望 2 条消息，得到 %d", q.Len())
	}

	// 验证顺序：1, 3
	msg1 := q.Dequeue()
	if msg1.ID != "1" {
		t.Errorf("第一个消息应为 1，得到 %s", msg1.ID)
	}
	msg2 := q.Dequeue()
	if msg2.ID != "3" {
		t.Errorf("第二个消息应为 3，得到 %s", msg2.ID)
	}
}

func TestQueue_CancelNotFound(t *testing.T) {
	q := NewQueue(10)
	q.Enqueue(Message{ID: "1", Content: "a"})
	if ok := q.Cancel("nonexistent"); ok {
		t.Fatal("取消不存在的消息应返回 false")
	}
}

func TestQueue_IsEmpty(t *testing.T) {
	q := NewQueue(10)
	if !q.IsEmpty() {
		t.Fatal("新队列应为空")
	}

	q.Enqueue(Message{ID: "1", Content: "a"})
	if q.IsEmpty() {
		t.Fatal("入队后队列不应为空")
	}

	q.Dequeue()
	if !q.IsEmpty() {
		t.Fatal("出队后队列应为空")
	}
}

func TestQueue_Len(t *testing.T) {
	q := NewQueue(10)
	if q.Len() != 0 {
		t.Fatalf("新队列 Len = %d, 期望 0", q.Len())
	}

	q.Enqueue(Message{ID: "1", Content: "a"})
	q.Enqueue(Message{ID: "2", Content: "b"})
	if q.Len() != 2 {
		t.Fatalf("Len = %d, 期望 2", q.Len())
	}
}

func TestQueue_MaxSize(t *testing.T) {
	q := NewQueue(2)

	if ok := q.Enqueue(Message{ID: "1", Content: "a"}); !ok {
		t.Fatal("第一个入队应成功")
	}
	if ok := q.Enqueue(Message{ID: "2", Content: "b"}); !ok {
		t.Fatal("第二个入队应成功")
	}
	// 超出上限
	if ok := q.Enqueue(Message{ID: "3", Content: "c"}); ok {
		t.Fatal("超出上限的入队应返回 false")
	}
}

func TestQueue_ConcurrentSafety(t *testing.T) {
	q := NewQueue(0) // 无限制
	var wg sync.WaitGroup
	n := 100

	// 并发入队
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.Enqueue(Message{ID: string(rune('0' + i)), Content: "msg"})
		}(i)
	}
	wg.Wait()

	if q.Len() != n {
		t.Fatalf("并发入队后期望 %d 条，得到 %d", n, q.Len())
	}

	// 并发出队
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Dequeue()
		}()
	}
	wg.Wait()

	if !q.IsEmpty() {
		t.Fatalf("全部出队后队列应为空，Len = %d", q.Len())
	}
}

func TestQueue_FIFOOrder(t *testing.T) {
	q := NewQueue(10)

	// 按顺序入队
	for i := 0; i < 5; i++ {
		q.Enqueue(Message{ID: string(rune('A' + i)), Content: "msg"})
	}

	// 验证 FIFO 顺序
	expected := []string{"A", "B", "C", "D", "E"}
	for _, exp := range expected {
		msg := q.Dequeue()
		if msg.ID != exp {
			t.Errorf("期望 %s, 得到 %s", exp, msg.ID)
		}
	}
}
