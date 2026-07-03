package permission

import (
	"context"
	"testing"
)

// ---------- ParseMode 测试 ----------

func TestParseMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr bool
	}{
		{"交互模式", "interactive", ModeInteractive, false},
		{"YOLO模式", "yolo", ModeYolo, false},
		{"DenyAll模式(短横线)", "deny-all", ModeDenyAll, false},
		{"DenyAll模式(下划线)", "deny_all", ModeDenyAll, false},
		{"未知模式", "unknown", "", true},
		{"空字符串", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMode(%q) 错误 = %v, wantErr = %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMode_Valid(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeInteractive, true},
		{ModeYolo, true},
		{ModeDenyAll, true},
		{Mode("unknown"), false},
		{Mode(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.Valid(); got != tt.want {
				t.Errorf("Mode(%q).Valid() = %v, 期望 %v", tt.mode, got, tt.want)
			}
		})
	}
}

// ---------- Rule 测试 ----------

func TestMatchRule(t *testing.T) {
	tests := []struct {
		name     string
		rule     Rule
		toolName string
		args     map[string]interface{}
		want     bool
	}{
		{
			name:     "工具名精确匹配",
			rule:     Rule{ToolPattern: "read_file", Allow: true},
			toolName: "read_file",
			want:     true,
		},
		{
			name:     "工具名不匹配",
			rule:     Rule{ToolPattern: "read_file", Allow: true},
			toolName: "write_file",
			want:     false,
		},
		{
			name:     "工具名glob匹配",
			rule:     Rule{ToolPattern: "read_*", Allow: true},
			toolName: "read_file",
			want:     true,
		},
		{
			name:     "工具名glob不匹配",
			rule:     Rule{ToolPattern: "read_*", Allow: true},
			toolName: "write_file",
			want:     false,
		},
		{
			name:     "工具名+参数模式匹配",
			rule:     Rule{ToolPattern: "write_file", ArgPattern: "*.txt", Allow: true},
			toolName: "write_file",
			args:     map[string]interface{}{"path": "test.txt"},
			want:     true,
		},
		{
			name:     "工具名匹配但参数不匹配",
			rule:     Rule{ToolPattern: "write_file", ArgPattern: "*.txt", Allow: true},
			toolName: "write_file",
			args:     map[string]interface{}{"path": "test.go"},
			want:     false,
		},
		{
			name:     "glob拒绝模式匹配",
			rule:     Rule{ToolPattern: "execute_*", Allow: false},
			toolName: "execute_command",
			want:     true, // 匹配到规则（规则本身是 deny）
		},
		{
			name:     "空args匹配空参数模式",
			rule:     Rule{ToolPattern: "simple_tool", ArgPattern: "", Allow: true},
			toolName: "simple_tool",
			args:     map[string]interface{}{},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchRule(tt.rule, tt.toolName, tt.args); got != tt.want {
				t.Errorf("MatchRule() = %v, 期望 %v", got, tt.want)
			}
		})
	}
}

func TestMatchRules_优先级(t *testing.T) {
	// 第一条匹配规则优先
	rules := []Rule{
		{ToolPattern: "read_*", Allow: true},
		{ToolPattern: "read_file", Allow: false},
	}
	got := MatchRules(rules, "read_file", nil)
	if got == nil {
		t.Fatal("MatchRules() 返回 nil，期望匹配到规则")
	}
	if got.Allow != true {
		t.Errorf("第一条规则 Allow=true 应优先，实际 Allow=%v", got.Allow)
	}
}

func TestMatchRules_无匹配(t *testing.T) {
	rules := []Rule{
		{ToolPattern: "write_*", Allow: true},
	}
	got := MatchRules(rules, "read_file", nil)
	if got != nil {
		t.Errorf("MatchRules() 期望 nil，实际得到 %+v", got)
	}
}

// ---------- Cache 测试 ----------

func TestCache_GetSet(t *testing.T) {
	c := NewCache()
	key := "read_file:path=test.txt"

	// 最初不存在
	if got := c.Get(key); got != nil {
		t.Errorf("空缓存 Get(%q) 期望 nil, 得到 %v", key, got)
	}

	// 设置后读取
	c.Set(key, true)
	if got := c.Get(key); got == nil || *got != true {
		t.Errorf("Get(%q) 期望 true, 得到 %v", key, got)
	}

	// 覆盖为 false
	c.Set(key, false)
	if got := c.Get(key); got == nil || *got != false {
		t.Errorf("Get(%q) 期望 false, 得到 %v", key, got)
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache()
	c.Set("tool1:", true)
	c.Set("tool2:", false)
	c.Clear()

	if got := c.Get("tool1:"); got != nil {
		t.Error("Clear() 后应返回 nil")
	}
	if got := c.Get("tool2:"); got != nil {
		t.Error("Clear() 后应返回 nil")
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache()
	c.Set("tool1:", true)
	c.Delete("tool1:")
	if got := c.Get("tool1:"); got != nil {
		t.Error("Delete() 后应返回 nil")
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     string
	}{
		{"无参数", "read_file", nil, "read_file:"},
		{"空参数", "read_file", map[string]interface{}{}, "read_file:"},
		{"单个参数", "write_file", map[string]interface{}{"path": "test.txt"}, "write_file:path=test.txt"},
		{"多个参数按key排序", "tool", map[string]interface{}{"b": "2", "a": "1"}, "tool:a=1 b=2"},
		{"整数参数值", "tool", map[string]interface{}{"count": 3}, "tool:count=3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CacheKey(tt.toolName, tt.args); got != tt.want {
				t.Errorf("CacheKey(%q, %v) = %q, 期望 %q", tt.toolName, tt.args, got, tt.want)
			}
		})
	}
}

