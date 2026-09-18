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
	Schema                    string            `json:"schema"`
	Provider                  string            `json:"provider"`
	Model                     string            `json:"model"`
	EndpointOrigin            string            `json:"endpoint_origin"`
	Fixture                   string            `json:"fixture"`
	Prompt                    string            `json:"prompt"`
	StartedAt                 string            `json:"started_at"`
	ElapsedMS                 int64             `json:"elapsed_ms"`
	AgentOK                   bool              `json:"agent_ok"`
	AgentError                string            `json:"agent_error,omitempty"`
	ValidationOK              bool              `json:"validation_ok"`
	ValidationErr             string            `json:"validation_error,omitempty"`
	FinalTextSHA              string            `json:"final_text_sha256,omitempty"`
	ToolCalls                 []toolSummary     `json:"tool_calls"`
	VerificationCommand       string            `json:"verification_command"`
	TestsExecuted             bool              `json:"tests_executed"`
	TestFilesSHA256           map[string]string `json:"test_files_sha256"`
	ImplementationFilesSHA256 map[string]string `json:"implementation_files_sha256"`
	TestExitCode              int               `json:"independent_test_exit_code"`
	TestOutputTail            string            `json:"independent_test_output_tail,omitempty"`
	FileChanged               bool              `json:"file_changed"`
}

type recorder struct {
	mu     sync.Mutex
	tools  []toolSummary
	secret string
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
		summary.Error = redactSecret(err.Error(), r.secret)
	} else if res != nil && res.IsError {
		summary.Error = redactSecret(truncate(res.Content, 240), r.secret)
	}
	r.tools = append(r.tools, summary)
}

func (r *recorder) snapshot() []toolSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]toolSummary(nil), r.tools...)
}

func newRecorder(secret string) *recorder { return &recorder{secret: secret} }

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
		// The runner's final stderr line is also an artifact/logging boundary.
		// Keep the Provider key out even when setup fails before a result file
		// can be written.
		fmt.Fprintln(os.Stderr, redactSecret(err.Error(), realProviderAPIKey()))
		os.Exit(1)
	}
}

