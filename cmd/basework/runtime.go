package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wly2lcl/basework/internal/compaction"
	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/loopdetect"
	"github.com/wly2lcl/basework/internal/observability"
	"github.com/wly2lcl/basework/internal/permission"
	intruntime "github.com/wly2lcl/basework/internal/runtime"
	"github.com/wly2lcl/basework/internal/subagent"
	runtimetools "github.com/wly2lcl/basework/internal/tools"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/lsp"
	"github.com/wly2lcl/basework/pkg/mcp"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/skill"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

type runtimeAgentOptions struct {
	Callback agent.Callback
	// Preset 是命令行 --preset 指定的启动预设（CFG-002）。
	// 空表示未指定；与配置文件 preset 不同值时属冲突（ADR 0006）。
	Preset string
	// ApprovalPrompt 覆盖交互模式下的用户提示实现（UI-002）。
	// nil = 用默认的终端问答（CLI）；TUI 传 broker 审批。
	ApprovalPrompt permission.PromptFunc
}

type runtimeAgent struct {
	Agent        agent.Agent
	ModelName    string
	ProviderName string
	// ---- 运行服务依赖（RUN-001）。service 为 nil 时按需懒构造。----
	sess        *session.JSONLStore
	sessionID   func() string
	jobManager  *jobs.Manager
	uiCallback  agent.Callback
	cbSwitch    *intruntime.CallbackSwitch
	service     intruntime.Service
	serviceOnce sync.Once

	// CapabilitySummary 是启动时展示的能力摘要，含 unknown 标记，避免把未知当支持。
	CapabilitySummary string
	// UnknownCapabilities 列出结论为 unknown 的能力；非空时启动入口会明确提示。
	UnknownCapabilities []string
}

