// Command real_provider runs one opt-in, real-model coding scenario.
//
// It is intentionally not part of the default test suite. Credentials and the
// endpoint are read only from BASEWORK_REAL_* environment variables. The
// result file contains redacted metadata and tool names, never the API key or
// the full model response.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

type toolSummary struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type resultFile struct {
	Schema         string        `json:"schema"`
	Provider       string        `json:"provider"`
	Model          string        `json:"model"`
	EndpointOrigin string        `json:"endpoint_origin"`
	Fixture        string        `json:"fixture"`
	Prompt         string        `json:"prompt"`
	StartedAt      string        `json:"started_at"`
	ElapsedMS      int64         `json:"elapsed_ms"`
	AgentOK        bool          `json:"agent_ok"`
	AgentError     string        `json:"agent_error,omitempty"`
	FinalTextSHA   string        `json:"final_text_sha256,omitempty"`
	ToolCalls      []toolSummary `json:"tool_calls"`
	TestExitCode   int           `json:"independent_test_exit_code"`
	TestOutputTail string        `json:"independent_test_output_tail,omitempty"`
	FileChanged    bool          `json:"file_changed"`
}

type recorder struct {
	mu    sync.Mutex
	tools []toolSummary
}

func (r *recorder) OnTextDelta(string)           {}
func (r *recorder) OnThinkingDelta(string)       {}
func (r *recorder) OnTurnEnd(*agent.Response)    {}
func (r *recorder) OnError(error)                {}
func (r *recorder) OnToolCallStart(llm.ToolCall) {}
func (r *recorder) OnToolCallEnd(call llm.ToolCall, res *tool.Result, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	summary := toolSummary{Name: call.Name, Success: err == nil && res != nil && !res.IsError}
	if err != nil {
		summary.Error = err.Error()
	} else if res != nil && res.IsError {
		summary.Error = truncate(res.Content, 240)
	}
	r.tools = append(r.tools, summary)
}

func (r *recorder) snapshot() []toolSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]toolSummary(nil), r.tools...)
}

func main() {
	var fixture string
	var output string
	flag.StringVar(&fixture, "fixture", os.Getenv("BASEWORK_REAL_FIXTURE"), "fixture directory; default tests/real_provider/fixtures/fixbug")
	flag.StringVar(&output, "output", os.Getenv("BASEWORK_REAL_OUTPUT"), "redacted JSON result path")
	flag.Parse()
	if fixture == "" {
		fixture = filepath.Join("tests", "real_provider", "fixtures", "fixbug")
	}
	if output == "" {
		output = "real-provider-result.json"
	}
	if err := run(fixture, output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(fixture, output string) error {
	providerName := envOr("BASEWORK_REAL_PROVIDER", "openai")
	apiKey := strings.TrimSpace(os.Getenv("BASEWORK_REAL_API_KEY"))
	baseURL := strings.TrimSpace(os.Getenv("BASEWORK_REAL_BASE_URL"))
	modelID := strings.TrimSpace(os.Getenv("BASEWORK_REAL_MODEL"))
	if apiKey == "" || baseURL == "" || modelID == "" {
		return errors.New("real provider runner requires BASEWORK_REAL_API_KEY, BASEWORK_REAL_BASE_URL, and BASEWORK_REAL_MODEL; no request was sent")
	}

	fixture, err := filepath.Abs(fixture)
	if err != nil {
		return fmt.Errorf("resolve fixture: %w", err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve output: %w", err)
	}
	work, err := os.MkdirTemp("", "basework-real-provider-")
	if err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}
	defer os.RemoveAll(work)
	if err := copyDir(fixture, work); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	before, err := os.ReadFile(filepath.Join(work, "calc.go"))
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}
	if err := os.Chdir(work); err != nil {
		return fmt.Errorf("enter fixture: %w", err)
	}

	model, err := provider.Create(provider.Config{Type: providerName, APIKey: apiKey, BaseURL: baseURL, ModelID: modelID})
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	store := session.NewMemoryStore()
	tools := builtin.AllWithRuntime(nil)
	prompt := "修复 calc.go 里的 Add 函数缺陷，并运行 go test ./... 独立验证结果。不要修改测试文件。"
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	started := time.Now().UTC()
	recorder := &recorder{}
	a, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(store),
		agent.WithSystemPrompt("你在一个独立的临时 Go 项目中工作。只修改实现文件，完成后说明验证命令。"),
		agent.WithTools(tools...),
		agent.WithMaxSteps(25),
		agent.WithCallback(recorder),
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	defer a.Close()
	resp, runErr := a.HandleMessage(ctx, prompt)

	testCode, testOutput := independentTest()
	after, readErr := os.ReadFile(filepath.Join(work, "calc.go"))
	result := resultFile{
		Schema:         "basework.real-provider.v1",
		Provider:       providerName,
		Model:          modelID,
		EndpointOrigin: redactURL(baseURL),
		Fixture:        filepath.Base(fixture),
		Prompt:         prompt,
		StartedAt:      started.Format(time.RFC3339),
		ElapsedMS:      time.Since(started).Milliseconds(),
		AgentOK:        runErr == nil && resp != nil,
		ToolCalls:      recorder.snapshot(),
		TestExitCode:   testCode,
		TestOutputTail: truncate(testOutput, 1000),
		FileChanged:    readErr == nil && string(before) != string(after),
	}
	if runErr != nil {
		result.AgentError = truncate(runErr.Error(), 500)
	}
	if resp != nil && len(resp.Message.Content) > 0 {
		var text strings.Builder
		for _, part := range resp.Message.Content {
			text.WriteString(part.Text)
		}
		sum := sha256.Sum256([]byte(text.String()))
		result.FinalTextSHA = hex.EncodeToString(sum[:])
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	if runErr != nil {
		return fmt.Errorf("agent failed; redacted result saved to %s: %w", output, runErr)
	}
	if testCode != 0 || !result.FileChanged {
		return fmt.Errorf("real-provider acceptance failed; redacted result saved to %s", output)
	}
	fmt.Printf("real-provider acceptance passed: result=%s test_exit_code=0 file_changed=true\n", output)
	return nil
}

func independentTest() (int, string) {
	cmd := exec.Command("go", "test", "./...")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	return -1, err.Error() + "\n" + string(out)
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(from, to); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid-url"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.Scheme + "://" + u.Host + u.Path
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
