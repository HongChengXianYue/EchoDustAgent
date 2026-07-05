package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

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
	db        *sql.DB
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

	store := Store{RootDir: rootDir, Workspace: workspace}

	// Open SQLite database
	dbPath := store.dbPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return Store{}, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return Store{}, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return Store{}, fmt.Errorf("set wal mode: %w", err)
	}

	// Create tables if they don't exist
	if err := store.createTables(db); err != nil {
		db.Close()
		return Store{}, fmt.Errorf("create tables: %w", err)
	}

	store.db = db

	// Auto-migrate legacy JSON files on first open
	if err := store.migrateLegacyJSON(); err != nil {
		_ = db.Close()
		return Store{}, fmt.Errorf("migrate legacy sessions: %w", err)
	}

	return store, nil
}

func (s Store) dbPath() string {
	if s.RootDir == "" || s.Workspace == "" {
		return ""
	}
	return filepath.Join(s.RootDir, "projects", slugify(s.Workspace), "sessions.db")
}

func (s Store) ProjectDir() string {
	if s.RootDir == "" || s.Workspace == "" {
		return ""
	}
	return filepath.Join(s.RootDir, "projects", slugify(s.Workspace))
}

func (s Store) createTables(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			workspace TEXT NOT NULL,
			title TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			model TEXT,
			wire_api TEXT,
			message_count INTEGER NOT NULL,
			last_user_preview TEXT,
			has_ui_snapshot INTEGER NOT NULL,
			state_json TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);
	`
	_, err := db.Exec(schema)
	return err
}

func (s Store) migrateLegacyJSON() error {
	projectDir := s.ProjectDir()
	if projectDir == "" {
		return nil
	}

	// Check if project directory exists
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Look for session directories (legacy format)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		sessionDir := filepath.Join(projectDir, sessionID)

		// Check if this session already exists in SQLite
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id = ?", sessionID).Scan(&count)
		if err != nil {
			return fmt.Errorf("check session existence: %w", err)
		}
		if count > 0 {
			continue // Already migrated
		}

		// Read legacy JSON files
		metaPath := filepath.Join(sessionDir, metaFileName)
		statePath := filepath.Join(sessionDir, stateFileName)

		meta, err := readJSONFile[Meta](metaPath)
		if err != nil {
			// Skip broken sessions
			continue
		}

		state, err := readJSONFile[State](statePath)
		if err != nil {
			// Skip broken sessions
			continue
		}

		// Insert into SQLite
		if err := s.insertSession(meta, state); err != nil {
			return fmt.Errorf("migrate session %s: %w", sessionID, err)
		}
	}

	return nil
}

func (s Store) insertSession(meta Meta, state State) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO sessions (
			session_id, workspace, title, created_at, updated_at,
			model, wire_api, message_count, last_user_preview,
			has_ui_snapshot, state_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		meta.SessionID,
		meta.Workspace,
		meta.Title,
		meta.CreatedAt.UTC().Format(time.RFC3339Nano),
		meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
		meta.Model,
		meta.WireAPI,
		meta.MessageCount,
		meta.LastUserPreview,
		boolToInt(meta.HasUISnapshot),
		string(stateJSON),
	)
	return err
}

func (s Store) List(limit int) ([]Meta, error) {
	if s.db == nil {
		return nil, fmt.Errorf("session store is not configured")
	}

	query := `
		SELECT session_id, workspace, title, created_at, updated_at,
		       model, wire_api, message_count, last_user_preview, has_ui_snapshot
		FROM sessions
		ORDER BY updated_at DESC, session_id DESC
	`
	args := []interface{}{}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metas := make([]Meta, 0)
	for rows.Next() {
		var meta Meta
		var createdAtStr, updatedAtStr string
		var hasUI int

		err := rows.Scan(
			&meta.SessionID,
			&meta.Workspace,
			&meta.Title,
			&createdAtStr,
			&updatedAtStr,
			&meta.Model,
			&meta.WireAPI,
			&meta.MessageCount,
			&meta.LastUserPreview,
			&hasUI,
		)
		if err != nil {
			return nil, err
		}

		meta.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		meta.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAtStr)
		meta.HasUISnapshot = hasUI != 0
		meta.Version = version

		metas = append(metas, meta)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return metas, nil
}

func (s Store) Load(sessionID string) (Record, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Record{}, fmt.Errorf("session id is required")
	}

	if s.db == nil {
		return Record{}, fmt.Errorf("session store is not configured")
	}

	var meta Meta
	var createdAtStr, updatedAtStr, stateJSON string
	var hasUI int

	err := s.db.QueryRow(`
		SELECT session_id, workspace, title, created_at, updated_at,
		       model, wire_api, message_count, last_user_preview, has_ui_snapshot, state_json
		FROM sessions
		WHERE session_id = ?
	`, sessionID).Scan(
		&meta.SessionID,
		&meta.Workspace,
		&meta.Title,
		&createdAtStr,
		&updatedAtStr,
		&meta.Model,
		&meta.WireAPI,
		&meta.MessageCount,
		&meta.LastUserPreview,
		&hasUI,
		&stateJSON,
	)

	if err == sql.ErrNoRows {
		return Record{}, os.ErrNotExist
	}
	if err != nil {
		return Record{}, err
	}

	meta.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
	meta.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAtStr)
	meta.HasUISnapshot = hasUI != 0
	meta.Version = version

	var state State
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return Record{}, fmt.Errorf("unmarshal state: %w", err)
	}

	return Record{Meta: meta, State: state}, nil
}

func (s Store) Save(request SaveRequest) (Meta, error) {
	if s.db == nil {
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

	if err := s.insertSession(meta, state); err != nil {
		return Meta{}, err
	}

	return meta, nil
}

func (s Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
