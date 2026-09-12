// Package tests 包含 basework 项目的架构约束测试。
//
// 这些测试是「文档描述的结构」与「代码实际结构」之间的护栏：
// 本项目历史上出现过 pkg 反向依赖 internal、以及 ARCHITECTURE.md 声称
// 「pkg 不依赖任何第三方库」而实际有 5 个的情况。规则写进测试，才不会再次漂移。
package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	modulePath = "github.com/wly2lcl/basework"

	pkgPrefix      = modulePath + "/pkg"
	internalPrefix = modulePath + "/internal"
	cmdPrefix      = modulePath + "/cmd"
)

// injectionWhitelist 登记 pkg/ 层允许存在的 Set* 注入点。
//
// 这些函数把「实现」从上层注入到 pkg 核心，是 pkg 保持可嵌入（不反向依赖
// internal）的必要手段；但每多一个注入点，核心的隐式全局状态就多一份，
// 因此要求逐个登记——新增注入点必须显式改动本表，防止悄悄膨胀。
var injectionWhitelist = map[string]string{
	"pkg/tool/builtin.SetEventBus":      "超时事件上报；接口定义在 builtin 本地，避免 pkg→internal",
	"pkg/tool/builtin.SetPathChecker":   "路径检查策略注入；接口定义在 builtin 本地",
	"pkg/tool/builtin.SetTimeoutConfig": "全局工具超时配置",
}

// repoRoot 向上查找包含 go.mod 的目录，返回仓库根路径。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取工作目录失败: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("未找到仓库根目录（不含 go.mod）")
		}
		dir = parent
	}
}

// walkGoFiles 遍历 root/dir 下所有 .go 文件（含测试文件），对每个文件调用 fn。
// fn 收到「仓库相对路径」（如 pkg/tool/builtin/timeout.go）与已解析的 AST。
func walkGoFiles(t *testing.T, root, dir string, mode parser.Mode, fn func(rel string, file *ast.File)) int {
	t.Helper()
	count := 0
	absDir := filepath.Join(root, dir)
	err := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, mode)
		if parseErr != nil {
			t.Errorf("解析 %s 失败: %v", path, parseErr)
			return nil
		}
		count++
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		fn(filepath.ToSlash(rel), file)
		return nil
	})
	if err != nil {
		t.Fatalf("遍历 %s 失败: %v", absDir, err)
	}
	return count
}

// TestArchPkgDoesNotImportInternal 保证 pkg/ 层不依赖 internal/ 与 cmd/ 层。
//
// 分层约定（见 ARCHITECTURE.md）：pkg 是可嵌入核心，internal/cmd 是终端产品实现。
// 一旦 pkg 反向依赖上层，任何嵌入方都会被拖入产品层依赖树，
// 「可嵌入」这一核心承诺即失效（也无法把 pkg 拆成独立 module）。
//
// 历史违规点：pkg/tool/builtin/{path_checker,timeout,timeout_test}.go。
func TestArchPkgDoesNotImportInternal(t *testing.T) {
	root := repoRoot(t)

	violations := 0
	inspected := walkGoFiles(t, root, "pkg", parser.ImportsOnly, func(rel string, file *ast.File) {
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range []struct{ prefix, layer string }{
				{internalPrefix, "internal"},
				{cmdPrefix, "cmd"},
			} {
				if importPath == forbidden.prefix || strings.HasPrefix(importPath, forbidden.prefix+"/") {
					t.Errorf("%s 导入了 %s 包 %q：pkg 层不得依赖 %s 层",
						rel, forbidden.layer, importPath, forbidden.layer)
					violations++
				}
			}
		}
	})

	// 防止路径判断出错导致测试空跑通过（假阴性）。
	if inspected == 0 {
		t.Fatalf("未在 %s 下发现任何 .go 文件，架构检查未生效", filepath.Join(root, "pkg"))
	}

	t.Logf("架构检查：已扫描 pkg 下 %d 个 .go 文件，发现 %d 处 pkg→上层 违规", inspected, violations)
}

// TestArchInternalDoesNotImportCmd 保证 internal/ 层不依赖 cmd/ 层。
//
// cmd/ 是 main 包（可执行入口），任何非 main 包依赖它都会导致无法复用与测试。
func TestArchInternalDoesNotImportCmd(t *testing.T) {
	root := repoRoot(t)

	violations := 0
	inspected := walkGoFiles(t, root, "internal", parser.ImportsOnly, func(rel string, file *ast.File) {
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == cmdPrefix || strings.HasPrefix(importPath, cmdPrefix+"/") {
				t.Errorf("%s 导入了 cmd 包 %q：internal 层不得依赖 cmd 层", rel, importPath)
				violations++
			}
		}
	})

	if inspected == 0 {
		t.Fatalf("未在 %s 下发现任何 .go 文件，架构检查未生效", filepath.Join(root, "internal"))
	}

	t.Logf("架构检查：已扫描 internal 下 %d 个 .go 文件，发现 %d 处 internal→cmd 违规", inspected, violations)
}

// TestArchInjectionPointsRegistered 保证 pkg/ 层的 Set* 注入点都被显式登记。
//
// 只允许白名单内的注入点存在；同时校验白名单没有残留（被删掉的注入点
// 必须从表里移除），避免白名单随时间腐化成一串失效条目。
func TestArchInjectionPointsRegistered(t *testing.T) {
	root := repoRoot(t)

	found := map[string]bool{}
	walkGoFiles(t, root, "pkg", 0, func(rel string, file *ast.File) {
		// 测试文件中的 Set* 不算对外注入点。
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}
			name := fn.Name.Name
			if !strings.HasPrefix(name, "Set") || len(name) <= 3 {
				continue
			}
			if r := rune(name[3]); r < 'A' || r > 'Z' {
				continue // 只关心导出的 Set*，忽略 Setter 之类的小写续写
			}
			found[dir+"."+name] = true
		}
	})

	if len(found) == 0 {
		t.Fatal("未在 pkg/ 下发现任何 Set* 注入点，检查未生效（路径或解析有问题）")
	}

	var unregistered []string
	for key := range found {
		if _, ok := injectionWhitelist[key]; !ok {
			unregistered = append(unregistered, key)
		}
	}
	sort.Strings(unregistered)
	for _, key := range unregistered {
		t.Errorf("发现未登记的注入点 %s：请确认其必要性，并加入 tests/arch_test.go 的 injectionWhitelist", key)
	}

	var stale []string
	for key := range injectionWhitelist {
		if !found[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("白名单条目 %s 已不存在：请从 tests/arch_test.go 的 injectionWhitelist 中移除", key)
	}

	t.Logf("架构检查：pkg 下共 %d 个注入点，白名单 %d 条，未登记 %d 个，残留 %d 条",
		len(found), len(injectionWhitelist), len(unregistered), len(stale))
}
