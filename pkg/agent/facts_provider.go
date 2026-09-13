package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wly2lcl/basework/pkg/session"
)

// FactsSummaryProvider 把工作区事实摘要暴露为 ContextProvider（CTX-002）。
//
// 依赖注入边界：本类型只做「事实集合 → 文本」的适配；文件读取与其
// 保护判定都通过 Hash 函数由产品层注入——受保护路径在产品层就该被
// 拒绝，摘要链路不会绕过权限检查去读文件。
type FactsSummaryProvider struct {
	// FactsBase 是会话数据根目录（事实文件在其 <base>/facts/<id>.json）。
	FactsBase string
	// WorkspaceID 是当前工作区标识。
	WorkspaceID string
	// Budget 是摘要字节预算；<=0 用 pkg/session 的默认值。
	Budget int
	// Hash 返回工作区相对路径的内容哈希；受保护路径应返回
	// ErrProtectedPath 或 ("", nil)，摘要会标注「未读取」而不读它。
	Hash func(relPath string) (string, error)
}

// ErrProtectedPath 由调用方的 Hash 实现返回，表示路径受保护不可读。
// 摘要对它的处理是标注而非报错——受保护不是故障，是策略。
var ErrProtectedPath = errors.New("facts summary: 路径受保护，不读取")

// NewFactsSummaryProvider 构造事实摘要提供者。workspaceID 为空或
// 缺少依赖时返回 nil——调用方把 nil 当「不注册此 Provider」处理。
func NewFactsSummaryProvider(factsBase, workspaceID string, budget int, hash func(string) (string, error)) *FactsSummaryProvider {
	if factsBase == "" || workspaceID == "" {
		return nil
	}
	return &FactsSummaryProvider{
		FactsBase:   factsBase,
		WorkspaceID: workspaceID,
		Budget:      budget,
		Hash:        hash,
	}
}

// Name 实现 ContextProvider。
func (p *FactsSummaryProvider) Name() string { return "facts_summary" }

// Collect 实现 ContextProvider。任何失败都以空文本返回（与既有
// Provider 的容错口径一致）：摘要是增强，不是依赖。
func (p *FactsSummaryProvider) Collect() (string, error) {
	w, err := session.LoadWorkspaceFacts(p.FactsBase, p.WorkspaceID)
	if err != nil {
		return "", nil // 拒绝未来版本等错误 → 无摘要，不阻塞运行
	}
	if w.Count() == 0 {
		return "", nil
	}
	summary := session.SummarizeFacts(w, session.FactsSummaryOptions{
		Budget: p.Budget,
		Hash:   p.wrapHash,
	})
	if summary.Text == "" || summary.Included == 0 {
		return "", nil
	}
	return summary.Text, nil
}

// wrapHash 把相对路径转成工作区内绝对路径再求哈希；越界路径直接拒绝。
func (p *FactsSummaryProvider) wrapHash(relPath string) (string, error) {
	if p.Hash == nil {
		return "", nil
	}
	if filepath.IsAbs(relPath) || containsDotDot(relPath) {
		return "", ErrProtectedPath
	}
	return p.Hash(relPath)
}

func containsDotDot(p string) bool {
	for _, part := range splitPath(p) {
		if part == ".." {
			return true
		}
	}
	return false
}

func splitPath(p string) []string {
	var parts []string
	for len(p) > 0 {
		i := 0
		for i < len(p) && p[i] != '/' {
			i++
		}
		if i > 0 {
			parts = append(parts, p[:i])
		}
		if i == len(p) {
			break
		}
		p = p[i+1:]
	}
	return parts
}

// FileHash 是产品层常用的哈希实现：读取文件内容求 SHA-256。
// 调用方应先做权限/保护判定再传入（本函数不做任何策略判断）。
func FileHash(absPath string) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("facts summary: 读取失败: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
