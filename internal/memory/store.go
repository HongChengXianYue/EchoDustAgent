package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Type classifies a stored memory.
type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
)

var validTypes = map[Type]bool{
	TypeUser:      true,
	TypeFeedback:  true,
	TypeProject:   true,
	TypeReference: true,
}

// Memory is one durable fact stored as Markdown.
type Memory struct {
	Name        string
	Title       string
	Description string
	Type        Type
	Body        string
}

// Store is the per-project auto-memory store.
type Store struct {
	Dir       string
	GlobalDir string
}

const indexFile = "MEMORY.md"

// StoreFor resolves the memory directories for a project under userDir.
func StoreFor(userDir, cwd string) Store {
	if strings.TrimSpace(userDir) == "" {
		return Store{}
	}
	return Store{
		Dir:       filepath.Join(userDir, "projects", slugify(absOf(cwd)), "memory"),
		GlobalDir: filepath.Join(userDir, "memory", "global"),
	}
}

func (s Store) dirs() []string {
	if s.GlobalDir != "" && s.GlobalDir != s.Dir {
		return []string{s.GlobalDir, s.Dir}
	}
	return []string{s.Dir}
}

func (s Store) dirFor(memoryType Type) string {
	if s.GlobalDir != "" && (memoryType == TypeUser || memoryType == TypeFeedback) {
		return s.GlobalDir
	}
	return s.Dir
}

// NormalizeType coerces a free-form type into a supported value.
func NormalizeType(value string) Type {
	memoryType := Type(strings.ToLower(strings.TrimSpace(value)))
	if validTypes[memoryType] {
		return memoryType
	}
	return TypeProject
}

// Save writes or updates one memory file and refreshes its index.
func (s Store) Save(memory Memory) (string, error) {
	memory.Type = NormalizeType(string(memory.Type))
	name := slug(firstNonEmpty(memory.Name, memory.Title, memory.Description))
	if name == "" {
		return "", fmt.Errorf("memory needs a name, title, or description")
	}
	memory.Name = name
	if strings.TrimSpace(memory.Title) == "" {
		memory.Title = titleFromSlug(name)
	}
	if strings.TrimSpace(memory.Description) == "" {
		return "", fmt.Errorf("memory description is required")
	}
	if strings.TrimSpace(memory.Body) == "" {
		return "", fmt.Errorf("memory body is required")
	}
	dir := s.dirFor(memory.Type)
	if dir == "" {
		return "", fmt.Errorf("memory store unavailable")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path, err := safeJoin(dir, name+".md")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(renderMemory(memory)), 0o644); err != nil {
		return "", err
	}
	if err := rewriteIndex(dir); err != nil {
		return path, err
	}
	for _, otherDir := range s.dirs() {
		if otherDir == "" || sameDir(otherDir, dir) {
			continue
		}
		otherPath, err := safeJoin(otherDir, name+".md")
		if err != nil {
			continue
		}
		if err := os.Remove(otherPath); err != nil && !os.IsNotExist(err) {
			return path, err
		}
		if err := rewriteIndex(otherDir); err != nil {
			return path, err
		}
	}
	return path, nil
}

// Archive removes an active memory and keeps a timestamped copy under .archive.
func (s Store) Archive(name string) (string, error) {
	name = slug(name)
	if name == "" {
		return "", fmt.Errorf("memory name is required")
	}
	if s.Dir == "" && s.GlobalDir == "" {
		return "", fmt.Errorf("memory store unavailable")
	}
	var archived string
	for _, dir := range s.dirs() {
		if dir == "" {
			continue
		}
		path, err := safeJoin(dir, name+".md")
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		archiveDir := filepath.Join(dir, ".archive")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return "", err
		}
		archivePath := filepath.Join(archiveDir, name+"-"+time.Now().UTC().Format("20060102T150405Z")+".md")
		if err := os.WriteFile(archivePath, data, 0o644); err != nil {
			return "", err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err := rewriteIndex(dir); err != nil {
			return "", err
		}
		archived = archivePath
	}
	return archived, nil
}

// Index returns the merged MEMORY.md contents in deterministic order.
func (s Store) Index() string {
	memories := s.List()
	if len(memories) == 0 {
		return ""
	}
	lines := make([]string, 0, len(memories))
	seen := map[string]bool{}
	for _, memory := range memories {
		if seen[memory.Name] {
			continue
		}
		seen[memory.Name] = true
		lines = append(lines, indexLine(memory))
	}
	return strings.Join(lines, "\n") + "\n"
}

// List returns all active memories, global memories first, then project memories.
func (s Store) List() []Memory {
	var memories []Memory
	seen := map[string]bool{}
	for _, dir := range s.dirs() {
		for _, memory := range listMemoriesIn(dir) {
			if seen[memory.Name] {
				continue
			}
			seen[memory.Name] = true
			memories = append(memories, memory)
		}
	}
	sort.SliceStable(memories, func(i, j int) bool {
		return memories[i].Name < memories[j].Name
	})
	return memories
}

