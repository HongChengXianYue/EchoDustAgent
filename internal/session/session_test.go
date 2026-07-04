package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-agent/internal/llm"
)

func TestStoreSaveLoadAndListRoundTrip(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
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
	_, err = store.Load("missing")
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestStoreSkipsBrokenMetaOnList(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sessions"), "/tmp/project")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	sessionDir := filepath.Join(store.ProjectDir(), "broken")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, metaFileName), []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile(meta) error = %v", err)
	}
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
	if !strings.HasPrefix(store.RootDir, home) {
		t.Fatalf("root dir = %q, want prefix %q", store.RootDir, home)
	}
}
