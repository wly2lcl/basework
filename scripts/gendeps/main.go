// Command gendeps 生成 basework 的模块依赖图，写入 docs/DEPGRAPH.md。
//
// 存在的意义：本项目发生过「文档描述的结构」与「代码实际结构」脱节——手写数字
// 漂移约 45%、两份互相矛盾的 ROADMAP、docs/ARCHITECTURE.md 声称「pkg/ 不依赖任何
// 第三方库」而实际有 5 个。本工具把层级结构、依赖关系与第三方依赖边界变成机器
// 生成物，并由 CI 做「新鲜度门禁」：改了代码却不重跑本工具，CI 即失败。
//
// 三条硬性约束：
//  1. 输出必须确定性——所有列表按字典序排序，且**不得包含时间戳**。
//     否则 CI 的 git diff --exit-code 比对会因为顺序抖动或时间变化而永久失败。
//  2. 构建约束感知——`//go:build` 会决定某个依赖是否参与默认构建，
//     用统一的 `go build` 判断会误报（例如 modernc.org/sqlite 只在 sqlite tag 下引入）。
//  3. 只使用标准库实现，符合仓库「核心不引入第三方依赖」的约定。
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	modulePath = "github.com/wly2lcl/basework"
	outputPath = "docs/DEPGRAPH.md"
)

// scanRoots 是参与依赖图扫描的顶层目录，按层级从下到上排列。
var scanRoots = []string{"pkg", "internal", "cmd"}

// layerDesc 描述各层的定位，用于生成文档中的层级概览。
var layerDesc = map[string]string{
	"pkg":      "可嵌入核心（稳定 API，不得依赖上层）",
	"internal": "终端产品专用逻辑（无兼容性承诺）",
	"cmd":      "可执行入口",
}

// approvedPkgDeps 是 pkg/ 层允许引入的第三方依赖白名单。
// 新增依赖必须同时改这里与 docs/ARCHITECTURE.md，否则 tests 中的边界检查会失败。
// 前缀匹配：登记 `golang.org/x/image` 即同时放行其子包（draw、webp 等）。
var approvedPkgDeps = map[string]string{
	"github.com/golang/snappy": "pkg/session 会话压缩",
	"golang.org/x/image":       "pkg/llm 图像处理",
	"golang.org/x/oauth2":      "pkg/provider Copilot OAuth 设备授权",
	"modernc.org/sqlite":       "pkg/session 与 pkg/memory 的持久化",
}

// pkgInfo 是单个包的依赖信息。
type pkgInfo struct {
	Path    string   // 相对仓库根的路径，如 pkg/tool/builtin
	Layer   string   // pkg / internal / cmd
	Files   int      // 非测试 .go 文件数
	Imports []string // 去重后的模块内依赖（相对路径），已排序
	ExtDeps []string // 去重后的第三方依赖，已排序
	// depCtx 记录「第三方依赖 → 引入它的文件所用构建约束集合」。
	// 构建约束为空集表示该依赖在默认构建下即被引入。
	depCtx map[string]map[string]bool
}

// hasDep 判断该包是否引入了指定第三方依赖，支持子包前缀匹配。
func (p pkgInfo) hasDep(dep string) bool {
	for _, d := range p.ExtDeps {
		if d == dep || strings.HasPrefix(d, dep+"/") {
			return true
		}
	}
	return false
}

// violation 表示一条层级或依赖边界违规。
type violation struct {
	From string
	To   string
	Rule string
}

func main() {
	out := flag.String("out", outputPath, "输出文件路径，传空字符串则只打印到标准输出")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "定位仓库根目录失败: %v\n", err)
		os.Exit(1)
	}

	pkgs, err := collect(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描依赖失败: %v\n", err)
		os.Exit(1)
	}
	if len(pkgs) == 0 {
		fmt.Fprintln(os.Stderr, "未扫描到任何 Go 包，依赖图未生效")
		os.Exit(1)
	}

	report, violations := render(pkgs)
	fmt.Print(report)

	if *out != "" {
		target := filepath.Join(root, *out)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(target, []byte(report), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写入 %s 失败: %v\n", target, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "\n已写入 %s\n", *out)
	}

	// 存在边界违规时以非零退出：若只把 ❌ 写进文档，那么「把带违规的文档提交进仓库」
	// 就能让 CI 的新鲜度门禁通过，门禁形同虚设。
	if violations > 0 {
		fmt.Fprintf(os.Stderr, "发现 %d 处依赖边界违规，详见 %s\n", violations, *out)
		os.Exit(1)
	}
}

