package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/observability"
)

var (
	profileDuration int
)

// profileCmd 表示 profile 子命令
var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "性能分析（pprof）管理",
	Long: `管理和收集性能分析数据。

支持启动/停止 pprof 服务器，以及生成 CPU、内存和 goroutine 分析数据。`,
	Run: func(cmd *cobra.Command, args []string) {
		showProfileStatus()
	},
}

// profileStartCmd 表示 profile start 子命令
var profileStartCmd = &cobra.Command{
	Use:   "start",
	Short: "启动 pprof 服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		return startProfiler()
	},
}

// profileStopCmd 表示 profile stop 子命令
var profileStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止 pprof 服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopProfiler()
	},
}

// profileCPUCmd 表示 profile cpu 子命令
var profileCPUCmd = &cobra.Command{
	Use:   "cpu",
	Short: "生成 CPU profile 文件",
	Long:  `通过 pprof 服务器获取 CPU profile 数据并保存为文件。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateCPUProfile()
	},
}

// profileMemoryCmd 表示 profile memory 子命令
var profileMemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "生成堆内存快照文件",
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateMemoryProfile()
	},
}

// profileGoroutineCmd 表示 profile goroutine 子命令
var profileGoroutineCmd = &cobra.Command{
	Use:   "goroutine",
	Short: "输出 goroutine 堆栈",
	RunE: func(cmd *cobra.Command, args []string) error {
		return dumpGoroutines()
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileStartCmd)
	profileCmd.AddCommand(profileStopCmd)
	profileCmd.AddCommand(profileCPUCmd)
	profileCmd.AddCommand(profileMemoryCmd)
	profileCmd.AddCommand(profileGoroutineCmd)

	profileCPUCmd.Flags().IntVar(&profileDuration, "duration", 30, "CPU profile 采样持续时间（秒）")
}

// getProfileAddr 获取 pprof 服务器地址
func getProfileAddr() string {
	host := "127.0.0.1"
	port := 6060
	return host + ":" + strconv.Itoa(port)
}

// showProfileStatus 显示 pprof 状态
func showProfileStatus() {
	addr := getProfileAddr()
	url := fmt.Sprintf("http://%s/debug/pprof/", addr)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("pprof 服务器状态: 未运行 (%s)\n", addr)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("pprof 服务器状态: 运行中 (%s)\n", addr)
	fmt.Println()
	fmt.Println("可用端点:")
	fmt.Println("  /debug/pprof/          — 概览")
	fmt.Println("  /debug/pprof/heap      — 堆内存快照")
	fmt.Println("  /debug/pprof/goroutine — goroutine 列表")
	fmt.Println("  /debug/pprof/profile   — CPU profile (30s)")
	fmt.Println("  /debug/pprof/trace     — 执行 trace")
	fmt.Println("  /debug/pprof/block     — 阻塞分析")
	fmt.Println("  /debug/pprof/mutex     — 互斥锁竞争")
	fmt.Println("  /debug/pprof/allocs    — 内存分配")
}

// startProfiler 启动 pprof 服务器
func startProfiler() error {
	cfg := observability.ProfilerConfig{
		Host: "127.0.0.1",
		Port: 6060,
	}
	p := observability.NewProfiler(cfg)

	if err := p.Start(); err != nil {
		return fmt.Errorf("启动 pprof 服务器失败: %w", err)
	}

	fmt.Printf("pprof 服务器已启动: http://%s/debug/pprof/\n", p.Addr())
	return nil
}

// stopProfiler 停止 pprof 服务器
func stopProfiler() error {
	cfg := observability.ProfilerConfig{
		Host: "127.0.0.1",
		Port: 6060,
	}
	p := observability.NewProfiler(cfg)

	// 尝试连接服务器以确认是否在运行
	addr := getProfileAddr()
	url := fmt.Sprintf("http://%s/debug/pprof/", addr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("pprof 服务器未在 %s 上运行", addr)
	}
	resp.Body.Close()

	if err := p.Stop(); err != nil {
		// 如果 Profiler 无法停止（因为是由外部启动的），尝试直接发请求无法停止
		// 所以这里需要更优雅的处理，但 Profiler 实例是新的，没有 server
		return fmt.Errorf("停止 pprof 服务器失败: %w", err)
	}

	fmt.Println("pprof 服务器已停止")
	return nil
}

// generateCPUProfile 生成 CPU profile
func generateCPUProfile() error {
	addr := getProfileAddr()
	url := fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", addr, profileDuration)

	fmt.Printf("正在收集 CPU profile（%d 秒）...\n", profileDuration)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("获取 CPU profile 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取 CPU profile 失败: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应数据失败: %w", err)
	}

	filename := fmt.Sprintf("cpu_%s.prof", time.Now().Format("20060102_150405"))
	if err := os.WriteFile(filename, body, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	fmt.Printf("CPU profile 已保存: %s (%d 字节)\n", filename, len(body))
	return nil
}

// generateMemoryProfile 生成堆内存快照
func generateMemoryProfile() error {
	addr := getProfileAddr()
	url := fmt.Sprintf("http://%s/debug/pprof/heap", addr)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("获取堆内存快照失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取堆内存快照失败: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应数据失败: %w", err)
	}

	filename := fmt.Sprintf("heap_%s.prof", time.Now().Format("20060102_150405"))
	if err := os.WriteFile(filename, body, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	fmt.Printf("堆内存快照已保存: %s (%d 字节)\n", filename, len(body))
	return nil
}

// dumpGoroutines 输出 goroutine 堆栈
func dumpGoroutines() error {
	addr := getProfileAddr()
	url := fmt.Sprintf("http://%s/debug/pprof/goroutine?debug=1", addr)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("获取 goroutine 信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取 goroutine 信息失败: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应数据失败: %w", err)
	}

	fmt.Println(string(body))
	return nil
}