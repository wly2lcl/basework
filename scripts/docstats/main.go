// Command docstats 统计 basework 仓库的代码规模与测试规模，并生成 docs/STATS.md。
//
// 存在的意义：文档里曾长期写着与真实代码量相差 45% 的数字（例如称 50,051 行、
// 1,372 个测试，实际为 72,514 行、1,571 个测试），因为数字全靠手写维护。
// 本工具让文档数字只有一个可信来源：运行 `make stats` 重新生成。
//
// 只使用标准库实现，符合仓库「核心不引入第三方依赖」的约定。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	modulePath = "github.com/wly2lcl/basework"
	outputPath = "docs/STATS.md"
	buildTags  = "sqlite memory"
)

// skipDirs 是不参与统计的目录。
var skipDirs = map[string]bool{
	".git":         true,
	"vendor":       true,
	"bin":          true,
	".opencode":    true,
	"node_modules": true,
}

// 统计口径：统计所有 .go 文件，测试代码指 *_test.go。
var (
	testFuncRe = regexp.MustCompile(`^func (Test|Benchmark|Fuzz)`)
	coverageRe = regexp.MustCompile(`coverage: ([0-9.]+)% of statements`)
)

// pkgStats 是单个目录的统计结果。
type pkgStats struct {
	Path      string // 相对仓库根的路径
	Files     int    // .go 文件总数
	TestFiles int    // *_test.go 文件数
	Lines     int    // 总行数
	TestLines int    // 测试文件行数
	TestFuncs int    // func Test/Benchmark/Fuzz 数量
}

// NonTestLines 返回非测试代码行数。
func (p pkgStats) NonTestLines() int { return p.Lines - p.TestLines }

// pkgCoverage 是单个包的测试覆盖率。
type pkgCoverage struct {
	Pkg      string
	Coverage float64
}

func main() {
	noCover := flag.Bool("no-cover", false, "跳过覆盖率统计（更快，但生成的文档缺少覆盖率数字）")
	out := flag.String("out", outputPath, "输出文件路径，传空字符串则只打印到标准输出")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "定位仓库根目录失败: %v\n", err)
		os.Exit(1)
	}

	stats, err := collectStats(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "统计代码规模失败: %v\n", err)
		os.Exit(1)
	}

	var coverages []pkgCoverage
	if !*noCover {
		coverages, err = collectCoverage(root)
		if err != nil {
			// 覆盖率失败不阻断：静态规模统计仍然有效。
			fmt.Fprintf(os.Stderr, "警告：覆盖率统计失败（%v），本次输出将不含覆盖率\n", err)
		}
	}

	report := render(root, stats, coverages)
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
}

// repoRoot 向上查找包含 go.mod 的目录。
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("未找到 go.mod")
		}
		dir = parent
	}
}

// collectStats 遍历仓库，按顶层目录聚合代码规模。
func collectStats(root string) ([]pkgStats, error) {
	byDir := map[string]*pkgStats{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		key := topLevelDir(rel)
		st, ok := byDir[key]
		if !ok {
			st = &pkgStats{Path: key}
			byDir[key] = st
		}

		isTest := strings.HasSuffix(path, "_test.go")
		st.Files++
		if isTest {
			st.TestFiles++
		}

		lines, testFuncs, readErr := countFile(path)
		if readErr != nil {
			return readErr
		}
		st.Lines += lines
		st.TestFuncs += testFuncs
		if isTest {
			st.TestLines += lines
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]pkgStats, 0, len(byDir))
	for _, st := range byDir {
		result = append(result, *st)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Lines != result[j].Lines {
			return result[i].Lines > result[j].Lines
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

// topLevelDir 返回相对路径的首段（单层目录用其自身）。
func topLevelDir(rel string) string {
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 1 {
		return "(根目录)"
	}
	return parts[0]
}

// countFile 返回文件有效行数与测试函数数量。
// 空行与纯注释行不计入行数，使「代码规模」更接近真实代码量。
func countFile(path string) (lines int, testFuncs int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inBlockComment := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if inBlockComment {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inBlockComment = false
				line = strings.TrimSpace(line[idx+2:])
			} else {
				continue
			}
		}
		if strings.HasPrefix(line, "/*") {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				line = strings.TrimSpace(line[idx+2:])
			} else {
				inBlockComment = true
				continue
			}
		}
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		lines++
		if testFuncRe.MatchString(line) {
			testFuncs++
		}
	}
	return lines, testFuncs, scanner.Err()
}

