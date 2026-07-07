package agent

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ContextProvider 接口定义了上下文信息提供者。
// 每个 Provider 负责收集某一类环境信息，返回纯文本描述。
type ContextProvider interface {
	// Name 返回提供者名称，用于调试和日志。
	Name() string
	// Collect 收集并返回上下文信息的文本表示。
	Collect() (string, error)
}

// ContextCollector 并行收集所有注册的 ContextProvider 的信息。
// 每个 Provider 独立运行，超时或失败时跳过，确保不阻塞主流程。
type ContextCollector struct {
	providers []ContextProvider
	timeout   time.Duration
}

// NewContextCollector 创建 ContextCollector。
// 默认超时 5 秒。
func NewContextCollector(providers ...ContextProvider) *ContextCollector {
	if len(providers) == 0 {
		providers = defaultProviders()
	}
	return &ContextCollector{
		providers: providers,
		timeout:   5 * time.Second,
	}
}

// WithTimeout 设置收集超时时间。
func (c *ContextCollector) WithTimeout(timeout time.Duration) *ContextCollector {
	c.timeout = timeout
	return c
}

// Collect 并行收集所有 Provider 的上下文信息。
// 每个 Provider 在独立的 goroutine 中运行，超时或出错时跳过。
// 返回按 Provider 名称排序拼接的文本。
func (c *ContextCollector) Collect(ctx context.Context) string {
	if len(c.providers) == 0 {
		return ""
	}

	type result struct {
		name string
		text string
	}

	results := make(chan result, len(c.providers))
	var wg sync.WaitGroup

	collectCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	for _, p := range c.providers {
		wg.Add(1)
		p := p // capture
		go func() {
			defer wg.Done()

			done := make(chan struct{})
			var text string
			go func() {
				t, err := p.Collect()
				if err == nil && t != "" {
					text = t
				}
				close(done)
			}()

			select {
			case <-done:
				results <- result{name: p.Name(), text: text}
			case <-collectCtx.Done():
				// 超时跳过 — 启动 cleanup goroutine 等待内层 goroutine 完成后释放
				results <- result{name: p.Name(), text: ""}
				go func() { <-done }()
			}
		}()
	}

	wg.Wait()
	close(results)

	var parts []string
	for r := range results {
		if r.text != "" {
			parts = append(parts, r.text)
		}
	}

	return strings.Join(parts, "\n")
}

// defaultProviders 返回默认的内置 ContextProvider 列表。
func defaultProviders() []ContextProvider {
	return []ContextProvider{
		&workdirProvider{},
		&gitProvider{},
		&platformProvider{},
		&projectProvider{},
	}
}