package skill

import (
	"encoding/json"
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

type registryFile struct {
	Skills []registryEntry `json:"skills"`
}

type registryEntry struct {
	Path string `json:"path,omitempty"`
	Manifest
}

type skillCandidate struct {
	root         string
	source       Source
	dir          string
	relativePath string
	bodyPath     string
	manifestPath string
	aggregated   Manifest
	hasMetadata  bool
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
	return registry, joinErrors(loadErrs)
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
	candidates := map[string]*skillCandidate{}
	registryEntries, registryPath, err := loadRootRegistryFile(root)
	if err != nil {
		*loadErrs = append(*loadErrs, err)
	}
	for _, entry := range registryEntries {
		if mergeErr := mergeRegistryEntry(candidates, root, source, registryPath, entry); mergeErr != nil {
			*loadErrs = append(*loadErrs, mergeErr)
		}
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
		if path == filepath.Join(root, "registry.json") || path == filepath.Join(root, "skill.json") {
			return nil
		}
		switch entry.Name() {
		case "SKILL.md":
			candidate := ensureCandidate(candidates, root, source, filepath.Dir(path))
			candidate.bodyPath = path
		case "skill.json":
			manifest, manifestErr := decodeManifestFile(path)
			if manifestErr != nil {
				*loadErrs = append(*loadErrs, manifestErr)
				return nil
			}
			candidate := ensureCandidate(candidates, root, source, filepath.Dir(path))
			candidate.applyManifest(manifest, path)
		}
		return nil
	})
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		skill, buildErr := candidates[key].build()
		if buildErr != nil {
			*loadErrs = append(*loadErrs, buildErr)
			continue
		}
		r.skills[strings.ToLower(skill.Name)] = skill
	}
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

func loadRootRegistryFile(root string) ([]registryEntry, string, error) {
	for _, filename := range []string{"registry.json", "skill.json"} {
		path := filepath.Join(root, filename)
		entries, exists, err := decodeRootRegistryFile(path)
		if !exists {
			continue
		}
		return entries, path, err
	}
	return nil, "", nil
}

func decodeRootRegistryFile(path string) ([]registryEntry, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("read skill registry %s: %w", path, err)
	}
	var file registryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, true, fmt.Errorf("decode skill registry %s: %w", path, err)
	}
	if file.Skills == nil {
		return nil, true, fmt.Errorf("invalid skill registry %s: top-level skills array is required", path)
	}
	for i := range file.Skills {
		if err := sanitizeManifest(&file.Skills[i].Manifest); err != nil {
			return nil, true, fmt.Errorf("invalid skill registry %s skill[%d]: %w", path, i, err)
		}
		file.Skills[i].Path = strings.TrimSpace(file.Skills[i].Path)
	}
	return file.Skills, true, nil
}

func mergeRegistryEntry(candidates map[string]*skillCandidate, root string, source Source, registryPath string, entry registryEntry) error {
	location := strings.TrimSpace(entry.Path)
	if location == "" {
		location = entry.Name
	}
	if location == "" {
		return fmt.Errorf("invalid skill registry %s: each skill needs path or name", registryPath)
	}
	dir, bodyPath, err := resolveSkillLocation(root, location)
	if err != nil {
		return fmt.Errorf("invalid skill registry %s entry %q: %w", registryPath, location, err)
	}
	candidate := ensureCandidate(candidates, root, source, dir)
	if bodyPath != "" {
		candidate.bodyPath = bodyPath
	}
	candidate.applyManifest(entry.Manifest, registryPath)
	return nil
}

func ensureCandidate(candidates map[string]*skillCandidate, root string, source Source, dir string) *skillCandidate {
	dir = filepath.Clean(dir)
	candidate, ok := candidates[dir]
	if ok {
		return candidate
	}
	relative := dir
	if rel, err := filepath.Rel(root, dir); err == nil {
		relative = rel
	}
	candidate = &skillCandidate{
		root:         root,
		source:       source,
		dir:          dir,
		relativePath: strings.TrimSpace(relative),
	}
	candidates[dir] = candidate
	return candidate
}

func (c *skillCandidate) applyManifest(manifest Manifest, metadataPath string) {
	c.aggregated = mergeManifest(c.aggregated, manifest)
	if strings.TrimSpace(metadataPath) != "" {
		c.manifestPath = metadataPath
	}
	c.hasMetadata = true
}

func (c *skillCandidate) build() (Skill, error) {
	if c == nil {
		return Skill{}, fmt.Errorf("skill candidate is nil")
	}
	bodyPath := strings.TrimSpace(c.bodyPath)
	if bodyPath == "" {
		bodyPath = filepath.Join(c.dir, "SKILL.md")
	}
	if _, err := os.Stat(bodyPath); err != nil {
		return Skill{}, fmt.Errorf("missing SKILL.md for %s: %w", c.dir, err)
	}
	manifest := defaultManifestForCandidate(c)
	manifest = mergeManifest(manifest, c.aggregated)
	if err := normalizeManifest(&manifest); err != nil {
		return Skill{}, fmt.Errorf("invalid skill metadata for %s: %w", c.dir, err)
	}
	return Skill{
		Manifest:     manifest,
		Dir:          c.dir,
		ManifestPath: c.manifestPath,
		SkillPath:    bodyPath,
		Source:       c.source,
	}, nil
}

func defaultManifestForCandidate(candidate *skillCandidate) Manifest {
	name := filepath.Base(candidate.dir)
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		name = "skill"
	}
	location := strings.TrimSpace(candidate.relativePath)
	if location == "" {
		location = filepath.Base(candidate.dir)
	}
	return Manifest{
		Name:        name,
		Description: fmt.Sprintf("Skill at %s. Add metadata in registry.json or skill.json for better retrieval.", location),
	}
}

func resolveSkillLocation(root string, location string) (string, string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(location) {
		location = filepath.Join(root, location)
	}
	location = filepath.Clean(location)
	lower := strings.ToLower(location)
	if strings.HasSuffix(lower, ".md") {
		return filepath.Dir(location), location, nil
	}
	return location, filepath.Join(location, "SKILL.md"), nil
}

func joinErrors(errs []error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		parts := make([]string, 0, len(filtered))
		for _, err := range filtered {
			parts = append(parts, err.Error())
		}
		return errors.New(strings.Join(parts, "; "))
	}
}