// repoRoot 向上查找包含 go.mod 的目录。
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("未找到 go.mod")
		}
		dir = parent
	}
}

// collect 遍历 scanRoots，解析每个目录的 import，得到包依赖集合。
func collect(root string) ([]pkgInfo, error) {
	byPath := map[string]*pkgInfo{}

	for _, layer := range scanRoots {
		layerRoot := filepath.Join(root, layer)
		if _, err := os.Stat(layerRoot); err != nil {
			continue
		}

		walkErr := filepath.WalkDir(layerRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// 依赖图只统计非测试文件：测试文件的 import 属于测试基建，
			// 会让「谁依赖谁」失真（例如测试里 import 别的包做集成验证）。
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			dir := filepath.Dir(rel)

			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
			if parseErr != nil {
				return fmt.Errorf("解析 %s 失败: %w", rel, parseErr)
			}

			info := byPath[dir]
			if info == nil {
				info = &pkgInfo{Path: dir, Layer: layer, depCtx: map[string]map[string]bool{}}
				byPath[dir] = info
			}
			info.Files++

			// 默认构建（无约束）用空字符串表示。
			ctxs := collectBuildConstraints(file)
			if len(ctxs) == 0 {
				ctxs = []string{""}
			}

			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				switch {
				case importPath == modulePath:
					info.Imports = append(info.Imports, "(root)")
				case strings.HasPrefix(importPath, modulePath+"/"):
					info.Imports = append(info.Imports, strings.TrimPrefix(importPath, modulePath+"/"))
				case isStdlib(importPath):
					// 标准库不计入依赖图。
				default:
					info.ExtDeps = append(info.ExtDeps, importPath)
					for _, c := range ctxs {
						if info.depCtx[importPath] == nil {
							info.depCtx[importPath] = map[string]bool{}
						}
						info.depCtx[importPath][c] = true
					}
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}

	result := make([]pkgInfo, 0, len(byPath))
	for _, info := range byPath {
		info.Imports = dedupSort(info.Imports)
		info.ExtDeps = dedupSort(info.ExtDeps)
		result = append(result, *info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// collectBuildConstraints 提取文件中的 `//go:build` 表达式（原样字符串）。
// 返回去重排序后的表达式列表；无约束文件返回空切片。
func collectBuildConstraints(file *ast.File) []string {
	set := map[string]bool{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(c.Text)
			if !strings.HasPrefix(text, "//go:build") {
				continue
			}
			expr, err := constraint.Parse(text)
			if err != nil {
				continue
			}
			set[expr.String()] = true
		}
	}
	var out []string
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// isStdlib 用「导入路径首段是否含点」判断是否标准库。
// 标准库路径首段不含 '.'（如 "net/http"），第三方域名必然含 '.'（如 "modernc.org/sqlite"）。
func isStdlib(importPath string) bool {
	first := importPath
	if idx := strings.Index(importPath, "/"); idx >= 0 {
		first = importPath[:idx]
	}
	return !strings.Contains(first, ".")
}

// isApprovedPkgDep 判断第三方依赖是否在 pkg/ 层白名单内（支持子包前缀匹配）。
func isApprovedPkgDep(dep string) bool {
	for approved := range approvedPkgDeps {
		if dep == approved || strings.HasPrefix(dep, approved+"/") {
			return true
		}
	}
	return false
}

// dedupSort 去重并排序。
func dedupSort(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// render 生成 Markdown 报告，并返回发现的边界违规数量。
func render(pkgs []pkgInfo) (string, int) {
	byPath := map[string]pkgInfo{}
	for _, p := range pkgs {
		byPath[p.Path] = p
	}

	// 只保留指向已知包的边，避免出现悬空节点。
	edges := map[string]map[string]bool{}
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if _, ok := byPath[imp]; !ok {
				continue
			}
			if edges[p.Path] == nil {
				edges[p.Path] = map[string]bool{}
			}
			edges[p.Path][imp] = true
		}
	}

	violations := detectViolations(pkgs, byPath)

	var b strings.Builder
	b.WriteString("# 模块依赖图（自动生成）\n\n")
	b.WriteString("> 本文件由 `make deps` 自动生成，请勿手工编辑。\n")
	b.WriteString("> 修改代码后重新运行 `make deps` 刷新即可；CI 会校验生成物是否与代码一致。\n\n")

	writeCaliber(&b)
	writeLayerOverview(&b, pkgs)
	writeLayerGraph(&b, edges, byPath)
	writePackageGraph(&b, pkgs, edges, byPath)
	writeEdgeList(&b, edges, byPath)
	writeFanInFanOut(&b, byPath, edges)
	writeViolations(&b, violations)
	writeThirdPartyDeps(&b, pkgs)

	return strings.TrimRight(b.String(), "\n") + "\n", len(violations)
}

// writeCaliber 输出统计口径说明。
func writeCaliber(b *strings.Builder) {
	b.WriteString("## 统计口径\n\n")
	b.WriteString("| 项 | 口径 |\n|---|---|\n")
	b.WriteString("| 扫描范围 | `pkg/`、`internal/`、`cmd/` 下的目录 |\n")
	b.WriteString("| 包判定 | 目录内含至少一个非测试 `.go` 文件 |\n")
	b.WriteString("| 依赖边 | 同一包内非测试 `.go` 文件的 import，去重 |\n")
	b.WriteString("| 标准库 | 导入路径首段不含 `.`（如 `net/http`），不计入依赖 |\n")
	b.WriteString("| 构建约束 | 从 `//go:build` 读取，用于判断第三方依赖是否参与默认构建 |\n")
	b.WriteString("| 排序 | 全部按字典序，保证输出确定性（CI 需要 `git diff --exit-code`） |\n")
	b.WriteString("| 时间戳 | 刻意不输出，否则新鲜度门禁将永久失败 |\n\n")
}

// writeLayerOverview 输出层级概览。
func writeLayerOverview(b *strings.Builder, pkgs []pkgInfo) {
	type agg struct{ pkgs, files int }
	byLayer := map[string]*agg{}
	for _, p := range pkgs {
		a := byLayer[p.Layer]
		if a == nil {
			a = &agg{}
			byLayer[p.Layer] = a
		}
		a.pkgs++
		a.files += p.Files
	}

	b.WriteString("## 层级概览\n\n")
	b.WriteString("| 层级 | 包数 | 非测试 .go 文件 | 定位 |\n|---|---|---|---|\n")
	for _, layer := range scanRoots {
		a := byLayer[layer]
		if a == nil {
			fmt.Fprintf(b, "| `%s/` | 0 | 0 | %s |\n", layer, layerDesc[layer])
			continue
		}
		fmt.Fprintf(b, "| `%s/` | %d | %d | %s |\n", layer, a.pkgs, a.files, layerDesc[layer])
	}
	b.WriteString("\n")
}

// writeLayerGraph 输出层级级依赖图（mermaid）。
func writeLayerGraph(b *strings.Builder, edges map[string]map[string]bool, byPath map[string]pkgInfo) {
	layerEdges := map[string]map[string]bool{}
	for from, tos := range edges {
		fromLayer := byPath[from].Layer
		for to := range tos {
			toLayer := byPath[to].Layer
			if fromLayer == toLayer {
				continue
			}
			if layerEdges[fromLayer] == nil {
				layerEdges[fromLayer] = map[string]bool{}
			}
			layerEdges[fromLayer][toLayer] = true
		}
	}

	b.WriteString("## 层级依赖图\n\n")
	b.WriteString("```mermaid\ngraph BT\n")
	for _, layer := range scanRoots {
		fmt.Fprintf(b, "  %s[\"%s/\"]\n", layer, layer)
	}
	for _, from := range scanRoots {
		var tos []string
		for to := range layerEdges[from] {
			tos = append(tos, to)
		}
		sort.Strings(tos)
		for _, to := range tos {
			fmt.Fprintf(b, "  %s --> %s\n", from, to)
		}
	}
	b.WriteString("```\n\n")
	b.WriteString("依赖方向为「上层 → 下层」。`pkg/` 不得指回 `internal/` 或 `cmd/`，\n")
	b.WriteString("该约束由 `tests/arch_test.go` 守护（见「层级违规检测」）。\n\n")
}

// writePackageGraph 输出包级依赖图（mermaid，按层分组）。
func writePackageGraph(b *strings.Builder, pkgs []pkgInfo, edges map[string]map[string]bool, byPath map[string]pkgInfo) {
	_ = byPath
	ids := map[string]string{}
	for i, p := range pkgs {
		ids[p.Path] = fmt.Sprintf("n%d", i)
	}

	b.WriteString("## 包级依赖图\n\n")
	b.WriteString("```mermaid\ngraph LR\n")
	for _, layer := range scanRoots {
		var members []pkgInfo
		for _, p := range pkgs {
			if p.Layer == layer {
				members = append(members, p)
			}
		}
		if len(members) == 0 {
			continue
		}
		fmt.Fprintf(b, "  subgraph %s[\"%s/\"]\n", layer, layer)
		b.WriteString("    direction TB\n")
		for _, p := range members {
			fmt.Fprintf(b, "    %s[\"%s\"]\n", ids[p.Path], p.Path)
		}
		b.WriteString("  end\n")
	}

	var edgeLines []string
	for from, tos := range edges {
		var sorted []string
		for to := range tos {
			sorted = append(sorted, to)
		}
		sort.Strings(sorted)
		for _, to := range sorted {
			edgeLines = append(edgeLines, fmt.Sprintf("  %s --> %s", ids[from], ids[to]))
		}
	}
	sort.Strings(edgeLines)
	for _, l := range edgeLines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")
}

// writeEdgeList 输出「谁依赖谁」的完整边清单。
func writeEdgeList(b *strings.Builder, edges map[string]map[string]bool, byPath map[string]pkgInfo) {
	b.WriteString("## 依赖边清单\n\n")
	b.WriteString("| 包 | 依赖 |\n|---|---|\n")

	var froms []string
	for from := range edges {
		froms = append(froms, from)
	}
	sort.Strings(froms)

	// 无出边的包也列出来，避免读者误以为遗漏。
	seen := map[string]bool{}
	for _, from := range froms {
		seen[from] = true
		var sorted []string
		for to := range edges[from] {
			sorted = append(sorted, to)
		}
		sort.Strings(sorted)
		fmt.Fprintf(b, "| `%s` | %s |\n", from, codeList(sorted))
	}
	var isolated []string
	for p := range byPath {
		if !seen[p] {
			isolated = append(isolated, p)
		}
	}
	sort.Strings(isolated)
	for _, p := range isolated {
		fmt.Fprintf(b, "| `%s` | （无模块内依赖） |\n", p)
	}
	b.WriteString("\n")
}

// rankLimit 限制排行表行数，避免大量计数为 1 的条目淹没信息。
const rankLimit = 12

// writeFanInFanOut 输出扇出/扇入排行。
func writeFanInFanOut(b *strings.Builder, byPath map[string]pkgInfo, edges map[string]map[string]bool) {
	_ = byPath
	fanOut := map[string]int{}
	fanIn := map[string]int{}
	for from, tos := range edges {
		fanOut[from] = len(tos)
		for to := range tos {
			fanIn[to]++
		}
	}

	b.WriteString("## 扇入 / 扇出\n\n")
	b.WriteString("扇入（有多少个包依赖它）高的包是「改动影响面最大」的包，重构时应优先关注\n")
	b.WriteString("其兼容性承诺；扇出则反映一个包的依赖复杂度。\n\n")

	writeRankTable(b, "扇入 Top（被依赖最多）", fanIn)
	b.WriteString("\n")
	writeRankTable(b, "扇出 Top（依赖最多）", fanOut)
	b.WriteString("\n")
}

// writeRankTable 输出一张排行表，仅列出计数大于 0 的包，最多 rankLimit 行。
func writeRankTable(b *strings.Builder, title string, counts map[string]int) {
	type kv struct {
		Path  string
		Count int
	}
	var items []kv
	for path, n := range counts {
		if n <= 0 {
			continue
		}
		items = append(items, kv{path, n})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Path < items[j].Path
	})

	fmt.Fprintf(b, "### %s\n\n", title)
	if len(items) == 0 {
		b.WriteString("（无）\n")
		return
	}
	b.WriteString("| 包 | 计数 |\n|---|---|\n")
	shown := len(items)
	if shown > rankLimit {
		shown = rankLimit
	}
	for _, it := range items[:shown] {
		fmt.Fprintf(b, "| `%s` | %d |\n", it.Path, it.Count)
	}
	if len(items) > rankLimit {
		fmt.Fprintf(b, "\n（共 %d 个包有非零计数，此处仅列前 %d）\n", len(items), rankLimit)
	}
}

// detectViolations 检查层级与第三方依赖边界违规。
func detectViolations(pkgs []pkgInfo, byPath map[string]pkgInfo) []violation {
	var out []violation
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			target, ok := byPath[imp]
			if !ok {
				// 指向本模块中不在扫描范围内的包（例如未纳入图的其他目录）。
				continue
			}
			if p.Layer == "pkg" && (target.Layer == "internal" || target.Layer == "cmd") {
				out = append(out, violation{p.Path, target.Path, "pkg/ 不得依赖 internal/ 或 cmd/"})
			}
			if p.Layer == "internal" && target.Layer == "cmd" {
				out = append(out, violation{p.Path, target.Path, "internal/ 不得依赖 cmd/"})
			}
		}
		// pkg/ 层的第三方依赖边界。
		if p.Layer == "pkg" {
			for _, dep := range p.ExtDeps {
				if !isApprovedPkgDep(dep) {
					out = append(out, violation{p.Path, dep, "pkg/ 引入未登记在白名单的第三方依赖（见 docs/ARCHITECTURE.md）"})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// writeViolations 输出违规检测结果。
func writeViolations(b *strings.Builder, violations []violation) {
	b.WriteString("## 层级违规检测\n\n")
	if len(violations) == 0 {
		b.WriteString("✅ 未发现层级或依赖边界违规。\n\n")
		return
	}
	b.WriteString("❌ 发现违规，必须修复：\n\n")
	b.WriteString("| 来源 | 目标 | 违反规则 |\n|---|---|---|\n")
	for _, v := range violations {
		fmt.Fprintf(b, "| `%s` | `%s` | %s |\n", v.From, v.To, v.Rule)
	}
	b.WriteString("\n")
}

// writeThirdPartyDeps 逐条列出 pkg/ 层的第三方依赖及其构建约束。
// 这是对 docs/ARCHITECTURE.md「pkg 层依赖策略」声明的可核对依据。
func writeThirdPartyDeps(b *strings.Builder, pkgs []pkgInfo) {
	type row struct {
		Pkg, Dep, Ctx string
	}
	var rows []row
	for _, p := range pkgs {
		if p.Layer != "pkg" {
			continue
		}
		for _, dep := range p.ExtDeps {
			ctxs := p.depCtx[dep]
			rows = append(rows, row{p.Path, dep, renderConstraints(ctxs)})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Pkg != rows[j].Pkg {
			return rows[i].Pkg < rows[j].Pkg
		}
		return rows[i].Dep < rows[j].Dep
	})

	b.WriteString("## pkg/ 层第三方依赖明细\n\n")
	b.WriteString("用于核对 docs/ARCHITECTURE.md 的依赖策略声明。构建约束为空表示该依赖在**默认构建**\n")
	b.WriteString("（不带 build tag）下即被引入；否则仅在使用对应 tag 时引入。\n\n")
	if len(rows) == 0 {
		b.WriteString("✅ `pkg/` 层未引入任何第三方依赖。\n\n")
		return
	}
	b.WriteString("| 包 | 第三方依赖 | 构建约束 | 白名单 |\n|---|---|---|---|\n")
	for _, r := range rows {
		approved := "❌ 未登记"
		if isApprovedPkgDep(r.Dep) {
			approved = "✅"
		}
		fmt.Fprintf(b, "| `%s` | `%s` | %s | %s |\n", r.Pkg, r.Dep, r.Ctx, approved)
	}
	b.WriteString("\n")
}

// renderConstraints 把构建约束集合渲染为可读文本。
// 空字符串代表「默认参与构建」。
func renderConstraints(ctxs map[string]bool) string {
	if len(ctxs) == 0 {
		return "默认构建"
	}
	var parts []string
	for c := range ctxs {
		if c == "" {
			parts = append(parts, "默认构建")
			continue
		}
		parts = append(parts, "`"+c+"`")
	}
	sort.Strings(parts)
	return strings.Join(parts, "、")
}

// codeList 把字符串列表渲染为反引号包裹的逗号列表。
func codeList(items []string) string {
	if len(items) == 0 {
		return "（无）"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "`" + s + "`"
	}
	return strings.Join(quoted, "、")
}
