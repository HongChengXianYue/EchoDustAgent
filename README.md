# local-agent

Minimal Go ReAct CLI agent using OpenAI-compatible native function calling.

## Run

```bash
export AGENT_API_KEY=...
export AGENT_BASE_URL=https://api.openai.com/v1
export AGENT_MODEL=gpt-4.1-mini
go run ./cmd/agent
```

Optional:

```bash
export AGENT_MAX_STEPS=10
```

## Built-in Tools

- `list_files`: list a workdir-relative directory.
- `find_files`: find files or directories by name or relative path under a directory.
- `read_file`: read a workdir-relative text file.
- `search_files`: search literal text under a directory.
- `write_file`: write a file and create parent directories.
- `replace_in_file`: replace exact text in a file.
- `run_command`: run a shell command in the workdir.
- `apply_patch`: apply a unified diff patch.

The agent only executes tools from provider-returned `tool_calls`. It does not parse assistant text as a JSON tool protocol.

## Tool Scheduling

The model may return multiple native `tool_calls` in one assistant turn. The agent prepares approvals first, then executes safe calls concurrently:

- Read-only/search/build-test calls can run in parallel.
- Workspace writes can run in parallel when they target different files.
- Writes to the same file are serialized.
- Unknown workspace writes, such as Git index changes, use a workspace-wide lock.
- Tool result messages are appended back to the conversation in the original tool-call order.

## CLI UI

The terminal UI prints assistant process text, tool calls, tool results, edit summaries, and final answers in block form:

- `Explored`: read/list/find/search tools.
- `Running` and `Ran`: shell commands.
- `Added` or `Edited`: file-writing tools with line-count summaries.

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
