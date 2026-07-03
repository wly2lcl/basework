package skill

import (
	"testing"
)

func TestSkillStruct(t *testing.T) {
	s := &Skill{
		Name:         "test-skill",
		Description:  "A test skill",
		Instructions: "Do something useful.",
		FilePath:     "/some/path/SKILL.md",
		Builtin:      false,
		Metadata:     map[string]string{"version": "1.0"},
	}

	if s.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", s.Name, "test-skill")
	}
	if s.Description != "A test skill" {
		t.Errorf("Description = %q, want %q", s.Description, "A test skill")
	}
	if s.Instructions != "Do something useful." {
		t.Errorf("Instructions = %q, want %q", s.Instructions, "Do something useful.")
	}
	if s.FilePath != "/some/path/SKILL.md" {
		t.Errorf("FilePath = %q, want %q", s.FilePath, "/some/path/SKILL.md")
	}
	if s.Builtin {
		t.Errorf("Builtin = true, want false")
	}
	if v := s.Metadata["version"]; v != "1.0" {
		t.Errorf("Metadata[version] = %q, want %q", v, "1.0")
	}
}

func TestToPromptXMLEmpty(t *testing.T) {
	if got := ToPromptXML(nil); got != "" {
		t.Errorf("ToPromptXML(nil) = %q, want empty", got)
	}
	if got := ToPromptXML([]*Skill{}); got != "" {
		t.Errorf("ToPromptXML([]) = %q, want empty", got)
	}
}

func TestToPromptXMLSingle(t *testing.T) {
	skills := []*Skill{
		{
			Name:         "test",
			Instructions: "Do something useful.",
		},
	}

	want := `<skills>
  <skill name="test">
Do something useful.
  </skill>
</skills>`
	if got := ToPromptXML(skills); got != want {
		t.Errorf("ToPromptXML:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestToPromptXMLMultiple(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Instructions: "Skill A instructions."},
		{Name: "b", Instructions: "Skill B instructions."},
	}

	got := ToPromptXML(skills)
	if len(got) == 0 {
		t.Fatal("ToPromptXML returned empty string")
	}

	// Check both skill names appear
	if !contains(got, `name="a"`) {
		t.Errorf("output missing skill 'a':\n%s", got)
	}
	if !contains(got, `name="b"`) {
		t.Errorf("output missing skill 'b':\n%s", got)
	}
	if !contains(got, "Skill A instructions") {
		t.Errorf("output missing instructions for 'a':\n%s", got)
	}
	if !contains(got, "Skill B instructions") {
		t.Errorf("output missing instructions for 'b':\n%s", got)
	}
}

func TestToPromptXMLTrimsInstructions(t *testing.T) {
	skills := []*Skill{
		{
			Name:         "ws",
			Instructions: "  \nLine one\n\nLine two\n  ",
		},
	}

	got := ToPromptXML(skills)
	// The instructions should be trimmed, so no leading/trailing whitespace
	if contains(got, "  \nLine one") {
		t.Errorf("instructions should be trimmed, got:\n%s", got)
	}
	if !contains(got, "Line one") || !contains(got, "Line two") {
		t.Errorf("instructions content missing:\n%s", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