// ---------- Checker.Check 测试 ----------

func TestChecker_Check_Yolo模式(t *testing.T) {
	c := NewChecker(ModeYolo, nil, nil)
	allowed, err := c.Check(context.Background(), "any_tool", nil)
	if err != nil {
		t.Fatalf("Yolo模式不应返回错误: %v", err)
	}
	if !allowed {
		t.Error("Yolo模式应始终允许")
	}
}

func TestChecker_Check_DenyAll模式(t *testing.T) {
	c := NewChecker(ModeDenyAll, nil, nil)
	allowed, err := c.Check(context.Background(), "any_tool", nil)
	if err != nil {
		t.Fatalf("DenyAll模式不应返回错误: %v", err)
	}
	if allowed {
		t.Error("DenyAll模式应始终拒绝")
	}
}

func TestChecker_Check_交互模式_规则优先(t *testing.T) {
	// 规则匹配时不应调用 PromptFunc
	promptCalled := false
	c := NewChecker(ModeInteractive, []Rule{
		{ToolPattern: "read_file", Allow: true},
	}, func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		promptCalled = true
		return false, false, nil
	})

	allowed, err := c.Check(context.Background(), "read_file", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !allowed {
		t.Error("规则匹配应允许")
	}
	if promptCalled {
		t.Error("规则匹配后不应调用 PromptFunc")
	}
}

func TestChecker_Check_交互模式_无规则时提示(t *testing.T) {
	var capturedTool string
	var capturedArgs map[string]interface{}
	c := NewChecker(ModeInteractive, nil, func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		capturedTool = toolName
		capturedArgs = args
		return true, false, nil
	})

	allowed, err := c.Check(context.Background(), "my_tool", map[string]interface{}{"key": "val"})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !allowed {
		t.Error("用户批准后应允许")
	}
	if capturedTool != "my_tool" {
		t.Errorf("toolName 期望 %q, 得到 %q", "my_tool", capturedTool)
	}
	if capturedArgs["key"] != "val" {
		t.Errorf("args 期望包含 key=val, 得到 %v", capturedArgs)
	}
}

func TestChecker_Check_交互模式_缓存决策(t *testing.T) {
	promptCount := 0
	c := NewChecker(ModeInteractive, nil, func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		promptCount++
		return true, true, nil // 缓存决策
	})

	// 第一次调用 — 应触发提示
	allowed1, _ := c.Check(context.Background(), "tool1", map[string]interface{}{"x": "1"})
	if !allowed1 {
		t.Error("第一次应允许")
	}
	if promptCount != 1 {
		t.Errorf("提示次数期望 1, 得到 %d", promptCount)
	}

	// 第二次相同调用 — 应命中缓存，不提示
	allowed2, _ := c.Check(context.Background(), "tool1", map[string]interface{}{"x": "1"})
	if !allowed2 {
		t.Error("第二次应允许")
	}
	if promptCount != 1 {
		t.Errorf("缓存命中后不应再次提示, 当前 %d", promptCount)
	}
}

func TestChecker_Check_交互模式_用户拒绝(t *testing.T) {
	c := NewChecker(ModeInteractive, nil, func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		return false, false, nil
	})
	allowed, err := c.Check(context.Background(), "danger_tool", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if allowed {
		t.Error("用户拒绝后应返回 false")
	}
}

func TestChecker_Check_交互模式_无PromptFunc(t *testing.T) {
	c := NewChecker(ModeInteractive, nil, nil)
	_, err := c.Check(context.Background(), "tool", nil)
	if err == nil {
		t.Error("无 PromptFunc 时应返回错误")
	}
}

func TestChecker_Check_未知模式(t *testing.T) {
	c := NewChecker(Mode("unknown_mode"), nil, nil)
	_, err := c.Check(context.Background(), "tool", nil)
	if err == nil {
		t.Error("未知模式时应返回错误")
	}
}

// ---------- NewCheckerFromConfig 测试 ----------

func TestNewCheckerFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"有效配置", Config{Enabled: true, Mode: "yolo"}, false},
		{"无效模式", Config{Enabled: true, Mode: "unknown"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCheckerFromConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCheckerFromConfig() 错误 = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// ---------- WithMode 测试 ----------

func TestWithMode(t *testing.T) {
	original := NewChecker(ModeInteractive, nil, nil)
	changed := original.WithMode(ModeYolo)

	// 原对象不受影响
	if original.Mode != ModeInteractive {
		t.Error("WithMode 不应修改原对象")
	}
	if changed.Mode != ModeYolo {
		t.Error("WithMode 应返回新模式的对象")
	}

	// 共享缓存
	original.Cache.Set("key:", true)
	if got := changed.Cache.Get("key:"); got == nil || *got != true {
		t.Error("WithMode 应共享 Cache")
	}
}

// ---------- MarshalArgs 测试 ----------

func TestMarshalArgs(t *testing.T) {
	args := map[string]interface{}{
		"path": "/tmp/file",
		"mode": "write",
	}
	result := MarshalArgs(args)
	if result == "" {
		t.Error("MarshalArgs 不应返回空字符串")
	}
	if len(result) == 0 {
		t.Error("MarshalArgs 应生成有效的 JSON 字符串")
	}
}

// ---------- formatArgs 测试 ----------

func TestFormatArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{"空参数", map[string]interface{}{}, ""},
		{"nil参数", nil, ""},
		{"单个参数", map[string]interface{}{"path": "a.txt"}, "path=a.txt"},
		{"多个参数按排序", map[string]interface{}{"z": "1", "a": "2"}, "a=2 z=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatArgs(tt.args)
			if got != tt.want {
				t.Errorf("formatArgs(%v) = %q, 期望 %q", tt.args, got, tt.want)
			}
		})
	}
}