func newRuntimeAgent(cfg *config.Config, opts runtimeAgentOptions) (*runtimeAgent, error) {
	// 启动预设（CFG-002，ADR 0006）：配置文件 preset 已在 Load 时展开；
	// 命令行 --preset 在此叠加。判定顺序：未知名 → 双重指定冲突 → 展开。
	// 同值不算冲突也不重复展开（ApplyPreset 幂等，但跳过更省事也更直白）。
	effectivePreset := cfg.Preset
	if opts.Preset != "" && opts.Preset != cfg.Preset {
		if !config.IsKnownPreset(opts.Preset) {
			return nil, fmt.Errorf("%w: --preset %q（可用：%v）",
				config.ErrUnknownPreset, opts.Preset, config.KnownPresets())
		}
		if cfg.Preset != "" {
			return nil, fmt.Errorf("%w: 配置文件 preset=%q 与命令行 --preset=%q 不一致",
				config.ErrPresetConflict, cfg.Preset, opts.Preset)
		}
		if err := config.ApplyPreset(cfg, opts.Preset); err != nil {
			return nil, err
		}
		effectivePreset = opts.Preset
	}

	providerType := cfg.EffectiveProviderName(os.Getenv("BASEWORK_PROVIDER"))

	// 自定义端点（CFG-004）。解析规则与 `config explain` 共用同一个纯函数
	// （ADR 0007），两边必须给出同一结果。
	//
	// 这里修掉的是 CFG-004 之前的缺陷：配置里写 base_url 没有任何路径进入请求，
	// 请求静默落到内置默认端点，最终以「鉴权失败」的形式误导用户。因此解析结果
	// 必须真的传给 provider.Create，并且形态写错时直接失败而不是退回默认端点。
	endpoint := cfg.ResolveEndpoint(providerType, os.Getenv("BASEWORK_BASE_URL"))
	if endpoint.IsCustom() {
		if err := config.ValidateBaseURL(endpointLabel(providerType, endpoint.Source), endpoint.BaseURL); err != nil {
			return nil, err
		}
	}

	model, err := provider.Create(provider.Config{
		Type:    providerType,
		ModelID: cfg.Model,
		APIKey:  providerAPIKey(cfg, providerType),
		BaseURL: endpoint.BaseURL,
		Options: buildProviderOptions(cfg, providerType, endpoint),
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LLM 模型失败: %w", err)
	}

	// 运行前做能力预检。Agent 循环确实依赖工具调用与流式响应，缺能力时必须明确失败
	// 并给出可选替代；静默禁用工具或擅自换模型都属于“把未知/不支持伪装成支持”。
	if err := checkRuntimeCapabilities(model, providerType, cfg.Model); err != nil {
		return nil, err
	}

	sess, err := session.NewJSONLStore(getSessionDir())
	if err != nil {
		return nil, fmt.Errorf("创建会话存储失败: %w", err)
	}

	pathChecker, err := configureBuiltinToolRuntime(cfg)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("获取工作目录失败: %w", err)
	}

	// 后台任务的归属要等 Agent 构造完成才知道，而工具必须在 Agent 之前构造。
	// holder 就是这两件事之间的桥：工具持有 Get，构造完成后回填会话 ID。
	jobOwner := &jobOwnerHolder{}
	jobManager := newRuntimeJobManager(jobOwner)

	// 资源归属（CFG-003）：从这里开始创建的一切可释放资源都登记进 scope。
	// agent.New 失败时由 defer 释放（逆序、不遗留）；成功后所有权移交给
	// cleanup 插件，agent.Close 触发同一份 scope.Close（幂等，双重释放安全）。
	scope := newRuntimeResourceScope()
	agentOwned := false
	defer func() {
		if !agentOwned {
			_ = scope.Close()
		}
	}()
	scope.Register("background-jobs", runtimeJobCleanup(jobManager, jobOwner))

	// 可审阅编辑工具：预览/提交/撤销走 internal/edits 完整流程，每一步落成
	// file.edited 会话事件。权限口径与敏感路径检查共用同一份配置（下面
	// configureBuiltinToolRuntime 返回的 pathChecker），不另起一套标准。
	editTool := runtimetools.NewEditFilesTool(cwd)
	editTool.CheckPath = pathChecker.CheckPath
	editTool.Sink = &runtimeEditEventSink{store: sess, sessionID: jobOwner.Get}
	editTool.Tracker = session.NewFileTracker()

	var permissionChecker *runtimePermissionChecker
	if cfg.Permission.Enabled {
		permissionChecker, err = newPermissionChecker(cfg, opts.ApprovalPrompt)
		if err != nil {
			return nil, err
		}
		for i, cleanup := range permissionChecker.Cleanups {
			scope.Register(fmt.Sprintf("permission-persistence-%d", i), cleanup)
		}
	}

	var coreChecker *permission.Checker
	if permissionChecker != nil {
		coreChecker = permissionChecker.Core
	}
	// LSP/MCP 的启动失败是非致命的（禁用并告警），但启动成功的资源必须登记，
	// 否则 agent.New 失败时它们就成了没人管的子进程。
	extraTools := initializeRuntimeExtensions(context.Background(), cfg, cwd, scope)
	builtinRuntime := newBuiltinRuntime(cfg, pathChecker)
	subAgentCoordinator := newRuntimeSubAgentCoordinator(cfg, model, cwd, coreChecker, permissionChecker, extraTools, builtinRuntime)

	tools := runtimeTools(cfg, cwd, coreChecker, extraTools, subAgentCoordinator,
		runtimeJobWiring{manager: jobManager, owner: jobOwner.Get, pathChecker: pathChecker,
			editTool: editTool, builtinRuntime: builtinRuntime})

	tools = filterToolsByAllowlist(tools, cfg.Tools.Allowed)

	// 只读硬保证（ADR 0006）：readonly 预设的最终工具表必须整体落在只读集合内。
	// 替换规则靠这里机制兜底，不靠「写配置的人小心」。
	if effectivePreset == config.PresetReadonly {
		if err := ensureReadonlyTools(tools); err != nil {
			return nil, err
		}
	}

	agentOpts := []agent.Option{
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithSystemPrompt(runtimeSystemPrompt(cfg, cwd, factsHashFunc(cwd, pathChecker))),
		agent.WithMaxSteps(cfg.MaxIterations),
		agent.WithTools(tools...),
	}

	auditMode, err := runtimeAuditMode()
	if err != nil {
		return nil, err
	}
	agentOpts = append(agentOpts, agent.WithAuditMode(auditMode))
	agentOpts = append(agentOpts, agent.WithPlugin(newRuntimeResourcePlugin(scope)))
	// 回调路由（RUN-003）：Agent 注册的永远是按运行绑定的路由器；TUI 传入
	// 的回调在每次 Start 时绑入、结束即解绑，取消/出错后的收尾增量不会
	// 落进下一次运行的界面。CLI 无 UI 回调，路由器保持空绑（转发为空操作）。
	cbSwitch := intruntime.NewCallbackSwitch()
	agentOpts = append(agentOpts, agent.WithCallback(cbSwitch))
	if cfg.Permission.Enabled {
		agentOpts = append(agentOpts, agent.WithPermissionChecker(permissionChecker.Adapter))
	}

	agt, err := agent.New(agentOpts...)
	if err != nil {
		// defer 里的 scope.Close 会释放本次已创建的全部资源（LSP/MCP 子进程、
		// job 管理器、权限持久化），不让失败的装配留下存活资源。
		return nil, fmt.Errorf("创建 Agent 失败: %w", err)
	}
	// 装配成功：资源所有权从 defer 移交给 cleanup 插件（agent.Close 触发）。
	agentOwned = true

	// 会话 ID 此刻才确定，回填给后台任务归属，并把上一进程遗留的非终态记录归并为
	// interrupted。归并失败只告警：它不该拦住启动，但也不能沉默——日志里若还留着
	// running，用户会以为那个任务仍在跑。
	bindRuntimeJobOwner(agt, jobOwner, jobManager)

	rt := &runtimeAgent{
		Agent:               agt,
		ModelName:           cfg.Model,
		ProviderName:        providerType,
		CapabilitySummary:   capabilitySummary(providerType, cfg.Model),
		UnknownCapabilities: unknownCapabilities(provider.CapabilitiesFor(providerType, cfg.Model)),
		sess:                sess,
		sessionID:           jobOwner.Get,
		jobManager:          jobManager,
		uiCallback:          opts.Callback,
		cbSwitch:            cbSwitch,
	}
	return rt, nil
}

