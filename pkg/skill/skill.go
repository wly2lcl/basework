// Package skill provides loading, parsing, and XML serialization of SKILL.md files.
package skill

import (
	"fmt"
	"strings"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Name         string
	Description  string
	Instructions string
	FilePath     string
	Builtin      bool
	Metadata     map[string]string
}

// ToPromptXML generates an XML representation of the given skills for system
// prompt injection. Each skill is wrapped in a <skill> element with a name
// attribute; the instructions body is placed as text content.
func ToPromptXML(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<skills>\n")
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("  <skill name=\"%s\">\n", s.Name))
		b.WriteString(strings.TrimSpace(s.Instructions))
		b.WriteString("\n  </skill>\n")
	}
	b.WriteString("</skills>")
	return b.String()
}