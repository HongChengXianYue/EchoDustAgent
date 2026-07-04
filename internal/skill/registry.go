package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Options struct {
	CWD        string
	UserDir    string
	ProjectDir string
}

type Registry struct {
	skills map[string]Skill
	order  []Skill
}

func LoadRegistry(options Options) (*Registry, error) {
	registry := &Registry{skills: map[string]Skill{}}
	cwd := absPath(strings.TrimSpace(options.CWD))
	userDir := resolveRoot(cwd, options.UserDir)
	projectDir := resolveRoot(cwd, options.ProjectDir)

	var loadErrs []error
	registry.loadRoot(userDir, SourceUser, &loadErrs)
	registry.loadRoot(projectDir, SourceProject, &loadErrs)
	registry.rebuildOrder()
	return registry, errors.Join(loadErrs...)
}

func (r *Registry) Empty() bool {
	return r == nil || len(r.order) == 0
}

func (r *Registry) All() []Skill {
	if r == nil {
		return nil
	}
	return append([]Skill(nil), r.order...)
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	skill, ok := r.skills[strings.ToLower(strings.TrimSpace(name))]
	return skill, ok
}

func (r *Registry) Retrieve(query string, topK int, minScore int) []Candidate {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 3
	}
	if minScore < 0 {
		minScore = 0
	}
	query = normalizeText(query)
	if query == "" {
		return nil
	}
	candidates := make([]Candidate, 0, len(r.order))
	for _, skill := range r.order {
		score := scoreSkill(query, skill)
		if score < minScore {
			continue
		}
		candidates = append(candidates, Candidate{Skill: skill, Score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return strings.ToLower(candidates[i].Skill.Name) < strings.ToLower(candidates[j].Skill.Name)
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates
}

func (r *Registry) loadRoot(root string, source Source, loadErrs *[]error) {
	if strings.TrimSpace(root) == "" {
		return
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		*loadErrs = append(*loadErrs, fmt.Errorf("scan skills root %s: %w", root, err))
		return
	}
	if !info.IsDir() {
		*loadErrs = append(*loadErrs, fmt.Errorf("skills root %s is not a directory", root))
		return
	}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			*loadErrs = append(*loadErrs, fmt.Errorf("walk skills root %s: %w", root, err))
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "skill.json" {
			return nil
		}
		skill, loadErr := loadSkill(path, source)
		if loadErr != nil {
			*loadErrs = append(*loadErrs, loadErr)
			return nil
		}
		r.skills[strings.ToLower(skill.Name)] = skill
		return nil
	})
}

func (r *Registry) rebuildOrder() {
	r.order = r.order[:0]
	for _, skill := range r.skills {
		r.order = append(r.order, skill)
	}
	sort.SliceStable(r.order, func(i, j int) bool {
		return strings.ToLower(r.order[i].Name) < strings.ToLower(r.order[j].Name)
	})
}

func resolveRoot(cwd string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if expanded, ok := expandHome(path); ok {
		path = expanded
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func expandHome(path string) (string, bool) {
	if path == "" || path[0] != '~' {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	if path == "~" {
		return home, true
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), true
	}
	return "", false
}

func absPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func normalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastSpace := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func scoreSkill(query string, skill Skill) int {
	score := 0
	name := normalizeText(skill.Name)
	description := normalizeText(skill.Description)
	summary := normalizeText(skill.Summary)
	triggerText := normalizeText(strings.Join(skill.Triggers, " "))
	if query == name {
		score += 400
	}
	if strings.Contains(query, name) || strings.Contains(name, query) {
		score += 180
	}
	for _, trigger := range skill.Triggers {
		triggerTextOne := normalizeText(trigger)
		if triggerTextOne != "" && (strings.Contains(query, triggerTextOne) || strings.Contains(triggerTextOne, query)) {
			score += 140
		}
	}
	for _, term := range uniqueTerms(query) {
		if !meaningfulTerm(term) {
			continue
		}
		if strings.Contains(name, term) {
			score += 70
		}
		if strings.Contains(summary, term) {
			score += 35
		}
		if strings.Contains(description, term) {
			score += 20
		}
		if strings.Contains(triggerText, term) {
			score += 45
		}
	}
	return score
}

func uniqueTerms(text string) []string {
	fields := strings.Fields(text)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func meaningfulTerm(term string) bool {
	runes := []rune(term)
	if len(runes) >= 2 {
		return true
	}
	return len(runes) == 1 && runes[0] > unicode.MaxASCII
}