// Service 返回运行服务视图（RUN-001）：把已组装的 Agent/会话/任务管理器
// 收拢到 internal/runtime 的契约后面。同一 runtimeAgent 返回同一实例，
// 让生命周期归一（Close 走 Service 或走 Agent 都只发生一次资源释放）。
func (rt *runtimeAgent) Service() intruntime.Service {
	rt.serviceOnce.Do(func() {
		svc, err := intruntime.NewLocal(intruntime.LocalDeps{
			Agent:          rt.Agent,
			Session:        rt.sess,
			SessionID:      rt.sessionID(),
			Jobs:           rt.jobManager,
			CallbackSwitch: rt.cbSwitch,
			UICallback:     rt.uiCallback,
		})
		if err != nil {
			// 构造失败只可能是缺 Agent——运行入口 guarantee 非空；
			// 记日志并保持 service 为 nil，调用方按无服务降级。
			log.Printf("[runtime] 构造运行服务失败: %v", err)
			return
		}
		rt.service = svc
	})
	return rt.service
}

// reportRuntimeCapabilities 在运行入口展示能力与 key 要求。
//
// 结论全部已知时只在 verbose 下打印；存在 unknown 时始终提示——未知不等于支持，
// 用户需要知道当前配置里哪些结论没有依据。
func reportRuntimeCapabilities(rt *runtimeAgent, verbose bool) {
	if rt == nil {
		return
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "能力: %s\n", rt.CapabilitySummary)
		fmt.Fprintf(os.Stderr, "API key: %s\n", keyRequirementText(rt.ProviderName, rt.ModelName))
	}
	if len(rt.UnknownCapabilities) > 0 {
		fmt.Fprintf(os.Stderr,
			"注意: %s 的能力结论为 unknown（未声明也未实测，不代表支持）: %s\n",
			rt.ModelName, strings.Join(rt.UnknownCapabilities, ", "))
		fmt.Fprintf(os.Stderr, "      用 `basework model info %s` 查看依据。\n", rt.ModelName)
	}
}

// keyRequirementText 返回该模型是否需要 API key 的可读文本。
func keyRequirementText(providerType, modelID string) string {
	entry, ok := findKnownModel(modelID)
	if ok {
		return entry.keyStatus()
	}
	if provider.RequiresAPIKey(providerType) {
		return "required"
	}
	return "not required"
}

// runtimeAuditMode 读取请求审计策略。
//
// 默认 compatible —— 保持既有行为：审计记录写不进去只记日志、继续发送请求。
// 设 BASEWORK_AUDIT_MODE=strict 时，审计记录写不进去会在发出任何 Provider 调用之前
// 中断本轮，于是「日志里没有 request.built」等价于「没有发出过请求」。
// 取值非法时明确报错，不静默回落到默认模式。
func runtimeAuditMode() (agent.AuditMode, error) {
	raw := strings.TrimSpace(os.Getenv("BASEWORK_AUDIT_MODE"))
	if raw == "" {
		return agent.AuditModeCompatible, nil
	}
	mode := agent.AuditMode(raw)
	if !mode.Valid() {
		return "", fmt.Errorf("BASEWORK_AUDIT_MODE=%q 不是合法取值（可选 compatible / strict）", raw)
	}
	return mode, nil
}

// checkRuntimeCapabilities 校验当前模型是否满足 Agent 循环的硬性能力要求。
func checkRuntimeCapabilities(model llm.Model, providerType, modelID string) error {
	reqs := []llm.RequiredCapability{
		{Cap: llm.CapTools, Reason: "Agent 循环需要工具调用"},
		{Cap: llm.CapStreaming, Reason: "交互式输出依赖流式响应"},
	}
	err := llm.CheckRequiredCapabilities(model, reqs)
	if err == nil {
		return nil
	}
	var missing *llm.MissingCapabilityError
	if errors.As(err, &missing) {
		missing.Mitigation = []string{
			"运行 `basework model list` 查看各模型的能力列，改用结论为 supported 的模型",
			fmt.Sprintf("确认 %s/%s 的模型 ID 拼写是否与 provider 实际提供的一致", providerType, modelID),
			"若使用自建端点，请在配置中显式指定该端点真实支持的模型",
		}
	}
	return fmt.Errorf("模型能力预检失败: %w", err)
}

// capabilitySummary 生成启动信息里的能力摘要。
func capabilitySummary(providerType, modelID string) string {
	caps := provider.CapabilitiesFor(providerType, modelID)
	parts := make([]string, 0, len(provider.CapabilityOrder()))
	for _, cap := range provider.CapabilityOrder() {
		parts = append(parts, fmt.Sprintf("%s=%s", cap, caps[cap].Support.ShortLabel()))
	}
	return strings.Join(parts, " ")
}

