# local-agent

Minimal Go ReAct CLI agent using OpenAI-compatible native function calling.

## Run

```bash
export AGENT_API_KEY=...
go run ./cmd/agent
```

The CLI writes runtime logs to `.local-agent/logs/agent.log` under the current workspace. This is useful for diagnosing provider, streaming, and runtime failures that may not be fully visible in the terminal UI.

Optional environment overrides:

```bash
export AGENT_BASE_URL=https://api.openai.com/v1
export AGENT_MODEL=gpt-4.1-mini
export AGENT_MAX_STEPS=20
```

## Configuration

Runtime tuning lives in `config.yaml`. The file keeps operational limits out of code while leaving protocol constants, tool names, shortcuts, and safety categories fixed in source.

Configured areas:

- `llm`: base URL, model, request timeout, and `parallel_tool_calls`.
- `agent`: maximum ReAct steps per user request and maximum parallel tool calls per assistant turn.
- `subagents`: delegate-task availability, concurrency, max steps, and result size.
- `memory`: persistent memory loading and user memory directory.
- `context`: stale tool-result pruning and conservative conversation compaction thresholds.
- `tools`: list/find/read/search limits, command and patch timeouts, output caps, and file-change preview lines.
- `ui`: separator width, live frame bounds, full-log viewer sizes, polling intervals, Markdown wrap width, and preview truncation lengths.

`AGENT_API_KEY` is intentionally loaded from the environment and is not stored in `config.yaml`. `AGENT_BASE_URL`, `AGENT_MODEL`, and `AGENT_MAX_STEPS` override the YAML values when set.

## Built-in Tools

- `list_files`: list a workdir-relative directory.
- `find_files`: find files or directories by name or relative path under a directory.
- `read_file`: read a workdir-relative text file.
- `read_file_range`: read selected line ranges from a workdir-relative text file.
- `search_files`: search literal text or regex text under a directory.
- `find_symbol`: search code symbols by name.
- `find_references`: find references to a Go symbol at a specific file position.
- `find_callers`: find callers of a Go symbol at a specific file position.
- `find_callees`: find callees of a Go symbol at a specific file position.
- `write_file`: write a file and create parent directories.
- `replace_in_file`: replace exact text in a file.
- `run_command`: run a shell command in the workdir.
- `apply_patch`: apply a unified diff patch.
- `git_status`: show concise workspace git status.
- `git_diff`: show unstaged or staged git diff.
- `git_log`: show recent commits in one-line format.
- `delegate_task`: delegate an isolated read-only research task to a subagent.
- `memory`: list, search, or read saved durable memories.
- `remember`: save or update a durable memory for future sessions.
- `forget`: archive a stale saved memory.

The agent only executes tools from provider-returned `tool_calls`. It does not parse assistant text as a JSON tool protocol.

## Streaming Output

When the configured OpenAI-compatible endpoint supports streaming chat completions, the agent now prefers streaming responses for assistant text. Partial assistant text is forwarded into runtime events and shown in the live terminal frame before the final answer block is rendered.

Tool calls still remain turn-based. The agent accumulates streamed assistant content, waits for the provider's final tool-call payload or final text completion, and then continues through the existing tool scheduler and final-answer rendering path.

## Memory

When enabled, memory loads once at startup and is appended to the system prompt after the stable base instructions. This keeps the base prompt cache-friendly while still giving the model durable project context.

Document memory is discovered from `REASONIX.md`, `AGENTS.md`, and `CLAUDE.md`, plus local variants such as `AGENTS.local.md`. Discovery starts from the workspace, stops at the nearest Git root, also reads the configured user directory, and supports single-line `@relative/path.md` imports.

Saved memories live under `memory.user_dir` as plain Markdown files:

- `memory/global`: user and feedback memories shared across projects.
- `projects/<workspace-slug>/memory`: project and reference memories for the current workspace.

The `memory` tool is read-only. `remember` and `forget` modify the user memory directory and therefore use the existing approval flow.

## Context Maintenance

At the start of each user request, the agent removes transient `update_todos` history, prunes stale oversized tool outputs outside the recent tail, and then checks whether older history should be compacted.

Pruning keeps the tool message and `tool_call_id` intact, preserving native function-call pairing while replacing only large old `output` fields with a short marker. Recent messages are protected so the current task can still use fresh tool output.

Compaction is conservative and only runs near the configured context window. It keeps the system prompt and recent tail verbatim, summarizes older messages with a no-tools LLM call, and inserts the result as a `<compaction-summary>` user message. If summarization fails or would not reduce context, the original history is kept.

