package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

func TestRedactURLRemovesCredentialsAndQuery(t *testing.T) {
	got := redactURL("https://user:secret@example.test/v1/?api_key=do-not-save#fragment")
	if got != "https://example.test/v1" {
		t.Fatalf("redactURL() = %q", got)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "do-not-save") {
		t.Fatalf("脱敏结果泄漏凭据: %q", got)
	}
}

func TestCopyDirCopiesFixtureContents(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.WriteFile(filepath.Join(src, "calc.go"), []byte("return a - b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "case.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"calc.go", filepath.Join("nested", "case.txt")} {
		if _, err := os.Stat(filepath.Join(dst, path)); err != nil {
			t.Fatalf("复制后缺少 %s: %v", path, err)
		}
	}
}

func TestRunRejectsModifiedTrustedTestsAndRestoresWorkingDirectory(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "go.mod"), []byte("module fixture\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "calc.go"), []byte("package main\n\nfunc Add(a, b int) int { return a - b }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "calc_test.go"), []byte("package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"bad\") } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		var delta map[string]any
		finish := "stop"
		if requests == 1 {
			args, _ := json.Marshal(map[string]string{"command": "printf 'package main\\n' > calc_test.go"})
			delta = map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-review", "type": "function",
				"function": map[string]any{"name": "bash", "arguments": string(args)},
			}}}
			finish = "tool_calls"
		} else {
			delta = map[string]any{"content": "done"}
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finish,
		}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
	}))
	defer server.Close()

	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEWORK_REAL_PROVIDER", "openai")
	t.Setenv("BASEWORK_REAL_API_KEY", "synthetic-review-key")
	t.Setenv("BASEWORK_REAL_BASE_URL", server.URL+"/v1")
	t.Setenv("BASEWORK_REAL_MODEL", "review-model")
	output := filepath.Join(t.TempDir(), "result.json")
	if err := run(fixture, output); err == nil {
		t.Fatal("修改受保护测试后，验收不应成功")
	}
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if current != original {
		t.Fatalf("runner 未恢复工作目录: %s", current)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var result resultFile
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.ValidationOK || !strings.Contains(result.ValidationErr, "protected fixture file changed") {
		t.Fatalf("验证失败原因未结构化记录: %+v", result)
	}
	if strings.Contains(string(data), "synthetic-review-key") {
		t.Fatal("结果文件泄漏合成密钥")
	}
}

func TestRunAcceptsCorrectImplementationWithTrustedTests(t *testing.T) {
	fixture := t.TempDir()
	writeReviewFixture(t, fixture, `package main

func Add(a, b int) int { return a - b }
`, `package main

import "testing"

func TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal("bad") } }
`)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		var delta map[string]any
		finish := "stop"
		if requests == 1 {
			args, _ := json.Marshal(map[string]string{"command": "printf 'package main\\n\\nfunc Add(a, b int) int { return a + b }\\n' > calc.go"})
			delta = map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-fix", "type": "function",
				"function": map[string]any{"name": "bash", "arguments": string(args)},
			}}}
			finish = "tool_calls"
		} else {
			delta = map[string]any{"content": "fixed and tested"}
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finish,
		}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
	}))
	defer server.Close()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEWORK_REAL_PROVIDER", "openai")
	t.Setenv("BASEWORK_REAL_API_KEY", "synthetic-review-key")
	t.Setenv("BASEWORK_REAL_BASE_URL", server.URL+"/v1")
	t.Setenv("BASEWORK_REAL_MODEL", "review-model")
	output := filepath.Join(t.TempDir(), "result.json")
	if err := run(fixture, output); err != nil {
		t.Fatalf("正确实现不应失败: %v", err)
	}
	if current, _ := os.Getwd(); current != original {
		t.Fatalf("runner 未恢复工作目录: %s", current)
	}
	var result resultFile
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.ValidationOK || !result.TestsExecuted || result.TestExitCode != 0 {
		t.Fatalf("可信测试结果不完整: %+v", result)
	}
	if result.TestFilesSHA256["calc_test.go"] == "" {
		t.Fatalf("未记录可信测试哈希: %+v", result.TestFilesSHA256)
	}
}

