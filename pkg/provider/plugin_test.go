package provider

import (
	"context"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// testPlugin 测试用插件
type testPlugin struct{}

func (p *testPlugin) ID() string { return "test-provider" }
func (p *testPlugin) Create(cfg Config) (llm.Model, error) {
	return &testModel{name: cfg.ModelID}, nil
}

// testModel 测试用 Model 实现
type testModel struct {
	name string
}

func (m *testModel) ID() string { return m.name }
func (m *testModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}
func (m *testModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	return nil, nil
}
func (m *testModel) Supports(cap llm.Capability) bool { return false }

func TestRegisterPlugin(t *testing.T) {
	// 清空注册表（测试隔离）
	oldRegistry := globalPluginRegistry
	globalPluginRegistry = &PluginRegistry{plugins: make(map[string]ProviderPlugin)}
	defer func() { globalPluginRegistry = oldRegistry }()

	plugin := &testPlugin{}
	err := RegisterPlugin(plugin)
	if err != nil {
		t.Fatalf("RegisterPlugin 失败: %v", err)
	}

	// 验证已注册
	if _, ok := globalPluginRegistry.plugins["test-provider"]; !ok {
		t.Error("插件 'test-provider' 应已被注册")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	oldRegistry := globalPluginRegistry
	globalPluginRegistry = &PluginRegistry{plugins: make(map[string]ProviderPlugin)}
	defer func() { globalPluginRegistry = oldRegistry }()

	plugin := &testPlugin{}
	if err := RegisterPlugin(plugin); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	// 重复注册应返回错误
	err := RegisterPlugin(plugin)
	if err == nil {
		t.Fatal("重复注册应返回错误")
	}
}

func TestGetPlugin(t *testing.T) {
	oldRegistry := globalPluginRegistry
	globalPluginRegistry = &PluginRegistry{plugins: make(map[string]ProviderPlugin)}
	defer func() { globalPluginRegistry = oldRegistry }()

	plugin := &testPlugin{}
	RegisterPlugin(plugin)

	// 获取已注册的插件
	p, ok := GetPlugin("test-provider")
	if !ok {
		t.Fatal("应能找到 'test-provider'")
	}
	if p.ID() != "test-provider" {
		t.Errorf("期望 ID=test-provider, 得到 %s", p.ID())
	}

	// 获取未注册的插件
	_, ok = GetPlugin("nonexistent")
	if ok {
		t.Error("不应找到 'nonexistent'")
	}
}

func TestListPlugins(t *testing.T) {
	oldRegistry := globalPluginRegistry
	globalPluginRegistry = &PluginRegistry{plugins: make(map[string]ProviderPlugin)}
	defer func() { globalPluginRegistry = oldRegistry }()

	// 初始应为空
	names := ListPlugins()
	if len(names) != 0 {
		t.Errorf("初始应无插件, 得到 %d", len(names))
	}

	// 注册两个插件
	RegisterPlugin(&testPlugin{})
	RegisterPlugin(&anotherTestPlugin{})

	names = ListPlugins()
	if len(names) != 2 {
		t.Errorf("期望 2 个插件, 得到 %d: %v", len(names), names)
	}
	// 验证包含两个 ID
	hasTest := false
	hasAnother := false
	for _, n := range names {
		if n == "test-provider" {
			hasTest = true
		}
		if n == "another-test" {
			hasAnother = true
		}
	}
	if !hasTest {
		t.Error("列表中应包含 'test-provider'")
	}
	if !hasAnother {
		t.Error("列表中应包含 'another-test'")
	}
}

func TestCreateFromPlugin(t *testing.T) {
	oldRegistry := globalPluginRegistry
	globalPluginRegistry = &PluginRegistry{plugins: make(map[string]ProviderPlugin)}
	defer func() { globalPluginRegistry = oldRegistry }()

	RegisterPlugin(&testPlugin{})

	model, err := CreateFromPlugin("test-provider", Config{ModelID: "my-model"})
	if err != nil {
		t.Fatalf("CreateFromPlugin 失败: %v", err)
	}
	if model.ID() != "my-model" {
		t.Errorf("期望 ModelID=my-model, 得到 %s", model.ID())
	}

	// 不存在的插件
	_, err = CreateFromPlugin("nonexistent", Config{})
	if err == nil {
		t.Fatal("不存在的插件应返回错误")
	}
}

// anotherTestPlugin 第二个测试插件
type anotherTestPlugin struct{}

func (p *anotherTestPlugin) ID() string { return "another-test" }
func (p *anotherTestPlugin) Create(cfg Config) (llm.Model, error) {
	return nil, nil
}

// compile-time check: 确保 testPlugin 实现了 ProviderPlugin
var _ ProviderPlugin = (*testPlugin)(nil)
