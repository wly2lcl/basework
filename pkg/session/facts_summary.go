package session

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 本文件实现受预算约束的工作区事实摘要（CTX-002）。
//
// 关键边界：
//   - **不扫描仓库**：摘要只遍历事实集合（去重后有限），不做任何目录
//     遍历——「无上限扫描」在这里结构性不可能。
//   - **哈希由调用方注入**：本包不读文件系统。读取与受保护路径的豁免
//     判定都由调用方的 hash 函数负责（它知道哪些路径不该碰）。
//   - **过期检测**：对文件事实记录上次见到的内容哈希；本次哈希不同
//     则该事实标记为「已过期」（文件在记录之后又被改过），不再当作
//     可靠上下文。哈希基线存在内存里，随进程生命周期。

// 摘要预算的默认值（字节）。给模型的项目摘要不宜超过这个量级。
const DefaultFactsSummaryBudget = 8 * 1024

// MaxHashPerBuild 是单次摘要最多计算哈希的文件数上限。
// 事实集合本身有界，但给每个文件读盘求哈希仍需封顶，避免一次摘要
// 触发成百上千次 IO。
const MaxHashPerBuild = 64

// FactsSummaryOptions 是摘要的配置。
type FactsSummaryOptions struct {
	// Budget 是摘要文本的字节预算；<=0 用 DefaultFactsSummaryBudget。
	Budget int
	// Hash 返回工作区相对路径的内容哈希。返回 ("", nil) 表示无法/不应
	// 读取（如受保护路径）——该事实不带过期标记，标注「未读取」。
	// 返回 err 则摘要跳过过期检测并记录原因。
	Hash func(relPath string) (string, error)
	// Now 可注入时钟；为空用 time.Now。
	Now func() time.Time
	// Stale 是跨构建共享的哈希基线表。nil 时每次构建新建（意味着
	// 过期检测只在本次构建内有效）——要检测「上次摘要之后文件变了」，
	// 调用方必须持有并复用同一个 tracker。
	Stale *StaleTracker
}

// StaleTracker 记录文件事实的内容哈希基线，检测「记录之后又被改过」。
type StaleTracker struct {
	mu     sync.Mutex
	hashes map[string]string
}

// NewStaleTracker 创建空的基线表。
func NewStaleTracker() *StaleTracker {
	return &StaleTracker{hashes: make(map[string]string)}
}

// Classify 返回路径的当前状态：
//   - unchanged：哈希与基线一致（或基线刚建立，视为新鲜）；
//   - stale：哈希与基线不同——事实已过期；
//   - unknown：无法读取（调用方返回空哈希），不做过期判定。
func (t *StaleTracker) Classify(relPath, currentHash string) string {
	if currentHash == "" {
		return "unknown"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, seen := t.hashes[relPath]
	t.hashes[relPath] = currentHash
	if seen && prev != currentHash {
		return "stale"
	}
	return "unchanged"
}

// FactsSummary 是摘要结果：文本 + 结构化统计，供调用方审计与展示。
type FactsSummary struct {
	Text       string
	Total      int // 事实总数
	Included   int // 摘要包含的条数
	Stale      int // 标记为过期的事实数
	Unreadable int // 因受保护/不可读而未做哈希的事实数
	Truncated  bool
}

// SummarizeFacts 生成预算内的项目事实摘要。
//
// 选择顺序（相关性优先）：file_modified > file_reverted > command >
// file_read；同类别内按 FirstSeen 新者在前。每行都带来源与时间——
// 「来源可追踪」是硬要求，没有来源的事实行不允许出现。
func SummarizeFacts(w *WorkspaceFacts, opts FactsSummaryOptions) *FactsSummary {
	budget := opts.Budget
	if budget <= 0 {
		budget = DefaultFactsSummaryBudget
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	facts := w.List()
	result := &FactsSummary{Total: len(facts)}
	if len(facts) == 0 {
		result.Text = ""
		return result
	}

	// 相关性权重：小者先选。
	weight := map[string]int{
		FactFileModified: 0,
		FactFileReverted: 1,
		FactCommand:      2,
		FactFileRead:     3,
	}
	sorted := append([]Fact(nil), facts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		wi, wj := weight[sorted[i].Kind], weight[sorted[j].Kind]
		if wi != wj {
			return wi < wj
		}
		return sorted[i].FirstSeen > sorted[j].FirstSeen
	})

	tracker := opts.Stale
	if tracker == nil {
		tracker = NewStaleTracker()
	}
	hashed := 0
	var b strings.Builder
	header := "工作区事实摘要（由会话编辑与后台任务记录折叠，供任务参考）：\n"
	if len(header) <= budget {
		b.WriteString(header)
	}

	for _, f := range sorted {
		if result.Truncated || b.Len() >= budget {
			break
		}
		line := ""
		switch {
		case f.Kind == FactFileModified || f.Kind == FactFileRead || f.Kind == FactFileReverted:
			state := "unchanged"
			if opts.Hash != nil && hashed < MaxHashPerBuild {
				hashed++
				h, err := opts.Hash(f.Path)
				switch {
				case err != nil:
					// 读不了（含受保护路径由调用方以错误表达）——不判定过期。
					state = "unreadable"
				default:
					state = tracker.Classify(f.Path, h)
				}
			} else {
				state = "unreadable"
			}
			result.Unreadable += boolToInt(state == "unreadable")
			tag := ""
			switch state {
			case "stale":
				tag = " [已过期:文件随后又变化，仅作历史参考]"
				result.Stale++
			case "unreadable":
				tag = " [未读取]"
			}
			line = fmt.Sprintf("- [%s] %s%s（来源 %s，首次 %s）\n",
				f.Kind, f.Path, tag, f.Source, f.FirstSeen)
		case f.Kind == FactCommand:
			line = fmt.Sprintf("- [command] 任务 %s（来源 %s，首次 %s）\n",
				f.Ref, f.Source, f.FirstSeen)
		default:
			continue // 未知类别不进摘要
		}

		// 预算裁剪：放不下这一行就截断（不是截半个词，是丢整行）。
		if b.Len()+len(line) > budget {
			result.Truncated = true
			break
		}
		b.WriteString(line)
		result.Included++
	}

	if result.Included < result.Total {
		result.Truncated = true
	}
	if result.Truncated {
		footer := fmt.Sprintf("…（共 %d 条事实，已按预算 %d 字节截断，截断时间 %s）\n",
			result.Total, budget, now().UTC().Format(time.RFC3339))
		if b.Len()+len(footer) <= budget {
			b.WriteString(footer)
		}
	}
	result.Text = b.String()
	return result
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
