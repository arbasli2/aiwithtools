package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Render expands template placeholders in the skill body:
//
//	{{ARGUMENTS}}            -> the args string the caller provided
//	{{include: <relpath>}}   -> contents of <relpath> resolved against
//	                            the skill folder, recursively expanded
//
// `args` may be empty (the agent-driven load_skill path doesn't pass
// arguments; only user-driven /<name> [args] does).
//
// Includes are guarded against path escapes (no leading slash, no `..`
// segments) and limited in depth to prevent runaway recursion.
func (m *Manager) Render(name, args string) (string, error) {
	s := m.Get(name)
	if s == nil {
		return "", fmt.Errorf("skill %q not found", name)
	}
	return renderWithDepth(s.Body, s.Dir, args, 0)
}

const maxIncludeDepth = 8

var includeRE = regexp.MustCompile(`\{\{\s*include:\s*([^}]+?)\s*\}\}`)

func renderWithDepth(body, dir, args string, depth int) (string, error) {
	if depth > maxIncludeDepth {
		return "", fmt.Errorf("include depth %d exceeded (cycle?)", maxIncludeDepth)
	}

	// {{ARGUMENTS}} first — simple substitution.
	body = strings.ReplaceAll(body, "{{ARGUMENTS}}", args)

	// {{include: path}} — find every match, replace with file contents.
	var firstErr error
	out := includeRE.ReplaceAllStringFunc(body, func(match string) string {
		if firstErr != nil {
			return match
		}
		sub := includeRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		rel := strings.TrimSpace(sub[1])
		if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
			firstErr = fmt.Errorf("invalid include path %q (must be relative, no `..`)", rel)
			return match
		}
		path := filepath.Join(dir, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			firstErr = fmt.Errorf("include %q: %w", rel, err)
			return match
		}
		// Allow nested includes inside included files.
		expanded, err := renderWithDepth(string(data), dir, args, depth+1)
		if err != nil {
			firstErr = err
			return match
		}
		return expanded
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}
