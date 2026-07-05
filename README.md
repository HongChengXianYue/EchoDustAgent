# Echo Dust Code

[![Go Version](https://img.shields.io/badge/Go-1.24.2+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![UI](https://img.shields.io/badge/UI-Bubble%20Tea%20TUI-1f6feb)]()
[![Protocol](https://img.shields.io/badge/LLM-OpenAI%20Compatible-0a7f5a)]()

面向终端的 Go 语言 Coding Agent，默认提供 Bubble Tea TUI、原生 function calling、工作区工具、审批流、持久记忆、MCP 集成、按需加载 skill 和只读 subagent。

A terminal-first coding agent written in Go with a Bubble Tea TUI, native function calling, workspace tools, approval flows, persistent memory, MCP integration, lazy-loaded skills, and read-only subagents.

---

## 概览 | Overview

- 面向 OpenAI 兼容接口，支持 `/chat/completions` 与 `/responses` 两种协议形态。
  Built for OpenAI-compatible endpoints and supports both `/chat/completions` and `/responses`.
- 默认维护 TTY 下的 `internal/tui`；`internal/ui` 仅保留为非 TTY/历史回退路径。
  The actively maintained interface is `internal/tui` for TTY sessions; `internal/ui` remains as a legacy/non-TTY fallback.
- 文件写入和 `git_diff` 会在主 TUI 中实时展示 inline diff，带行号、红绿整行铺色和多语言语法高亮。
  File edits and `git_diff` render as inline diffs in the main TUI with line numbers, full-row add/remove coloring, and multi-language syntax highlighting.
- 支持 `delegate_task` 只读子代理、MCP 动态工具发现、Markdown 持久记忆和 `/resume` 会话恢复。
  Supports read-only `delegate_task` subagents, dynamically discovered MCP tools, Markdown-backed persistent memory, and `/resume` session restore.
- 支持按请求检索 top-k skill，并通过 `invoke_skill` 在真正需要时才加载 `SKILL.md`。
  Supports request-scoped top-k skill retrieval and loads `SKILL.md` only when `invoke_skill` is actually called.

---

## 亮点 | Highlights

- `Provider-agnostic / 模型提供方无关`
  支持 OpenAI、DeepSeek、Qwen、本地 OpenAI-compatible 网关和任意自托管兼容服务。
  Works with OpenAI, DeepSeek, Qwen, local OpenAI-compatible gateways, and self-hosted compatible services.
- `Bubble Tea TUI / 主维护终端界面`
  默认 TTY 模式使用 Bubble Tea TUI，支持滚轮滚动、实时 transcript、审批内联交互、subagent 面板和 slash suggestions。
  TTY mode uses a Bubble Tea TUI with mouse-wheel scrolling, live transcripts, inline approvals, a subagent panel, and slash suggestions.
- `Inline Diff Review / 实时代码改动审阅`
  `write_file`、`replace_in_file`、`apply_patch` 和 `git_diff` 的结果会直接插入 transcript，便于边执行边审阅。
  Results from `write_file`, `replace_in_file`, `apply_patch`, and `git_diff` are inserted into the transcript for immediate review.
- `Safety-first / 安全优先`
  工具执行前做风险分类；高风险操作进入审批；永久黑名单命令会直接拒绝。
  Tool calls are risk-classified before execution; higher-risk operations require approval; permanently banned destructive commands are rejected outright.
- `Persistent Memory / 持久记忆`
  启动时加载用户级与项目级 Markdown 记忆，并通过 `memory`、`remember`、`forget` 工具维护。
  Loads user-level and project-level Markdown memories at startup and manages them through `memory`, `remember`, and `forget`.
- `Subagents / 子代理`
  用 `delegate_task` 将大范围只读分析隔离到独立上下文，默认最多并发 5 个子代理。
  Uses `delegate_task` to isolate broad read-only analysis in separate contexts, with up to 5 concurrent subagents by default.
- `MCP Support / MCP 支持`
  通过 stdio 连接 MCP server，并把远端工具注册为 `mcp__<server>__<tool>`。
  Connects to MCP servers over stdio and exposes their tools as `mcp__<server>__<tool>`.
- `Lazy Skills / 按需技能加载`
  启动时只注册 skill manifest 元数据；每次用户请求再检索候选 skill，把 top-k 摘要注入上下文，并在模型调用 `invoke_skill` 时才真正读取 `SKILL.md` 执行。
  Only skill manifests are registered at startup; each user request retrieves candidate skills, injects top-k summaries into context, and reads `SKILL.md` only when the model calls `invoke_skill`.

---

## 快速开始 | Quick Start

### 前置要求 | Prerequisites

- Go `1.24.2+`
- Node.js `18+`（仅在通过 npm 全局安装或发布 npm 包时需要）
  Node.js `18+` is only needed for global npm installation or npm publishing.

### 获取源码 | Clone

```bash
git clone https://github.com/HongChengXianYue/EchoDustAgent.git
cd EchoDustAgent
```

### 方式一：全局安装 | Install Globally With npm

```bash
npm install -g @hongchengxianyue/echo-dust-code
export AGENT_API_KEY=sk-...
echo-dust-code
```

安装后可用的全局启动命令有 3 个：

After installation, the package exposes three equivalent global commands:

```bash
echo-dust-code
echocode
edc
```

更新全局版本：

Update the global installation with:

```bash
npm update -g @hongchengxianyue/echo-dust-code
```

或直接升级到最新发布版：

Or jump straight to the latest published version:

```bash
npm install -g @hongchengxianyue/echo-dust-code@latest
```

### 方式二：源码运行 | Run From Source

```bash
export AGENT_API_KEY=sk-...
go run ./cmd/agent
```

可选地，你也可以先编译：

You can also build first:

```bash
go build -o echo-dust-code ./cmd/agent
./echo-dust-code
```

程序会读取 `config.yaml`，在 TTY 下进入交互式 TUI，在非 TTY 下退回到旧渲染器。

The agent resolves configuration, starts the interactive TUI in TTY sessions, and falls back to the legacy renderer in non-TTY environments.

### 配置文件查找顺序 | Config Resolution Order

配置文件按以下顺序查找：

The config file is resolved in this order:

1. `AGENT_CONFIG_FILE` 指定的显式路径
   the explicit path from `AGENT_CONFIG_FILE`
2. 当前工作目录下的 `./config.yaml`
   `./config.yaml` in the current working directory
3. 用户级默认配置 `~/.echo-dust-code/config.yaml`
   the user-level default config at `~/.echo-dust-code/config.yaml`

这意味着通过 npm 全局安装后，你可以把通用配置放在 `~/.echo-dust-code/config.yaml`，然后在任意项目目录直接执行 `echo-dust-code`、`echocode` 或 `edc`。

This means a global npm install can keep shared defaults in `~/.echo-dust-code/config.yaml` while still allowing per-project overrides via `./config.yaml`, and you can launch the agent with `echo-dust-code`, `echocode`, or `edc`.

---

## 环境变量 | Environment Variables

| 变量 Variable | 说明 Description | 必填 Required |
|---|---|---|
| `AGENT_API_KEY` | LLM 服务的 API key / API key for the LLM provider | Yes |
| `AGENT_CONFIG_FILE` | 显式指定配置文件路径，优先级高于 `./config.yaml` 和 `~/.echo-dust-code/config.yaml` / Explicit config file path that overrides both `./config.yaml` and `~/.echo-dust-code/config.yaml` | No |
| `AGENT_BASE_URL` | 覆盖 `llm.base_url` / Override `llm.base_url` | No |
| `AGENT_MODEL` | 覆盖 `llm.model` / Override `llm.model` | No |
| `AGENT_WIRE_API` | 覆盖 `llm.wire_api`，可选 `chat_completions` 或 `responses` / Override `llm.wire_api` (`chat_completions` or `responses`) | No |
| `AGENT_LLM_MAX_RETRIES` | 覆盖 `llm.max_retries`，控制 chat 失败后的自动重试次数 / Override `llm.max_retries` for automatic chat retries | No |
| `AGENT_LLM_RETRY_BACKOFF_MILLISECONDS` | 覆盖 `llm.retry_backoff_milliseconds`，控制两次 chat 重试之间的等待时间 / Override `llm.retry_backoff_milliseconds` between chat retries | No |
| `AGENT_MAX_STEPS` | 覆盖 `agent.max_steps` / Override `agent.max_steps` | No |
| `AGENT_STEP_TIMING_ENABLED` | 覆盖 `agent.step_timing_enabled`，开启逐 step 耗时事件 / Override `agent.step_timing_enabled` to emit per-step timing events | No |

`AGENT_API_KEY` 故意只从环境变量读取，不写入 `config.yaml`。

`AGENT_API_KEY` is intentionally loaded only from the environment and is never stored in `config.yaml`.

---

## npm 打包与发布 | npm Packaging And Release

- npm 包本身只分发一个很薄的 Node.js 启动器。
  The npm package ships a thin Node.js launcher only.
- 实际运行的 Go 二进制会在 `postinstall` 阶段按平台从 GitHub Releases 下载。
  The real Go binary is downloaded from GitHub Releases during `postinstall`.
- 支持的目标平台为 `darwin/linux/win32` + `x64/arm64`。
  Supported targets are `darwin/linux/win32` with `x64/arm64`.
- 发布时推送 `vX.Y.Z` tag，GitHub Actions 会校验 `package.json` 版本与 tag 一致，再构建 release 资产、上传 GitHub Releases，并通过 npm trusted publishing 执行 `npm publish`。
  Publishing a `vX.Y.Z` tag triggers GitHub Actions to verify the `package.json` version, build release assets, upload GitHub Releases, and publish to npm via trusted publishing.
- 当前 workflow 不再依赖长期 `NPM_TOKEN`；需要先在 npm 后台把这个 GitHub 仓库配置为 trusted publisher。
  The workflow no longer depends on a long-lived `NPM_TOKEN`; you must configure this GitHub repository as a trusted publisher in npm before the first release.

---

## 默认行为 | Default Behavior

- TTY 模式下默认走 Bubble Tea TUI。
  TTY sessions default to the Bubble Tea TUI.
- 非 TTY 模式下使用旧 `internal/ui` 渲染器。
  Non-TTY sessions use the legacy `internal/ui` renderer.
- 启动时会加载：
  On startup the agent loads:
  - `config.yaml`
  - 用户级和项目级 memory
    user-level and project-level memory
  - MCP server 声明
    MCP server declarations
  - skill manifest 元数据（不是完整 skill 正文）
    skill manifest metadata (not the full skill bodies)
  - 会话持久化配置（用于 `/resume`）
    session persistence configuration (used by `/resume`)

---

## 内置工具 | Built-in Tools

当前包含 22 个内置工具，以及运行时自动发现的 MCP 工具。

There are currently 22 built-in tools, plus MCP tools discovered at runtime.

| 类别 Category | 工具 Tools |
|---|---|
| 文件 I/O File I/O | `list_files`, `find_files`, `read_file`, `read_file_range`, `write_file`, `replace_in_file` |
| 搜索与代码关系 Search & code graph helpers | `search_files`, `find_symbol`, `find_references`, `find_callers`, `find_callees` |
| Shell 与补丁 Shell & patching | `run_command`, `apply_patch`, `git_status`, `git_diff`, `git_log` |
| 委托 Delegation | `delegate_task`, `invoke_skill` |
| 记忆 Memory | `memory`, `remember`, `forget` |
| UI 控制 UI control | `update_todos` |
| MCP | `mcp__<server>__<tool>` |

### 调度策略 | Scheduling Rules

- 单轮 assistant 回复最多允许 10 个非 `update_todos` 工具调用。
  A single assistant turn may schedule up to 10 non-`update_todos` tool calls.
- 只读、搜索、构建测试类调用尽量并行执行。
  Read-only, search, and build/test calls are parallelized when possible.
- 不同文件的工作区写入可以并发。
  Workspace writes to different files may run concurrently.
- 同一文件写入会串行化。
  Writes to the same file are serialized.
- 未知写入目标会升级为工作区级锁。
  Writes with unknown targets are protected by a workspace-wide lock.

---

## Slash 命令 | Slash Commands

| 命令 Command | 说明 Description |
|---|---|
| `/info` | 显示当前 workdir、模型、session id、MCP 工具和日志文件路径。 Show workdir, model, session id, MCP tools, and log file path. |
| `/model` | 显示当前模型；切换模型尚未实现。 Show the active model; switching is not implemented yet. |
| `/resume` | 列出当前 workspace 的历史 session，并在 TUI 中进入恢复选择。 List saved sessions for the current workspace and open the restore picker in the TUI. |
| `/exit` | 退出程序。 Exit the agent. |
| `/quit` | 退出程序。 Exit the agent. |

---

## TUI 说明 | TUI Notes

主界面会持续展示 transcript、subagent 面板、输入框和实时 diff；审批请求以内联方式插入当前上下文，而不是跳出全屏 modal。

The main screen continuously shows the transcript, the subagent panel, the input box, and live diffs; approval requests are rendered inline instead of as a full-screen modal.

### 快捷键 | Keyboard Shortcuts

| 按键 Key | 作用 Action |
|---|---|
| `Ctrl+C` | 运行中中断当前任务；空闲时退出。 Interrupt the active run, or quit when idle. |
| `Ctrl+E` | 展开或收起最近工具进度。 Expand/collapse recent tool progress. |
| `Ctrl+T` | 打开全屏工具日志查看器。 Open the full-screen tool log viewer. |
| `↑/↓` 或 `j/k` | 在 viewer、审批或选择列表中导航。 Navigate viewers, approvals, or pickers. |
| `PgUp/PgDn` | 分页滚动当前 viewport。 Page through the active viewport. |
| `Home/End` | 跳到顶部或底部。 Jump to top or bottom. |
| `Enter` | 提交输入；在 subagent 列表中进入详情；在审批中确认。 Submit input, open subagent details, or confirm approval. |
| `Esc` | 关闭全屏 viewer、退出子代理详情或取消当前选择器。 Close the full-screen viewer, leave subagent detail, or cancel the active picker. |
| 鼠标滚轮 Mouse wheel | 滚动主 transcript 或子代理详情。 Scroll the main transcript or subagent detail viewport. |

### Diff 展示 | Diff Rendering

- 统一显示为 inline diff，而不是 raw patch 文本。
  Diffs are rendered as inline review rows rather than raw patch text.
- 新增行是绿色整行背景，删除行是红色整行背景。
  Added lines use a full-row green background; removed lines use a full-row red background.
- 带行号和 `+/-` 标记。
  Includes line numbers and `+/-` markers.
- 会根据文件路径或内容为代码行做多语言语法高亮。
  Applies multi-language syntax highlighting based on file path or content heuristics.

---

## 审批与安全 | Approval and Safety

工具调用在执行前会进入风险分类：

Tool calls are risk-classified before execution:

```text
read_only → search_inspect → build_test → workspace_write
    → vcs_local → network_dependency → external_or_destructive → system_privileged
```

- `read_only`、`search_inspect`、`build_test` 默认自动放行。
  `read_only`, `search_inspect`, and `build_test` are auto-approved.
- 更高风险类别会进入 TUI 审批交互。
  Higher-risk categories go through the TUI approval flow.
- 永久黑名单会直接拒绝危险命令，例如根目录删除、磁盘格式化和原始块设备覆写。
  Permanently banned destructive commands are rejected outright, including root filesystem deletion, disk formatting, and raw block-device overwrites.

---

## 子代理 | Subagents

`delegate_task` 用于把大范围只读分析隔离到独立上下文。父 agent 只接收结论，不接收完整历史。

`delegate_task` isolates broad read-only analysis into separate contexts. The parent agent receives only the final conclusion, not the full subagent history.

```text
delegate_task
  ├── fresh message history
  ├── no recursive delegation
  ├── configurable concurrency (default: 5)
  └── result truncated to result_max_bytes
```

适合场景：

Good use cases:

- 大范围代码库梳理
  broad codebase surveys
- 架构路径追踪
  architecture tracing
- “当前项目还缺什么”这类只读审查
  read-only “what is missing?” audits

---

## Skills | 技能系统

Skill 用于把可选能力注册成“元数据先行、正文按需加载”的能力包。

Skills package optional capabilities into metadata-first, body-on-demand units.

```text
startup
  └── register registry metadata only

per user request
  ├── retrieve top-k matching skills
  ├── inject summaries into model context
  └── expose invoke_skill

invoke_skill
  ├── validate input against input_schema
  ├── load SKILL.md lazily
  ├── restrict tools by permissions.tools
  └── run in an isolated internal agent
```

推荐结构：

Recommended layout:

```text
skills/
├── registry.json
└── <skill-name>/
   └── SKILL.md
```

`registry.json` 示例：

Example `registry.json`:

```json
{
  "skills": [
    {
      "name": "reviewer",
      "path": "reviewer",
      "description": "Review code changes for bugs and regressions.",
      "summary": "Focused code review skill for risky diffs.",
      "input_schema": {
        "type": "object",
        "properties": {
          "focus": { "type": "string" }
        },
        "additionalProperties": false
      },
      "permissions": {
        "tools": ["read_file", "search_files", "git_diff"]
      },
      "triggers": ["code review", "review diff", "bug risk"]
    }
  ]
}
```

目录内 `skill.json` 仍然兼容，但现在是可选覆盖层，不再要求每个 skill 都单独带一份。

Directory-local `skill.json` files are still supported, but they are now optional overrides instead of a per-skill requirement.

如果某个 skill 目录只有 `SKILL.md`，系统也会按目录名注册一个最小元数据版本，但检索质量会明显依赖名字匹配；要获得更稳定的召回，仍建议在根级 `registry.json` 里补全描述、触发词和权限。

If a skill directory only contains `SKILL.md`, the loader still registers a minimal path-based entry, but retrieval quality will mostly depend on name matching. For reliable recall, add description, triggers, and permissions in the root-level `registry.json`.

默认会同时扫描：

By default the agent scans both:

- `~/.echo-dust-code/skills`
- `<workspace>/skills`

---

## 记忆系统 | Memory System

默认记忆目录结构如下：

The default memory directory layout is:

```text
~/.echo-dust-code/
├── memory/
│   ├── global/
│   └── projects/<slug>/
├── session/
├── mcp/servers.json
└── ECHO-DUST-CODE.md
```

- `memory/` 用于持久 Markdown 记忆。
  `memory/` stores persistent Markdown memory.
- `session/` 用于 `/resume` 会话保存与恢复。
  `session/` stores sessions used by `/resume`.
- `mcp/servers.json` 用于 MCP server 声明。
  `mcp/servers.json` declares MCP servers.
- `ECHO-DUST-CODE.md` 类似全局用户级 AGENTS 指令文件。
  `ECHO-DUST-CODE.md` works like a global user-level AGENTS file.

---

## 配置 | Configuration

运行时主要由 `config.yaml` 控制。

Runtime behavior is mainly controlled by `config.yaml`.

| 配置段 Section | 说明 Purpose |
|---|---|
| `llm` | 基础地址、模型、协议形态、请求超时、多工具返回开关。 Base URL, model, wire API, request timeout, parallel tool return mode. |
| `agent` | 主 agent 的初始步数预算、自适应扩展、绝对上限。 Main agent step budget, adaptive extensions, absolute limits. |
| `subagents` | 子代理开关、并发数、步数预算、回传大小限制。 Subagent enablement, concurrency, step budget, and result size cap. |
| `skills` | skill 注册根目录、top-k 检索数量和最低匹配分。 Skill roots, top-k retrieval count, and minimum match score. |
| `memory` | 记忆系统开关与用户目录。 Memory enablement and user directory. |
| `mcp` | MCP 目录、启动超时、请求超时。 MCP directory, startup timeout, and request timeout. |
| `session` | `/resume` 所依赖的 session 持久化。 Session persistence used by `/resume`. |
| `context` | 旧工具输出裁剪与历史压缩阈值。 Old tool-result pruning and history compaction thresholds. |
| `tools` | 文件读取、搜索、命令执行、patch 输出上限。 File read, search, command, and patch output limits. |
| `ui` | TUI 分隔线、实时区高度、viewer 轮询和预览截断。 TUI separators, live-frame bounds, viewer polling, and preview caps. |

---

## 架构 | Architecture

```text
cmd/agent/main.go
     │
     ├── internal/agent/       # ReAct loop, subagents, step budget
     ├── internal/approval/    # Risk classification and approval flow
     ├── internal/config/      # YAML + env loading
     ├── internal/context/     # Tool-result pruning and compaction
     ├── internal/llm/         # OpenAI-compatible client
     ├── internal/mcp/         # MCP stdio integration
     ├── internal/memory/      # Persistent Markdown memory
     ├── internal/runtimeevent/# UI event protocol
     ├── internal/skill/       # Skill manifest registry + retrieval
     ├── internal/tools/       # Built-in workspace tools
     ├── internal/tui/         # Maintained Bubble Tea TUI
     └── internal/ui/          # Legacy / non-TTY fallback renderer
```

### 主循环 | Core Loop

```text
Agent.Run(ctx, input)
  ├── pruneTransientToolHistory()
  ├── pruneStaleToolResults()
  ├── maybeCompact()
  ├── activateSkills()
  └── chatWithTools() → execute → loop until final answer or step limit
```

---

## 项目结构 | Project Structure

```text
echo-dust-code/
├── AGENTS.md
├── cmd/agent/
│   ├── main.go
│   └── slash.go
├── docs/
│   ├── WORKLOG.md
│   ├── TUI_MIGRATION_PLAN.md
│   └── TUI_SCROLL_MEMORY_ISSUE.md
├── internal/
│   ├── agent/
│   ├── approval/
│   ├── config/
│   ├── context/
│   ├── llm/
│   ├── logs/
│   ├── mcp/
│   ├── memory/
│   ├── runtimeevent/
│   ├── session/
│   ├── skill/
│   ├── tools/
│   ├── tui/
│   └── ui/
├── skills/
├── config.yaml
├── go.mod
└── README.md
```

---

## 开发与验证 | Development and Verification

```bash
# 运行全部测试 / run all tests
go test ./...

# 运行 vet
go vet ./...

# 格式化 Go 代码
gofmt -w cmd internal
```

说明：

Notes:

- 代码改动默认要求跑 `go test ./...`；涉及 agent、tool、UI、LLM 行为时也应跑 `go vet ./...`。
  Code changes are expected to run `go test ./...`; changes affecting agent/tool/UI/LLM behavior should also run `go vet ./...`.
- 大改动后需要追加 `docs/WORKLOG.md`，并使用中文记录。
  Large changes must append an entry to `docs/WORKLOG.md`, written in Chinese.

---

## 贡献说明 | Contributing

欢迎贡献，但请先遵守下面几条项目约束：

Contributions are welcome, but please follow these project conventions:

1. 大改动先开 issue 或 PR discussion。
   Open an issue or PR discussion first for major changes.
2. 保持 OpenAI-compatible 原生 function calling，不要引入 assistant 文本 JSON tool 协议。
   Keep native OpenAI-compatible function calling; do not introduce assistant-text JSON tool protocols.
3. 涉及 TUI/交互界面的新增行为，优先修改 `internal/tui/`。
   For UI or interaction changes, prefer `internal/tui/`.
4. Git 提交信息必须使用中文。
   Git commit messages must be written in Chinese.
5. 大改动后追加 `docs/WORKLOG.md`。
   Append to `docs/WORKLOG.md` after large changes.

---

## 许可证 | License

本项目采用 MIT License。详情见 [LICENSE](LICENSE)。

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

---

## 致谢 | Acknowledgments

- 灵感来自 [Claude Code](https://claude.ai/code) 和 [Codex CLI](https://github.com/openai/codex)。
  Inspired by [Claude Code](https://claude.ai/code) and [Codex CLI](https://github.com/openai/codex).
- Markdown 终端渲染基于 [Glamour](https://github.com/charmbracelet/glamour)。
  Markdown terminal rendering is powered by [Glamour](https://github.com/charmbracelet/glamour).
- TUI 基于 [Bubble Tea](https://github.com/charmbracelet/bubbletea)、[Bubbles](https://github.com/charmbracelet/bubbles) 和 [Lip Gloss](https://github.com/charmbracelet/lipgloss)。
  The TUI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).