// configureBuiltinToolRuntime 装配 builtin 工具依赖的全局运行时配置。
//
// 返回路径检查器本身，让产品层的其他工具（后台 bash）能复用同一个实例，
// 而不是各自 new 一个——两份检查器一旦配置不同，就变成两条权限口径。
func configureBuiltinToolRuntime(cfg *config.Config) (*permission.PathChecker, error) {
	builtin.SetTimeoutConfig(builtin.TimeoutConfig{
		DefaultTimeout: cfg.Tools.Timeout.Default,
		Overrides:      cfg.Tools.Timeout.Overrides,
	})
	if !cfg.Observability.Enabled {
		builtin.SetEventBus(nil)
	}

	level := cfg.Security.ProtectionLevel
	if level == "" {
		level = "strict"
	}
	protection, err := permission.ParseProtectionLevel(level)
	if err != nil {
		return nil, fmt.Errorf("解析敏感路径保护级别失败: %w", err)
	}
	pathChecker := permission.NewPathChecker(
		cfg.Security.SensitivePaths.Block,
		cfg.Security.SensitivePaths.Allow,
		protection,
	)
	builtin.SetPathChecker(pathChecker)
	return pathChecker, nil
}

func baseTools(cfg *config.Config, rt *builtin.Runtime) []tool.Tool {
	tools := builtin.AllWithRuntime(rt)
	for _, t := range tools {
		if bash, ok := t.(*builtin.BashTool); ok {
			bash.PermissionMode = "default"
			if cfg.Permission.Enabled && cfg.Permission.Mode != "" {
				bash.PermissionMode = cfg.Permission.Mode
			}
			bash.BlockedCommands = cfg.Permission.CommandBlacklist.BlockedCommands
		}
	}
	return tools
}

// newBuiltinRuntime 从配置构造 builtin 工具的实例级运行时注入（CFG-003）。
//
// PathChecker 与 Timeout 与 configureBuiltinToolRuntime 写进全局的是同一份
// 配置——区别在于全局值会被同进程的其他实例覆盖，实例注入不会。Overrides
// 拷贝一份：config.Store 的 Mutate 可能替换底层 map，工具实例不应跟着漂移。
// EventBus 留空（回落全局）：产品的超时事件消费方目前走 agent 层观测总线，
// builtin 级 bus 保持既有「未设置即不发布」的行为。
func newBuiltinRuntime(cfg *config.Config, pathChecker *permission.PathChecker) *builtin.Runtime {
	overrides := make(map[string]int, len(cfg.Tools.Timeout.Overrides))
	for name, seconds := range cfg.Tools.Timeout.Overrides {
		overrides[name] = seconds
	}
	timeout := builtin.TimeoutConfig{
		DefaultTimeout: cfg.Tools.Timeout.Default,
		Overrides:      overrides,
	}
	return &builtin.Runtime{
		PathChecker: pathChecker,
		Timeout:     &timeout,
	}
}

// runtimeJobWiring 是后台任务工具需要的运行时依赖。
//
// 单独成结构而不是继续加形参：主 Agent 与子代理的差别只是"给不给这套依赖"，
// 用零值表示"不给"，调用点一眼能看出子代理没有后台任务能力。
type runtimeJobWiring struct {
	manager     *jobs.Manager
	owner       func() string
	pathChecker *permission.PathChecker
	// editTool 是主 Agent 的可审阅编辑工具（edit_files）。nil 表示不注册
	//（子代理走零值）；主 Agent 由 newRuntimeAgent 构造后传入。
	editTool *runtimetools.EditFilesTool
	// builtinRuntime 是 builtin 工具的实例级运行时注入（CFG-003）。
	// nil 时 builtin 工具回落包级全局——与旧嵌入入口行为一致。
	builtinRuntime *builtin.Runtime
}

func runtimeTools(cfg *config.Config, workDir string, checker *permission.Checker, extraTools []tool.Tool, subAgentCoordinator *subagent.Coordinator, wiring runtimeJobWiring) []tool.Tool {
	tools := baseTools(cfg, wiring.builtinRuntime)
	tools = append(tools,
		runtimetools.NewApplyPatchTool(workDir),
		runtimetools.NewTodoWriteTool("basework-runtime"),
		runtimetools.NewQuestionTool(checker),
		runtimetools.NewWebFetchTool(cfg),
		runtimetools.NewWebSearchTool(cfg),
	)
	if wiring.editTool != nil {
		tools = append(tools, wiring.editTool)
	}
	if subAgentCoordinator != nil {
		tools = append(tools, subagent.NewSubAgentTool(subAgentCoordinator))
	}
	tools = append(tools, runtimeJobTools(cfg, wiring)...)
	tools = append(tools, extraTools...)
	return tools
}

