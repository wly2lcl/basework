package skill

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and manages Skill definitions from SKILL.md files across
// multiple search paths. Earlier paths take precedence for same-name skills.
type Loader struct {
	paths  []string
	skills map[string]*Skill
}

// NewLoader creates a Loader that searches the given paths in order. The first
// path is treated as the user path and takes precedence over builtin paths.
func NewLoader(paths ...string) *Loader {
	return &Loader{
		paths:  paths,
		skills: make(map[string]*Skill),
	}
}

// Discover scans the configured search paths for SKILL.md files, parses them,
// and populates the loader's skill map. User paths override builtin paths for
// skills with the same name. Invalid files are skipped with a warning logged.
func (l *Loader) Discover() error {
	for _, base := range l.paths {
		info, err := os.Stat(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("[skill] warning: cannot stat search path %q: %v", base, err)
			continue
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.Walk(base, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible entries
			}
			if fi.IsDir() || fi.Name() != "SKILL.md" {
				return nil
			}

			skill, err := parseSkillFile(path)
			if err != nil {
				log.Printf("[skill] warning: skipping invalid SKILL.md %q: %v", path, err)
				return nil
			}

			// Determine if this is a builtin path (not the first path)
			isBuiltin := len(l.paths) > 0 && base != l.paths[0]

			// Dedup: user path overrides builtin
			existing, exists := l.skills[skill.Name]
			if exists && !existing.Builtin {
				// Already have a user version, skip
				return nil
			}

			skill.Builtin = isBuiltin
			l.skills[skill.Name] = skill
			return nil
		})
		if err != nil {
			log.Printf("[skill] warning: error walking path %q: %v", base, err)
		}
	}
	return nil
}

// Active returns all discovered skills as a slice.
func (l *Loader) Active() []*Skill {
	all := make([]*Skill, 0, len(l.skills))
	for _, s := range l.skills {
		all = append(all, s)
	}
	return all
}

// Get returns the skill with the given name, or nil if not found.
func (l *Loader) Get(name string) *Skill {
	return l.skills[name]
}

// ── SKILL.md parser ────────────────────────────────────────────────────────

// parseSkillFile reads a SKILL.md file, extracts YAML frontmatter and the
// markdown body, and returns a Skill.
func parseSkillFile(path string) (*Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, 64)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, errSkillEmpty
	}

	// Must start with YAML frontmatter delimiter
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, errSkillNoFrontmatter
	}

	// Find closing delimiter
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return nil, errSkillNoFrontmatter
	}

	// Parse YAML frontmatter lines (between first and second ---)
	frontmatter := lines[1:endIdx]
	meta := parseFrontmatter(frontmatter)
	name := meta["name"]
	description := meta["description"]

	if name == "" {
		return nil, errSkillNoName
	}

	// Extract metadata keys that aren't name/description
	metadata := make(map[string]string)
	for k, v := range meta {
		if k != "name" && k != "description" {
			metadata[k] = v
		}
	}

	// The remaining lines after the closing --- are the instructions body
	body := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))

	return &Skill{
		Name:         name,
		Description:  description,
		Instructions: body,
		FilePath:     path,
		Metadata:     metadata,
	}, nil
}

// parseFrontmatter parses simple YAML-like key-value lines. It handles:
//   - key: value  (scalar values)
//   - Nested keys under a parent key are stored as "parent.child" = value
//
// It does not support arrays or complex nested structures.
func parseFrontmatter(lines []string) map[string]string {
	result := make(map[string]string)
	var currentKey string

	for _, line := range lines {
		raw := line
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Indented line → continuation under currentKey
		if len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') && currentKey != "" {
			if idx := strings.Index(trimmed, ":"); idx >= 0 {
				subKey := strings.TrimSpace(trimmed[:idx])
				subVal := strings.TrimSpace(trimmed[idx+1:])
				subVal = stripQuotes(subVal)
				result[currentKey+"."+subKey] = subVal
			}
			continue
		}

		// Top-level key: value pair
		if idx := strings.Index(trimmed, ":"); idx >= 0 {
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+1:])
			val = stripQuotes(val)
			currentKey = key
			result[key] = val
		}
	}
	return result
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Sentinel errors used during parsing.
var (
	errSkillEmpty         = fmtE("skill file is empty")
	errSkillNoFrontmatter = fmtE("missing or malformed YAML frontmatter (---)")
	errSkillNoName        = fmtE("skill has no name in frontmatter")
)

func fmtE(msg string) error {
	return &parseError{msg: msg}
}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
