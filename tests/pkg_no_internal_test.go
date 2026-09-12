// Package tests 包含 basework 项目的架构约束测试。
package tests

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modulePath     = "github.com/wly2lcl/basework"
	internalPrefix = modulePath + "/internal"
)

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

// TestPkgDoesNotImportInternal 保证 pkg/ 层不依赖 internal/ 层。
//
// 分层约定（见 ARCHITECTURE.md）：pkg 是可嵌入核心，internal 是终端产品专用实现。
// 一旦 pkg 反向依赖 internal，任何嵌入方都会被拖入产品层依赖树，
// "可嵌入" 这一核心承诺即失效（也无法把 pkg 拆成独立 module）。
//
// 该测试是架构护栏，防止此类回归。历史违规点：
// pkg/tool/builtin/{path_checker,timeout,timeout_test}.go。
func TestPkgDoesNotImportInternal(t *testing.T) {
	root := repoRoot(t)
	pkgRoot := filepath.Join(root, "pkg")

	inspected := 0
	violations := 0

	walkErr := filepath.WalkDir(pkgRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Errorf("解析 %s 失败: %v", path, parseErr)
			return nil
		}
		inspected++

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == internalPrefix || strings.HasPrefix(importPath, internalPrefix+"/") {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s 导入了 internal 包 %q：pkg 层不得依赖 internal 层", rel, importPath)
				violations++
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历 %s 失败: %v", pkgRoot, walkErr)
	}

	// 防止路径判断出错导致测试空跑通过（假阴性）。
	if inspected == 0 {
		t.Fatalf("未在 %s 下发现任何 .go 文件，架构检查未生效", pkgRoot)
	}

	t.Logf("架构检查：已扫描 pkg 下 %d 个 .go 文件，发现 %d 处 pkg→internal 违规", inspected, violations)
}
