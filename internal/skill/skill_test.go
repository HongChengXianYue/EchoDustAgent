package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistryLoadsMetadataWithoutReadingSkillBody(t *testing.T) {
	root := t.TempDir()
	createSkillDir(t, root, "reviewer", "# Skill\n\nBody")
	writeRootRegistry(t, root, registryEntry{
		Path: "reviewer",
		Manifest: Manifest{
			Name:        "reviewer",
			Description: "Review code changes for bugs and regressions.",
			Triggers:    []string{"code review", "bug risk"},
			InputSchema: json.RawMessage(`{"type":"object","properties":{"focus":{"type":"string"}},"additionalProperties":false}`),
		},
	})
	skillPath := filepath.Join(root, "reviewer", "SKILL.md")
	if err := os.Chmod(skillPath, 0o000); err != nil {
		t.Fatalf("chmod skill body: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillPath, 0o644) })

	registry, err := LoadRegistry(Options{CWD: root, ProjectDir: "."})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	if registry.Empty() {
		t.Fatal("registry should not be empty")
	}
	skill, ok := registry.Get("reviewer")
	if !ok {
		t.Fatal("reviewer skill not found")
	}
	if skill.Summary != skill.Description {
		t.Fatalf("summary = %q, want description fallback", skill.Summary)
	}
	if _, err := skill.ReadBody(); err == nil {
		t.Fatal("ReadBody() error = nil, want permission error")
	}
}

func TestRegistryRetrieveRanksByNameDescriptionAndTriggers(t *testing.T) {
	root := t.TempDir()
	createSkillDir(t, root, "reviewer", "# Skill\n\nBody")
	createSkillDir(t, root, "formatter", "# Skill\n\nBody")
	writeRootRegistry(t, root,
		registryEntry{
			Path: "reviewer",
			Manifest: Manifest{
				Name:        "reviewer",
				Description: "Review code changes for bugs and regressions.",
				Triggers:    []string{"code review", "review pull request"},
			},
		},
		registryEntry{
			Path: "formatter",
			Manifest: Manifest{
				Name:        "formatter",
				Description: "Format source files and normalize styling.",
				Triggers:    []string{"format code"},
			},
		},
	)

	registry, err := LoadRegistry(Options{CWD: root, ProjectDir: "."})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	candidates := registry.Retrieve("please review this diff for bugs", 2, 200)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want 1 match", candidates)
	}
	if candidates[0].Skill.Name != "reviewer" {
		t.Fatalf("top candidate = %q, want reviewer", candidates[0].Skill.Name)
	}
}

func TestLoadRegistryMergesRootRegistryAndSkillOverride(t *testing.T) {
	root := t.TempDir()
	createSkillDir(t, root, "reviewer", "# Skill\n\nBody")
	writeRootRegistry(t, root, registryEntry{
		Path: "reviewer",
		Manifest: Manifest{
			Name:        "reviewer",
			Description: "Registry description.",
			Summary:     "Registry summary.",
			Triggers:    []string{"code review"},
		},
	})
	writeSkillManifest(t, filepath.Join(root, "reviewer"), Manifest{
		Description: "Local description.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"focus":{"type":"string"}},"additionalProperties":false}`),
		Permissions: Permissions{Tools: []string{"read_file", "git_diff"}},
	})

	registry, err := LoadRegistry(Options{CWD: root, ProjectDir: "."})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	skill, ok := registry.Get("reviewer")
	if !ok {
		t.Fatal("reviewer skill not found")
	}
	if skill.Description != "Local description." {
		t.Fatalf("description = %q, want local override", skill.Description)
	}
	if skill.Summary != "Registry summary." {
		t.Fatalf("summary = %q, want registry summary retained", skill.Summary)
	}
	if got := strings.Join(skill.Permissions.Tools, ","); got != "git_diff,read_file" {
		t.Fatalf("permissions = %q, want git_diff,read_file", got)
	}
	if skill.ManifestPath != filepath.Join(root, "reviewer", "skill.json") {
		t.Fatalf("manifest path = %q, want local skill.json", skill.ManifestPath)
	}
}

func TestLoadRegistryFallsBackToBodyOnlySkill(t *testing.T) {
	root := t.TempDir()
	createSkillDir(t, root, "reviewer", "# Skill\n\nBody")

	registry, err := LoadRegistry(Options{CWD: root, ProjectDir: "."})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	skill, ok := registry.Get("reviewer")
	if !ok {
		t.Fatal("reviewer skill not found")
	}
	if skill.Name != "reviewer" {
		t.Fatalf("name = %q, want reviewer", skill.Name)
	}
	if !strings.Contains(skill.Description, "Add metadata in registry.json or skill.json") {
		t.Fatalf("description = %q, want fallback metadata hint", skill.Description)
	}
}

func TestLoadRegistrySupportsRootSkillJSONAlias(t *testing.T) {
	root := t.TempDir()
	createSkillDir(t, root, "reviewer", "# Skill\n\nBody")
	writeRootSkillJSONAlias(t, root, registryEntry{
		Path: "reviewer",
		Manifest: Manifest{
			Name:        "reviewer",
			Description: "Alias registry description.",
		},
	})

	registry, err := LoadRegistry(Options{CWD: root, ProjectDir: "."})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	skill, ok := registry.Get("reviewer")
	if !ok {
		t.Fatal("reviewer skill not found")
	}
	if skill.Description != "Alias registry description." {
		t.Fatalf("description = %q, want alias registry description", skill.Description)
	}
}

func TestValidateInputEnforcesRequiredAndAdditionalProperties(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"path":{"type":"string"},
			"retries":{"type":"integer"}
		},
		"required":["path"],
		"additionalProperties":false
	}`)
	if err := ValidateInput(schema, json.RawMessage(`{"path":"README.md","retries":2}`)); err != nil {
		t.Fatalf("ValidateInput(valid) error = %v", err)
	}
	if err := ValidateInput(schema, json.RawMessage(`{"retries":2}`)); err == nil || !strings.Contains(err.Error(), "input.path is required") {
		t.Fatalf("ValidateInput(missing required) error = %v", err)
	}
	if err := ValidateInput(schema, json.RawMessage(`{"path":"README.md","extra":true}`)); err == nil || !strings.Contains(err.Error(), "input.extra is not allowed") {
		t.Fatalf("ValidateInput(extra property) error = %v", err)
	}
}

func createSkillDir(t *testing.T, root string, dir string, body string) {
	t.Helper()
	skillDir := filepath.Join(root, dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		body = "# Skill\n\nBody"
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}
}

func writeSkillManifest(t *testing.T, skillDir string, manifest Manifest) {
	t.Helper()
	if len(manifest.InputSchema) == 0 {
		manifest.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func writeRootRegistry(t *testing.T, root string, entries ...registryEntry) {
	t.Helper()
	writeRootRegistryFile(t, filepath.Join(root, "registry.json"), entries...)
}

func writeRootSkillJSONAlias(t *testing.T, root string, entries ...registryEntry) {
	t.Helper()
	writeRootRegistryFile(t, filepath.Join(root, "skill.json"), entries...)
}

func writeRootRegistryFile(t *testing.T, path string, entries ...registryEntry) {
	t.Helper()
	data, err := json.MarshalIndent(map[string]any{
		"skills": entries,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}
