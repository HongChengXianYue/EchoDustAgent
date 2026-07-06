package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"local-agent/internal/ui"
)

// errExit is a sentinel error used by slash handlers to request process exit.
var errExit = errors.New("exit")

type slashHandler func(args []string) (string, error)

type slashCommand struct {
	desc    string
	handler slashHandler
}

type slashRouter struct {
	startup   *ui.StartupInfo
	sessions  *sessionRuntime
	isRunning func() bool
	commands  map[string]slashCommand
}

func newSlashRouter(startup *ui.StartupInfo, sessions *sessionRuntime, isRunning func() bool) *slashRouter {
	router := &slashRouter{
		startup:   startup,
		sessions:  sessions,
		isRunning: isRunning,
		commands:  map[string]slashCommand{},
	}
	router.commands["info"] = slashCommand{
		desc:    "show startup details (workdir, model, session id, mcp tools, log file)",
		handler: router.slashInfo,
	}
	router.commands["model"] = slashCommand{
		desc:    "show or switch the active LLM model",
		handler: router.slashModel,
	}
	router.commands["new"] = slashCommand{
		desc:    "start a fresh session conversation",
		handler: router.slashNew,
	}
	router.commands["resume"] = slashCommand{
		desc:    "list or resume saved sessions for the current workspace",
		handler: router.slashResume,
	}
	router.commands["exit"] = slashCommand{
		desc:    "exit the agent",
		handler: router.slashExit,
	}
	router.commands["quit"] = slashCommand{
		desc:    "exit the agent",
		handler: router.slashExit,
	}
	router.commands["init"] = slashCommand{
		desc:    "generate ECHODUST.md project instruction file",
		handler: router.slashInit,
	}
	return router
}

func (r *slashRouter) Dispatch(input string) (handled bool, shouldExit bool) {
	output, handled, shouldExit := r.DispatchText(input)
	if !handled {
		return false, false
	}
	if strings.TrimSpace(output) != "" {
		fmt.Fprintln(os.Stdout, output)
	}
	return handled, shouldExit
}

func (r *slashRouter) DispatchText(input string) (output string, handled bool, shouldExit bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false, false
	}
	name, args := parseSlash(input)
	cmd, ok := r.commands[name]
	if !ok {
		return fmt.Sprintf("unknown command: /%s\n%s", name, r.helpText()), true, false
	}
	output, err := cmd.handler(args)
	if err != nil {
		if errors.Is(err, errExit) {
			return output, true, true
		}
		if strings.TrimSpace(output) != "" {
			return output + "\n" + err.Error(), true, false
		}
		return err.Error(), true, false
	}
	return output, true, false
}

func (r *slashRouter) CommandList() []ui.CommandSuggestion {
	cmds := make([]ui.CommandSuggestion, 0, len(r.commands))
	for name, cmd := range r.commands {
		cmds = append(cmds, ui.CommandSuggestion{Name: name, Desc: cmd.desc})
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

func (r *slashRouter) helpText() string {
	var out strings.Builder
	out.WriteString("available commands:\n")
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&out, "  /%-8s %s\n", name, r.commands[name].desc)
	}
	return strings.TrimRight(out.String(), "\n")
}

func (r *slashRouter) slashInfo(_ []string) (string, error) {
	var out strings.Builder
	ui.RenderStartupDetails(&out, r.currentStartup())
	return strings.TrimRight(out.String(), "\n"), nil
}

func (r *slashRouter) slashExit(_ []string) (string, error) {
	return "", errExit
}

func (r *slashRouter) slashModel(args []string) (string, error) {
	if len(args) == 0 {
		return fmt.Sprintf("current model: %s", r.currentStartup().Model), nil
	}
	return "", fmt.Errorf("/model switch not yet implemented (requested: %s)", strings.Join(args, " "))
}

