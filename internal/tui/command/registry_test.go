// Package command 测试
package command

import (
	"testing"
)

// TestNewRegistry 测试创建注册表
func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() 返回了 nil")
	}
}

// TestRegisterAndGet 测试注册和获取命令
func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	cmd := &Command{
		Name:        "test",
		Description: "测试命令",
		Handler:     func(args string) error { return nil },
	}

	if err := r.Register(cmd); err != nil {
		t.Fatalf("Register 失败: %v", err)
	}

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("Get('test') 应返回 true")
	}
	if got.Name != "test" {
		t.Fatalf("期望命令名为 'test'，得到 %s", got.Name)
	}
}

// TestRegisterDuplicate 测试注册重复命令
func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	cmd1 := &Command{Name: "dup", Handler: func(args string) error { return nil }}
	cmd2 := &Command{Name: "dup", Handler: func(args string) error { return nil }}

	if err := r.Register(cmd1); err != nil {
		t.Fatalf("第一次注册失败: %v", err)
	}
	if err := r.Register(cmd2); err == nil {
		t.Fatal("重复注册应返回错误")
	}
}

// TestRegisterEmptyName 测试注册空名命令
func TestRegisterEmptyName(t *testing.T) {
	r := NewRegistry()
	cmd := &Command{Name: "", Handler: func(args string) error { return nil }}
	if err := r.Register(cmd); err == nil {
		t.Fatal("空名命令注册应返回错误")
	}
}

// TestList 测试列出命令
func TestList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "b", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "a", Handler: func(args string) error { return nil }})

	cmds := r.List()
	if len(cmds) != 2 {
		t.Fatalf("期望 2 个命令，得到 %d", len(cmds))
	}
	// 应排序
	if cmds[0].Name != "a" || cmds[1].Name != "b" {
		t.Fatalf("命令应按字母序排序，得到 %s, %s", cmds[0].Name, cmds[1].Name)
	}
}

// TestSearch 测试搜索命令
func TestSearch(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "help", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "clear", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "theme", Handler: func(args string) error { return nil }})

	// 空查询返回所有
	all := r.Search("")
	if len(all) != 3 {
		t.Fatalf("空查询期望 3 个结果，得到 %d", len(all))
	}

	// 前缀匹配（fuzzy 也通过编辑距离匹配到 theme）
	matches := r.Search("hel")
	if len(matches) < 1 {
		t.Fatalf("'hel' 应至少匹配到 'help'，得到 %d 个结果", len(matches))
	}
	if matches[0].Name != "help" {
		t.Fatalf("'hel' 的最佳匹配应为 'help'，得到 %s", matches[0].Name)
	}

	// 无匹配
	matches = r.Search("xyz")
	if len(matches) != 0 {
		t.Fatalf("'xyz' 不应匹配任何命令，得到 %d", len(matches))
	}
}

// TestExecute 测试执行命令
func TestExecute(t *testing.T) {
	r := NewRegistry()
	executed := false
	r.Register(&Command{
		Name: "test",
		Handler: func(args string) error {
			executed = true
			return nil
		},
	})

	if err := r.Execute("test"); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if !executed {
		t.Fatal("命令未被执行")
	}
}

// TestExecuteUnknown 测试执行未知命令
func TestExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	err := r.Execute("unknown")
	if err == nil {
		t.Fatal("执行未知命令应返回错误")
	}
}

// TestExecuteEmpty 测试执行空输入
func TestExecuteEmpty(t *testing.T) {
	r := NewRegistry()
	if err := r.Execute(""); err != nil {
		t.Fatalf("空输入应返回 nil，得到 %v", err)
	}
	if err := r.Execute("  "); err != nil {
		t.Fatalf("空白输入应返回 nil，得到 %v", err)
	}
}

// TestExecuteWithArgs 测试带参数执行
func TestExecuteWithArgs(t *testing.T) {
	r := NewRegistry()
	var capturedArgs string
	r.Register(&Command{
		Name: "echo",
		Handler: func(args string) error {
			capturedArgs = args
			return nil
		},
	})

	if err := r.Execute("echo hello world"); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if capturedArgs != "hello world" {
		t.Fatalf("期望参数 'hello world'，得到 %q", capturedArgs)
	}
}