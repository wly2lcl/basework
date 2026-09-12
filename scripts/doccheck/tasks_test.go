package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, path, body string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func taskFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "docs/TASKS.md", `| ID | 阶段 | 任务卡 | 状态 | 依赖 | 负责人 | 证据 |
|---|---|---|---|---|---|---|
| DOC-001 | M0 | [文档](tasks/base.md#doc-001) | 完成 | — | reviewer | [记录](development/evidence/DOC-001.md) |
| DEV-001 | M1 | [开发](tasks/base.md#dev-001) | 待办 | DOC-001 | — | — |
`)
	writeFixture(t, root, "docs/tasks/base.md", "<a id=\"doc-001\"></a>\n**前置任务**：—。\n<a id=\"dev-001\"></a>\n**前置任务**：DOC-001。\n")
	writeFixture(t, root, "docs/development/evidence/DOC-001.md", "## 基线\nfixture\n## 变更\nlayout\n## 验证\nchecks passed\n## 剩余与交接\nnone\n## 结论\naccepted\n")
	return root
}

func TestTaskBoardRejectsInvalidProgress(t *testing.T) {
	cases := []struct{ name, old, replacement, want string }{
		{"valid", "", "", ""},
		{"unknown dependency", "| DOC-001 | — | — |", "| LOST-001 | — | — |", "依赖不存在"},
		{"cycle", "| 完成 | — | reviewer |", "| 完成 | DEV-001 | reviewer |", "循环依赖"},
		{"premature completion", "| 待办 | DOC-001 | — | — |", "| 完成 | DOC-001 | reviewer | — |", "证据必须"},
		{"invalid status", "| 待办 | DOC-001 |", "| 很快完成 | DOC-001 |", "非法任务状态"},
		{"bad anchor", "base.md#dev-001", "base.md#wrong", "任务卡及 ID 锚点"},
		{"no owner", "| reviewer |", "| — |", "需要负责人"},
		{"duplicate ID", "| DEV-001 | M1 |", "| DOC-001 | M1 |", "ID 重复"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := taskFixture(t)
			path := filepath.Join(root, "docs/TASKS.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.old != "" {
				writeFixture(t, root, "docs/TASKS.md", strings.Replace(string(body), tc.old, tc.replacement, 1))
			}
			tasks, problems := checkTasks(root)
			if tc.want == "" {
				if len(problems) > 0 || len(tasks) != 2 {
					t.Fatalf("tasks=%v problems=%v", tasks, problems)
				}
				return
			}
			for _, p := range problems {
				if strings.Contains(p.Msg, tc.want) {
					return
				}
			}
			t.Fatalf("expected %q, got %v", tc.want, problems)
		})
	}
}

func TestTaskBoardRequiresEvidenceAndRealAnchor(t *testing.T) {
	for _, path := range []string{"docs/development/evidence/DOC-001.md", "docs/tasks/base.md"} {
		t.Run(path, func(t *testing.T) {
			root := taskFixture(t)
			writeFixture(t, root, path, "unrelated content")
			_, problems := checkTasks(root)
			if len(problems) == 0 {
				t.Fatal("invalid evidence or missing anchor accepted")
			}
		})
	}
}

func TestCentralizedDocsLayoutAndPackageContract(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "pkg/example/example.go", "package example\n")
	writeFixture(t, root, "README.md", "[docs](docs/README.md)\n")
	writeFixture(t, root, "docs/README.md", "# docs\n")
	writeFixture(t, root, ".github/PULL_REQUEST_TEMPLATE.md", "## 变更\n")
	writeFixture(t, root, ".github/ISSUE_TEMPLATE/bug.md", "## 错误\n")
	writeFixture(t, root, "docs/reference/pkg/example.md", strings.Join(requiredREADMEHeadings, "\n"))
	if n, p := checkPkgREADMEs(root); n != 1 || len(p) != 0 {
		t.Fatalf("%d %v", n, p)
	}
	if _, p := checkLinks(root); len(p) != 0 {
		t.Fatal(p)
	}
	writeFixture(t, root, "ROADMAP.md", "old duplicate")
	writeFixture(t, root, "pkg/example/README.md", "old duplicate")
	if _, p := checkLinks(root); len(p) != 2 {
		t.Fatalf("duplicate locations accepted: %v", p)
	}
	writeFixture(t, root, "docs/reference/pkg/example.md", "## 用途\n")
	if _, p := checkPkgREADMEs(root); len(p) == 0 {
		t.Fatal("incomplete contract accepted")
	}
	writeFixture(t, root, "docs/broken.md", "[bad](missing.md)\n")
	_, problems := checkLinks(root)
	for _, p := range problems {
		if strings.Contains(p.Msg, "相对链接失效") {
			return
		}
	}
	t.Fatal("broken link accepted")
}
