package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeEmptyIncludesGuidelines(t *testing.T) {
	base := "BASE\nPROMPT"
	got := Compose(base, &Set{})
	if !strings.Contains(got, "Long-Term Memory Guidelines") {
		t.Fatalf("Compose empty missing guidelines: %q", got)
	}
	if !strings.HasPrefix(got, base) {
		t.Fatalf("Compose should preserve base prefix, got %q", got)
	}
}

func TestComposeNilIsIdentity(t *testing.T) {
	base := "BASE\nPROMPT"
	if got := Compose(base, nil); got != base {
		t.Fatalf("Compose nil = %q, want base", got)
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
	// Project/ancestor scope no longer auto-loads any filenames, so we exercise
	// the import mechanism through the local-scope AGENTS.local.md entry point.
	if err := os.WriteFile(filepath.Join(project, "AGENTS.local.md"), []byte("Local rule.\n@extra.md"), 0o644); err != nil {
		t.Fatalf("write AGENTS.local.md: %v", err)
	}

	set := Load(Options{CWD: project})
	block := set.Block()
	for _, want := range []string{"Local rule.", "Imported rule."} {
		if !strings.Contains(block, want) {
			t.Fatalf("memory block missing %q:\n%s", want, block)
		}
	}
}

func TestLoadDiscoversEchoDustCodeGlobalPrompt(t *testing.T) {
	userDir := t.TempDir()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "ECHO-DUST-CODE.md"), []byte("Global echo dust code rule."), 0o644); err != nil {
		t.Fatalf("write ECHO-DUST-CODE.md: %v", err)
	}

	set := Load(Options{CWD: project, UserDir: userDir})
	block := set.Block()
	if !strings.Contains(block, "Global echo dust code rule.") {
		t.Fatalf("memory block missing ECHO-DUST-CODE.md:\n%s", block)
	}
	if got := filepath.Base(set.DocPath(ScopeUser)); got != "ECHO-DUST-CODE.md" {
		t.Fatalf("DocPath user = %s, want ECHO-DUST-CODE.md", got)
	}
}

func TestLoadStillDiscoversLegacyLocalAgentGlobalPrompt(t *testing.T) {
	userDir := t.TempDir()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "LOCAL-AGENT.md"), []byte("Legacy local-agent rule."), 0o644); err != nil {
		t.Fatalf("write LOCAL-AGENT.md: %v", err)
	}

	set := Load(Options{CWD: project, UserDir: userDir})
	block := set.Block()
	if !strings.Contains(block, "Legacy local-agent rule.") {
		t.Fatalf("memory block missing legacy LOCAL-AGENT.md:\n%s", block)
	}
	if got := filepath.Base(set.DocPath(ScopeUser)); got != "LOCAL-AGENT.md" {
		t.Fatalf("DocPath user = %s, want LOCAL-AGENT.md", got)
	}
}

func TestDocPathPrefersExistingConvention(t *testing.T) {
	userDir := t.TempDir()
	// Both user-scope candidates exist; DocPath should pick the first one
	// listed in userDocNames (ECHO-DUST-CODE.md before LOCAL-AGENT.md).
	if err := os.WriteFile(filepath.Join(userDir, "ECHO-DUST-CODE.md"), []byte("Primary global rule."), 0o644); err != nil {
		t.Fatalf("write ECHO-DUST-CODE.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "LOCAL-AGENT.md"), []byte("Legacy global rule."), 0o644); err != nil {
		t.Fatalf("write LOCAL-AGENT.md: %v", err)
	}
	set := Load(Options{CWD: t.TempDir(), UserDir: userDir})
	if got := filepath.Base(set.DocPath(ScopeUser)); got != "ECHO-DUST-CODE.md" {
		t.Fatalf("DocPath user = %s, want ECHO-DUST-CODE.md", got)
	}
	// Local scope has no matching file in this test; DocPath falls back to the
	// canonical default filename.
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
