package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-agent/internal/llm"
)

// jsonMarshal is a test helper that marshals value to indented JSON bytes.
func jsonMarshal(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func TestStoreSaveLoadAndListRoundTrip(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 3, 23, 15, 30, 0, time.UTC)
	meta, err := store.Save(SaveRequest{
		SessionID: "20260703T231530Z-a1b2",
		CreatedAt: now,
		Now:       now.Add(2 * time.Minute),
		Model:     "gpt-5.5",
		WireAPI:   "responses",
		Conversation: []llm.Message{
			{Role: "user", Content: "Implement slash resume"},
			{Role: "assistant", Content: "Working on it"},
		},
		UI: &UISnapshot{
			Blocks: []TranscriptBlockSnapshot{
				{Kind: "user", Body: "Implement slash resume"},
				{Kind: "assistant", Body: "Working on it", Markdown: true},
			},
			Subagents: []SubagentSnapshot{
				{Index: 1, Task: "Inspect config", Status: "done", TokenTotal: 42},
			},
			Tokens: TokenSnapshot{Prompt: 10, Completion: 5, Total: 15, Cached: 2},
		},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if meta.SessionID != "20260703T231530Z-a1b2" {
		t.Fatalf("session id = %q", meta.SessionID)
	}
	if meta.Title != "Implement slash resume" {
		t.Fatalf("title = %q", meta.Title)
	}
	if !meta.HasUISnapshot {
		t.Fatalf("expected ui snapshot to be recorded")
	}

	record, err := store.Load(meta.SessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(record.State.Conversation) != 2 {
		t.Fatalf("conversation len = %d", len(record.State.Conversation))
	}
	if record.State.UI == nil || len(record.State.UI.Blocks) != 2 || len(record.State.UI.Subagents) != 1 {
		t.Fatalf("ui snapshot = %#v", record.State.UI)
	}

	metas, err := store.List(10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 1 || metas[0].SessionID != meta.SessionID {
		t.Fatalf("List() = %#v", metas)
	}
}

func TestStoreListSortsByUpdatedAtDescending(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	first := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	second := first.Add(5 * time.Minute)
	for _, item := range []struct {
		id  string
		now time.Time
	}{
		{id: "20260703T100000Z-aaaa", now: first},
		{id: "20260703T100500Z-bbbb", now: second},
	} {
		if _, err := store.Save(SaveRequest{
			SessionID: item.id,
			CreatedAt: item.now,
			Now:       item.now,
			Conversation: []llm.Message{
				{Role: "user", Content: item.id},
			},
		}); err != nil {
			t.Fatalf("Save(%s) error = %v", item.id, err)
		}
	}
	metas, err := store.List(0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("List() len = %d", len(metas))
	}
	if metas[0].SessionID != "20260703T100500Z-bbbb" || metas[1].SessionID != "20260703T100000Z-aaaa" {
		t.Fatalf("List() order = %#v", metas)
	}
}

func TestStoreLoadMissingSession(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	_, err = store.Load("missing")
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want os.ErrNotExist", err)
	}
}

// TestStoreSkipsBrokenMetaOnList verifies that legacy JSON directories with
// broken meta.json files are skipped during migration and don't appear in List.
func TestStoreSkipsBrokenMetaOnList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	projectDir := filepath.Join(root, "projects", slugify("/tmp/project"))
	if err := os.MkdirAll(filepath.Join(projectDir, "broken"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Write invalid JSON to simulate a broken legacy session
	if err := os.WriteFile(filepath.Join(projectDir, "broken", metaFileName), []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile(meta) error = %v", err)
	}

	store, err := OpenStore(root, "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	metas, err := store.List(0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("List() = %#v, want empty", metas)
	}
}

func TestOpenStoreExpandsHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, err := OpenStore("~/sessions", "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if !strings.HasPrefix(store.RootDir, home) {
		t.Fatalf("root dir = %q, want prefix %q", store.RootDir, home)
	}
}

// TestStoreAutoMigratesLegacyJSON verifies that legacy JSON session directories
// are automatically migrated to SQLite on first OpenStore call.
func TestStoreAutoMigratesLegacyJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	projectDir := filepath.Join(root, "projects", slugify("/tmp/project"))
	sessionDir := filepath.Join(projectDir, "20260703T120000Z-abcd")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Write legacy meta.json
	meta := Meta{
		Version:         1,
		SessionID:       "20260703T120000Z-abcd",
		Workspace:       "/tmp/project",
		Title:           "legacy session",
		CreatedAt:       time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 7, 3, 12, 5, 0, 0, time.UTC),
		Model:           "gpt-4",
		WireAPI:         "chat",
		MessageCount:    2,
		LastUserPreview: "hello",
		HasUISnapshot:   false,
	}
	if err := writeJSONFile(filepath.Join(sessionDir, metaFileName), meta); err != nil {
		t.Fatalf("write meta.json: %v", err)
	}

	// Write legacy state.json
	state := State{
		Version: 1,
		Conversation: []llm.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	if err := writeJSONFile(filepath.Join(sessionDir, stateFileName), state); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	// Open store should auto-migrate
	store, err := OpenStore(root, "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	// Verify the migrated session is accessible
	record, err := store.Load("20260703T120000Z-abcd")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record.Meta.Title != "legacy session" {
		t.Fatalf("title = %q", record.Meta.Title)
	}
	if len(record.State.Conversation) != 2 {
		t.Fatalf("conversation len = %d", len(record.State.Conversation))
	}
	if record.State.Conversation[0].Content != "hello" {
		t.Fatalf("first message = %q", record.State.Conversation[0].Content)
	}

	// Verify List returns the migrated session
	metas, err := store.List(0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 1 || metas[0].SessionID != "20260703T120000Z-abcd" {
		t.Fatalf("List() = %#v", metas)
	}
}

// TestStoreSaveOverwrite verifies that saving with the same session ID updates the record.
func TestStoreSaveOverwrite(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	if _, err := store.Save(SaveRequest{
		SessionID: "20260703T120000Z-aaaa",
		CreatedAt: now,
		Now:       now,
		Conversation: []llm.Message{
			{Role: "user", Content: "first"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Save again with same ID but different content
	later := now.Add(10 * time.Minute)
	if _, err := store.Save(SaveRequest{
		SessionID: "20260703T120000Z-aaaa",
		CreatedAt: now,
		Now:       later,
		Conversation: []llm.Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response"},
			{Role: "user", Content: "second"},
		},
	}); err != nil {
		t.Fatalf("Save() overwrite error = %v", err)
	}

	record, err := store.Load("20260703T120000Z-aaaa")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(record.State.Conversation) != 3 {
		t.Fatalf("conversation len = %d, want 3", len(record.State.Conversation))
	}
	if record.Meta.MessageCount != 3 {
		t.Fatalf("message_count = %d, want 3", record.Meta.MessageCount)
	}

	// List should still show only one session
	metas, err := store.List(0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("List() len = %d, want 1", len(metas))
	}
}

// TestStoreIdempotentMigration verifies that running migration twice does not
// duplicate or corrupt data.
func TestStoreIdempotentMigration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	projectDir := filepath.Join(root, "projects", slugify("/tmp/project"))
	sessionDir := filepath.Join(projectDir, "20260703T120000Z-abcd")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	meta := Meta{
		Version:         1,
		SessionID:       "20260703T120000Z-abcd",
		Workspace:       "/tmp/project",
		Title:           "legacy session",
		CreatedAt:       time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 7, 3, 12, 5, 0, 0, time.UTC),
		Model:           "gpt-4",
		WireAPI:         "chat",
		MessageCount:    2,
		LastUserPreview: "hello",
	}
	if err := writeJSONFile(filepath.Join(sessionDir, metaFileName), meta); err != nil {
		t.Fatalf("write meta.json: %v", err)
	}
	state := State{
		Version: 1,
		Conversation: []llm.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	if err := writeJSONFile(filepath.Join(sessionDir, stateFileName), state); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	// First open: migrates
	store1, err := OpenStore(root, "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() #1 error = %v", err)
	}
	store1.Close()

	// Second open: should not fail or duplicate
	store2, err := OpenStore(root, "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() #2 error = %v", err)
	}
	defer store2.Close()

	metas, err := store2.List(0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("List() len = %d, want 1 (idempotent)", len(metas))
	}
	if metas[0].SessionID != "20260703T120000Z-abcd" {
		t.Fatalf("SessionID = %q", metas[0].SessionID)
	}

	record, err := store2.Load("20260703T120000Z-abcd")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(record.State.Conversation) != 2 {
		t.Fatalf("conversation len = %d", len(record.State.Conversation))
	}
}

// writeJSONFile is a test helper for writing JSON files (legacy migration tests).
func writeJSONFile(path string, value any) error {
	data, err := jsonMarshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
