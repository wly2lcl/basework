package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServerConfig 是 LSP 服务器配置。
type ServerConfig struct {
	Command string
	Args    []string
	Env     map[string]string
}

// Config 是 LSP Manager 的配置。
type Config struct {
	Servers map[string]ServerConfig
}

// DefaultServers 默认 LSP 服务器映射。
var DefaultServers = map[string]ServerConfig{
	"go":         {Command: "gopls"},
	"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
	"javascript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
	"python":     {Command: "pyright-langserver", Args: []string{"--stdio"}},
}

// extensionToLanguage 文件扩展名到语言名的映射。
var extensionToLanguage = map[string]string{
	".go":  "go",
	".ts":  "typescript",
	".tsx": "typescript",
	".js":  "javascript",
	".jsx": "javascript",
	".py":  "python",
}

// Detect 根据工作区文件自动检测需要的 LSP 服务器。
// 扫描工作区根目录的文件扩展名，检查对应 LSP 命令是否在 PATH 中。
func Detect(workspacePath string) map[string]ServerConfig {
	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		return nil
	}

	// 收集需要的语言集合
	langs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		lang, ok := extensionToLanguage[ext]
		if ok {
			langs[lang] = true
		}
	}

	if len(langs) == 0 {
		return nil
	}

	// 对每种语言，检查 LSP 命令是否在 PATH 中
	result := make(map[string]ServerConfig)
	for lang := range langs {
		cfg, ok := DefaultServers[lang]
		if !ok {
			continue
		}
		// 检查命令是否在 PATH 中
		if _, err := exec.LookPath(cfg.Command); err != nil {
			continue
		}
		result[lang] = cfg
	}

	return result
}