func (r *slashRouter) slashNew(args []string) (string, error) {
	if len(args) != 0 {
		return "", fmt.Errorf("usage: /new")
	}
	if r.sessions == nil {
		return "", fmt.Errorf("session runtime is unavailable")
	}
	if r.isRunning != nil && r.isRunning() {
		return "", fmt.Errorf("/new is unavailable while the agent is running")
	}
	saved, err := r.sessions.StartNewSession()
	if err != nil {
		return "", err
	}
	if saved.SessionID != "" {
		return fmt.Sprintf("started a new session (previous session saved as %s)", saved.SessionID), nil
	}
	return "started a new session", nil
}

func (r *slashRouter) slashResume(args []string) (string, error) {
	if r.sessions == nil || !r.sessions.Enabled() {
		return "", fmt.Errorf("session persistence is disabled")
	}
	if r.isRunning != nil && r.isRunning() {
		return "", fmt.Errorf("/resume is unavailable while the agent is running")
	}
	if len(args) == 0 {
		return r.renderRecentSessions()
	}
	if len(args) > 1 {
		return "", fmt.Errorf("usage: /resume [latest|session-id-prefix]")
	}
	meta, err := r.sessions.Resume(args[0])
	if err != nil {
		return "", err
	}
	return resumedSessionNotice(meta), nil
}

func (r *slashRouter) renderRecentSessions() (string, error) {
	metas, err := r.sessions.Recent(10)
	if err != nil {
		return "", err
	}
	if len(metas) == 0 {
		return "no saved sessions for the current workspace", nil
	}
	current := ""
	if r.sessions != nil {
		current = r.sessions.CurrentSessionID()
	}
	var out strings.Builder
	out.WriteString("recent sessions:\n")
	for _, meta := range metas {
		prefix := " "
		if meta.SessionID == current && current != "" {
			prefix = "*"
		}
		fmt.Fprintf(&out, "%s %s  %s", prefix, meta.SessionID, meta.UpdatedAt.UTC().Format(time.RFC3339))
		if title := strings.TrimSpace(meta.Title); title != "" {
			fmt.Fprintf(&out, "  %s", title)
		} else if preview := strings.TrimSpace(meta.LastUserPreview); preview != "" {
			fmt.Fprintf(&out, "  %s", preview)
		}
		out.WriteString("\n")
	}
	out.WriteString("\nUse /resume latest or /resume <session-id>.")
	return strings.TrimRight(out.String(), "\n"), nil
}

func (r *slashRouter) currentStartup() ui.StartupInfo {
	if r.startup == nil {
		return ui.StartupInfo{}
	}
	info := *r.startup
	if r.sessions != nil {
		info.SessionID = r.sessions.CurrentSessionID()
	}
	return info
}

// parseSlash splits "/cmd arg1 arg2" into ("cmd", ["arg1", "arg2"]).
func parseSlash(input string) (name string, args []string) {
	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return name, nil
	}
	return name, strings.Fields(parts[1])
}

// slashInit generates an ECHODUST.md template in the project root.
func (r *slashRouter) slashInit(_ []string) (string, error) {
	if r.startup == nil || r.startup.Workdir == "" {
		return "", fmt.Errorf("workdir not available")
	}

	// Keep /init aligned with memory.ScopeProject, which is defined as the
	// current workspace directory rather than the git repository root.
	agentsPath := filepath.Join(r.startup.Workdir, "ECHODUST.md")

	// Check if file already exists
	if _, err := os.Stat(agentsPath); err == nil {
		return "", fmt.Errorf("ECHODUST.md already exists at %s", agentsPath)
	}

	template := `# Project Instructions

This file contains project-specific instructions for the AI agent.
It will be automatically loaded and injected into the system prompt.

## Overview

<!-- Describe your project here -->

## Coding Standards

<!-- Add your coding standards and conventions -->

## Important Notes

<!-- Add any important context about the project -->
`

	if err := os.WriteFile(agentsPath, []byte(template), 0o644); err != nil {
		return "", fmt.Errorf("failed to create ECHODUST.md: %w", err)
	}

	return fmt.Sprintf("Created ECHODUST.md at %s", agentsPath), nil
}