// runtimeJobTools 构造后台任务相关工具（启动 / 列举 / 读输出 / 取消）。
//
// wiring.manager 为 nil 时返回空：宁可不注册，也不注册"调用即报错"的假能力。
// 权限口径与同步 bash 保持一致——同一份黑名单、同一个路径检查器实例。
func runtimeJobTools(cfg *config.Config, wiring runtimeJobWiring) []tool.Tool {
	if wiring.manager == nil {
		return nil
	}
	background := runtimetools.NewBackgroundBashTool("", wiring.manager)
	// 归属走函数晚绑定：会话 ID 在工具构造时还不存在。
	background.OwnerFunc = wiring.owner
	background.PermissionMode = "default"
	if cfg.Permission.Enabled && cfg.Permission.Mode != "" {
		background.PermissionMode = cfg.Permission.Mode
	}
	background.BlockedCommands = cfg.Permission.CommandBlacklist.BlockedCommands
	background.DefaultTimeout, background.NoTimeout = runtimeBackgroundTimeout(cfg)
	if wiring.pathChecker != nil {
		background.CheckPath = wiring.pathChecker.CheckPath
	}

	tools := []tool.Tool{background}
	return append(tools, runtimetools.NewJobTools(wiring.owner, wiring.manager)...)
}

// runtimeBackgroundTimeout 解析后台 bash 的超时口径。
//
// 直接复用 builtin.TimeoutConfig，而不是自己读配置字段：同步 bash 走的是
// GetTimeout("bash")，两边用同一份解析才能保证"用户改一次配置、两个工具都跟着变"。
// 解析出 0 表示配置显式关闭超时，用 NoTimeout 表达，而不是回落成 30 秒。
func runtimeBackgroundTimeout(cfg *config.Config) (seconds int, disabled bool) {
	resolved := builtin.TimeoutConfig{
		DefaultTimeout: cfg.Tools.Timeout.Default,
		Overrides:      cfg.Tools.Timeout.Overrides,
	}.GetTimeout("bash")
	if resolved <= 0 {
		return 0, true
	}
	return int(resolved / time.Second), false
}

func runtimeBehaviorOptions(cfg *config.Config, model llm.Model, subAgentCoordinator *subagent.Coordinator) []agent.Option {
	var opts []agent.Option
	if c := newRuntimeCompactor(cfg, model); c != nil {
		opts = append(opts, agent.WithCompactor(c))
	}
	if d := newRuntimeLoopDetector(cfg); d != nil {
		opts = append(opts, agent.WithLoopDetector(d))
	}
	if bus := newRuntimeEventBus(cfg); bus != nil {
		adapter := observability.NewEventBusAdapter(bus)
		builtin.SetEventBus(adapter)
		opts = append(opts, agent.WithEventBus(adapter))
	}
	if subAgentCoordinator != nil {
		opts = append(opts, agent.WithSubAgentRunner(subagent.NewSubAgentRunnerAdapter(subAgentCoordinator)))
	}
	return opts
}

func newRuntimeCompactor(cfg *config.Config, model llm.Model) agent.Compactor {
	if !cfg.Compaction.Enabled {
		return nil
	}
	compactionCfg := compaction.Config{
		Enabled:    cfg.Compaction.Enabled,
		Strategy:   cfg.Compaction.Strategy,
		Threshold:  cfg.Compaction.Threshold,
		WindowSize: cfg.Compaction.WindowSize,
	}
	var strategy compaction.Strategy
	switch cfg.Compaction.Strategy {
	case "summarization":
		strategy = compaction.NewSummarizationStrategy(compaction.NewLLMSummarizer(model))
	case "selective":
		strategy = compaction.NewSelectiveStrategy(cfg.Compaction.WindowSize)
	default:
		strategy = compaction.NewSlidingWindowStrategy(cfg.Compaction.WindowSize)
	}
	return compaction.NewEngine(compactionCfg, strategy)
}

func newRuntimeLoopDetector(cfg *config.Config) agent.LoopDetector {
	if !cfg.LoopDetect.Enabled {
		return nil
	}
	loopCfg := loopdetect.Config{
		Enabled:           cfg.LoopDetect.Enabled,
		RepeatedThreshold: cfg.LoopDetect.RepeatedThreshold,
		ToolLoopThreshold: cfg.LoopDetect.ToolLoopThreshold,
		ResponseStrategy:  cfg.LoopDetect.ResponseStrategy,
		CustomPatterns:    cfg.LoopDetect.CustomPatterns,
	}
	return loopdetect.NewLoopDetectorAdapter(loopdetect.NewDetector(loopCfg))
}

func newRuntimeEventBus(cfg *config.Config) *observability.EventBus {
	if !cfg.Observability.Enabled {
		return nil
	}
	return observability.NewEventBus()
}

