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
- `read_file`: read a workdir-relative text file.
- `search_files`: search literal text under a directory.
- `write_file`: write a file and create parent directories.
- `replace_in_file`: replace exact text in a file.
- `run_command`: run a shell command in the workdir.
- `apply_patch`: apply a unified diff patch.

The agent only executes tools from provider-returned `tool_calls`. It does not parse assistant text as a JSON tool protocol.

## CLI UI

The terminal UI prints assistant process text, tool calls, tool results, edit summaries, and final answers in block form:

- `Explored`: read/list/search tools.
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

Read-only, search, and build/test calls run without prompting. Higher-risk calls show a selectable CLI approval prompt with `allow`, `always`, and `deny`; use arrow keys, `j`/`k`, `tab`, or shortcuts like `a` and `d`. `always` is remembered only for the current process and only for the exact same tool call.

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
