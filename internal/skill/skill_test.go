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
	createSkill(t, root, "reviewer", Manifest{
		Name:        "reviewer",
		Description: "Review code changes for bugs and regressions.",
		Triggers:    []string{"code review", "bug risk"},
		InputSchema: json.RawMessage(`{"type":"object","properties":{"focus":{"type":"string"}},"additionalProperties":false}`),
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
	createSkill(t, root, "reviewer", Manifest{
		Name:        "reviewer",
		Description: "Review code changes for bugs and regressions.",
		Triggers:    []string{"code review", "review pull request"},
	})
	createSkill(t, root, "formatter", Manifest{
		Name:        "formatter",
		Description: "Format source files and normalize styling.",
		Triggers:    []string{"format code"},
	})

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

func createSkill(t *testing.T, root string, dir string, manifest Manifest) {
	t.Helper()
	skillDir := filepath.Join(root, dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
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
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n\nBody"), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}
}
