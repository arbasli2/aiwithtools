package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MissingDirIsEmptyManager(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "no-such-dir"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Errorf("expected empty, got %d skills", len(m.List()))
	}
}

func TestLoad_SkipsDirsWithoutSKILLmd(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "not-a-skill"))
	mustWrite(t, filepath.Join(dir, "real-skill", "SKILL.md"),
		"---\ndescription: hello\n---\nBody")
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 1 || m.List()[0].Name != "real-skill" {
		t.Errorf("expected 1 skill, got %+v", m.List())
	}
}

func TestLoad_ParsesFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "translate", "SKILL.md"), `---
name: translate
description: Translate text to a target language
---
Translate the text to {{ARGUMENTS}}.`)

	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get("translate")
	if s == nil {
		t.Fatal("translate skill missing")
	}
	if s.Description != "Translate text to a target language" {
		t.Errorf("desc = %q", s.Description)
	}
	if !strings.Contains(s.Body, "{{ARGUMENTS}}") {
		t.Errorf("body missing ARGUMENTS placeholder: %q", s.Body)
	}
}

func TestLoad_NoFrontmatterUsesDirName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "naked", "SKILL.md"), "Just a body, no frontmatter.")
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get("naked")
	if s == nil {
		t.Fatal("expected skill named after dir")
	}
	if s.Description != "" {
		t.Errorf("desc should be empty, got %q", s.Description)
	}
}

func TestRender_ArgumentsSubstituted(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "t", "SKILL.md"), `---
description: x
---
Hello, {{ARGUMENTS}}!`)
	m, _ := Load(dir)
	got, err := m.Render("t", "world")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello, world!" {
		t.Errorf("got %q", got)
	}
}

func TestRender_IncludeExpanded(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "r", "SKILL.md"), `---
description: x
---
Top.
{{include: references/checklist.md}}
End.`)
	mustWrite(t, filepath.Join(dir, "r", "references", "checklist.md"), "ITEM 1\nITEM 2")

	m, _ := Load(dir)
	got, err := m.Render("r", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ITEM 1\nITEM 2") {
		t.Errorf("include not expanded: %q", got)
	}
}

func TestRender_RejectsPathEscape(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "bad", "SKILL.md"), `---
description: x
---
{{include: ../etc/passwd}}`)
	m, _ := Load(dir)
	if _, err := m.Render("bad", ""); err == nil {
		t.Error("expected error for ../ include")
	}
}

func TestRender_RejectsAbsoluteInclude(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "bad", "SKILL.md"), `---
description: x
---
{{include: /etc/passwd}}`)
	m, _ := Load(dir)
	if _, err := m.Render("bad", ""); err == nil {
		t.Error("expected error for absolute include")
	}
}

func TestRender_NestedIncludesExpand(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "n", "SKILL.md"), `---
description: x
---
{{include: a.md}}`)
	mustWrite(t, filepath.Join(dir, "n", "a.md"), "A says: {{include: b.md}}")
	mustWrite(t, filepath.Join(dir, "n", "b.md"), "hello from B")
	m, _ := Load(dir)
	got, err := m.Render("n", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "A says: hello from B") {
		t.Errorf("nested include failed: %q", got)
	}
}

func TestRender_UnknownSkillError(t *testing.T) {
	m, _ := Load(t.TempDir())
	if _, err := m.Render("nope", ""); err == nil {
		t.Error("expected error for unknown skill")
	}
}

// helpers

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
