package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"local-agent/internal/agent"
	"local-agent/internal/config"
	"local-agent/internal/session"
	"local-agent/internal/ui"
)

type sessionUI interface {
	SessionSnapshot() session.UISnapshot
	LoadSessionSnapshot(snapshot session.UISnapshot)
	AppendInfoBlock(title, body string)
}

type sessionHandle struct {
	ID        string
	CreatedAt time.Time
}

type sessionRuntime struct {
	mu      sync.Mutex
	store   *session.Store
	agent   *agent.Agent
	ui      sessionUI
	startup *ui.StartupInfo
	model   string
	wireAPI string
	current sessionHandle
}

func newSessionRuntime(cfg config.SessionConfig, workspace string, codingAgent *agent.Agent, startup *ui.StartupInfo) (*sessionRuntime, error) {
	runtime := &sessionRuntime{
		agent:   codingAgent,
		startup: startup,
	}
	if startup != nil {
		runtime.model = startup.Model
		runtime.wireAPI = startup.WireAPI
	}
	if !cfg.Enabled {
		return runtime, nil
	}
	store, err := session.OpenStore(cfg.Dir, workspace)
	if err != nil {
		return nil, err
	}
	runtime.store = &store
	return runtime, nil
}

func (r *sessionRuntime) SetUI(sessionUI sessionUI) {
	r.mu.Lock()
	r.ui = sessionUI
	r.mu.Unlock()
}

func (r *sessionRuntime) Enabled() bool {
	return r != nil && r.store != nil
}

func (r *sessionRuntime) CurrentSessionID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current.ID
}

func (r *sessionRuntime) Recent(limit int) ([]session.Meta, error) {
	if !r.Enabled() {
		return nil, fmt.Errorf("session persistence is disabled")
	}
	r.mu.Lock()
	store := *r.store
	r.mu.Unlock()
	return store.List(limit)
}

func (r *sessionRuntime) SaveConversationOnly() error {
	if !r.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.saveLocked(nil)
	return err
}

func (r *sessionRuntime) SaveUISnapshot(snapshot session.UISnapshot) error {
	if !r.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.saveLocked(&snapshot)
	return err
}

func (r *sessionRuntime) saveLocked(snapshot *session.UISnapshot) (session.Meta, error) {
	conversation := r.agent.ConversationMessages()
	if len(conversation) == 0 {
		return session.Meta{}, nil
	}
	meta, err := r.store.Save(session.SaveRequest{
		SessionID:    r.current.ID,
		CreatedAt:    r.current.CreatedAt,
		Model:        r.model,
		WireAPI:      r.wireAPI,
		Conversation: conversation,
		UI:           snapshot,
	})
	if err != nil {
		return session.Meta{}, err
	}
	r.current = sessionHandle{ID: meta.SessionID, CreatedAt: meta.CreatedAt}
	if r.startup != nil {
		r.startup.SessionID = meta.SessionID
	}
	return meta, nil
}

func (r *sessionRuntime) Resume(ref string) (session.Meta, error) {
	if !r.Enabled() {
		return session.Meta{}, fmt.Errorf("session persistence is disabled")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metas, err := r.store.List(0)
	if err != nil {
		return session.Meta{}, err
	}
	meta, err := resolveSessionRef(metas, ref)
	if err != nil {
		return session.Meta{}, err
	}
	record, err := r.store.Load(meta.SessionID)
	if err != nil {
		return session.Meta{}, err
	}
	if err := r.agent.RestoreConversation(record.State.Conversation); err != nil {
		return session.Meta{}, err
	}
	r.current = sessionHandle{ID: meta.SessionID, CreatedAt: meta.CreatedAt}
	if r.startup != nil {
		r.startup.SessionID = meta.SessionID
	}
	if r.ui != nil {
		var snapshot session.UISnapshot
		if record.State.UI != nil {
			snapshot = *record.State.UI
		}
		r.ui.LoadSessionSnapshot(snapshot)
		r.ui.AppendInfoBlock("Session", resumedSessionNotice(meta))
	}
	return meta, nil
}

func resolveSessionRef(metas []session.Meta, ref string) (session.Meta, error) {
	ref = strings.TrimSpace(ref)
	if len(metas) == 0 {
		return session.Meta{}, fmt.Errorf("no saved sessions for the current workspace")
	}
	if ref == "" || ref == "latest" {
		return metas[0], nil
	}
	for _, meta := range metas {
		if meta.SessionID == ref {
			return meta, nil
		}
	}
	matches := make([]session.Meta, 0, len(metas))
	for _, meta := range metas {
		if strings.HasPrefix(meta.SessionID, ref) {
			matches = append(matches, meta)
		}
	}
	switch len(matches) {
	case 0:
		return session.Meta{}, fmt.Errorf("session %q not found", ref)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, meta := range matches {
			ids = append(ids, meta.SessionID)
		}
		return session.Meta{}, fmt.Errorf("session %q is ambiguous: %s", ref, strings.Join(ids, ", "))
	}
}

func resumedSessionNotice(meta session.Meta) string {
	updated := meta.UpdatedAt.UTC().Format(time.RFC3339)
	if title := strings.TrimSpace(meta.Title); title != "" {
		return fmt.Sprintf("Resumed session %s (%s)\n%s", meta.SessionID, updated, title)
	}
	return fmt.Sprintf("Resumed session %s (%s)", meta.SessionID, updated)
}
