package builtin

import (
	"fmt"
	"regexp"
)

// 默认黑名单模式列表
var defaultBlacklistPatterns = []*regexp.Regexp{
	// 1. 删根 (rm -rf / 或 rm -rf /*)
	regexp.MustCompile(`rm\s+-rf\s+/\*?$`),
	// 2. 格式化文件系统
	regexp.MustCompile(`mkfs`),
	// 3. 设备读取操作
	regexp.MustCompile(`dd\s+if=/dev/`),
	// 4. 权限清空
	regexp.MustCompile(`chmod.*000\s+.*/`),
	// 5. fork 炸弹
	regexp.MustCompile(`:\(\)\{.*:\|:.*\};:`),
	// 6. 管道到 shell
	regexp.MustCompile(`\|\s*(sh|bash|zsh)\s*$`),
	// 7. 远程下载执行
	regexp.MustCompile(`(wget|curl)\s+.*\|\s*(sh|bash)`),
	// 8. 直接写入磁盘
	regexp.MustCompile(`>/dev/(sd[a-z]|nvme|hd[a-z])`),
	// 9. 创建交换分区
	regexp.MustCompile(`mkswap\s+/dev/`),
	// 10. 系统关机
	regexp.MustCompile(`^(halt|poweroff|reboot|shutdown)\s*$`),
	// 11. 设备写入操作
	regexp.MustCompile(`dd\s+of=/dev/`),
	// 12. 重定向到磁盘设备
	regexp.MustCompile(`>\s*/dev/sd[a-z]`),
}

// CheckBlacklist 检查命令是否匹配黑名单模式。
// cmd: 要检查的命令字符串
// extraPatterns: 用户自定义的额外正则模式（从 config.blocked_commands 读取）
// 返回值: matched（是否命中）, matchedPattern（命中的模式描述），err（正则编译错误）
func CheckBlacklist(cmd string, extraPatterns []string) (matched bool, matchedPattern string, err error) {
	// 检查内置黑名单
	for _, re := range defaultBlacklistPatterns {
		if re.MatchString(cmd) {
			return true, re.String(), nil
		}
	}

	// 检查用户自定义黑名单
	for _, pattern := range extraPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, "", fmt.Errorf("编译自定义黑名单模式 %q 失败: %w", pattern, err)
		}
		if re.MatchString(cmd) {
			return true, pattern, nil
		}
	}

	return false, "", nil
}