func run(fixture, output string) error {
	providerName := envOr("BASEWORK_REAL_PROVIDER", "openai")
	apiKey := realProviderAPIKey()
	baseURL := strings.TrimSpace(os.Getenv("BASEWORK_REAL_BASE_URL"))
	modelID := strings.TrimSpace(os.Getenv("BASEWORK_REAL_MODEL"))
	if apiKey == "" || baseURL == "" || modelID == "" {
		return errors.New("real provider runner requires AGNES_API_KEY (or BASEWORK_REAL_API_KEY), BASEWORK_REAL_BASE_URL, and BASEWORK_REAL_MODEL; no request was sent")
	}

	fixture, err := filepath.Abs(fixture)
	if err != nil {
		return fmt.Errorf("resolve fixture: %w", err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve output: %w", err)
	}
	trusted, err := os.MkdirTemp("", "basework-real-provider-trusted-")
	if err != nil {
		return fmt.Errorf("create trusted fixture: %w", err)
	}
	defer os.RemoveAll(trusted)
	if err := copyDir(fixture, trusted); err != nil {
		return fmt.Errorf("copy trusted fixture: %w", err)
	}
	trustedSnapshot, err := snapshotFiles(trusted)
	if err != nil {
		return fmt.Errorf("snapshot trusted fixture: %w", err)
	}
	for rel, snapshot := range trustedSnapshot {
		if snapshot.Kind == "symlink" {
			return fmt.Errorf("trusted fixture may not contain symlink: %s", rel)
		}
	}
	work, err := os.MkdirTemp("", "basework-real-provider-")
	if err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}
	defer os.RemoveAll(work)
	if err := copyDir(trusted, work); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	originalCWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("read current directory: %w", err)
	}
	defer func() { _ = os.Chdir(originalCWD) }()
	if err := os.Chdir(work); err != nil {
		return fmt.Errorf("enter fixture: %w", err)
	}

	model, err := provider.Create(provider.Config{Type: providerName, APIKey: apiKey, BaseURL: baseURL, ModelID: modelID})
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	store := session.NewMemoryStore()
	agentHome := filepath.Join(work, ".basework-home")
	if err := prepareSandboxHome(agentHome); err != nil {
		return fmt.Errorf("create agent home: %w", err)
	}
	tools := builtin.AllWithRuntime(&builtin.Runtime{Environment: sanitizedEnvironmentForHome(agentHome)})
	prompt := "修复 calc.go 里的 Add 函数缺陷，并运行 go test ./... 独立验证结果。不要修改测试文件。"
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	started := time.Now().UTC()
	recorder := newRecorder(apiKey)
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

	verification, verifyErr := prepareVerification(trusted, work, trustedSnapshot)
	testCode := -1
	testOutput := "verification workspace was not prepared"
	testsExecuted := false
	if verifyErr == nil {
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 60*time.Second)
		testCode, testOutput, testsExecuted = independentTest(verifyCtx, verification.Dir)
		verifyCancel()
		defer os.RemoveAll(verification.Dir)
	}
	if verifyErr != nil {
		testOutput = "verification workspace rejected: " + verifyErr.Error()
	}
	fileChanged := verification != nil && verification.Changed
	var testFiles, implementationFiles map[string]string
	if verification != nil {
		testFiles = verification.TestFiles
		implementationFiles = verification.ImplementationFiles
	}
	result := resultFile{
		Schema:                    "basework.real-provider.v1",
		Provider:                  providerName,
		Model:                     modelID,
		EndpointOrigin:            redactURL(baseURL),
		Fixture:                   filepath.Base(fixture),
		Prompt:                    prompt,
		StartedAt:                 started.Format(time.RFC3339),
		ElapsedMS:                 time.Since(started).Milliseconds(),
		AgentOK:                   runErr == nil && resp != nil,
		ValidationOK:              verifyErr == nil && testsExecuted && testCode == 0 && fileChanged,
		ToolCalls:                 recorder.snapshot(),
		VerificationCommand:       "go test -count=1 -run ^TestAdd$ ./...",
		TestsExecuted:             testsExecuted,
		TestFilesSHA256:           testFiles,
		ImplementationFilesSHA256: implementationFiles,
		TestExitCode:              testCode,
		TestOutputTail:            redactSecret(truncate(testOutput, 1000), apiKey),
		FileChanged:               fileChanged,
	}
	if runErr != nil {
		result.AgentError = redactSecret(truncate(runErr.Error(), 500), apiKey)
	}
	if verifyErr != nil {
		result.ValidationErr = redactSecret(truncate(verifyErr.Error(), 500), apiKey)
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
		return fmt.Errorf("agent failed; redacted result saved to %s: %s", output, redactSecret(runErr.Error(), apiKey))
	}
	if !result.ValidationOK {
		return fmt.Errorf("real-provider acceptance failed; redacted result saved to %s", output)
	}
	fmt.Printf("real-provider acceptance passed: result=%s test_exit_code=0 file_changed=true\n", output)
	return nil
}

// realProviderAPIKey reads the provider credential without exposing it to the
// model's tool environment. AGNES_API_KEY is the preferred name for the
// Agnes acceptance workflow; BASEWORK_REAL_API_KEY remains a generic local
// override for existing fixtures and non-Agnes providers.
func realProviderAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("AGNES_API_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("BASEWORK_REAL_API_KEY"))
}

