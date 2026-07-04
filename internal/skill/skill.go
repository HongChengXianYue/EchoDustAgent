package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Source string

const (
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

type Permissions struct {
	Tools []string `json:"tools,omitempty"`
}

type Manifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Summary     string          `json:"summary,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Permissions Permissions     `json:"permissions,omitempty"`
	Triggers    []string        `json:"triggers,omitempty"`
}

type Skill struct {
	Manifest
	Dir          string
	ManifestPath string
	SkillPath    string
	Source       Source
}

type Candidate struct {
	Skill Skill
	Score int
}

func loadSkill(manifestPath string, source Source) (Skill, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Skill{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Skill{}, fmt.Errorf("decode manifest %s: %w", manifestPath, err)
	}
	dir := filepath.Dir(manifestPath)
	skillPath := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		return Skill{}, fmt.Errorf("missing SKILL.md for %s: %w", manifestPath, err)
	}
	if err := normalizeManifest(&manifest); err != nil {
		return Skill{}, fmt.Errorf("invalid manifest %s: %w", manifestPath, err)
	}
	return Skill{
		Manifest:     manifest,
		Dir:          dir,
		ManifestPath: manifestPath,
		SkillPath:    skillPath,
		Source:       source,
	}, nil
}

func normalizeManifest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Summary = strings.TrimSpace(manifest.Summary)
	if manifest.Name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.ContainsAny(manifest.Name, "\r\n\t") {
		return fmt.Errorf("name must not contain control whitespace")
	}
	if manifest.Description == "" {
		return fmt.Errorf("description is required")
	}
	if manifest.Summary == "" {
		manifest.Summary = manifest.Description
	}
	if len(bytesTrimSpace(manifest.InputSchema)) == 0 {
		manifest.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	}
	if err := validateSchemaDefinition(manifest.InputSchema); err != nil {
		return fmt.Errorf("input_schema: %w", err)
	}
	manifest.Permissions.Tools = dedupeTrimmed(manifest.Permissions.Tools)
	manifest.Triggers = dedupeTrimmed(manifest.Triggers)
	return nil
}

func (s Skill) ReadBody() (string, error) {
	data, err := os.ReadFile(s.SkillPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s Skill) InputSchemaSummary() string {
	return schemaSummary(s.InputSchema)
}

func (s Skill) PermissionSummary() string {
	if len(s.Permissions.Tools) == 0 {
		return "none"
	}
	return strings.Join(s.Permissions.Tools, ", ")
}

func (s Skill) TriggerSummary() string {
	if len(s.Triggers) == 0 {
		return "none"
	}
	return strings.Join(s.Triggers, "; ")
}

func dedupeTrimmed(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