// collectCoverage 运行带覆盖率的测试并解析每包覆盖率。
func collectCoverage(root string) ([]pkgCoverage, error) {
	cmd := exec.Command("go", "test", "-tags", buildTags, "-cover", "-count=1", "./...")
	cmd.Dir = root
	cmd.Stderr = os.Stderr

	outBytes, err := cmd.Output()
	if err != nil {
		// go test 在存在失败包时返回非 0，但仍可能输出有用结果；
		// 若完全没有输出才视为失败。
		if len(outBytes) == 0 {
			return nil, err
		}
	}

	var result []pkgCoverage
	for _, line := range strings.Split(string(outBytes), "\n") {
		m := coverageRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// 只接受形如 "ok  \t<pkg>\t<耗时>\tcoverage: X% of statements" 的行。
		// 没有测试文件的包会输出 "\t<pkg>\t\tcoverage: 0.0% of statements"
		// （首字段为空），此时包名会错位到第二个字段，必须排除。
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || strings.TrimSpace(parts[0]) != "ok" {
			continue
		}
		pkg := strings.TrimSpace(parts[1])
		if pkg == "" {
			continue
		}

		var pct float64
		if _, scanErr := fmt.Sscanf(m[1], "%f", &pct); scanErr != nil {
			continue
		}
		result = append(result, pkgCoverage{Pkg: pkg, Coverage: pct})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Coverage != result[j].Coverage {
			return result[i].Coverage < result[j].Coverage
		}
		return result[i].Pkg < result[j].Pkg
	})
	return result, nil
}

// render 生成 Markdown 报告。
func render(root string, stats []pkgStats, coverages []pkgCoverage) string {
	var total, totalTest, totalFiles, totalTestFiles, totalFuncs int
	for _, st := range stats {
		total += st.Lines
		totalTest += st.TestLines
		totalFiles += st.Files
		totalTestFiles += st.TestFiles
		totalFuncs += st.TestFuncs
	}
	totalNonTest := total - totalTest

	var b strings.Builder
	b.WriteString("# 代码统计（自动生成）\n\n")
	b.WriteString("> 本文件由 `make stats` 自动生成，请勿手工编辑。\n")
	b.WriteString("> 修改代码后重新运行 `make stats` 刷新即可。\n\n")

	b.WriteString("## 统计口径\n\n")
	b.WriteString("| 项 | 口径 |\n|---|---|\n")
	b.WriteString("| 代码行数 | 非空、非纯注释行；不含 `/* */` 块内注释 |\n")
	b.WriteString("| 测试代码 | 文件名以 `_test.go` 结尾的 `.go` 文件 |\n")
	b.WriteString("| 测试用例 | 匹配 `func Test\\|Benchmark\\|Fuzz` 的函数声明数 |\n")
	b.WriteString("| 排除目录 | `.git`、`vendor`、`bin`、`.opencode`、`node_modules` |\n")
	b.WriteString("| 覆盖率 | `go test -tags \"" + buildTags + "\" -cover ./...`，仅统计含语句的包 |\n\n")

	fmt.Fprintf(&b, "生成时间：%s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	b.WriteString("## 总览\n\n")
	b.WriteString("| 指标 | 数值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 代码总行数 | %d |\n", total)
	fmt.Fprintf(&b, "| 非测试代码行数 | %d |\n", totalNonTest)
	fmt.Fprintf(&b, "| 测试代码行数 | %d |\n", totalTest)
	if total > 0 {
		fmt.Fprintf(&b, "| 测试代码占比 | %.1f%% |\n", float64(totalTest)*100/float64(total))
	}
	fmt.Fprintf(&b, "| `.go` 文件数 | %d |\n", totalFiles)
	fmt.Fprintf(&b, "| `_test.go` 文件数 | %d |\n", totalTestFiles)
	fmt.Fprintf(&b, "| 测试用例数 | %d |\n", totalFuncs)
	b.WriteString("\n")

	b.WriteString("## 按目录分布\n\n")
	b.WriteString("| 目录 | 总行数 | 非测试行数 | 测试行数 | .go 文件 | 测试文件 | 测试用例 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, st := range stats {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %d |\n",
			st.Path, st.Lines, st.NonTestLines(), st.TestLines, st.Files, st.TestFiles, st.TestFuncs)
	}
	b.WriteString("\n")

	if len(coverages) > 0 {
		b.WriteString("## 测试覆盖率（升序，优先关注靠前项）\n\n")
		b.WriteString("| 包 | 覆盖率 |\n|---|---|\n")
		for _, c := range coverages {
			pkg := strings.TrimPrefix(c.Pkg, modulePath+"/")
			fmt.Fprintf(&b, "| `%s` | %.1f%% |\n", pkg, c.Coverage)
		}
		b.WriteString("\n")
	}

	return b.String()
}