func independentTest(ctx context.Context, dir string) (int, string, bool) {
	home, err := os.MkdirTemp("", "basework-real-provider-verify-home-")
	if err != nil {
		return -1, "verification home setup failed: " + err.Error(), false
	}
	defer removeAllWithRetry(home)
	if err := prepareSandboxHome(home); err != nil {
		return -1, "verification home setup failed: " + err.Error(), false
	}
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-run", "^TestAdd$", "./...")
	cmd.Dir = dir
	cmd.Env = sanitizedEnvironmentForHome(home)
	// Bound os/exec's pipe-drain wait as a second line of defense if a
	// misbehaving test escapes the process-tree snapshot and keeps stdout or
	// stderr open after cancellation.
	cmd.WaitDelay = 2 * time.Second
	configureTestProcess(cmd)
	out := &cappedBuffer{limit: 64 * 1024}
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Run()
	if ctx.Err() != nil {
		return -1, "verification timed out", false
	}
	if err == nil {
		text := out.String()
		if strings.Contains(text, "[no tests to run]") || !strings.Contains(text, "ok") {
			return 1, text + "\nexpected test did not execute", false
		}
		return 0, text, true
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), out.String(), false
	}
	return -1, err.Error() + "\n" + out.String(), false
}

// removeAllWithRetry tolerates a short filesystem race while a canceled child
// finishes closing files under its isolated HOME. The retry is bounded so a
// genuinely stuck process cannot make the runner hang indefinitely.
func removeAllWithRetry(path string) {
	const attempts = 20
	for i := 0; i < attempts; i++ {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = os.RemoveAll(path)
}

type cappedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			b.data = append(b.data, p[:remaining]...)
			b.truncated = true
		} else {
			b.data = append(b.data, p...)
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(b.data)
	if b.truncated {
		text += "\n... (verification output truncated)"
	}
	return text
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
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(from)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, to); err != nil {
				return err
			}
			continue
		}
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

type fileSnapshot struct {
	Kind string
	Hash string
}

type verificationWorkspace struct {
	Dir                 string
	Changed             bool
	TestFiles           map[string]string
	ImplementationFiles map[string]string
}

func snapshotFiles(root string) (map[string]fileSnapshot, error) {
	files := make(map[string]fileSnapshot)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(rel)] = fileSnapshot{Kind: "symlink", Hash: hashString(target)}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			files[filepath.ToSlash(rel)] = fileSnapshot{Kind: "other", Hash: info.Mode().String()}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = fileSnapshot{Kind: "regular", Hash: hashBytes(data)}
		return nil
	})
	return files, err
}

func isImplementationFile(rel string) bool {
	return strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
}

func prepareVerification(trusted, work string, trustedFiles map[string]fileSnapshot) (*verificationWorkspace, error) {
	workingFiles, err := snapshotFiles(work)
	if err != nil {
		return nil, fmt.Errorf("snapshot model workspace: %w", err)
	}
	for rel, expected := range trustedFiles {
		got, ok := workingFiles[rel]
		if !ok {
			return nil, fmt.Errorf("trusted fixture file removed: %s", rel)
		}
		if !isImplementationFile(rel) && got != expected {
			return nil, fmt.Errorf("protected fixture file changed: %s", rel)
		}
		if isImplementationFile(rel) && got.Kind != "regular" {
			return nil, fmt.Errorf("implementation file changed type: %s", rel)
		}
	}
	for rel := range workingFiles {
		if _, ok := trustedFiles[rel]; !ok {
			return nil, fmt.Errorf("model workspace contains untrusted new file: %s", rel)
		}
	}

	verifyDir, err := os.MkdirTemp("", "basework-real-provider-verify-")
	if err != nil {
		return nil, fmt.Errorf("create verification workspace: %w", err)
	}
	cleanup := func(e error) (*verificationWorkspace, error) {
		_ = os.RemoveAll(verifyDir)
		return nil, e
	}
	if err := copyDir(trusted, verifyDir); err != nil {
		return cleanup(fmt.Errorf("copy trusted verification fixture: %w", err))
	}
	result := &verificationWorkspace{
		Dir:                 verifyDir,
		TestFiles:           make(map[string]string),
		ImplementationFiles: make(map[string]string),
	}
	for rel, expected := range trustedFiles {
		if isImplementationFile(rel) {
			result.ImplementationFiles[rel] = workingFiles[rel].Hash
			if workingFiles[rel].Hash != expected.Hash {
				result.Changed = true
				data, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(rel)))
				if err != nil {
					return cleanup(fmt.Errorf("read changed implementation %s: %w", rel, err))
				}
				if err := os.WriteFile(filepath.Join(verifyDir, filepath.FromSlash(rel)), data, 0o600); err != nil {
					return cleanup(fmt.Errorf("copy changed implementation %s: %w", rel, err))
				}
			}
		} else if strings.HasSuffix(rel, "_test.go") {
			result.TestFiles[rel] = expected.Hash
		}
	}
	return result, nil
}

