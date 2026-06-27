package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Set is all memory loaded for one agent session.
type Set struct {
	Docs    []Source
	Store   Store
	Index   string
	CWD     string
	UserDir string
}

// Options configures startup memory discovery.
type Options struct {
	CWD     string
	UserDir string
}

// Load discovers memory best-effort. Missing files simply produce an empty set.
func Load(options Options) *Set {
	cwd := strings.TrimSpace(options.CWD)
	if cwd == "" {
		cwd = "."
	}
	userDir := strings.TrimSpace(options.UserDir)
	if expanded, ok := expandHome(userDir); ok {
		userDir = expanded
	}
	if userDir != "" {
		userDir = absOf(userDir)
	}
	cwd = absOf(cwd)
	store := StoreFor(userDir, cwd)
	return &Set{
		Docs:    discoverDocs(cwd, userDir),
		Store:   store,
		Index:   store.Index(),
		CWD:     cwd,
		UserDir: userDir,
	}
}

// DocPath returns the canonical document-memory path for a scope.
func (s *Set) DocPath(scope Scope) string {
	if s == nil {
		return ""
	}
	dir := s.CWD
	names := docNames
	def := defaultDocName
	switch scope {
	case ScopeUser:
		if s.UserDir == "" {
			return ""
		}
		dir = s.UserDir
		names = userDocNames
		def = defaultUserDocName
	case ScopeLocal:
		names = localNames
		def = defaultLocalName
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(dir, def)
}

// Empty reports whether Compose can leave the base prompt byte-for-byte intact.
func (s *Set) Empty() bool {
	return s == nil
}

// Block renders deterministic Markdown for the system prompt.
func (s *Set) Block() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Memory\n\n")
	hasContent := false
	if len(s.Docs) > 0 {
		b.WriteString("Persistent context loaded from memory files. Treat it as durable, user-authored guidance for this project.\n")
		hasContent = true
		for _, doc := range s.Docs {
			fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", doc.Path, doc.Scope, strings.TrimSpace(doc.Body))
		}
	}
	if index := strings.TrimSpace(s.Index); index != "" {
		if hasContent {
			b.WriteString("\n")
		}
		b.WriteString("## Saved memories\n\n")
		b.WriteString("Saved durable facts from earlier sessions. Treat them as background context, not proof that the code still matches them. Use the `memory` tool to read a full fact when its index entry is relevant, and `forget` to archive stale ones.\n\n")
		b.WriteString(index)
		if s.Store.Dir != "" || s.Store.GlobalDir != "" {
			b.WriteString("\n\n(stored under ")
			b.WriteString(strings.Join(nonEmpty(s.Store.dirs()), " and "))
			b.WriteString(")\n")
		}
		hasContent = true
	}
	b.WriteString("\n")
	b.WriteString(memoryGuidelines)
	if !hasContent {
		// No docs or saved memories yet, but guidelines still apply
		b.WriteString("\nNo memories loaded yet. Use `remember` to start saving durable facts.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Compose appends memory after the stable base prompt, preserving the base as
// the most cacheable prefix.
func Compose(base string, set *Set) string {
	block := set.Block()
	if block == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimRight(base, "\n") + "\n\n" + block
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
