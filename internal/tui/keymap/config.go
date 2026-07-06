// Package keymap 提供键盘绑定系统
package keymap

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config 键绑定配置文件结构
type Config struct {
	Bindings []KeyBinding `json:"bindings"`
}

// LoadKeybindings 从 JSON 文件加载键绑定配置
func LoadKeybindings(path string) ([]KeyBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在时返回默认绑定
			return DefaultBindings, nil
		}
		return nil, fmt.Errorf("读取键绑定文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析键绑定文件失败: %w", err)
	}

	if err := ValidateBindings(cfg.Bindings); err != nil {
		return nil, fmt.Errorf("键绑定验证失败: %w", err)
	}

	// 合并默认绑定和自定义绑定（自定义覆盖默认）
	bindings := make([]KeyBinding, 0, len(DefaultBindings)+len(cfg.Bindings))
	override := make(map[string]bool)
	for _, b := range cfg.Bindings {
		key := string(b.Layer) + ":" + b.Key
		override[key] = true
	}

	for _, b := range DefaultBindings {
		key := string(b.Layer) + ":" + b.Key
		if !override[key] {
			bindings = append(bindings, b)
		}
	}
	bindings = append(bindings, cfg.Bindings...)

	return bindings, nil
}

// SaveKeybindings 保存键绑定配置到 JSON 文件
func SaveKeybindings(path string, bindings []KeyBinding) error {
	cfg := Config{Bindings: bindings}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化键绑定配置失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}