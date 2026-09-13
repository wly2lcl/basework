package observability

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestProfiler_DefaultConfig(t *testing.T) {
	cfg := DefaultProfilerConfig()
	if cfg.Host != "127.0.0.1" {
		t.Errorf("默认 Host = %q, 期望 %q", cfg.Host, "127.0.0.1")
	}
	if cfg.Port != 6060 {
		t.Errorf("默认 Port = %d, 期望 %d", cfg.Port, 6060)
	}
}

func TestProfiler_NewProfiler(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 6061})
	if p == nil {
		t.Fatal("NewProfiler 不应返回 nil")
	}
	if p.Addr() != "127.0.0.1:6061" {
		t.Errorf("Addr = %q, 期望 %q", p.Addr(), "127.0.0.1:6061")
	}
	if p.IsRunning() {
		t.Error("新建 Profiler 不应处于运行状态")
	}
}

func TestProfiler_StartAndStop(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	// 启动
	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	if !p.IsRunning() {
		t.Error("Start 后 IsRunning 应为 true")
	}

	if p.Addr() == "127.0.0.1:0" {
		t.Error("端口为 0 时 Start 后 Addr 应回填内核分配的实际端口")
	}
}

// 端口为 0 时内核随机分配，多个 Profiler 可共存（并行测试/多实例不互踩）
func TestProfiler_PortZeroConcurrentInstances(t *testing.T) {
	ps := make([]*Profiler, 4)
	addrs := make(map[string]bool)
	for i := range ps {
		ps[i] = NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})
		if err := ps[i].Start(); err != nil {
			t.Fatalf("第 %d 个实例 Start 失败: %v", i, err)
		}
		if addrs[ps[i].Addr()] {
			t.Errorf("第 %d 个实例地址 %s 与之前实例重复", i, ps[i].Addr())
		}
		addrs[ps[i].Addr()] = true
	}
	for i := range ps {
		if err := ps[i].Stop(); err != nil {
			t.Errorf("第 %d 个实例 Stop 失败: %v", i, err)
		}
	}
}

// 端口被占用时 Start 必须同步报错，不能静默谎报成功（服务器实际没起来）
func TestProfiler_StartPortConflict(t *testing.T) {
	p1 := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})
	if err := p1.Start(); err != nil {
		t.Fatalf("第一个实例 Start 失败: %v", err)
	}
	defer func() {
		if err := p1.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	p2 := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: p1.port})
	if err := p2.Start(); err == nil {
		t.Error("端口被占用时 Start 应返回错误")
		_ = p2.Stop()
	}
}

func TestProfiler_StartDuplicate(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	// 重复启动应返回错误
	err := p.Start()
	if err == nil {
		t.Error("重复启动应返回错误")
	}
}

func TestProfiler_StopWithoutStart(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	// 未启动时 Stop 应返回错误
	err := p.Stop()
	if err == nil {
		t.Error("未启动时 Stop 应返回错误")
	}
}

func TestProfiler_Endpoints(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	// Listen 在 Start 内同步完成，连接会进入内核 backlog，无需等待

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/debug/pprof/", http.StatusOK},
		{"/debug/pprof/cmdline", http.StatusOK},
		{"/debug/pprof/symbol", http.StatusOK},
		{"/debug/pprof/heap", http.StatusOK},
		{"/debug/pprof/goroutine", http.StatusOK},
		{"/debug/pprof/block", http.StatusOK},
		{"/debug/pprof/mutex", http.StatusOK},
		{"/debug/pprof/allocs", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			url := fmt.Sprintf("http://%s%s", p.Addr(), tt.path)
			resp, err := http.Get(url)
			if err != nil {
				t.Fatalf("GET %s 失败: %v", url, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("GET %s 状态码 = %d, 期望 %d", url, resp.StatusCode, tt.wantStatus)
			}

			// 确保有响应体
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("读取响应体失败: %v", err)
			}
			if len(body) == 0 {
				t.Errorf("GET %s 响应体为空", url)
			}
		})
	}
}

func TestProfiler_ProfileEndpoint(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	// CPU profile 使用短持续时间（1秒）
	url := fmt.Sprintf("http://%s/debug/pprof/profile?seconds=1", p.Addr())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s 失败: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	if len(body) == 0 {
		t.Error("响应体为空")
	}
}

func TestProfiler_TraceEndpoint(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	// Trace 使用短持续时间
	url := fmt.Sprintf("http://%s/debug/pprof/trace?seconds=1", p.Addr())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s 失败: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	if len(body) == 0 {
		t.Error("响应体为空")
	}
}

func TestProfiler_Concurrency(t *testing.T) {
	p := NewProfiler(ProfilerConfig{Host: "127.0.0.1", Port: 0})

	if err := p.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Errorf("Stop 失败: %v", err)
		}
	}()

	// 并发访问多个端点
	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			url := fmt.Sprintf("http://%s/debug/pprof/heap", p.Addr())
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
			}
			done <- struct{}{}
		}()
	}

	timeout := time.After(5 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-done:
			// 正常完成
		case <-timeout:
			t.Fatal("并发请求超时")
		}
	}
}