// Read returns one memory by slug.
func (s Store) Read(name string) (Memory, bool) {
	name = slug(name)
	for _, memory := range s.List() {
		if memory.Name == name {
			return memory, true
		}
	}
	return Memory{}, false
}

// Search returns simple ranked text matches.
func (s Store) Search(query string, memoryType Type, limit int) []Memory {
	queryTerms := tokens(query)
	if len(queryTerms) == 0 {
		return nil
	}
	type scored struct {
		memory Memory
		score  int
	}
	var scoredMemories []scored
	for _, memory := range s.List() {
		if memoryType != "" && memory.Type != memoryType {
			continue
		}
		text := strings.ToLower(memory.Title + " " + memory.Description + " " + memory.Body)
		score := 0
		for _, term := range queryTerms {
			score += strings.Count(text, term)
		}
		if score > 0 {
			scoredMemories = append(scoredMemories, scored{memory: memory, score: score})
		}
	}
	sort.SliceStable(scoredMemories, func(i, j int) bool {
		if scoredMemories[i].score == scoredMemories[j].score {
			return scoredMemories[i].memory.Name < scoredMemories[j].memory.Name
		}
		return scoredMemories[i].score > scoredMemories[j].score
	})
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	if len(scoredMemories) > limit {
		scoredMemories = scoredMemories[:limit]
	}
	out := make([]Memory, 0, len(scoredMemories))
	for _, item := range scoredMemories {
		out = append(out, item.memory)
	}
	return out
}

func listMemoriesIn(dir string) []Memory {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var memories []Memory
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() == indexFile {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		memory, ok := parseMemory(string(data), strings.TrimSuffix(entry.Name(), ".md"))
		if ok {
			memories = append(memories, memory)
		}
	}
	sort.SliceStable(memories, func(i, j int) bool {
		return memories[i].Name < memories[j].Name
	})
	return memories
}

func rewriteIndex(dir string) error {
	if dir == "" {
		return nil
	}
	memories := listMemoriesIn(dir)
	indexPath := filepath.Join(dir, indexFile)
	if len(memories) == 0 {
		if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	lines := make([]string, 0, len(memories))
	for _, memory := range memories {
		lines = append(lines, indexLine(memory))
	}
	return os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func renderMemory(memory Memory) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeFrontmatter(&b, "name", memory.Name)
	writeFrontmatter(&b, "title", strings.TrimSpace(memory.Title))
	writeFrontmatter(&b, "description", strings.TrimSpace(memory.Description))
	writeFrontmatter(&b, "type", string(NormalizeType(string(memory.Type))))
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(memory.Body))
	b.WriteString("\n")
	return b.String()
}

func writeFrontmatter(b *strings.Builder, key string, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(strconv.Quote(value))
	b.WriteString("\n")
}

func parseMemory(text, fallbackName string) (Memory, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Memory{}, false
	}
	memory := Memory{Name: slug(fallbackName), Type: TypeProject, Body: trimmed}
	if strings.HasPrefix(trimmed, "---\n") || strings.HasPrefix(trimmed, "---\r\n") {
		parts := strings.SplitN(trimmed, "---", 3)
		if len(parts) == 3 {
			meta := parseFrontmatter(parts[1])
			memory.Name = slug(firstNonEmpty(meta["name"], fallbackName))
			memory.Title = meta["title"]
			memory.Description = meta["description"]
			memory.Type = NormalizeType(meta["type"])
			memory.Body = strings.TrimSpace(parts[2])
		}
	}
	if memory.Name == "" {
		return Memory{}, false
	}
	if strings.TrimSpace(memory.Title) == "" {
		memory.Title = titleFromSlug(memory.Name)
	}
	if strings.TrimSpace(memory.Description) == "" {
		memory.Description = oneLine(memory.Body)
	}
	return memory, true
}

func parseFrontmatter(text string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	return values
}

func indexLine(memory Memory) string {
	title := strings.TrimSpace(memory.Title)
	if title == "" {
		title = titleFromSlug(memory.Name)
	}
	description := oneLine(memory.Description)
	return fmt.Sprintf("- [%s](%s.md) (%s) - %s", title, memory.Name, NormalizeType(string(memory.Type)), description)
}

func slugify(path string) string {
	replacer := strings.NewReplacer(string(os.PathSeparator), "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(path)
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func titleFromSlug(value string) string {
	parts := strings.Split(slug(value), "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func oneLine(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	line := strings.Join(fields, " ")
	if len(line) > 180 {
		return line[:180] + "..."
	}
	return line
}

func tokens(value string) []string {
	seen := map[string]bool{}
	var out []string
	for _, field := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(field) < 2 || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func safeJoin(base, name string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("base directory is empty")
	}
	path := filepath.Join(base, name)
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes memory store")
	}
	return path, nil
}
