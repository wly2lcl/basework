package config

import (
	"errors"
	"fmt"
	"strings"
)

// 本文件实现启动预设（CFG-002）。覆盖次序与数组替换规则见
// docs/adr/0006-startup-presets.md，这里只做规则授权的实现：
//
//	内置默认 → 配置文件（preset 字段加载时展开） → 命令行 --preset
//
// 数组字段（tools.allowed）一律整体替换，永不并集；未指定预设时不新增
// 任何覆盖步骤，行为与没有本文件之前完全一致。

// 预设名称。展开逻辑见 ApplyPreset。
const (
	// PresetReadonly 把工具表裁剪为只读集合，并强制开启权限检查。
	// 运行入口会对最终工具表做白名单子集校验，任何写/执行能力泄漏即启动失败。
	PresetReadonly = "readonly"
	// PresetCoding 表示完整编码能力：不裁剪工具，不改权限。
	// 它存在是为了让调用点可以显式声明意图，而不是靠「没写 readonly」暗示。
	PresetCoding = "coding"
)

// 预设相关的可判定错误。调用方按 errors.Is 判定，不匹配错误文本。
var (
	// ErrUnknownPreset 表示预设名不存在。
	ErrUnknownPreset = errors.New("config: 未知预设")
	// ErrPresetConflict 表示预设与配置文件的双重指定相互冲突。
	ErrPresetConflict = errors.New("config: 预设与配置冲突")
)

// readonlyToolset 是只读预设的完整工具白名单。
//
// 与 internal/subagent 的只读子代理工具集同口径：read/grep/glob + LSP 查询。
// 刻意不含 question/web_fetch 等「看似无害」的工具——只读承诺的边界宁窄勿宽，
// 放宽需要在 ADR 0006 里明说，而不是顺手加一个名字。
var readonlyToolset = []string{
	"read",
	"grep",
	"glob",
	"lsp_definition",
	"lsp_diagnostics",
}

// ReadonlyToolset 返回只读预设的完整工具白名单（拷贝）。
// 供运行入口做「只读不泄漏」的最终校验，与 ApplyPreset 的展开结果同源。
func ReadonlyToolset() []string {
	out := make([]string, len(readonlyToolset))
	copy(out, readonlyToolset)
	return out
}

// IsKnownPreset 报告预设名是否存在。运行入口在冲突判定前先校验名字，
// 让「写错了预设名」报未知而不是报冲突。
func IsKnownPreset(name string) bool {
	for _, known := range KnownPresets() {
		if name == known {
			return true
		}
	}
	return false
}

// KnownPresets 返回全部预设名（稳定排序）。
func KnownPresets() []string {
	return []string{PresetCoding, PresetReadonly}
}

// ApplyPreset 把名为 name 的预设展开到 cfg 上。
//
// name 为空表示未指定预设：直接返回 nil，cfg 不被改动——「未指定时旧行为」
// 由这里保证，而不是靠调用方记得跳过。
//
// 冲突判定（ADR 0006）：
//   - readonly 定义了 tools.allowed，若 cfg 已显式给出该列表，双重指定报错；
//   - 未知预设名报错。
//   - 其余字段预设直接覆盖（预设就是「一揽子覆盖」的语义，不算冲突）。
//
// 数组替换规则：readonly 的工具白名单整体替换 cfg.Tools.Allowed，永不并集。
func ApplyPreset(cfg *Config, name string) error {
	switch name {
	case "":
		return nil
	case PresetCoding:
		// 完整能力：不裁剪、不改权限。显式存在即意图。
		return nil
	case PresetReadonly:
		// 幂等：如果白名单已经是只读集合（比如 Load 展开过一次，命令行
		// 又指定了同值预设），再次展开不算「显式指定」冲突。不同值才是
		// 双重指定——那意味着两处各说各话，必须二选一。
		if !sameStrings(cfg.Tools.Allowed, readonlyToolset) && len(cfg.Tools.Allowed) > 0 {
			return fmt.Errorf("%w: 配置文件已显式给出 tools.allowed，与 readonly 预设的定义冲突；二选一（ADR 0006）", ErrPresetConflict)
		}
		allowed := make([]string, len(readonlyToolset))
		copy(allowed, readonlyToolset)
		cfg.Tools.Allowed = allowed
		// 只读承诺需要权限层兜底：强制开启权限检查。
		cfg.Permission.Enabled = true
		if cfg.Permission.Mode == "" {
			cfg.Permission.Mode = "default"
		}
		return nil
	default:
		return fmt.Errorf("%w: %q（可用：%v）", ErrUnknownPreset, name, KnownPresets())
	}
}

// sameStrings 判断两个字符串切片是否逐元素相同。
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ToolAllowed 判断工具名是否通过 allowlist。allowlist 为空表示不限制。
// 条目以 `*` 结尾时做前缀匹配（如 `lsp_*`），其余做精确匹配。
// 裸 `*` 不是通配（前缀为空串会匹配一切，等于「写 * 想放行全部」——
// 那是空白名单的语义，误写应当显式失败而不是悄悄全放）。
func ToolAllowed(allowlist []string, toolName string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, entry := range allowlist {
		if entry == toolName {
			return true
		}
		// 前缀通配：`lsp_*` 匹配 lsp_definition、lsp_diagnostics 等。
		if len(entry) > 1 && entry[len(entry)-1] == '*' &&
			strings.HasPrefix(toolName, entry[:len(entry)-1]) {
			return true
		}
	}
	return false
}
