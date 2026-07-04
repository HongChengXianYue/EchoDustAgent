// Package memory implements cache-friendly persistent memory for echo dust code.
//
// It follows the Reasonix shape: project/user memory files are discovered once
// at startup and folded into the system prompt, while saved facts live in plain
// Markdown files that can be listed, searched, updated, and archived through
// tools.
package memory

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Scope labels where a memory document was discovered.
type Scope string

const (
	ScopeUser     Scope = "user"
	ScopeAncestor Scope = "ancestor"
	ScopeProject  Scope = "project"
	ScopeLocal    Scope = "local"
)

// docNames lists project/ancestor-scoped document filenames scanned at startup.
// REASONIX.md, AGENTS.md and CLAUDE.md used to live here but have been removed
// on purpose: the project no longer auto-loads those filenames. The slice is
// kept (instead of deleted) so DocPath and discoverDocs keep their shape for
// future additions.
var docNames = []string{}

// Keep the legacy LOCAL-AGENT.md fallback so existing user setups continue to
// load after the branding rename. REASONIX.md, AGENTS.md and CLAUDE.md were
// intentionally removed from this list alongside docNames.
var userDocNames = []string{"ECHO-DUST-CODE.md", "LOCAL-AGENT.md"}
var localNames = []string{"REASONIX.local.md", "AGENTS.local.md", "CLAUDE.local.md"}

const (
	defaultDocName     = "AGENTS.md"
	defaultUserDocName = "ECHO-DUST-CODE.md"
	defaultLocalName   = "AGENTS.local.md"
	maxImportDepth     = 5
)

// Source is one loaded memory document with provenance.
type Source struct {
	Path  string
	Scope Scope
	Body  string
}

func discoverDocs(cwd, userDir string) []Source {
	var out []Source
	seen := docSeen{}
	if userDir != "" {
		out = append(out, loadFrom(userDir, userDocNames, ScopeUser, &seen)...)
	}
	for _, dir := range ancestorsToRoot(cwd) {
		scope := ScopeAncestor
		if sameDir(dir, cwd) {
			scope = ScopeProject
		}
		out = append(out, loadFrom(dir, docNames, scope, &seen)...)
	}
	out = append(out, loadFrom(cwd, localNames, ScopeLocal, &seen)...)
	return out
}

func loadFrom(dir string, names []string, scope Scope, seen *docSeen) []Source {
	var out []Source
	for _, name := range names {
		path := filepath.Join(dir, name)
		body, info, ok := readDoc(path)
		if !ok {
			continue
		}
		if seen != nil && !seen.add(info) {
			continue
		}
		body = resolveImports(body, dir, map[string]bool{absOf(path): true}, 0)
		out = append(out, Source{Path: path, Scope: scope, Body: body})
	}
	return out
}

func readDoc(path string) (string, os.FileInfo, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", nil, false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", nil, false
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return "", nil, false
	}
	return body, info, true
}

type docSeen struct {
	infos []os.FileInfo
}

func (s *docSeen) add(info os.FileInfo) bool {
	for _, previous := range s.infos {
		if os.SameFile(previous, info) {
			return false
		}
	}
	s.infos = append(s.infos, info)
	return true
}

func ancestorsToRoot(cwd string) []string {
	abs := absOf(cwd)
	root := gitRoot(abs)
	if root == "" {
		return []string{abs}
	}
	var chain []string
	for dir := abs; ; dir = filepath.Dir(dir) {
		chain = append(chain, dir)
		if sameDir(dir, root) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func gitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveImports(body, baseDir string, seen map[string]bool, depth int) string {
	if depth >= maxImportDepth {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		target, ok := importTarget(line)
		if !ok {
			continue
		}
		path := resolvePath(target, baseDir)
		abs := absOf(path)
		if seen[abs] {
			lines[i] = line + "  <!-- skipped: import cycle -->"
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		seen[abs] = true
		lines[i] = resolveImports(strings.TrimSpace(string(data)), filepath.Dir(path), seen, depth+1)
		delete(seen, abs)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func importTarget(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "@") || strings.Contains(trimmed, " ") || len(trimmed) == 1 {
		return "", false
	}
	return strings.TrimPrefix(trimmed, "@"), true
}

func resolvePath(path, baseDir string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(baseDir, path)
}

func absOf(path string) string {
	if expanded, ok := expandHome(path); ok {
		path = expanded
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func expandHome(path string) (string, bool) {
	switch {
	case path == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return path, false
		}
		return home, true
	case strings.HasPrefix(path, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return path, false
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), true
	default:
		return path, false
	}
}

func sameDir(left, right string) bool {
	return absOf(left) == absOf(right)
}
