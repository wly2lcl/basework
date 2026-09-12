// Command doccheck 检查集中式文档、ADR、包契约、链接及单源任务看板。
// -progress 校验后从 docs/TASKS.md 计算进度，不维护另一份状态。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const adrDir = "docs/adr"

// 这些目录不参与链接扫描。
var skipDirs = map[string]bool{
	".git":         true,
	"vendor":       true,
	"bin":          true,
	"node_modules": true,
	".opencode":    true,
	".workbuddy":   true,
}

// adrNameRe 匹配 ADR 文件名：四位序号 + 短横线 + kebab-case 标题。
var adrNameRe = regexp.MustCompile(`^\d{4}-[a-z0-9]+(-[a-z0-9]+)*\.md$`)

// statusRe 从 `- Status: Accepted` 这类行中提取状态值。
var statusRe = regexp.MustCompile(`(?m)^\s*[-*]?\s*\*{0,2}Status\*{0,2}\s*[:：]\s*(\S.*?)\s*$`)

// linkRe 提取 markdown 链接与图片的相对目标（去掉 # 锚点）。
var linkRe = regexp.MustCompile(`!?\[[^\]]*\]\(([^)#\s]+?)(?:#[^)]*)?\)`)

// requiredSections 是 ADR 必须齐备的三节。
var requiredSections = []string{"## 背景", "## 决定", "## 后果"}

// pkgREADMEsRoot 是包契约的生效范围。
const pkgREADMEsRoot = "pkg"

// requiredREADMEHeadings 是每个 pkg 包契约必须齐备的五节。
//
// 选这些节是因为它们回答的是「读源码看不出来」的问题：
// 这个包干什么、怎么配、从哪扩展、有什么坑。
var requiredREADMEHeadings = []string{"## 用途", "## 配置", "## 扩展点", "## Model Experience", "## Known Limitations"}

// validStatusPrefixes 是允许的 Status 取值前缀。
var validStatusPrefixes = []string{"Proposed", "Accepted", "Deprecated", "Superseded"}

// problem 表示一条校验失败。
type problem struct {
	File string
	Msg  string
}

func main() {
	progress := flag.Bool("progress", false, "显示任务进度")
	flag.Parse()
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "定位仓库根目录失败: %v\n", err)
		os.Exit(1)
	}

	var problems []problem

	adrChecked, adrProblems := checkADR(root)
	problems = append(problems, adrProblems...)

	linkChecked, linkProblems := checkLinks(root)
	problems = append(problems, linkProblems...)

	pkgChecked, pkgProblems := checkPkgREADMEs(root)
	problems = append(problems, pkgProblems...)

	tasks, taskProblems := checkTasks(root)
	problems = append(problems, taskProblems...)

	// 假阴性防护：扫描范围没生效时不能「通过」。
	if adrChecked == 0 {
		fmt.Fprintf(os.Stderr, "未在 %s 下发现任何 ADR，检查未生效\n", adrDir)
		os.Exit(1)
	}
	if linkChecked == 0 {
		fmt.Fprintln(os.Stderr, "未扫描到任何 markdown 文件，检查未生效")
		os.Exit(1)
	}
	if pkgChecked == 0 {
		fmt.Fprintf(os.Stderr, "未在 %s 下发现任何 Go 包，包契约检查未生效\n", pkgREADMEsRoot)
		os.Exit(1)
	}

	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "\n发现 %d 处文档问题：\n\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", p.File, p.Msg)
		}
		os.Exit(1)
	}

	fmt.Printf("文档校验通过：ADR %d 个，markdown %d 个，pkg 包契约 %d 个，链接全部有效。\n",
		adrChecked, linkChecked, pkgChecked)
	if *progress {
		printProgress(tasks)
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

// checkADR 校验 docs/adr 下的 ADR 结构，返回检查数量与问题列表。
func checkADR(root string) (int, []problem) {
	dir := filepath.Join(root, adrDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, []problem{{adrDir, fmt.Sprintf("读取目录失败: %v", err)}}
	}

	var problems []problem
	checked := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		rel := adrDir + "/" + name

		body, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			problems = append(problems, problem{rel, fmt.Sprintf("读取失败: %v", readErr)})
			continue
		}
		text := string(body)

		if name == "TEMPLATE.md" {
			// 模板本身不按 NNNN 命名，但必须包含三节骨架，否则模板与规则会漂移。
			checked++
			for _, want := range requiredSections {
				if !strings.Contains(text, want) {
					problems = append(problems, problem{rel, fmt.Sprintf("模板缺少必需小节 %q", want)})
				}
			}
			continue
		}

		checked++

		if !adrNameRe.MatchString(name) {
			problems = append(problems, problem{rel, "文件名应为 NNNN-kebab-title.md（四位数字序号 + 小写短横线标题）"})
		}

		for _, want := range requiredSections {
			if !strings.Contains(text, want) {
				problems = append(problems, problem{rel, fmt.Sprintf("缺少必需小节 %q", want)})
			}
		}

		m := statusRe.FindStringSubmatch(text)
		if m == nil {
			problems = append(problems, problem{rel, "缺少 Status 字段（形如 `- Status: Accepted`）"})
		} else if !hasValidStatusPrefix(m[1]) {
			problems = append(problems, problem{rel,
				fmt.Sprintf("Status 取值 %q 不合法，应为 %s 之一", m[1], strings.Join(validStatusPrefixes, " / "))})
		}
	}

	return checked, problems
}

