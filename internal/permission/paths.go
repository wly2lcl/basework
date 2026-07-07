package permission

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// DefaultSensitivePaths 默认敏感路径黑名单。
var DefaultSensitivePaths = []string{
	".git/",
	".svn/",
	".hg/",
	"/.ssh/",
	"/.aws/",
	"/.gnupg/",
	"/.config/gcloud/",
	"/etc/shadow",
	"/etc/sudoers",
}

// ProtectionLevel 保护级别。
type ProtectionLevel string

const (
	// ProtectionStrict 严格模式：拒绝访问黑名单路径。
	ProtectionStrict ProtectionLevel = "strict"
	// ProtectionWarn 警告模式：允许访问但记录审计日志。
	ProtectionWarn ProtectionLevel = "warn"
	// ProtectionOff 关闭模式：不检查路径。
	ProtectionOff ProtectionLevel = "off"
)

// ParseProtectionLevel 解析保护级别字符串。
func ParseProtectionLevel(s string) (ProtectionLevel, error) {
	switch s {
	case "strict":
		return ProtectionStrict, nil
	case "warn":
		return ProtectionWarn, nil
	case "off":
		return ProtectionOff, nil
	default:
		return "", fmt.Errorf("unknown protection level: %q (valid: strict, warn, off)", s)
	}
}

// PathChecker 路径安全检查器。
type PathChecker struct {
	level   ProtectionLevel
	blocked []string     // 黑名单（默认 + 自定义）
	allowed []string     // 白名单
	auditor *AuditLogger // 可选，warn 模式下记录
}

// NewPathChecker 创建路径检查器。
// blocked: 额外的黑名单路径（与默认黑名单合并）
// allowed: 白名单路径（覆盖黑名单）
// level: 保护级别
func NewPathChecker(blocked, allowed []string, level ProtectionLevel) *PathChecker {
	// 合并默认黑名单和自定义黑名单
	merged := make([]string, len(DefaultSensitivePaths)+len(blocked))
	copy(merged, DefaultSensitivePaths)
	copy(merged[len(DefaultSensitivePaths):], blocked)

	return &PathChecker{
		level:   level,
		blocked: merged,
		allowed: allowed,
	}
}

// WithAudit 返回一个配置了审计日志记录器的 PathChecker（不变更原对象）。
func (pc *PathChecker) WithAudit(auditor *AuditLogger) *PathChecker {
	return &PathChecker{
		level:   pc.level,
		blocked: pc.blocked,
		allowed: pc.allowed,
		auditor: auditor,
	}
}

// CheckPath 检查路径是否被允许访问。
// 返回：
//   - allowed: 是否允许访问
//   - reason: 如果不允许，原因说明
func (pc *PathChecker) CheckPath(path string) (bool, string) {
	if path == "" {
		return true, ""
	}

	// off 模式：不检查
	if pc.level == ProtectionOff {
		return true, ""
	}

	// 规范化路径
	normalized, err := NormalizePath(path)
	if err != nil {
		// 规范化失败时使用原始路径继续检查
		normalized = path
	}

	// 白名单优先
	for _, allow := range pc.allowed {
		if pathMatch(allow, normalized) || pathMatch(allow, path) {
			return true, ""
		}
	}

	// 检查黑名单
	if pc.IsSensitive(normalized) || pc.IsSensitive(path) {
		switch pc.level {
		case ProtectionStrict:
			return false, fmt.Sprintf("路径 %q 在黑名单中", path)
		case ProtectionWarn:
			if pc.auditor != nil {
				pc.auditor.Record(AuditRecord{
					ToolName: "path_checker",
					Decision: "allowed",
					Context:  FormatAuditContext(map[string]interface{}{"path": path, "reason": "敏感路径（warn 模式）"}),
				})
			}
			return true, ""
		default:
			return true, ""
		}
	}

	return true, ""
}

// IsSensitive 检查路径是否在黑名单中。
func (pc *PathChecker) IsSensitive(path string) bool {
	if path == "" {
		return false
	}

	normalized := normalizeForPolicyMatch(path)

	// 检查黑名单
	for _, pattern := range pc.blocked {
		if pathMatch(pattern, normalized) {
			return true
		}
	}
	return false
}

// NormalizePath 规范化路径。
// - 相对路径 → 绝对路径（基于当前工作目录）
// - 展开 ~ 为用户主目录
// - 清理路径（去除 ..、. 等）
func NormalizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}

	if strings.HasPrefix(path, "/") {
		return pathpkg.Clean(path), nil
	}

	// 展开 ~
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home dir: %w", err)
		}
		if len(path) == 1 {
			path = home
		} else {
			path = filepath.Join(home, path[1:])
		}
	}

	// 相对路径转绝对路径
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve absolute path: %w", err)
		}
		path = abs
	}

	// 清理路径
	path = filepath.Clean(path)

	return path, nil
}

func normalizeForPolicyMatch(path string) string {
	path = filepath.ToSlash(path)
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

// pathMatch 检查路径是否匹配模式。
// 支持：精确匹配、子串匹配（目录模式如 ".git/"、"//.ssh/"）、glob 模式匹配。
func pathMatch(pattern, path string) bool {
	pattern = normalizeForPolicyMatch(pattern)
	path = normalizeForPolicyMatch(path)

	// 精确匹配
	if pattern == path {
		return true
	}

	// 目录模式匹配（模式以 / 或 \\ 结尾，如 ".git/"、"/.ssh/"）
	if strings.HasSuffix(pattern, "/") || strings.HasSuffix(pattern, "\\") {
		trimmed := strings.TrimRight(pattern, "/\\")

		// 构造用于子串搜索的组件模式
		// 模式 "/.ssh/" → 在路径中搜索 "/.ssh/"
		// 模式 ".git/" → 在路径中搜索 "/.git/"
		var searchStr string
		sep := "/"
		if strings.HasPrefix(trimmed, sep) {
			// 模式以分隔符开头（如 "/.ssh"），直接拼接
			searchStr = trimmed + sep
		} else {
			// 模式不以分隔符开头（如 ".git"），添加分隔符
			searchStr = sep + trimmed + sep
		}

		// 检查路径中是否包含该组件
		if strings.Contains(path, searchStr) {
			return true
		}

		// 检查路径是否以该组件开头（相对路径，如 ".git/config"）
		if strings.HasPrefix(path, trimmed+sep) || path == trimmed {
			return true
		}

		// 检查路径是否以该组件结尾（绝对路径，如 "/etc/sudoers.d" 需特殊处理）
		if strings.HasSuffix(path, sep+trimmed) {
			return true
		}
	}

	// glob 模式匹配（整个路径）
	if matched, err := filepath.Match(pattern, path); err == nil && matched {
		return true
	}

	// 也尝试匹配路径末尾部分（如 "shadow" 匹配 "/etc/shadow"）
	base := path
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if matched, err := filepath.Match(pattern, base); err == nil && matched {
		return true
	}

	return false
}
