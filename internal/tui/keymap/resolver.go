// Package keymap 提供键盘绑定系统
package keymap

// Resolver 按键事件解析器
type Resolver struct {
	bindings map[Layer]map[string]Action
}

// NewResolver 创建按键解析器
func NewResolver(bindings []KeyBinding) *Resolver {
	r := &Resolver{
		bindings: make(map[Layer]map[string]Action),
	}
	for _, b := range bindings {
		if _, ok := r.bindings[b.Layer]; !ok {
			r.bindings[b.Layer] = make(map[string]Action)
		}
		r.bindings[b.Layer][b.Key] = b.Action
	}
	return r
}

// Resolve 解析按键到动作
// key: 按键字符串（如 "ctrl+c", "enter"）
// currentLayer: 当前层
// 返回 (动作, 是否找到)
func (r *Resolver) Resolve(key string, currentLayer Layer) (Action, bool) {
	// 先查当前层
	if actions, ok := r.bindings[currentLayer]; ok {
		if action, found := actions[key]; found {
			return action, true
		}
	}

	// 再查全局层
	if currentLayer != LayerGlobal {
		if actions, ok := r.bindings[LayerGlobal]; ok {
			if action, found := actions[key]; found {
				return action, true
			}
		}
	}

	return "", false
}

// GetBindings 返回指定层的所有绑定
func (r *Resolver) GetBindings(layer Layer) map[string]Action {
	if actions, ok := r.bindings[layer]; ok {
		result := make(map[string]Action, len(actions))
		for k, v := range actions {
			result[k] = v
		}
		return result
	}
	return nil
}

// GetActionForKey 查找指定键在所有层中的动作
func (r *Resolver) GetActionForKey(key string) (Action, Layer, bool) {
	for layer, actions := range r.bindings {
		if action, ok := actions[key]; ok {
			return action, layer, true
		}
	}
	return "", "", false
}