func sanitizedEnvironment() []string { return sanitizedEnvironmentForHome("") }

// sanitizedEnvironmentForHome returns the small, explicit environment needed
// by the model's shell and the independent Go test. The runner must not expose
// arbitrary host variables: an untrusted model can run `env`, and variables
// such as GOFLAGS can also silently change the trusted test command.
func sanitizedEnvironmentForHome(home string) []string {
	allowed := map[string]bool{
		"PATH": true, "LANG": true, "LC_ALL": true, "TERM": true, "CI": true,
		"GOCACHE": true, "GOMODCACHE": true, "GOPATH": true, "GOROOT": true,
		"NO_PROXY": true, "HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true,
		"SYSTEMROOT": true, "WINDIR": true, "PATHEXT": true, "COMSPEC": true,
	}
	values := os.Environ()
	filtered := make([]string, 0, len(values)+6)
	seen := make(map[string]bool)
	for _, value := range values {
		name, raw, ok := strings.Cut(value, "=")
		if !ok {
			continue
		}
		canonical := strings.ToUpper(name)
		if !allowed[canonical] || seen[canonical] {
			continue
		}
		if canonical == "HTTP_PROXY" || canonical == "HTTPS_PROXY" || canonical == "ALL_PROXY" {
			raw = redactProxyURL(raw)
			if raw == "" {
				continue
			}
		}
		filtered = append(filtered, canonical+"="+raw)
		seen[canonical] = true
	}
	if home != "" {
		filtered = setEnvironmentValue(filtered, seen, "HOME", home)
		filtered = setEnvironmentValue(filtered, seen, "USERPROFILE", home)
		tmp := filepath.Join(home, "tmp")
		filtered = setEnvironmentValue(filtered, seen, "TMPDIR", tmp)
		filtered = setEnvironmentValue(filtered, seen, "TMP", tmp)
		filtered = setEnvironmentValue(filtered, seen, "TEMP", tmp)
		filtered = setEnvironmentValue(filtered, seen, "GOCACHE", filepath.Join(home, "go-cache"))
		filtered = setEnvironmentValue(filtered, seen, "GOMODCACHE", filepath.Join(home, "go-mod-cache"))
		filtered = setEnvironmentValue(filtered, seen, "GOPATH", filepath.Join(home, "go-path"))
	}
	return filtered
}

func prepareSandboxHome(home string) error {
	for _, dir := range []string{"tmp", "go-cache", "go-mod-cache", "go-path"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func setEnvironmentValue(values []string, seen map[string]bool, name, value string) []string {
	for i, current := range values {
		currentName, _, ok := strings.Cut(current, "=")
		if ok && strings.EqualFold(currentName, name) {
			values[i] = name + "=" + value
			seen[name] = true
			return values
		}
	}
	values = append(values, name+"="+value)
	seen[name] = true
	return values
}

func redactProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.User = nil
	// Proxy credentials are sometimes carried as query parameters (for
	// example, a signed proxy URL). They are not needed by the child process
	// and must not cross the runner's environment boundary.
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func redactSecret(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashString(value string) string { return hashBytes([]byte(value)) }

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
