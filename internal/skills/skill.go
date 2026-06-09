// Package skills implements the Anthropic Agent Skills standard:
// each skill is a directory containing SKILL.md with YAML frontmatter
// (name, description). Lightweight metadata is exposed to the model
// so it can request a full skill body on demand via the load_skill
// built-in tool. Skills can also be invoked explicitly by the user
// via `/<name> [args]` slash commands.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is one entry under ~/.config/aiwithtools/skills/<name>/.
type Skill struct {
	Name        string // derived from directory name unless frontmatter overrides
	Description string // frontmatter description
	Body        string // raw SKILL.md body after frontmatter, before template expansion
	Dir         string // absolute path to the skill folder, used for {{include}} resolution
}

// Manager holds the discovered skills indexed by name.
type Manager struct {
	skills map[string]*Skill
	order  []string // discovery order, alphabetised for stable listing
}

// NewEmpty returns a Manager with no skills. Useful as a fallback when
// Load fails and the caller wants a usable (no-op) manager rather than
// handling nil throughout.
func NewEmpty() *Manager {
	return &Manager{skills: map[string]*Skill{}}
}

// Load scans the given directory for skills. Each subdirectory containing
// a SKILL.md becomes one skill. Returns an empty manager (no error) when
// the directory doesn't exist, so a missing config dir is not fatal.
func Load(dir string) (*Manager, error) {
	m := &Manager{skills: map[string]*Skill{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		mdPath := filepath.Join(skillDir, "SKILL.md")
		data, err := os.ReadFile(mdPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // not a skill, just a folder
			}
			return nil, fmt.Errorf("read %s: %w", mdPath, err)
		}
		s, err := parseSkill(data, e.Name(), skillDir)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", mdPath, err)
		}
		m.skills[s.Name] = s
		m.order = append(m.order, s.Name)
	}
	sort.Strings(m.order)
	return m, nil
}

// List returns all loaded skills in stable (alphabetical) order.
func (m *Manager) List() []*Skill {
	out := make([]*Skill, 0, len(m.order))
	for _, n := range m.order {
		out = append(out, m.skills[n])
	}
	return out
}

// Get returns the skill by name, or nil if not found.
func (m *Manager) Get(name string) *Skill {
	return m.skills[name]
}

// Names returns just the names, used to populate the load_skill tool's
// enum / description.
func (m *Manager) Names() []string {
	return append([]string(nil), m.order...)
}

// frontmatter is the YAML head we parse out of SKILL.md.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseSkill(data []byte, dirName, dir string) (*Skill, error) {
	body, fm, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	s := &Skill{
		Name:        fm.Name,
		Description: strings.TrimSpace(fm.Description),
		Body:        body,
		Dir:         dir,
	}
	if s.Name == "" {
		s.Name = dirName // fall back to directory name
	}
	return s, nil
}

// splitFrontmatter extracts a `---`-fenced YAML head if present.
// Returns body (everything after the second ---), the parsed
// frontmatter, and any error. Tolerant of both LF and CRLF line
// endings on both the opening and closing fences.
func splitFrontmatter(data []byte) (string, frontmatter, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		// no frontmatter
		return strings.TrimLeft(text, " \t\r\n"), frontmatter{}, nil
	}
	rest := strings.TrimPrefix(text, "---\n")
	rest = strings.TrimPrefix(rest, "---\r\n")

	// Find the closing fence, accepting either LF or CRLF before it.
	endLF := strings.Index(rest, "\n---")
	endCRLF := strings.Index(rest, "\r\n---")
	end, fenceLen := endLF, len("\n---")
	if endCRLF >= 0 && (end < 0 || endCRLF < end) {
		end, fenceLen = endCRLF, len("\r\n---")
	}
	if end < 0 {
		return "", frontmatter{}, fmt.Errorf("frontmatter opened with --- but never closed")
	}
	yamlPart := rest[:end]
	body := rest[end+fenceLen:]
	body = strings.TrimLeft(body, "\r\n")

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return "", frontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	return body, fm, nil
}
