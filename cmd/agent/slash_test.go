package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-agent/internal/agent"
	"local-agent/internal/config"
	"local-agent/internal/llm"
	"local-agent/internal/session"
	"local-agent/internal/tools"
	"local-agent/internal/ui"
)

func newTestSlashRouter(t *testing.T) (*slashRouter, *sessionRuntime) {
	t.Helper()
	startup := ui.StartupInfo{
		Workdir: "/tmp/project",
		Model:   "demo-model",
		WireAPI: "responses",
		LogFile: "/tmp/project/agent.log",
	}
	codingAgent := agent.NewWithWorkspace(nil, tools.NewRegistry(), 4, "/tmp/project")
	sessions, err := newSessionRuntime(config.SessionConfig{
		Enabled: true,
		Dir:     t.TempDir(),
	}, "/tmp/project", codingAgent, &startup)
	if err != nil {
		t.Fatalf("newSessionRuntime() error = %v", err)
	}
	router := newSlashRouter(&startup, sessions, func() bool { return false })
	return router, sessions
}

func TestDispatchSlashTextInfo(t *testing.T) {
	router := newSlashRouter(&ui.StartupInfo{
		Workdir: "/tmp/project",
		Model:   "demo-model",
		WireAPI: "responses",
		LogFile: "/tmp/project/agent.log",
	}, nil, func() bool { return false })

	output, handled, shouldExit := router.DispatchText("/info")
	if !handled {
		t.Fatal("expected /info to be handled")
	}
	if shouldExit {
		t.Fatal("did not expect /info to request exit")
	}
	for _, want := range []string{"/tmp/project", "demo-model", "responses", "session id:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("/info output missing %q: %q", want, output)
		}
	}
}

func TestDispatchSlashTextUnknownCommand(t *testing.T) {
	router := newSlashRouter(&ui.StartupInfo{}, nil, func() bool { return false })
	output, handled, shouldExit := router.DispatchText("/missing")
	if !handled {
		t.Fatal("expected unknown slash command to be handled")
	}
	if shouldExit {
		t.Fatal("did not expect unknown slash command to exit")
	}
	if !strings.Contains(output, "unknown command: /missing") || !strings.Contains(output, "available commands:") {
		t.Fatalf("unexpected unknown command output: %q", output)
	}
}

