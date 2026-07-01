# EchoDustAgent

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-150%20passing-brightgreen)]()
[![Dependencies](https://img.shields.io/badge/direct_deps-2-yellowgreen)]()

A minimal, provider-agnostic ReAct CLI agent written in Go. Native function calling, event-driven terminal UI, persistent memory, subagents, and MCP integration — in a single binary with only 2 direct dependencies.

---

## ✨ Highlights

- ** Provider Agnostic** — Works with any OpenAI-compatible endpoint (OpenAI, DeepSeek, Qwen, local models, AnyRouter). Supports both `/chat/completions` and `/responses` APIs.
- **⚡ Streaming** — Real-time assistant text streaming with automatic fallback when providers omit usage data.
- ** Persistent Memory** — File-based Markdown memory that persists across sessions. Global and project-scoped memory with automatic discovery of `AGENTS.md`/`CLAUDE.md`/`REASONIX.md`.
- **🤖 Subagents** — Delegate read-only research tasks to isolated subagents. Run up to 5 concurrent subagents to keep main context small.
- ** Safety-First** — 8-tier risk classification, permanent destructive command blacklist, approval prompts for writes.
- **📦 Minimal Footprint** — Only 2 direct dependencies (`glamour`, `x/term`). ~17K lines of Go. Single binary.
- ** Event-Driven UI** — Custom terminal rendering with live Todo/Tools frame, full-screen log viewer (`Ctrl+T`), and Markdown final answer rendering.
- ** MCP Support** — Connect to MCP servers via stdio transport. Auto-discover and register tools as `mcp__<server>__<tool>`.
- **📊 Adaptive Execution** — Step budget auto-extends based on progress, with loop detection and cost controls.
- **🔄 Context Maintenance** — Automatic stale tool result pruning + LLM-based compaction near context window limits.

---

##  Quick Start

### Prerequisites

- Go 1.24+

### Installation

```bash
git clone https://github.com/HongChengXianYue/EchoDustAgent.git
cd EchoDustAgent
```

### Run

```bash
export AGENT_API_KEY=sk-...
go run ./cmd/agent
```

The agent loads configuration from `config.yaml` and enters an interactive loop. Type your request and press Enter.

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `AGENT_API_KEY` | API key for the LLM provider | ✅ Yes |
| `AGENT_BASE_URL` | Override `llm.base_url` | No |
| `AGENT_MODEL` | Override `llm.model` | No |
| `AGENT_WIRE_API` | Override `llm.wire_api` (`chat_completions` or `responses`) | No |
| `AGENT_MAX_STEPS` | Override `agent.max_steps` | No |

---

## 📋 Features

### Built-in Tools (21+)

| Category | Tools |
|----------|-------|
| **File I/O** | `list_files`, `find_files`, `read_file`, `read_file_range`, `write_file`, `replace_in_file` |
| **Search** | `search_files`, `find_symbol`, `find_references`, `find_callers`, `find_callees` |
| **Shell** | `run_command`, `apply_patch`, `git_status`, `git_diff`, `git_log` |
| **Delegation** | `delegate_task` |
| **Memory** | `memory`, `remember`, `forget` |
| **UI** | `update_todos` |
| **MCP** | `mcp__<server>__<tool>` (auto-discovered) |

### Tool Scheduling

- Up to 10 tool calls per assistant turn
- Read/search/build-test calls run in **parallel**
- Workspace writes to different files run **concurrently**
- Same-file writes are **serialized**
- Unknown writes (e.g., git index) use a **workspace-wide lock**

### Subagents

```
delegate_task → isolated read-only agent
                 ├── Fresh message history
                 ├── No recursive delegation
                 ├── Configurable concurrency (default: 5)
                 └── Result truncated to 16KB
```

Use `delegate_task` for broad codebase analysis, architecture review, or "what is missing?" audits. The main agent receives only the final answer.

### Approval Pipeline

Tool calls are classified into 8 risk categories before execution:

```
read_only → search_inspect → build_test → workspace_write
    → vcs_local → network_dependency → external_or_destructive → system_privileged
```

- **Auto-allowed**: `read_only`, `search_inspect`, `build_test`
- **User prompt**: Higher-risk categories (arrow keys, `j`/`k`, shortcuts)
- **Permanent blacklist**: `sudo rm -rf /`, root filesystem deletion, disk formatting, block-device overwrites

### Memory System

```
~/.local-agent/
├── memory/
│   ├── global/              # Shared across projects
│   └── projects/<slug>/     # Project-specific
├── LOCAL-AGENT.md           # Global instructions (like Codex global AGENTS)
└── mcp/servers.json         # MCP server declarations
```

Memory is loaded once at startup and appended to the system prompt. Saved memories are plain Markdown files.

---

## ️ Architecture

```
cmd/agent/main.go
     │
     ├── config.yaml + env vars
     ├── internal/llm/          # OpenAI-compatible client (streaming + Responses API)
     ├── internal/tools/        # 21+ built-in workspace tools
     ├── internal/mcp/          # MCP server integration (stdio transport)
     ├── internal/memory/       # Persistent Markdown memory store
     ├── internal/agent/        # Core ReAct loop, subagents, step budget
     ├── internal/approval/     # Risk classification & approval pipeline
     ├── internal/context/      # Context maintenance (prune + compact)
     └── internal/ui/           # Event-driven terminal UI
```

### Core Loop

```
Agent.Run(ctx, input) (string, error)
     │
     ├── pruneTransientToolHistory()    # Strip update_todos
     ├── pruneStaleToolResults()        # Replace old tool output >8KB
     ├── maybeCompact()                 # LLM summarization near window limit
     └── chatWithTools() → execute → loop until final answer or step limit
```

### Adaptive Step Budget

```
Initial budget (max_steps)
     │
     ├── Tools succeeding + TODO work open? → Extend by step_extension_size
     ├── Loop detected? → Stop extending
     └── Absolute max (absolute_max_steps) → Hard stop
```

---

## ️ Configuration

Runtime tuning lives in `config.yaml`. Key sections:

| Section | Purpose |
|---------|---------|
| `llm` | Base URL, model, wire API, request timeout, parallel tool calls |
| `agent` | Initial/adaptive ReAct step budget, max parallel tool calls |
| `subagents` | Delegate-task availability, concurrency, adaptive step budget |
| `memory` | Persistent memory loading, user memory directory |
| `mcp` | MCP server enablement, timeouts |
| `context` | Stale tool-result pruning, compaction thresholds |
| `tools` | List/find/read/search limits, command timeouts, output caps |
| `ui` | Separator width, live frame bounds, Markdown wrap, polling intervals |

`AGENT_API_KEY` is intentionally loaded from the environment and **never** stored in `config.yaml`.

---

## ️ Terminal UI

```
┌─────────────────────────────────────────────────────────┐
│  [>] Fix auth middleware timeout                        │  ← Todo
│  [ ] Update error handling in handler.go                │
│  [✓] Add test for edge case                             │
─────────────────────────────────────────────────────────┤
│  Tools                                                  │  ← Tools (Ctrl+E to expand)
│  ▸ read_file  handler.go                                │
│  ▸ replace_in_file  +3 lines, -1 line                   │
├─────────────────────────────────────────────────────────┤
│  Tokens: 12,450 / 128,000                               │  ← Live token usage
└─────────────────────────────────────────────────────────┘
```

**Keyboard Shortcuts:**

| Key | Action |
|-----|--------|
| `Ctrl+E` | Expand/collapse recent tool progress |
| `Ctrl+T` | Open full-screen tool log viewer |
| `↑/↓` or `j/k` | Navigate tool log viewer |
| `PgUp/PgDn` | Page through tool log |
| `g` / `G` | Jump to top/bottom |
| `q` / `Esc` | Close full-screen viewer |

The final answer is rendered as Markdown with custom dark styling.

---

##  Project Structure

```
EchoDustAgent/
├── cmd/agent/
│   ├── main.go                 # Entry point: config, wiring, input loop
│   └── slash.go                # /info, /model, /exit, /quit commands
├── internal/
│   ├── agent/                  # Core ReAct agent loop
│   ├── approval/               # Risk classification & approval pipeline
│   ├── config/                 # YAML + env config loading
│   ├── context/                # Context maintenance (prune + compact)
│   ├── llm/                    # OpenAI-compatible LLM client
│   ├── logs/                   # Runtime log file writer
│   ├── mcp/                    # MCP server integration
│   ├── memory/                 # Persistent Markdown memory store
│   ├── runtimeevent/           # Event types for UI communication
│   ├── tools/                  # 21+ built-in workspace tools
│   └── ui/                     # Event-driven terminal UI
├── docs/
│   ├── WORKLOG.md              # Detailed changelog (Chinese)
│   ── TUI_MIGRATION_PLAN.md
├── config.yaml                 # Runtime configuration
├── go.mod
└── README.md
```

**Stats:** 81 Go source files · ~17,370 lines of Go · 11 internal packages

---

##  Testing

```bash
# Run all tests
go test ./...

# Run with custom Go cache
env GOCACHE=/tmp/local-agent-go-build go test ./...

# Run vet
go vet ./...
```

**Current status:** 150 tests passing · 0 failing

---

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

For major changes, please open an issue first to discuss what you would like to change.

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

##  Acknowledgments

- Inspired by [Claude Code](https://claude.ai/code) and [Codex CLI](https://github.com/openai/codex)
- Built with [Glamour](https://github.com/charmbracelet/glamour) for Markdown rendering
- Uses [x/term](https://golang.org/x/term) for terminal TTY detection
