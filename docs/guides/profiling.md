# 性能分析指南

本文档介绍 basework 的性能分析功能，包括 pprof 集成、CLI 命令和基准测试套件。

---

## pprof 概述

pprof 是 Go 语言内置的性能分析工具，可以实时采集运行时数据。basework 通过 HTTP 端点暴露 pprof 接口，支持 CPU、内存、goroutine 等多种分析类型。

### 启用方式

通过配置文件启用：

```json
{
  "profiling": {
    "enabled": true,
    "host": "127.0.0.1",
    "port": 6060
  }
}
```

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `enabled` | `false` | 是否启用 pprof |
| `host` | `127.0.0.1` | 监听地址，建议仅本地 |
| `port` | `6060` | HTTP 监听端口 |

也可通过环境变量启用：

```bash
export BASEWORK_PROFILING_ENABLED=true
export BASEWORK_PROFILING_PORT=6060
```

---

## pprof HTTP 端点

启用后可通过浏览器或工具访问以下端点：

| 端点 | 说明 |
|------|------|
| `/debug/pprof/` | pprof 首页 |
| `/debug/pprof/profile?seconds=30` | CPU 采样（默认 30 秒） |
| `/debug/pprof/heap` | 堆内存分配 |
| `/debug/pprof/goroutine` | goroutine 堆栈 |
| `/debug/pprof/goroutine?debug=2` | goroutine 详细信息 |
| `/debug/pprof/threadcreate` | 线程创建记录 |
| `/debug/pprof/block` | 阻塞事件 |
| `/debug/pprof/mutex` | 互斥锁竞争 |

示例：

```bash
# 查看 pprof 首页
open http://127.0.0.1:6060/debug/pprof/

# 采集 30 秒 CPU profile
curl -o cpu.pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30

# 获取当前堆内存快照
curl -o heap.pprof http://127.0.0.1:6060/debug/pprof/heap
```

---

## CLI 命令

basework 提供 `profile` 子命令，直接生成性能分析文件。

### CPU Profile

```bash
# 采集 30 秒 CPU profile（默认）
basework profile cpu

# 指定采集时长
basework profile cpu --seconds 60

# 指定输出文件
basework profile cpu --output /tmp/cpu.pprof
```

### Memory Profile

```bash
# 采集堆内存快照
basework profile memory

# 采集所有内存类型（含栈内存）
basework profile memory --all
```

### Goroutine Profile

```bash
# 采集 goroutine 堆栈
basework profile goroutine

# 检测 goroutine 泄漏
basework profile goroutine --leak-detect
```

### 综合报告

```bash
# 生成综合性能报告（包含 CPU、内存、goroutine）
basework profile report --output /tmp/report/
```

---

## 性能基准测试

basework 内置 benchmark 套件，共 77 项基准测试，覆盖核心路径。

### 运行基准测试

```bash
# 运行所有基准测试
go test -bench=. ./tests/benchmark/

# 运行特定基准测试
go test -bench=BenchmarkTokenCount ./tests/benchmark/

# 运行并生成内存分配报告
go test -bench=. -benchmem ./tests/benchmark/
```

### 基准测试分类

| 类别 | 测试项 | 说明 |
|------|--------|------|
| Token 计数 | `BenchmarkTokenCount` | token 估算性能 |
| 流式响应 | `BenchmarkStreamResponse` | 流式数据处理延迟 |
| 工具执行 | `BenchmarkToolExecution` | 内置工具执行性能 |
| 会话读写 | `BenchmarkSessionReadWrite` | 会话持久化性能 |
| Web Fetch | `BenchmarkWebFetch` | HTTP 请求性能 |
| 消息序列化 | `BenchmarkMessageSerialize` | 消息 JSON 序列化 |

### 基准测试示例

```bash
# 基准测试结果示例
$ go test -bench=BenchmarkTokenCount -benchtime=5s ./tests/benchmark/
BenchmarkTokenCount-8   	   50000	     28943 ns/op	    1234 B/op	      12 allocs/op
```

---

## 分析工具使用

### go tool pprof 交互模式

```bash
# 启动交互式分析
go tool pprof cpu.pprof

# 常用交互命令
(pprof) top          # 查看 top 热点
(pprof) top10        # 查看前 10
(pprof) list         # 查看源码标注
(pprof) web          # 生成 SVG 火焰图
(pprof) pdf          # 生成 PDF 报告
(pprof) quit         # 退出
```

### Web 界面

```bash
# 启动 web 界面
go tool pprof -http=:8080 cpu.pprof
```

在浏览器中可查看：
- 火焰图（Flame Graph）
- 调用图（Call Graph）
- 峰值内存（Peak Memory）
- 源代码标注（Source View）

### 对比分析

```bash
# 对比两次 profile 的差异
go tool pprof -base baseline.pprof current.pprof

# 对比模式中，正值表示增加，负值表示减少
```

---

## 性能优化建议

### CPU 密集场景

- 使用 CPU profile 定位热点函数
- 关注 `top` 命令中占比超过 10% 的函数
- 检查是否频繁触发 GC（GC 时间 > 5% 需关注）

### 内存密集场景

- 使用 heap profile 定位大对象分配
- 关注 `alloc_space` 模式而非 `inuse_space`
- 检查是否存在内存泄漏（持续增长的 inuse）

### Goroutine 泄漏

- 定期采集 goroutine profile
- 对比空闲和繁忙时的 goroutine 数量
- 关注 `runtime.gopark` 状态的 goroutine

---

## 常见问题

### 为什么无法访问 pprof 页面？

检查以下可能原因：
1. `profiling.enabled` 是否设置为 `true`
2. 端口是否被其他程序占用
3. 是否配置了 `host: 127.0.0.1`（外部机器无法访问）
4. 防火墙是否阻止了该端口

### 性能分析对 Agent 有影响吗？

pprof CPU profiling 约带来 5-10% 的性能开销（采样期间）。
建议仅在需要排查性能问题时启用，生产环境默认关闭。

### benchmark 测试需要多久？

77 项基准测试全部运行约需 3-5 分钟（取决于机器性能）。
可使用 `-benchtime` 参数减少单次测试时间。

### 如何比较不同版本的性能？

```bash
# 在 v0.3.0 版本采集基线
git checkout v0.3.0
go test -bench=. -benchmem ./tests/benchmark/ > baseline.txt

# 在当前版本运行对比
go test -bench=. -benchmem ./tests/benchmark/ > current.txt

# 使用 benchstat 分析
go install golang.org/x/perf/cmd/benchstat@latest
benchstat baseline.txt current.txt
```

---

## 相关文档

- [配置参考](configuration.md) — 性能分析配置项
- [安全配置指南](security.md) — 安全相关配置
- [CLI 使用指南](cli-guide.md) — 完整 CLI 命令参考