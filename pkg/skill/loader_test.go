package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// ── SKILL.md parsing tests ──────────────────────────────────────────────────

func TestParseSkillFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: my-skill
description: A test skill
---
Do something useful.

With multiple lines.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := parseSkillFile(path)
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if s.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", s.Name, "my-skill")
	}
	if s.Description != "A test skill" {
		t.Errorf("Description = %q, want %q", s.Description, "A test skill")
	}
	if s.Instructions != "Do something useful.\n\nWith multiple lines." {
		t.Errorf("Instructions = %q", s.Instructions)
	}
	if s.FilePath != path {
		t.Errorf("FilePath = %q, want %q", s.FilePath, path)
	}
	if s.Builtin {
		t.Errorf("Builtin should be false by default")
	}
}

func TestParseSkillFileWithMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: meta-skill
description: Skill with metadata
license: MIT
metadata:
  author: test
  version: "1.0"
---
Instructions here.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := parseSkillFile(path)
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if s.Name != "meta-skill" {
		t.Errorf("Name = %q, want %q", s.Name, "meta-skill")
	}
	if s.Metadata["license"] != "MIT" {
		t.Errorf("Metadata[license] = %q, want %q", s.Metadata["license"], "MIT")
	}
	if s.Metadata["metadata.author"] != "test" {
		t.Errorf("Metadata[metadata.author] = %q, want %q", s.Metadata["metadata.author"], "test")
	}
	if s.Metadata["metadata.version"] != "1.0" {
		t.Errorf("Metadata[metadata.version] = %q, want %q", s.Metadata["metadata.version"], "1.0")
	}
}

func TestParseSkillFileNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `Just some markdown without frontmatter.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseSkillFile(path)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseSkillFileNoName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
description: Missing name field
---
Instructions.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseSkillFile(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseSkillFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseSkillFile(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParseSkillFileIncompleteFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: test
description: no closing delimiter
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseSkillFile(path)
	if err == nil {
		t.Fatal("expected error for incomplete frontmatter")
	}
}

// ── Loader tests ────────────────────────────────────────────────────────────

func TestLoaderNew(t *testing.T) {
	l := NewLoader("/some/path")
	if l == nil {
		t.Fatal("NewLoader returned nil")
	}
	if l.Active() == nil {
		t.Fatal("Active() returned nil")
	}
	if len(l.Active()) != 0 {
		t.Errorf("Active() should be empty, got %d", len(l.Active()))
	}
	if l.Get("anything") != nil {
		t.Error("Get() on empty loader should return nil")
	}
}

func TestLoaderDiscoverFindsFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: my-test-skill
description: Found by discover
---
Do something.
`
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(dir)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	skills := l.Active()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-test-skill" {
		t.Errorf("skill Name = %q, want %q", skills[0].Name, "my-test-skill")
	}
	if skills[0].Builtin {
		t.Errorf("single-path skill should not be marked builtin")
	}

	if got := l.Get("my-test-skill"); got == nil {
		t.Error("Get('my-test-skill') returned nil")
	}
}

func TestLoaderUserOverridesBuiltin(t *testing.T) {
	// Create user path with a skill
	userDir := t.TempDir()
	userSkillDir := filepath.Join(userDir, "skills", "shared-skill")
	if err := os.MkdirAll(userSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	userContent := `---
name: shared-skill
description: User version
---
User instructions.
`
	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create builtin path with the same skill name
	builtinDir := t.TempDir()
	builtinSkillDir := filepath.Join(builtinDir, "skills", "shared-skill")
	if err := os.MkdirAll(builtinSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	builtinContent := `---
name: shared-skill
description: Builtin version
---
Builtin instructions.
`
	if err := os.WriteFile(filepath.Join(builtinSkillDir, "SKILL.md"), []byte(builtinContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Loader with user path first, builtin path second
	l := NewLoader(userDir, builtinDir)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	s := l.Get("shared-skill")
	if s == nil {
		t.Fatal("Get('shared-skill') returned nil")
	}
	if s.Description != "User version" {
		t.Errorf("expected user version (Description=%q), got %q", "User version", s.Description)
	}
	if s.Instructions != "User instructions." {
		t.Errorf("expected user instructions, got %q", s.Instructions)
	}
	if s.Builtin {
		t.Errorf("user skill should not be marked builtin")
	}
}

func TestLoaderBuiltinMarked(t *testing.T) {
	userDir := t.TempDir()
	builtinDir := t.TempDir()

	// Only the builtin path has the skill
	skillDir := filepath.Join(builtinDir, "skills", "builtin-only")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: builtin-only
description: Only in builtin
---
Do stuff.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(userDir, builtinDir)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	s := l.Get("builtin-only")
	if s == nil {
		t.Fatal("Get('builtin-only') returned nil")
	}
	if !s.Builtin {
		t.Errorf("skill from non-primary path should be marked builtin")
	}
}

func TestLoaderSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()

	// Valid skill
	validDir := filepath.Join(dir, "skills", "valid")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte("---\nname: valid\n---\nOK\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Invalid skill (no name)
	invalidDir := filepath.Join(dir, "skills", "invalid")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte("---\ndescription: no name\n---\nBad\n"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(dir)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	skills := l.Active()
	if len(skills) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(skills))
	}
	if skills[0].Name != "valid" {
		t.Errorf("expected 'valid', got %q", skills[0].Name)
	}
}

func TestLoaderMultiplePaths(t *testing.T) {
	path1 := t.TempDir()
	path2 := t.TempDir()

	// Skill A in path1
	dirA := filepath.Join(path1, "skills", "skill-a")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "SKILL.md"), []byte("---\nname: skill-a\ndescription: First path\n---\nA\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Skill B in path2
	dirB := filepath.Join(path2, "skills", "skill-b")
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "SKILL.md"), []byte("---\nname: skill-b\ndescription: Second path\n---\nB\n"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(path1, path2)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if s := l.Get("skill-a"); s == nil {
		t.Error("skill-a not found")
	}
	if s := l.Get("skill-b"); s == nil {
		t.Error("skill-b not found")
	}
	if len(l.Active()) != 2 {
		t.Errorf("expected 2 skills, got %d", len(l.Active()))
	}
}

func TestLoaderNonExistentPath(t *testing.T) {
	l := NewLoader("/nonexistent/path/that/does/not/exist")
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover on nonexistent path: %v", err)
	}
	if len(l.Active()) != 0 {
		t.Errorf("expected 0 skills from nonexistent path, got %d", len(l.Active()))
	}
}

func TestLoaderIgnoresNonSkillFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a file that is not SKILL.md
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Not a skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a SKILL.md not in a subdirectory
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: root-skill\n---\nRoot\n"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(dir)
	if err := l.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if s := l.Get("root-skill"); s == nil {
		t.Error("expected root SKILL.md to be discovered")
	}
}
