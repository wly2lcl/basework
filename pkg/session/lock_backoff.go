package session

import (
	"math/rand/v2"
	"time"
)

// lockBackoff 控制非阻塞文件锁的轮询节奏。固定 100ms 睡眠会让刚释放锁的
// 进程在等待者醒来前再次抢到锁，多进程高争用下等待者会一直饿到超时；
// 从 1ms 起步、指数递增到 50ms 并加随机抖动，让等待者能在持锁者的写入
// 间隙参与竞争。
type lockBackoff struct {
	delay time.Duration
}

func (b *lockBackoff) wait() {
	if b.delay == 0 {
		b.delay = time.Millisecond
	}
	half := int64(b.delay / 2)
	time.Sleep(time.Duration(half + rand.Int64N(half+1)))
	b.delay *= 2
	if b.delay > 50*time.Millisecond {
		b.delay = 50 * time.Millisecond
	}
}
