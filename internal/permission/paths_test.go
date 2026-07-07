package permission

import (
	"path/filepath"
	"testing"
)

func TestDefaultSensitivePaths(t *testing.T) {
	// 默认黑名单包含预期路径
	expected := []string{
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
	if len(DefaultSensitivePaths) != len(expected) {
		t.Errorf("DefaultSensitivePaths 长度 = %d, 期望 %d", len(DefaultSensitivePaths), len(expected))
	}
	for i, p := range expected {
		if DefaultSensitivePaths[i] != p {
			t.Errorf("DefaultSensitivePaths[%d] = %q, 期望 %q", i, DefaultSensitivePaths[i], p)
		}
	}
}

func TestParseProtectionLevel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ProtectionLevel
		wantErr bool
	}{
		{"严格模式", "strict", ProtectionStrict, false},
		{"警告模式", "warn", ProtectionWarn, false},
		{"关闭模式", "off", ProtectionOff, false},
		{"未知模式", "unknown", "", true},
		{"空字符串", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProtectionLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProtectionLevel(%q) 错误 = %v, wantErr = %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseProtectionLevel(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathChecker_StrictMode(t *testing.T) {
	pc := NewPathChecker(nil, nil, ProtectionStrict)

	tests := []struct {
		name  string
		path  string
		allow bool
	}{
		{"git 目录", ".git/config", false},
		{"git 目录（绝对路径）", "/home/user/project/.git/config", false},
		{"svn 目录", ".svn/entries", false},
		{"hg 目录", ".hg/store", false},
		{"etc shadow", "/etc/shadow", false},
		{"etc sudoers", "/etc/sudoers", false},
		{"普通文件", "/tmp/test.txt", true},
		{"普通目录", "/home/user/docs", true},
		{"空路径", "", true},
		{"ssh 目录", "/home/user/.ssh/id_rsa", false},
		{"aws 目录", "/home/user/.aws/credentials", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := pc.CheckPath(tt.path)
			if allowed != tt.allow {
				t.Errorf("CheckPath(%q) = %v, 期望 %v, 原因: %s", tt.path, allowed, tt.allow, reason)
			}
		})
	}
}

func TestPathChecker_WarnMode(t *testing.T) {
	// warn 模式应允许访问敏感路径
	pc := NewPathChecker(nil, nil, ProtectionWarn)

	tests := []struct {
		name  string
		path  string
		allow bool
	}{
		{"敏感路径应允许", ".git/config", true},
		{"普通路径应允许", "/tmp/test.txt", true},
		{"etc shadow", "/etc/shadow", true},
		{"空路径", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := pc.CheckPath(tt.path)
			if allowed != tt.allow {
				t.Errorf("CheckPath(%q) = %v, 期望 %v", tt.path, allowed, tt.allow)
			}
		})
	}
}

func TestPathChecker_WarnMode_WithAudit(t *testing.T) {
	// warn 模式下有 auditor 时应记录日志
	auditor := NewAuditLogger()
	pc := NewPathChecker(nil, nil, ProtectionWarn).WithAudit(auditor)

	allowed, _ := pc.CheckPath("/etc/shadow")
	if !allowed {
		t.Error("warn 模式应允许访问")
	}
}

func TestPathChecker_OffMode(t *testing.T) {
	pc := NewPathChecker(nil, nil, ProtectionOff)

	// off 模式不检查，所有路径都允许
	allowed, reason := pc.CheckPath(".git/config")
	if !allowed {
		t.Errorf("off 模式应允许所有路径, 原因: %s", reason)
	}

	allowed, reason = pc.CheckPath("/etc/shadow")
	if !allowed {
		t.Errorf("off 模式应允许所有路径, 原因: %s", reason)
	}
}

func TestPathChecker_WhitelistOverride(t *testing.T) {
	// 白名单应覆盖黑名单
	allowed := []string{"/home/user/project/.git/index"}
	pc := NewPathChecker(nil, allowed, ProtectionStrict)

	// 白名单中的路径应允许
	allowed1, reason := pc.CheckPath("/home/user/project/.git/index")
	if !allowed1 {
		t.Errorf("白名单路径应被允许, 原因: %s", reason)
	}

	// 其他敏感路径仍应拒绝
	allowed2, _ := pc.CheckPath("/etc/shadow")
	if allowed2 {
		t.Error("非白名单的敏感路径应被拒绝")
	}
}

func TestPathChecker_CustomBlocked(t *testing.T) {
	// 自定义黑名单
	blocked := []string{"/my/custom/secret"}
	pc := NewPathChecker(blocked, nil, ProtectionStrict)

	allowed, reason := pc.CheckPath("/my/custom/secret")
	if allowed {
		t.Errorf("自定义黑名单路径应被拒绝, 原因: %s", reason)
	}

	// 默认黑名单仍生效
	allowed2, _ := pc.CheckPath(".git/config")
	if allowed2 {
		t.Error("默认黑名单路径仍应被拒绝")
	}
}

func TestNormalizePath_Absolute(t *testing.T) {
	path, err := NormalizePath("/etc/passwd")
	if err != nil {
		t.Fatalf("NormalizePath 不应返回错误: %v", err)
	}
	if path != "/etc/passwd" {
		t.Errorf("绝对路径不变, 得到 %q", path)
	}
}

func TestNormalizePath_Relative(t *testing.T) {
	path, err := NormalizePath("test.txt")
	if err != nil {
		t.Fatalf("NormalizePath 不应返回错误: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("相对路径应转为绝对路径, 得到 %q", path)
	}
	if filepath.Base(path) != "test.txt" {
		t.Errorf("文件名应保留, 得到 %q", filepath.Base(path))
	}
}

func TestNormalizePath_HomeExpansion(t *testing.T) {
	path, err := NormalizePath("~/test.txt")
	if err != nil {
		t.Fatalf("NormalizePath 不应返回错误: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("~ 展开后应为绝对路径, 得到 %q", path)
	}
}

func TestNormalizePath_Clean(t *testing.T) {
	path, err := NormalizePath("/a/b/../c/./d")
	if err != nil {
		t.Fatalf("NormalizePath 不应返回错误: %v", err)
	}
	if path != "/a/c/d" {
		t.Errorf("路径应被清理, 得到 %q", path)
	}
}

func TestNormalizePath_Empty(t *testing.T) {
	_, err := NormalizePath("")
	if err == nil {
		t.Error("空路径应返回错误")
	}
}

func TestPathChecker_ExactMatch(t *testing.T) {
	pc := NewPathChecker(nil, nil, ProtectionStrict)

	// 测试精确匹配（如 /etc/shadow）
	allowed, _ := pc.CheckPath("/etc/shadow")
	if allowed {
		t.Error("精确匹配 /etc/shadow 应被拒绝")
	}
}

func TestPathChecker_PrefixMatch(t *testing.T) {
	pc := NewPathChecker(nil, nil, ProtectionStrict)

	// 测试前缀匹配（目录模式）
	tests := []struct {
		path  string
		allow bool
	}{
		{".git/config", false},
		{"/home/user/project/.git/HEAD", false},
		{".gitignore", true}, // .gitignore 不是 .git/ 目录下的文件
		{"/tmp/mygitfile", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			allowed, _ := pc.CheckPath(tt.path)
			if allowed != tt.allow {
				t.Errorf("CheckPath(%q) = %v, 期望 %v", tt.path, allowed, tt.allow)
			}
		})
	}
}

func TestPathChecker_WithAudit(t *testing.T) {
	auditor := NewAuditLogger()
	pc := NewPathChecker(nil, nil, ProtectionStrict).WithAudit(auditor)

	// strict 模式拒绝，不记录
	allowed, _ := pc.CheckPath(".git/config")
	if allowed {
		t.Error("strict 模式应拒绝敏感路径")
	}
}

func TestIsSensitive(t *testing.T) {
	pc := NewPathChecker(nil, nil, ProtectionStrict)

	tests := []struct {
		path    string
		sensive bool
	}{
		{".git/config", true},
		{"/etc/shadow", true},
		{"/tmp/test.txt", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := pc.IsSensitive(tt.path)
			if got != tt.sensive {
				t.Errorf("IsSensitive(%q) = %v, 期望 %v", tt.path, got, tt.sensive)
			}
		})
	}
}

func TestNewPathChecker_CustomBlockedEmpty(t *testing.T) {
	// 自定义黑名单为空时，使用默认黑名单
	pc := NewPathChecker(nil, nil, ProtectionStrict)
	if len(pc.blocked) != len(DefaultSensitivePaths) {
		t.Errorf("blocked 长度 = %d, 期望 %d", len(pc.blocked), len(DefaultSensitivePaths))
	}
}

func TestNewPathChecker_CustomBlockedWithDefaults(t *testing.T) {
	// 自定义黑名单应追加到默认黑名单
	custom := []string{"/my/secret"}
	pc := NewPathChecker(custom, nil, ProtectionStrict)
	expectedLen := len(DefaultSensitivePaths) + len(custom)
	if len(pc.blocked) != expectedLen {
		t.Errorf("blocked 长度 = %d, 期望 %d", len(pc.blocked), expectedLen)
	}
}

func TestPathChecker_GlobPattern(t *testing.T) {
	// 使用 glob 模式作为自定义黑名单
	blocked := []string{"/tmp/secret-*"}
	pc := NewPathChecker(blocked, nil, ProtectionStrict)

	allowed, reason := pc.CheckPath("/tmp/secret-file.txt")
	if allowed {
		t.Errorf("glob 模式匹配应被拒绝, 原因: %s", reason)
	}

	allowed2, _ := pc.CheckPath("/tmp/other-file.txt")
	if !allowed2 {
		t.Error("不匹配 glob 模式的路径应允许")
	}
}

func TestPathChecker_WarnModeNoAudit(t *testing.T) {
	// warn 模式但无 auditor，不记录
	pc := NewPathChecker(nil, nil, ProtectionWarn)

	allowed, _ := pc.CheckPath("/etc/shadow")
	if !allowed {
		t.Error("warn 模式应允许访问")
	}
}
