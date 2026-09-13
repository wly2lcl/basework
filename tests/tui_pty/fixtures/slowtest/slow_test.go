package slowtest

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestSlowCheck 模拟一次耗时检查：写自己的 PID 供「取消后子进程退出」判定，
// 然后长时间阻塞。
//
// 写 PID 的路径由 SHIP003_PIDFILE 指定；未设置时只阻塞不写。
func TestSlowCheck(t *testing.T) {
	if p := os.Getenv("SHIP003_PIDFILE"); p != "" {
		if err := os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
			t.Logf("写 PID 文件失败: %v", err)
		}
	}
	time.Sleep(120 * time.Second)
}