func TestResumeListsRecentSessions(t *testing.T) {
	router, sessions := newTestSlashRouter(t)
	now := time.Date(2026, 7, 3, 23, 15, 30, 0, time.UTC)
	store := sessions.store
	if _, err := (*store).Save(session.SaveRequest{
		SessionID: "20260703T231530Z-a1b2",
		CreatedAt: now,
		Now:       now,
		Conversation: []llm.Message{
			{Role: "user", Content: "first"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := (*store).Save(session.SaveRequest{
		SessionID: "20260703T231830Z-c3d4",
		CreatedAt: now.Add(3 * time.Minute),
		Now:       now.Add(3 * time.Minute),
		Conversation: []llm.Message{
			{Role: "user", Content: "second"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	output, handled, shouldExit := router.DispatchText("/resume")
	if !handled || shouldExit {
		t.Fatalf("/resume handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "recent sessions:") || !strings.Contains(output, "20260703T231830Z-c3d4") {
		t.Fatalf("unexpected /resume output: %q", output)
	}
}

func TestResumeLatestAndPrefix(t *testing.T) {
	router, sessions := newTestSlashRouter(t)
	now := time.Date(2026, 7, 3, 23, 15, 30, 0, time.UTC)
	store := sessions.store
	for _, item := range []struct {
		id    string
		title string
		when  time.Time
	}{
		{id: "20260703T231530Z-a1b2", title: "older", when: now},
		{id: "20260703T231830Z-c3d4", title: "newer", when: now.Add(3 * time.Minute)},
	} {
		if _, err := (*store).Save(session.SaveRequest{
			SessionID: item.id,
			CreatedAt: item.when,
			Now:       item.when,
			Conversation: []llm.Message{
				{Role: "user", Content: item.title},
			},
		}); err != nil {
			t.Fatalf("Save(%s) error = %v", item.id, err)
		}
	}

	output, handled, shouldExit := router.DispatchText("/resume latest")
	if !handled || shouldExit {
		t.Fatalf("/resume latest handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "20260703T231830Z-c3d4") {
		t.Fatalf("unexpected latest output: %q", output)
	}
	if sessions.CurrentSessionID() != "20260703T231830Z-c3d4" {
		t.Fatalf("current session id = %q", sessions.CurrentSessionID())
	}

	output, handled, shouldExit = router.DispatchText("/resume 20260703T231530Z")
	if !handled || shouldExit {
		t.Fatalf("/resume prefix handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "20260703T231530Z-a1b2") {
		t.Fatalf("unexpected prefix output: %q", output)
	}
}

func TestResumeRestoresLegacySyntheticSystemMessages(t *testing.T) {
	router, sessions := newTestSlashRouter(t)
	now := time.Date(2026, 7, 4, 0, 15, 30, 0, time.UTC)
	store := sessions.store
	if _, err := (*store).Save(session.SaveRequest{
		SessionID: "20260704T001530Z-a1b2",
		CreatedAt: now,
		Now:       now,
		Conversation: []llm.Message{
			{Role: "user", Content: "inspect project"},
			{Role: "system", Content: "Background delegate_task result.\nSubagent-1 task: inspect\nStatus: completed"},
			{Role: "assistant", Content: "done"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	output, handled, shouldExit := router.DispatchText("/resume latest")
	if !handled || shouldExit {
		t.Fatalf("/resume latest handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "20260704T001530Z-a1b2") {
		t.Fatalf("unexpected latest output: %q", output)
	}
	conversation := sessions.agent.ConversationMessages()
	if len(conversation) != 3 {
		t.Fatalf("ConversationMessages() len = %d", len(conversation))
	}
	if conversation[1].Role != "user" {
		t.Fatalf("legacy system message not sanitized: %#v", conversation[1])
	}
}

func TestResumeRejectsAmbiguousPrefixAndRunningState(t *testing.T) {
	startup := ui.StartupInfo{
		Workdir: "/tmp/project",
		Model:   "demo-model",
		WireAPI: "responses",
		LogFile: "/tmp/project/agent.log",
	}
	codingAgent := agent.NewWithWorkspace(nil, tools.NewRegistry(), 4, "/tmp/project")
	sessions, err := newSessionRuntime(config.SessionConfig{
		Enabled: true,
		Dir:     t.TempDir(),
	}, "/tmp/project", codingAgent, &startup)
	if err != nil {
		t.Fatalf("newSessionRuntime() error = %v", err)
	}
	store := sessions.store
	now := time.Date(2026, 7, 3, 23, 15, 30, 0, time.UTC)
	for _, id := range []string{"20260703T231530Z-a1b2", "20260703T231530Z-b2c3"} {
		if _, err := (*store).Save(session.SaveRequest{
			SessionID: id,
			CreatedAt: now,
			Now:       now,
			Conversation: []llm.Message{
				{Role: "user", Content: id},
			},
		}); err != nil {
			t.Fatalf("Save(%s) error = %v", id, err)
		}
	}
	router := newSlashRouter(&startup, sessions, func() bool { return false })
	output, handled, shouldExit := router.DispatchText("/resume 20260703T231530Z")
	if !handled || shouldExit {
		t.Fatalf("/resume ambiguous handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "ambiguous") {
		t.Fatalf("unexpected ambiguous output: %q", output)
	}

	runningRouter := newSlashRouter(&startup, sessions, func() bool { return true })
	output, handled, shouldExit = runningRouter.DispatchText("/resume latest")
	if !handled || shouldExit {
		t.Fatalf("/resume running handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "unavailable while the agent is running") {
		t.Fatalf("unexpected running-state output: %q", output)
	}
}

func TestInitCreatesEchoDustInCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	workspace := filepath.Join(repo, "services", "agent")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	router := newSlashRouter(&ui.StartupInfo{Workdir: workspace}, nil, func() bool { return false })

	output, handled, shouldExit := router.DispatchText("/init")
	if !handled || shouldExit {
		t.Fatalf("/init handled=%v shouldExit=%v", handled, shouldExit)
	}

	workspaceDoc := filepath.Join(workspace, "ECHODUST.md")
	if _, err := os.Stat(workspaceDoc); err != nil {
		t.Fatalf("workspace ECHODUST.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "ECHODUST.md")); !os.IsNotExist(err) {
		t.Fatalf("repo-root ECHODUST.md should not be created, err=%v", err)
	}
	if !strings.Contains(output, workspaceDoc) {
		t.Fatalf("unexpected /init output: %q", output)
	}
}

func TestInitRejectsExistingEchoDust(t *testing.T) {
	workspace := t.TempDir()
	docPath := filepath.Join(workspace, "ECHODUST.md")
	if err := os.WriteFile(docPath, []byte("# existing"), 0o644); err != nil {
		t.Fatalf("write ECHODUST.md: %v", err)
	}
	router := newSlashRouter(&ui.StartupInfo{Workdir: workspace}, nil, func() bool { return false })

	output, handled, shouldExit := router.DispatchText("/init")
	if !handled || shouldExit {
		t.Fatalf("/init handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "already exists") || !strings.Contains(output, docPath) {
		t.Fatalf("unexpected /init existing-file output: %q", output)
	}
}