func newRuntimeSubAgentCoordinator(cfg *config.Config, model llm.Model, workDir string, checker *permission.Checker, permissionChecker *runtimePermissionChecker, extraTools []tool.Tool, builtinRuntime *builtin.Runtime) *subagent.Coordinator {
	if !cfg.SubAgent.Enabled {
		return nil
	}
	subAgentCfg := subagent.Config{
		Enabled:       cfg.SubAgent.Enabled,
		DefaultType:   cfg.SubAgent.DefaultType,
		CostLimit:     cfg.SubAgent.CostLimit,
		MaxConcurrent: cfg.SubAgent.MaxConcurrent,
	}
	factory := func(ctx context.Context, task *subagent.Task) (subagent.AgentTaskRunner, error) {
		childTools := runtimeSubAgentTools(cfg, workDir, checker, extraTools, task.AgentType, builtinRuntime)
		childOpts := []agent.Option{
			agent.WithModel(model),
			agent.WithSession(session.NewMemoryStore()),
			agent.WithSystemPrompt(runtimeSubAgentPrompt(cfg, workDir, task)),
			agent.WithMaxSteps(cfg.MaxIterations),
			agent.WithTools(childTools...),
		}
		if permissionChecker != nil {
			childOpts = append(childOpts, agent.WithPermissionChecker(permissionChecker.Adapter))
		}
		if c := newRuntimeCompactor(cfg, model); c != nil {
			childOpts = append(childOpts, agent.WithCompactor(c))
		}
		if d := newRuntimeLoopDetector(cfg); d != nil {
			childOpts = append(childOpts, agent.WithLoopDetector(d))
		}
		child, err := agent.New(childOpts...)
		if err != nil {
			return nil, err
		}
		return &runtimeSubAgentRunner{agent: child}, nil
	}
	return subagent.NewCoordinator(factory, subAgentCfg)
}

func runtimeSubAgentTools(cfg *config.Config, workDir string, checker *permission.Checker, extraTools []tool.Tool, agentType subagent.AgentType, builtinRuntime *builtin.Runtime) []tool.Tool {
	if agentType == subagent.TypeReadonly {
		return readonlyRuntimeTools(cfg, extraTools, builtinRuntime)
	}
	// 子代理不接后台任务：它和主 Agent 同属一个会话，拿到这套工具就等于
	// 能看见、能取消主 Agent 的任务，那不是子代理该有的边界。
	return runtimeTools(cfg, workDir, checker, extraTools, nil, runtimeJobWiring{builtinRuntime: builtinRuntime})
}

// bindRuntimeJobOwner 把 Agent 的会话 ID 回填给后台任务归属，并归并上一进程的遗留记录。
//
// 两步必须成对出现：只回填不归并，日志里会留着上次进程的 running，用户会以为任务
// 还在跑；只归并不回填，则本次新起的任务全都记在空归属下。
func bindRuntimeJobOwner(agt agent.Agent, owner *jobOwnerHolder, mgr *jobs.Manager) {
	provider, ok := agt.(agent.SessionIDProvider)
	if !ok {
		return
	}
	sessionID := provider.SessionID()
	if sessionID == "" {
		return
	}
	owner.Set(sessionID)
	if mgr == nil {
		return
	}
	n, err := mgr.ReconcileOwner(sessionID)
	if err != nil {
		// 不拦住启动，但要出声：归并失败意味着记录可能仍显示 running。
		log.Printf("[runtime] warning: 后台任务历史归并失败（记录可能仍显示 running）: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[runtime] 已把 %d 条上一进程遗留的后台任务标记为 interrupted；不会自动重跑", n)
	}
}

func readonlyRuntimeTools(cfg *config.Config, extraTools []tool.Tool, rt *builtin.Runtime) []tool.Tool {
	allowed := map[string]bool{
		"read": true,
		"grep": true,
		"glob": true,
	}
	var tools []tool.Tool
	for _, t := range baseTools(cfg, rt) {
		if allowed[t.Name()] {
			tools = append(tools, t)
		}
	}
	for _, t := range extraTools {
		name := t.Name()
		if strings.HasPrefix(name, "lsp_") || name == "mcp_read" || name == "mcp_prompt" {
			tools = append(tools, t)
		}
	}
	return tools
}

func runtimeSubAgentPrompt(cfg *config.Config, workDir string, task *subagent.Task) string {
	// 子代理不注入事实摘要（hash 为 nil 时 runtimeFactsSummaryPrompt 跳过）：
	// 摘要属于主会话的项目上下文，子代理自带任务级 Context。
	prompt := runtimeSystemPrompt(cfg, workDir, nil)
	if task.AgentType == subagent.TypeReadonly {
		prompt += "\n\n你是只读子代理。只能读取和分析信息，不要修改文件、执行写入操作或运行破坏性命令。"
	}
	if len(task.Context) > 0 {
		if data, err := json.Marshal(task.Context); err == nil {
			prompt += "\n\n子代理上下文：" + string(data)
		}
	}
	return strings.TrimSpace(prompt)
}

type runtimeSubAgentRunner struct {
	agent agent.Agent
}

func (r *runtimeSubAgentRunner) HandleMessage(ctx context.Context, input string) (*subagent.TaskResult, error) {
	resp, err := r.agent.HandleMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return &subagent.TaskResult{
		Content:      responseText(resp),
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	}, nil
}

