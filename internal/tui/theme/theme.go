// Package theme 提供 TUI 主题系统
package theme

// ColorRole 定义颜色角色——语义化的颜色标识符
type ColorRole string

const (
	ColorBackground  ColorRole = "background"
	ColorForeground  ColorRole = "foreground"
	ColorAccent      ColorRole = "accent"
	ColorBorder      ColorRole = "border"
	ColorStatusBar   ColorRole = "statusbar_bg"
	ColorStatusFg    ColorRole = "statusbar_fg"
	ColorUserMsg     ColorRole = "user_msg"
	ColorAssistMsg   ColorRole = "assistant_msg"
	ColorToolCall    ColorRole = "tool_call"
	ColorError       ColorRole = "error"
	ColorThinking    ColorRole = "thinking"
	ColorInput       ColorRole = "input"
	ColorInputPrefix ColorRole = "input_prefix"
	ColorHighlight   ColorRole = "highlight"
	ColorSelection   ColorRole = "selection"
	ColorSuccess     ColorRole = "success"
	ColorWarning     ColorRole = "warning"
)

// 所有必需的颜色角色
var requiredRoles = []ColorRole{
	ColorBackground, ColorForeground, ColorAccent, ColorBorder,
	ColorStatusBar, ColorStatusFg, ColorUserMsg, ColorAssistMsg,
	ColorToolCall, ColorError, ColorThinking, ColorInput,
	ColorInputPrefix, ColorHighlight, ColorSelection,
	ColorSuccess, ColorWarning,
}

// Theme 定义一套完整的颜色方案
type Theme struct {
	Name   string               `json:"name"`
	Colors map[ColorRole]string `json:"colors"`
}

// LipglossColor 返回 lipgloss.Color 值
func (t *Theme) LipglossColor(role ColorRole) string {
	if c, ok := t.Colors[role]; ok {
		return c
	}
	return "255" // 默认白色
}

// Get 返回颜色字符串
func (t *Theme) Get(role ColorRole) string {
	if c, ok := t.Colors[role]; ok {
		return c
	}
	return "255"
}

// darkThemeColors 深色主题颜色定义
var darkThemeColors = map[ColorRole]string{
	ColorBackground:  "236",
	ColorForeground:  "255",
	ColorAccent:      "63",
	ColorBorder:      "63",
	ColorStatusBar:   "63",
	ColorStatusFg:    "255",
	ColorUserMsg:     "39",
	ColorAssistMsg:   "255",
	ColorToolCall:    "214",
	ColorError:       "196",
	ColorThinking:    "245",
	ColorInput:       "255",
	ColorInputPrefix: "39",
	ColorHighlight:   "226",
	ColorSelection:   "63",
	ColorSuccess:     "120",
	ColorWarning:     "228",
}

// lightThemeColors 浅色主题颜色定义
var lightThemeColors = map[ColorRole]string{
	ColorBackground:  "255",
	ColorForeground:  "16",
	ColorAccent:      "27",
	ColorBorder:      "27",
	ColorStatusBar:   "27",
	ColorStatusFg:    "255",
	ColorUserMsg:     "19",
	ColorAssistMsg:   "16",
	ColorToolCall:    "166",
	ColorError:       "160",
	ColorThinking:    "102",
	ColorInput:       "16",
	ColorInputPrefix: "19",
	ColorHighlight:   "21",
	ColorSelection:   "27",
	ColorSuccess:     "28",
	ColorWarning:     "130",
}

// draculaThemeColors Dracula 主题颜色定义
var draculaThemeColors = map[ColorRole]string{
	ColorBackground:  "235",
	ColorForeground:  "255",
	ColorAccent:      "141",
	ColorBorder:      "141",
	ColorStatusBar:   "141",
	ColorStatusFg:    "235",
	ColorUserMsg:     "117",
	ColorAssistMsg:   "255",
	ColorToolCall:    "215",
	ColorError:       "196",
	ColorThinking:    "248",
	ColorInput:       "255",
	ColorInputPrefix: "117",
	ColorHighlight:   "228",
	ColorSelection:   "141",
	ColorSuccess:     "84",
	ColorWarning:     "228",
}

// monokaiThemeColors Monokai 主题颜色定义
var monokaiThemeColors = map[ColorRole]string{
	ColorBackground:  "233",
	ColorForeground:  "255",
	ColorAccent:      "141",
	ColorBorder:      "141",
	ColorStatusBar:   "141",
	ColorStatusFg:    "233",
	ColorUserMsg:     "81",
	ColorAssistMsg:   "255",
	ColorToolCall:    "222",
	ColorError:       "197",
	ColorThinking:    "248",
	ColorInput:       "255",
	ColorInputPrefix: "81",
	ColorHighlight:   "228",
	ColorSelection:   "141",
	ColorSuccess:     "83",
	ColorWarning:     "228",
}

// 内置主题
var (
	DarkTheme = &Theme{
		Name:   "dark",
		Colors: darkThemeColors,
	}
	LightTheme = &Theme{
		Name:   "light",
		Colors: lightThemeColors,
	}
	DraculaTheme = &Theme{
		Name:   "dracula",
		Colors: draculaThemeColors,
	}
	MonokaiTheme = &Theme{
		Name:   "monokai",
		Colors: monokaiThemeColors,
	}
	DefaultTheme = DarkTheme
)

var builtinThemes map[string]*Theme

func init() {
	builtinThemes = make(map[string]*Theme)
	for _, t := range []*Theme{DarkTheme, LightTheme, DraculaTheme, MonokaiTheme} {
		builtinThemes[t.Name] = t
	}
}

// GetTheme 根据名称获取主题
func GetTheme(name string) (*Theme, bool) {
	t, ok := builtinThemes[name]
	return t, ok
}

// ListThemes 列出所有可用主题名
func ListThemes() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	return names
}
