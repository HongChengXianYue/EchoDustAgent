package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeEmptyIsIdentity(t *testing.T) {
	base := "BASE\nPROMPT"
	if got := Compose(base, &Set{}); got != base {
		t.Fatalf("Compose empty = %q, want base", got)
	}
}

func TestLoadDiscoversDocsAndImports(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "extra.md"), []byte("Imported rule."), 0o644); err != nil {
		t.Fatalf("write import: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("Project rule.\n@extra.md"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.local.md"), []byte("Local rule."), 0o644); err != nil {
		t.Fatalf("write AGENTS.local.md: %v", err)
	}

	set := Load(Options{CWD: project})
	block := set.Block()
	for _, want := range []string{"Project rule.", "Imported rule.", "Local rule."} {
		if !strings.Contains(block, want) {
			t.Fatalf("memory block missing %q:\n%s", want, block)
		}
	}
}

func TestLoadDiscoversLocalAgentGlobalPrompt(t *testing.T) {
	userDir := t.TempDir()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "LOCAL-AGENT.md"), []byte("Global local-agent rule."), 0o644); err != nil {
		t.Fatalf("write LOCAL-AGENT.md: %v", err)
	}

	set := Load(Options{CWD: project, UserDir: userDir})
	block := set.Block()
	if !strings.Contains(block, "Global local-agent rule.") {
		t.Fatalf("memory block missing LOCAL-AGENT.md:\n%s", block)
	}
	if got := filepath.Base(set.DocPath(ScopeUser)); got != "LOCAL-AGENT.md" {
		t.Fatalf("DocPath user = %s, want LOCAL-AGENT.md", got)
	}
}

func TestDocPathPrefersExistingConvention(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("Claude rule."), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	set := Load(Options{CWD: project})
	if got := filepath.Base(set.DocPath(ScopeProject)); got != "CLAUDE.md" {
		t.Fatalf("DocPath project = %s, want CLAUDE.md", got)
	}
	if got := filepath.Base(set.DocPath(ScopeLocal)); got != "AGENTS.local.md" {
		t.Fatalf("DocPath local = %s, want AGENTS.local.md", got)
	}
}

func TestStoreSaveIndexSearchReadAndArchive(t *testing.T) {
	userDir := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	store := StoreFor(userDir, project)

	path, err := store.Save(Memory{
		Name:        "Prefers Tabs",
		Title:       "Prefers tabs",
		Description: "User prefers tab indentation",
		Type:        TypeUser,
		Body:        "Use tabs when editing Go-adjacent configuration examples.",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.HasPrefix(path, store.GlobalDir) {
		t.Fatalf("user memory path = %s, want under %s", path, store.GlobalDir)
	}
	if index := store.Index(); !strings.Contains(index, "prefers-tabs.md") {
		t.Fatalf("Index missing memory:\n%s", index)
	}
	if hits := store.Search("tab indentation", TypeUser, 8); len(hits) != 1 || hits[0].Name != "prefers-tabs" {
		t.Fatalf("Search hits = %#v", hits)
	}
	memory, ok := store.Read("prefers-tabs")
	if !ok || !strings.Contains(memory.Body, "tabs") {
		t.Fatalf("Read() = %#v, %v", memory, ok)
	}
	archive, err := store.Archive("prefers-tabs")
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if archive == "" {
		t.Fatalf("Archive path is empty")
	}
	if index := store.Index(); strings.Contains(index, "prefers-tabs") {
		t.Fatalf("Index still contains archived memory:\n%s", index)
	}
}