func (r *runtimeSubAgentRunner) Close() error {
	return r.agent.Close()
}

type runtimePermissionChecker struct {
	Core     *permission.Checker
	Adapter  agent.PermissionChecker
	Cleanups []func() error
}

func newPermissionChecker(cfg *config.Config, promptOverride permission.PromptFunc) (*runtimePermissionChecker, error) {
	mode := cfg.Permission.Mode
	if mode == "" {
		mode = "deny-all"
	}
	parsedMode, err := permission.ParseMode(mode)
	if err != nil {
		return nil, fmt.Errorf("解析权限模式失败: %w", err)
	}
	persistence, err := openRuntimePermissionPersistence(cfg)
	if err != nil {
		return nil, err
	}

	// UI-002：TUI 模式传入 broker 审批 prompt（图形确认）；CLI 保持终端问答。
	prompt := newRuntimePermissionPrompt(os.Stdin, os.Stdout)
	if promptOverride != nil {
		prompt = promptOverride
	}
	var checker *permission.Checker
	if persistence.Store != nil {
		checker = permission.NewCheckerWithStore(parsedMode, persistence.Store, prompt)
	} else {
		checker = permission.NewChecker(parsedMode, nil, prompt)
	}
	if persistence.AuditLogger != nil {
		checker = checker.WithAudit(persistence.AuditLogger)
	}

	level := cfg.Security.ProtectionLevel
	if level == "" {
		level = "strict"
	}
	protection, err := permission.ParseProtectionLevel(level)
	if err != nil {
		return nil, fmt.Errorf("解析敏感路径保护级别失败: %w", err)
	}
	pathChecker := permission.NewPathChecker(
		cfg.Security.SensitivePaths.Block,
		cfg.Security.SensitivePaths.Allow,
		protection,
	)
	return &runtimePermissionChecker{
		Core:     checker,
		Adapter:  permission.NewPathPermissionAdapter(checker, pathChecker),
		Cleanups: persistence.Cleanups,
	}, nil
}

// initializeRuntimeExtensions 启动 LSP/MCP 扩展并把工具返回给调用方。
//
// 资源归属：启动成功的每个扩展都把释放动作登记进 scope（由调用方决定交给
// cleanup 插件还是在失败路径上直接释放）；启动失败的扩展不登记、不遗留。
// 扩展启动失败是非致命的：禁用并告警，产品继续可用。
func initializeRuntimeExtensions(ctx context.Context, cfg *config.Config, workDir string, scope *runtimeResourceScope) []tool.Tool {
	var tools []tool.Tool

	if lspManager, err := startLSPManager(ctx, workDir); err != nil {
		log.Printf("[runtime] warning: LSP disabled: %v", err)
	} else {
		tools = append(tools, lsp.Tools(lspManager)...)
		scope.Register("lsp", lspManager.Stop)
	}

	if mcpManager, err := startMCPManager(ctx, cfg); err != nil {
		log.Printf("[runtime] warning: MCP disabled: %v", err)
	} else if mcpManager != nil {
		tools = append(tools, runtimetools.NewMCPReadTool(mcpManager), runtimetools.NewMCPPromptTool(mcpManager))
		tools = append(tools, mcpManager.Tools()...)
		scope.Register("mcp", mcpManager.Close)
	}

	return tools
}

type runtimePermissionPersistence struct {
	Store       permission.Store
	AuditLogger *permission.AuditLogger
	Cleanups    []func() error
}

func newRuntimePermissionPrompt(stdin *os.File, stdout *os.File) permission.PromptFunc {
	return func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		default:
		}

		fmt.Fprintf(stdout, "\n权限请求: 工具 %s\n", toolName)
		if len(args) > 0 {
			fmt.Fprintf(stdout, "参数:\n%s\n", permission.MarshalArgs(args))
		}
		fmt.Fprint(stdout, "允许执行？[y] 本次允许 / [a] 始终允许 / [n] 本次拒绝 / [d] 始终拒绝: ")

		reader := bufio.NewReader(stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false, false, fmt.Errorf("读取权限输入失败: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, false, nil
		case "a", "always":
			return true, true, nil
		case "d", "deny", "never":
			return false, true, nil
		default:
			return false, false, nil
		}
	}
}

func startLSPManager(ctx context.Context, workDir string) (*lsp.Manager, error) {
	manager := lsp.NewManager(lsp.Config{})
	if err := manager.Start(ctx, workDir); err != nil {
		return nil, err
	}
	return manager, nil
}

func startMCPManager(ctx context.Context, cfg *config.Config) (*mcp.Manager, error) {
	if len(cfg.MCPConfigs) == 0 {
		return nil, nil
	}
	manager := mcp.NewManager()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for name, raw := range cfg.MCPConfigs {
		serverCfg, err := parseMCPServerConfig(raw)
		if err != nil {
			log.Printf("[runtime] warning: skip MCP server %q: %v", name, err)
			continue
		}
		if err := manager.Connect(connectCtx, name, serverCfg); err != nil {
			log.Printf("[runtime] warning: MCP server %q connect failed: %v", name, err)
			continue
		}
	}
	return manager, nil
}