## Subagents

The main agent exposes `delegate_task` for independent read-only research and focused code investigation.

- Each subagent gets a fresh message history and does not inherit the parent conversation.
- The parent receives only the subagent final answer as the `delegate_task` tool result.
- Subagents do not receive `delegate_task`, so they cannot recursively spawn more subagents.
- Subagent tool calls still use the same pre-use safety path: classification, write-impact analysis, permanent blacklist checks, and approval policy.
- Subagents can use file read/search tools and `run_command`, but command calls outside `read_only`, `search_inspect`, or `build_test` are denied by the read-only policy.
- Broad codebase analysis, architecture review, and “what is missing?” style project audits should delegate focused research first instead of reading many files directly in the main context.
- When broad analysis has independent areas, the main agent should issue multiple `delegate_task` calls in one turn, for example architecture, tools, UI, config, tests, and security.
- Subagent tool activity is forwarded into the parent Tools log and marked with a `Subagent` prefix.

By default, at most two subagents run concurrently and each subagent can use up to eight ReAct steps.

## Tool Scheduling

The model may return multiple native `tool_calls` in one assistant turn. The agent prepares approvals first, then executes safe calls concurrently:

- At most 10 non-`update_todos` tool calls are accepted from one assistant turn. Extra calls receive tool error results.
- At most 10 accepted tool calls execute concurrently, including multiple calls to the same tool with different arguments.
- Read-only/search/build-test calls can run in parallel.
- Workspace writes can run in parallel when they target different files.
- Writes to the same file are serialized.
- Unknown workspace writes, such as Git index changes, use a workspace-wide lock.
- Tool result messages are appended back to the conversation in the original tool-call order.

## TODO Workflow

The agent exposes an internal native function tool named `update_todos`. For concrete workspace tasks, the model must create a todo list before calling workspace tools. Each user request gets a fresh todo list; it is not persisted across turns or sessions. `update_todos` is implemented in the tools layer as a UI-only control tool and is pruned from long-term model history after each run.

During a concrete task, the terminal UI keeps `Todo` above the tools area and redraws it as progress changes:

- `[>]`: in progress
- `[ ]`: pending
- `[✓]`: completed

## CLI UI

The terminal UI prints assistant process text, tool calls, tool results, edit summaries, and final answers in block form:

- `Todo`: current task list.
- `Tools`: collapsed by default; press `Ctrl+E` during the current run to expand or collapse recent tool progress.
- `Full tool log`: press `Ctrl+T` during the current run to open the complete tool log in a full-screen viewer; press `q`, `Esc`, or `Ctrl+T` again to close it.
- `Subagent`: delegated read-only research tasks.
- `Explored`: read/list/find/search tools when `Tools` is expanded.
- `Running` and `Ran`: shell commands.
- `Added` or `Edited`: file-writing tools with line-count summaries.

The live `Todo`/`Tools` frame is bounded to the terminal viewport. Expanded tool details show recent events and truncate long lines/output so toggling does not leave repeated snapshots in scrollback.

The full-screen tool log viewer uses the alternate terminal screen, refreshes while tools are still running, and supports `↑`/`↓`, `j`/`k`, `PgUp`/`PgDn`, `g`, and `G` for navigation.

Interactive input supports left/right cursor movement, backspace, and up/down history in TTY sessions. Non-TTY input falls back to plain line reading.

## Approval

Tool calls are classified by risk before execution:

- `read_only`
- `search_inspect`
- `build_test`
- `workspace_write`
- `vcs_local`
- `network_dependency`
- `external_or_destructive`
- `system_privileged`

Read-only, search, and build/test calls run without prompting. Higher-risk calls show a selectable CLI approval prompt; use arrow keys, `j`/`k`, `tab`, or shortcuts like `a` and `d`.

Workspace writes ask for `always` or `deny`. `always` allows workspace writes for the current CLI session, so different files can be written concurrently while same-file writes remain serialized.

Writes outside the workspace ask for `allow`, `always`, or `deny`. External-write `always` only applies to the current tool-call loop and is not remembered for the session.

Permanent safety rules run before approval. Commands such as `sudo rm -rf /`, root filesystem deletion, Windows drive deletion, disk formatting, and raw block-device overwrites are blocked outright and cannot be approved.

## Verify

```bash
go test ./...
go vet ./...
```

When the default Go build cache is not writable, use:

```bash
env GOCACHE=/tmp/local-agent-go-build go test ./...
env GOCACHE=/tmp/local-agent-go-build go vet ./...
```