func TestIndependentTestHonorsTimeoutAndBoundsOutput(t *testing.T) {
	fixture := t.TempDir()
	writeReviewFixture(t, fixture, `package main

func Add(a, b int) int { return a + b }
`, `package main

import (
 "os/exec"
 "testing"
 "time"
)

func TestAdd(t *testing.T) {
 child := exec.Command("sh", "-c", "sleep 30")
 if err := child.Start(); err != nil { t.Fatal(err) }
 time.Sleep(30 * time.Second)
}
`)
	ctx, cancel := context.WithTimeout(t.Context(), 750*time.Millisecond)
	defer cancel()
	started := time.Now()
	code, output, executed := independentTest(ctx, fixture)
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("验证超时未及时终止: %s", elapsed)
	}
	if code == 0 || executed || !strings.Contains(output, "timed out") {
		t.Fatalf("超时结果不符: code=%d executed=%v output=%q", code, executed, output)
	}
}

func writeReviewFixture(t *testing.T, dir, implementation, tests string) {
	t.Helper()
	for name, data := range map[string]string{
		"go.mod":       "module fixture\n\ngo 1.26\n",
		"calc.go":      implementation,
		"calc_test.go": tests,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunnerEnvironmentDoesNotExposeProviderKeyToBash(t *testing.T) {
	const secret = "synthetic-runner-key"
	const otherSecret = "synthetic-other-provider-key"
	t.Setenv("BASEWORK_REAL_API_KEY", secret)
	t.Setenv("AWS_SECRET_ACCESS_KEY", otherSecret)
	t.Setenv("GOFLAGS", "-run=none")
	env := sanitizedEnvironment()
	for _, value := range env {
		if strings.HasPrefix(value, "GOFLAGS=") || strings.HasPrefix(value, "AWS_SECRET_ACCESS_KEY=") {
			t.Fatalf("不安全变量仍进入 runner 环境: %q", value)
		}
	}
	tool := &builtin.BashTool{Runtime: &builtin.Runtime{Environment: env}}
	result, err := tool.Execute(t.Context(), []byte(`{"command":"printenv BASEWORK_REAL_API_KEY AWS_SECRET_ACCESS_KEY; exit 1"}`))
	if err != nil || result == nil {
		t.Fatalf("Bash 环境隔离探针失败: %v", err)
	}
	if strings.Contains(result.Content, secret) || strings.Contains(result.Content, otherSecret) {
		t.Fatal("Bash 子进程读取到了 Provider key")
	}
}

func TestSanitizedEnvironmentUsesSandboxHomeAndRedactsProxyCredentials(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "https://proxy-user:proxy-pass@example.test:8443")
	home := filepath.Join(t.TempDir(), "home")
	env := sanitizedEnvironmentForHome(home)
	values := make(map[string]string)
	for _, value := range env {
		name, raw, ok := strings.Cut(value, "=")
		if ok {
			values[name] = raw
		}
	}
	if values["HOME"] != home || values["USERPROFILE"] != home {
		t.Fatalf("未设置隔离 home: %#v", values)
	}
	if values["TMPDIR"] != filepath.Join(home, "tmp") {
		t.Fatalf("未设置隔离临时目录: %#v", values)
	}
	if values["GOCACHE"] != filepath.Join(home, "go-cache") || values["GOMODCACHE"] != filepath.Join(home, "go-mod-cache") || values["GOPATH"] != filepath.Join(home, "go-path") {
		t.Fatalf("未设置隔离 Go 缓存/路径: %#v", values)
	}
	if strings.Contains(values["HTTPS_PROXY"], "proxy-pass") || strings.Contains(values["HTTPS_PROXY"], "proxy-user") {
		t.Fatalf("代理凭据未脱敏: %q", values["HTTPS_PROXY"])
	}
}