func parseMCPServerConfig(raw interface{}) (mcp.ServerConfig, error) {
	var cfg mcp.ServerConfig
	data, err := json.Marshal(raw)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.LoadConfig()
	return cfg, cfg.Validate()
}

func runtimeSystemPrompt(cfg *config.Config, workDir string, factsHash func(string) (string, error)) string {
	prompt := cfg.SystemPrompt
	skillPrompt := runtimeSkillPrompt(cfg, workDir)
	if skillPrompt != "" {
		if prompt == "" {
			prompt = skillPrompt
		} else {
			prompt = prompt + "\n\n" + skillPrompt
		}
	}
	// 工作区事实摘要（CTX-002）：默认关闭；开启后注入 system prompt，
	// 其内容随 request.built 审计指纹落账（「按审计策略注入」）。
	if factsSummary := runtimeFactsSummaryPrompt(cfg, workDir, factsHash); factsSummary != "" {
		if prompt == "" {
			prompt = factsSummary
		} else {
			prompt = prompt + "\n\n" + factsSummary
		}
	}
	return prompt
}

// runtimeFactsSummaryPrompt 按配置开关生成事实摘要段。
// 未开启返回空——不加载事实、不读任何文件、不触碰受保护路径。
func runtimeFactsSummaryPrompt(cfg *config.Config, workDir string, factsHash func(string) (string, error)) string {
	if cfg.FactsSummary == nil || !cfg.FactsSummary.Enabled || factsHash == nil {
		return ""
	}
	provider := agent.NewFactsSummaryProvider(getSessionDir(), runtimeWorkspaceID(), cfg.FactsSummary.BudgetKB*1024, factsHash)
	if provider == nil {
		return ""
	}
	text, err := provider.Collect()
	if err != nil || text == "" {
		return ""
	}
	return text
}

// factsHashFunc 构造事实摘要的哈希函数：先过敏感路径检查（与工具层
// 同一策略），受保护路径返回 ErrProtectedPath（摘要标注「未读取」），
// 其余读文件求 SHA-256。任何读取错误都不阻塞摘要。
func factsHashFunc(workDir string, pathChecker *permission.PathChecker) func(string) (string, error) {
	return func(relPath string) (string, error) {
		abs := filepath.Join(workDir, relPath)
		if pathChecker != nil {
			if allowed, reason := pathChecker.CheckPath(abs); !allowed {
				return "", fmt.Errorf("%w: %s (%s)", agent.ErrProtectedPath, relPath, reason)
			}
		}
		return agent.FileHash(abs)
	}
}

func runtimeSkillPrompt(cfg *config.Config, workDir string) string {
	paths := runtimeSkillPaths(cfg, workDir)
	loader := skill.NewLoader(paths...)
	if err := loader.Discover(); err != nil {
		log.Printf("[runtime] warning: skill discovery failed: %v", err)
		return ""
	}
	xml := skill.ToPromptXML(loader.Active())
	if xml == "" {
		return ""
	}
	return "可用技能定义如下；当任务匹配技能描述时优先遵循对应技能指令：\n" + xml
}

func runtimeSkillPaths(cfg *config.Config, workDir string) []string {
	paths := []string{filepath.Join(workDir, ".basework", "skills")}
	if cfg.Templates.CustomDir != "" {
		paths = append(paths, filepath.Join(cfg.Templates.CustomDir, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, ".basework", "skills"),
			filepath.Join(home, ".config", "basework", "skills"),
		)
	}
	return paths
}

type runtimeCleanupPlugin struct {
	cleanups []func() error
}

// filterToolsByAllowlist 按白名单过滤工具表（CFG-002）。allowlist 为空时
// 原样返回，不做拷贝——「不限制」是常见路径，避免每次启动都复制整表。
// 条目支持 `*` 前缀匹配，规则见 config.ToolAllowed。
func filterToolsByAllowlist(tools []tool.Tool, allowlist []string) []tool.Tool {
	if len(allowlist) == 0 {
		return tools
	}
	filtered := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if config.ToolAllowed(allowlist, t.Name()) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// ensureReadonlyTools 校验最终工具表整体落在只读集合内（ADR 0006 的硬保证）。
// 预设展开、白名单过滤、以及未来任何改动工具表的路径，都必须过这道闸：
// 只要有一个写/执行类工具漏进来，启动就失败，而不是降级运行。
func ensureReadonlyTools(tools []tool.Tool) error {
	readonlySet := config.ReadonlyToolset()
	for _, t := range tools {
		if !config.ToolAllowed(readonlySet, t.Name()) {
			return fmt.Errorf("只读预设出现能力泄漏（工具 %q 不在只读集合内），拒绝启动", t.Name())
		}
	}
	return nil
}
