package observability

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
)

// Profiler 性能分析器
type Profiler struct {
	mu      sync.Mutex
	server  *http.Server
	running bool
	host    string
	port    int
}

// ProfilerConfig 分析器配置
type ProfilerConfig struct {
	Host string
	Port int
}

// DefaultProfilerConfig 返回默认配置
func DefaultProfilerConfig() ProfilerConfig {
	return ProfilerConfig{
		Host: "127.0.0.1",
		Port: 6060,
	}
}

// NewProfiler 创建性能分析器
func NewProfiler(cfg ProfilerConfig) *Profiler {
	return &Profiler{
		host: cfg.Host,
		port: cfg.Port,
	}
}

// Start 启动 pprof HTTP 服务器
func (p *Profiler) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("pprof 服务器已在 %s 上运行", p.Addr())
	}

	mux := http.NewServeMux()
	// 注册所有 pprof 端点
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))

	addr := p.Addr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// 绑定失败必须同步返回，不能让调用方误以为服务器已启动
		return fmt.Errorf("pprof 服务器监听 %s 失败: %w", addr, err)
	}

	// 端口为 0 时回填内核分配的实际端口，Addr() 之后返回真实地址
	if p.port == 0 {
		if tcpLn, ok := ln.(*net.TCPListener); ok {
			p.port = tcpLn.Addr().(*net.TCPAddr).Port
		}
	}

	p.server = &http.Server{
		Handler: mux,
	}

	// 启动 HTTP 服务器，捕获 server 引用避免闭包中的竞态
	srv := p.server
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Printf("pprof 服务器错误: %v\n", err)
		}
	}()

	p.running = true
	return nil
}

// Stop 停止 pprof HTTP 服务器
func (p *Profiler) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return fmt.Errorf("pprof 服务器未运行")
	}

	if p.server == nil {
		p.running = false
		return nil
	}

	// 优雅关闭
	if err := p.server.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("关闭 pprof 服务器失败: %w", err)
	}

	p.running = false
	p.server = nil
	return nil
}

// IsRunning 返回服务器是否运行中
func (p *Profiler) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// Addr 返回服务器监听地址。配置端口为 0 时，Start 之后返回内核分配的实际端口；
// Start 之前返回配置原样（host:0）。
func (p *Profiler) Addr() string {
	return fmt.Sprintf("%s:%d", p.host, p.port)
}