// hasValidStatusPrefix 判断状态值是否以合法关键字开头。
func hasValidStatusPrefix(status string) bool {
	for _, p := range validStatusPrefixes {
		if strings.HasPrefix(status, p) {
			return true
		}
	}
	return false
}

// checkLinks 校验全仓 markdown 的相对链接，返回检查数量与问题列表。
func checkLinks(root string) (int, []problem) {
	var problems []problem
	checked := 0

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
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			problems = append(problems, problem{rel, fmt.Sprintf("读取失败: %v", readErr)})
			return nil
		}
		checked++

		isGitHubTemplate := strings.HasPrefix(rel, ".github/ISSUE_TEMPLATE/") || rel == ".github/PULL_REQUEST_TEMPLATE.md"
		if !strings.HasPrefix(rel, "docs/") && rel != "README.md" && filepath.Base(rel) != "AGENTS.md" && !isGitHubTemplate {
			problems = append(problems, problem{rel, "文档正文必须集中在 docs/；根 README 仅导航，AGENTS 为执行指令例外"})
		}
		fileDir := filepath.Dir(path)
		for _, m := range linkRe.FindAllStringSubmatch(string(content), -1) {
			target := m[1]
			if target == "" || strings.HasPrefix(target, "#") {
				continue
			}
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "mailto:") {
				continue
			}
			if strings.HasPrefix(target, "<") || strings.HasPrefix(target, "/") {
				continue
			}

			resolved := filepath.Join(fileDir, filepath.FromSlash(target))
			if _, statErr := os.Stat(resolved); statErr != nil {
				problems = append(problems, problem{rel, fmt.Sprintf("相对链接失效: %s", target)})
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, problem{".", fmt.Sprintf("遍历仓库失败: %v", err)})
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Msg < problems[j].Msg
	})
	return checked, problems
}

// checkPkgREADMEs 校验 pkg/ 下每个 Go 包都有集中契约且五节齐备。
//
// 「是 Go 包」的判定是「目录下有非 _test.go 的 .go 文件」。这样自然排除了
// pkg/ 根目录（只有集成测试）与纯资源目录（如 pkg/agent/templates），
// 不需要维护一份手写清单——手写清单一定会漂移。
func checkPkgREADMEs(root string) (int, []problem) {
	base := filepath.Join(root, pkgREADMEsRoot)
	var problems []problem
	checked := 0

	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		hasGo, goErr := hasPackageGoFiles(path)
		if goErr != nil {
			problems = append(problems, problem{rel, fmt.Sprintf("读取目录失败: %v", goErr)})
			return nil
		}
		if !hasGo {
			return nil
		}

		checked++

		docRel := "docs/reference/" + rel + ".md"
		readmePath := filepath.Join(root, filepath.FromSlash(docRel))
		body, readErr := os.ReadFile(readmePath)
		if readErr != nil {
			problems = append(problems, problem{docRel,
				"缺少集中包契约（用途 / 配置 / 扩展点 / Model Experience / Known Limitations）"})
			return nil
		}
		text := string(body)
		for _, want := range requiredREADMEHeadings {
			if !strings.Contains(text, want) {
				problems = append(problems, problem{docRel, fmt.Sprintf("缺少必需小节 %q", want)})
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, problem{pkgREADMEsRoot, fmt.Sprintf("遍历失败: %v", err)})
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Msg < problems[j].Msg
	})
	return checked, problems
}

// hasPackageGoFiles 判断目录下是否存在非测试的 .go 文件。
func hasPackageGoFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true, nil
		}
	}
	return false, nil
}
