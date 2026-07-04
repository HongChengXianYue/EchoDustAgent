package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"local-agent/internal/llm"
)

const (
	metaFileName  = "meta.json"
	stateFileName = "state.json"
	version       = 1
)

type Meta struct {
	Version         int       `json:"version"`
	SessionID       string    `json:"session_id"`
	Workspace       string    `json:"workspace"`
	Title           string    `json:"title,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Model           string    `json:"model,omitempty"`
	WireAPI         string    `json:"wire_api,omitempty"`
	MessageCount    int       `json:"message_count"`
	LastUserPreview string    `json:"last_user_preview,omitempty"`
	HasUISnapshot   bool      `json:"has_ui_snapshot,omitempty"`
}

type State struct {
	Version      int           `json:"version"`
	Conversation []llm.Message `json:"conversation,omitempty"`
	UI           *UISnapshot   `json:"ui,omitempty"`
}

type Record struct {
	Meta  Meta
	State State
}

type UISnapshot struct {
	Blocks    []TranscriptBlockSnapshot `json:"blocks,omitempty"`
	Subagents []SubagentSnapshot        `json:"subagents,omitempty"`
	Tokens    TokenSnapshot             `json:"tokens,omitempty"`
}

type TranscriptBlockSnapshot struct {
	Kind     string `json:"kind"`
	Title    string `json:"title,omitempty"`
	Body     string `json:"body,omitempty"`
	Markdown bool   `json:"markdown,omitempty"`
}

type SubagentSnapshot struct {
	Index      int                       `json:"index"`
	Task       string                    `json:"task,omitempty"`
	Status     string                    `json:"status,omitempty"`
	LastTitle  string                    `json:"last_title,omitempty"`
	Prompt     int                       `json:"prompt,omitempty"`
	TokenTotal int                       `json:"token_total,omitempty"`
	Cached     int                       `json:"cached,omitempty"`
	Blocks     []TranscriptBlockSnapshot `json:"blocks,omitempty"`
}

type TokenSnapshot struct {
	Prompt     int `json:"prompt,omitempty"`
	Completion int `json:"completion,omitempty"`
	Total      int `json:"total,omitempty"`
	Cached     int `json:"cached,omitempty"`
}

type SaveRequest struct {
	SessionID    string
	CreatedAt    time.Time
	Model        string
	WireAPI      string
	Conversation []llm.Message
	UI           *UISnapshot
	Now          time.Time
}

type Store struct {
	RootDir   string
	Workspace string
}

func OpenStore(rootDir, workspace string) (Store, error) {
	rootDir = absPath(expandHome(strings.TrimSpace(rootDir)))
	workspace = absPath(workspace)
	if rootDir == "" {
		return Store{}, fmt.Errorf("session root dir is required")
	}
	if workspace == "" {
		return Store{}, fmt.Errorf("workspace is required")
	}
	return Store{RootDir: rootDir, Workspace: workspace}, nil
}

func (s Store) ProjectDir() string {
	if s.RootDir == "" || s.Workspace == "" {
		return ""
	}
	return filepath.Join(s.RootDir, "projects", slugify(s.Workspace))
}

func (s Store) List(limit int) ([]Meta, error) {
	projectDir := s.ProjectDir()
	if projectDir == "" {
		return nil, fmt.Errorf("session store is not configured")
	}
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	metas := make([]Meta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath, err := safeJoin(filepath.Join(projectDir, entry.Name()), metaFileName)
		if err != nil {
			continue
		}
		meta, err := readJSONFile[Meta](metaPath)
		if err != nil || strings.TrimSpace(meta.SessionID) == "" {
			continue
		}
		metas = append(metas, meta)
	}
	sort.SliceStable(metas, func(i, j int) bool {
		if metas[i].UpdatedAt.Equal(metas[j].UpdatedAt) {
			return metas[i].SessionID > metas[j].SessionID
		}
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	if limit > 0 && len(metas) > limit {
		metas = metas[:limit]
	}
	return metas, nil
}

func (s Store) Load(sessionID string) (Record, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Record{}, fmt.Errorf("session id is required")
	}
	sessionDir, err := safeJoin(s.ProjectDir(), sessionID)
	if err != nil {
		return Record{}, err
	}
	metaPath, err := safeJoin(sessionDir, metaFileName)
	if err != nil {
		return Record{}, err
	}
	statePath, err := safeJoin(sessionDir, stateFileName)
	if err != nil {
		return Record{}, err
	}
	meta, err := readJSONFile[Meta](metaPath)
	if err != nil {
		return Record{}, err
	}
	state, err := readJSONFile[State](statePath)
	if err != nil {
		return Record{}, err
	}
	return Record{Meta: meta, State: state}, nil
}

func (s Store) Save(request SaveRequest) (Meta, error) {
	projectDir := s.ProjectDir()
	if projectDir == "" {
		return Meta{}, fmt.Errorf("session store is not configured")
	}
	now := request.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		var err error
		sessionID, err = newSessionID(now)
		if err != nil {
			return Meta{}, err
		}
	}
	createdAt := request.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	state := State{
		Version:      version,
		Conversation: copyMessages(request.Conversation),
	}
	if request.UI != nil {
		snapshot := cloneUISnapshot(*request.UI)
		state.UI = &snapshot
	}
	meta := Meta{
		Version:         version,
		SessionID:       sessionID,
		Workspace:       s.Workspace,
		Title:           titleFromConversation(state.Conversation, sessionID),
		CreatedAt:       createdAt,
		UpdatedAt:       now,
		Model:           strings.TrimSpace(request.Model),
		WireAPI:         strings.TrimSpace(request.WireAPI),
		MessageCount:    len(state.Conversation),
		LastUserPreview: latestUserPreview(state.Conversation),
		HasUISnapshot:   state.UI != nil,
	}
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return Meta{}, err
	}
	sessionDir, err := safeJoin(projectDir, sessionID)
	if err != nil {
		return Meta{}, err
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return Meta{}, err
	}
	metaPath, err := safeJoin(sessionDir, metaFileName)
	if err != nil {
		return Meta{}, err
	}
	statePath, err := safeJoin(sessionDir, stateFileName)
	if err != nil {
		return Meta{}, err
	}
	if err := writeJSONAtomic(metaPath, meta); err != nil {
		return Meta{}, err
	}
	if err := writeJSONAtomic(statePath, state); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func copyMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for i := range out {
		if len(messages[i].ToolCalls) > 0 {
			out[i].ToolCalls = append([]llm.ToolCall(nil), messages[i].ToolCalls...)
		}
	}
	return out
}

func cloneUISnapshot(snapshot UISnapshot) UISnapshot {
	out := snapshot
	out.Blocks = append([]TranscriptBlockSnapshot(nil), snapshot.Blocks...)
	out.Subagents = make([]SubagentSnapshot, 0, len(snapshot.Subagents))
	for _, subagent := range snapshot.Subagents {
		copySubagent := subagent
		copySubagent.Blocks = append([]TranscriptBlockSnapshot(nil), subagent.Blocks...)
		out.Subagents = append(out.Subagents, copySubagent)
	}
	return out
}

func readJSONFile[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func newSessionID(now time.Time) (string, error) {
	var suffix [2]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(suffix[:]), nil
}

func titleFromConversation(messages []llm.Message, sessionID string) string {
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		title := collapseWhitespace(strings.TrimSpace(message.Content))
		if title != "" {
			return truncateUTF8(title, 80)
		}
	}
	return "Session " + sessionID
}

func latestUserPreview(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		preview := collapseWhitespace(strings.TrimSpace(messages[i].Content))
		if preview != "" {
			return truncateUTF8(preview, 120)
		}
	}
	return ""
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func slugify(path string) string {
	replacer := strings.NewReplacer(string(os.PathSeparator), "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(path)
}

func safeJoin(base, name string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("base directory is empty")
	}
	path := filepath.Join(base, name)
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes session store")
	}
	return path, nil
}

func expandHome(path string) string {
	switch {
	case path == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	case strings.HasPrefix(path, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	default:
		return path
	}
}

func absPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
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
