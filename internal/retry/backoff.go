package retry

import (
	"math"
	"math/rand"
	"time"
)

// Backoff 计算第 attempt 次重试的退避时间。
//
// 公式：min(base * 2^attempt, max) + jitter
//   - base: 基础延迟（默认 2s）
//   - max: 最大延迟（默认 60s）
//   - jitter: 0~1s 的随机抖动，防止惊群效应
//
// attempt 从 0 开始计数，即首次重试使用 attempt=0。
func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// 计算指数退避：base * 2^attempt
	expDelay := float64(base) * math.Pow(2, float64(attempt))

	// 限制最大值
	if expDelay > float64(max) {
		expDelay = float64(max)
	}

	// 添加随机抖动（0~1 秒）
	jitter := time.Duration(rand.Int63n(int64(time.Second)))

	return time.Duration(expDelay) + jitter
}
