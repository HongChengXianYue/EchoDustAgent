# Worklog

## 2026-06-13 - Project Bootstrap

- Summary: Created the minimal Go CLI agent with native OpenAI-compatible function calling, built-in workspace tools, and environment-based config.
- Main modules: `cmd/agent`, `internal/agent`, `internal/llm`, `internal/tools`, `internal/config`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: The first version intentionally omitted harness, memory, MCP, approval prompts, and rich UI.

## 2026-06-13 - Tool Schema Compatibility

- Summary: Fixed optional-only tool schemas so they omit `required` instead of emitting `required: null`, matching provider expectations and tiny-agent style.
- Main modules: `internal/tools`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: Prevents DeepSeek-compatible function calling from rejecting `list_files`.

## 2026-06-13 - Codex-Style CLI UI And Project Rules

- Summary: Planned and implemented runtime events, Codex-style block rendering, line editing with history, file-change metadata, and project documentation rules.
- Main modules: `internal/runtimeevent`, `internal/ui`, `internal/agent`, `internal/tools`, `cmd/agent`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: Approval flow is interface-only for now; default behavior still allows all tool executions.

## 2026-06-13 - Casual Input Tool Gating

- Summary: Prevented simple greetings and small talk from exposing workspace tools, so inputs like `hello` answer directly instead of exploring files.
- Main modules: `internal/agent`, `internal/llm`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: Concrete workspace tasks still receive the normal function tool list.

## 2026-06-13 - Prompt-Only Tool Use Guidance

- Summary: Removed hardcoded casual-input tool filtering and kept the "do not use tools for greetings" behavior as system prompt guidance only.
- Main modules: `internal/agent`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: Tools are still exposed to the model; the model is instructed not to use them unless the user asks for concrete workspace work.
