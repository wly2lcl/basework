package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// projectProvider 检测项目类型和关键文件信息。
type projectProvider struct{}

func (p *projectProvider) Name() string {
	return "project"
}

func (p *projectProvider) Collect() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	projectType := p.detectType(dir)
	keyFiles := p.findKeyFiles(dir)

	var parts []string
	if projectType != "" {
		parts = append(parts, "Project type: "+projectType)
	}
	if len(keyFiles) > 0 {
		parts = append(parts, "Key files: "+strings.Join(keyFiles, ", "))
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n"), nil
}

// detectType 根据项目文件检测项目类型。
func (p *projectProvider) detectType(dir string) string {
	indicators := []struct {
		file string
		typ  string
	}{
		{"go.mod", "Go"},
		{"package.json", "JavaScript/TypeScript"},
		{"Cargo.toml", "Rust"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Gemfile", "Ruby"},
		{"build.gradle", "Java/Kotlin"},
		{"pom.xml", "Java/Maven"},
		{"composer.json", "PHP"},
		{"CMakeLists.txt", "C/C++"},
		{"Makefile", "Generic"},
	}

	for _, ind := range indicators {
		if _, err := os.Stat(filepath.Join(dir, ind.file)); err == nil {
			return ind.typ
		}
	}
	return ""
}

// findKeyFiles 查找项目中的关键配置文件。
func (p *projectProvider) findKeyFiles(dir string) []string {
	var files []string
	candidates := []string{
		".gitignore", ".editorconfig", "Dockerfile",
		"docker-compose.yml", "Makefile", "README.md",
	}

	for _, f := range candidates {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			files = append(files, f)
		}
	}
	return files
}