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

## 2026-06-13 - Plain Terminal Answer Style

- Summary: Tightened assistant response style for terminal output and added lightweight renderer cleanup for Markdown headings, tables, bold markers, inline backticks, and decorative emoji.
- Main modules: `internal/agent`, `internal/ui`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build go test ./...`; `env GOCACHE=/tmp/local-agent-go-build go vet ./...`.
- Notes: Tool outputs remain literal; cleanup only applies to assistant process/final text.

## 2026-06-13 - Markdown Final Rendering

- Summary: Replaced final-answer plaintext cleanup with terminal Markdown rendering using `github.com/charmbracelet/glamour`, while keeping lightweight cleanup for intermediate assistant process messages.
- Main modules: `internal/ui`, `internal/agent`, `go.mod`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`; `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`.
- Notes: Final summaries can use Markdown tables/headings and render cleanly in the terminal.

## 2026-06-13 - Markdown Heading Style

- Summary: Added a custom Glamour dark style for final-answer rendering that removes visible Markdown heading prefixes like `###` while keeping table, code, and emphasis rendering.
- Main modules: `internal/ui`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`; `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`.
- Notes: This addresses heading markers showing in terminal-rendered final answers.

## 2026-06-13 - Softer Markdown Code Blocks

- Summary: Disabled Chroma highlighting for final-answer Markdown code blocks so directory trees and plain text blocks render quietly instead of showing heavy red syntax coloring.
- Main modules: `internal/ui`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`; `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`.
- Notes: Inline code and tables still render through Glamour; only fenced code block styling was softened.

## 2026-06-13 - Approval Categories And Permanent Blacklist

- Summary: Added an approval subsystem with eight risk categories, CLI `allow` / `always` / `deny` decisions, process-local `always` memory, and a permanent command blacklist that cannot be approved.
- Main modules: `internal/approval`, `internal/agent`, `internal/runtimeevent`, `cmd/agent`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`; `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`; `env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`.
- Notes: Low-risk read/search/build-test calls run without prompting. Higher-risk calls ask in the CLI. Commands such as root filesystem deletion, Windows drive deletion, disk formatting, and raw block-device overwrites are blocked before approval.

## 2026-06-13 - Selectable Approval Prompt

- Summary: Replaced typed approval input with a selectable prompt using arrow keys, `j`/`k`, `tab`, and direct shortcuts. Approval blocks now summarize write arguments by path, line count, and byte count instead of printing large file content.
- Main modules: `internal/approval`, `internal/ui`.
- Verification: `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`; `env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`; `env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`.
- Notes: Non-TTY high-risk approvals still default to deny.

## 2026-06-14 - 并发工具调度

- 摘要：新增多个原生 tool call 的并发执行、写入目标锁、工作区写入的 session 级审批，以及工作区外写入的 loop 级审批。
- 主要模块：`internal/agent`、`internal/approval`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/approval`。
- 备注：同文件写入会串行执行，未知工作区写入会使用工作区级锁。工具结果消息仍按模型返回的 tool call 原始顺序写回会话。

## 2026-06-14 - 路径查找工具

- 摘要：参考 tiny-agent 增加 `find_files` 只读工具，用于按文件名、目录名或相对路径递归查找工作区内容；系统提示词也明确要求查找位置或确认路径是否存在时优先使用 `find_files`，避免只用 `list_files` 查看当前目录第一层就下结论。
- 主要模块：`internal/tools`、`internal/agent`、`internal/approval`、`internal/ui`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/approval ./internal/tools`。
- 备注：`find_files` 默认跳过 `.git`、`.agents`、`.codex`、`.codegraph`、`.cursor`、`node_modules` 等目录；它用于路径查找，不替代文件内容搜索。

## 2026-06-14 - TODO 工作流

- 摘要：新增 `update_todos` 原生伪工具和 `todo_update` 运行时事件，要求具体工作任务在调用 workspace 工具前先创建 TODO，并在终端以任务列表块展示进度。
- 主要模块：`internal/agent`、`internal/runtimeevent`、`internal/ui`、`internal/approval`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：TODO 状态只在单次用户请求内有效，不跨轮次或 session 持久化；简单最终回答不强制创建 TODO。

## 2026-06-14 - TODO 固定区和工具折叠

- 摘要：新增 `run_start`/`run_end` 运行时事件，将一次任务的终端输出组织为上方 TODO 固定区和下方 Tools 区；Tools 默认折叠，TTY 中可用 `Ctrl+E` 在当前任务内展开或收起。
- 主要模块：`internal/agent`、`internal/runtimeevent`、`internal/ui`、`cmd/agent`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：TODO 完成态改为绿色 `[✓]`，不再使用 `[x]`；审批输入期间会暂停工具区快捷键监听，避免抢占 stdin；实时重绘前会先回到行首，并在 live frame 中显式输出 CRLF，避免 raw mode 下多次刷新后画面斜向漂移。

## 2026-06-14 - Live Frame 展开区限幅

- 摘要：为 TODO/Tools live frame 增加终端视口高度和宽度限制；Tools 展开态只展示最近工具事件，并截断长行和长输出，避免 `Ctrl+E` 展开后因滚屏在历史日志中留下重复块。
- 主要模块：`internal/ui`、`README.md`、`go.mod`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：终端无法可靠擦除已经滚入 scrollback 的旧内容，因此 live frame 必须保持在视口内；完整工具结果仍保留在模型上下文中，终端展开区只做可读预览。

## 2026-06-14 - 全量工具日志查看

- 摘要：新增 `Ctrl+T` 全量工具日志查看器，在 alternate screen 中展示当前任务的完整工具事件和结果输出；`Ctrl+E` 继续作为稳定的短预览。
- 主要模块：`internal/ui`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：查看器支持 `q`、`Esc` 或再次按 `Ctrl+T` 退出，支持方向键、`j`/`k`、`PgUp`/`PgDn`、`g` 和 `G` 导航；查看期间会暂停 live frame 渲染，退出后恢复当前任务区。

## 2026-06-15 - TODO 工具下沉和审批重绘修复

- 摘要：将 `update_todos` 从 agent 层迁移到 `internal/tools`，作为 UI-only 原生工具管理当前任务 TODO；每轮结束后清理 `update_todos` 的历史 tool call，避免下一轮模型继承旧计划；审批结束后的 live frame 重绘会一并清理审批选项行。
- 主要模块：`internal/tools`、`internal/agent`、`internal/runtimeevent`、`internal/ui`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：`update_todos` 仍通过原生 function calling 执行，但不再作为长期对话记忆的一部分；完整 TODO 状态只在当前用户请求内有效。

## 2026-06-15 - 运行参数配置化

- 摘要：新增根目录 `config.yaml`，将 LLM 请求超时与并发 tool call、Agent 最大步数、工具读取/搜索/命令/补丁限制、文件变更预览行数，以及 UI live frame/full log/Markdown/预览截断参数改为启动时加载；入口程序会把配置映射到 LLM client、工具注册和 UI renderer。
- 主要模块：`config.yaml`、`cmd/agent`、`internal/config`、`internal/llm`、`internal/tools`、`internal/ui`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`env XDG_CACHE_HOME=/tmp/local-agent-cache GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod /go/bin/staticcheck ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test -race ./internal/agent ./internal/ui`。
- 备注：`AGENT_API_KEY` 仍只从环境变量读取，不写入配置文件；`AGENT_BASE_URL`、`AGENT_MODEL`、`AGENT_MAX_STEPS` 会覆盖 YAML 值。协议枚举、工具名、快捷键和安全分类仍保留在代码中，不作为运行调优参数开放。

## 2026-06-15 - UI 渲染器拆分

- 摘要：将过大的 `internal/ui/renderer.go` 按职责拆分为入口事件处理、live frame、TODO、工具日志、文件变更、Markdown final 和审批详情多个文件，保持 `BlockRenderer` 对外 API 和渲染行为不变。
- 主要模块：`internal/ui`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/ui`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`git diff --check`。
- 备注：这是结构性拆分，没有改变终端 UI 的显示协议；`renderer.go` 从 903 行降到约 200 行，后续可继续按工具事件类型细化 `renderer_tools.go`。

## 2026-06-15 - Agent 工具调度拆分

- 摘要：将 `internal/agent/agent.go` 中的工具调度、审批、写入目标锁、TODO 伪工具执行和 function tool 列表生成拆到独立文件，保留 `Agent.Run` 主循环、消息管理和事件入口在主文件中。
- 主要模块：`internal/agent`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/agent`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`git diff --check`。
- 备注：这是同包结构拆分，不改变 tool call 并发、审批、TODO 或消息写回顺序；`agent.go` 降到约 140 行。

## 2026-06-15 - 只读 Subagent

- 摘要：新增 `delegate_task` 原生工具，用于启动隔离消息历史的只读研究子代理；子代理只把最终结论作为 tool result 回传，不继承父会话、不暴露 `delegate_task`，并复用现有工具调度安全链路。
- 主要模块：`internal/agent`、`internal/approval`、`internal/config`、`internal/ui`、`cmd/agent`、`README.md`、`config.yaml`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/agent`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/config`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/ui`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`git diff --check`。
- 备注：默认最多并发 2 个子代理，每个最多 8 步，结果最多回传 12288 字节；子代理可以使用安全命令，但写入、高风险或永久黑名单命令会被现有 pre-use 安全策略拒绝。

## 2026-06-15 - Subagent 使用策略强化

- 摘要：强化主 Agent 系统提示词和 `delegate_task` 工具描述，要求大范围代码分析、架构审查、项目缺失项排查等任务优先委托子代理，避免主上下文直接读取大量文件；同时要求最终回答自洽，不引用隐藏工具日志或不可见分析过程。
- 主要模块：`internal/agent`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/agent`。
- 备注：这是提示词层面的行为约束，没有新增硬编码意图分类。

## 2026-06-15 - Subagent 日志与全量日志实时刷新

- 摘要：将子代理内部工具事件转发到父 Agent 的 Tools 日志，并用 `Subagent` 前缀标记来源；强化提示词要求大范围分析拆成多个独立 `delegate_task`；`Ctrl+T` 全量工具日志查看器改为轮询最新日志文本，打开后仍会实时刷新。
- 主要模块：`internal/agent`、`internal/runtimeevent`、`internal/ui`、`README.md`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/agent`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/ui`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`git diff --check`。
- 备注：子代理的 `run_start`、`run_end`、`todo_update` 和 `final` 不转发到父 UI，避免破坏父任务的 TODO/live frame；只转发工具过程、审批、错误和中间助手消息。

## 2026-06-15 - 并行工具调用上限

- 摘要：新增 `agent.max_parallel_tool_calls` 配置，默认限制单轮 assistant 回复最多 10 个非 `update_todos` 工具调用；系统提示词同步说明同一工具不同参数实例也计入上限；调度层会拒绝超出上限的调用并用信号量限制实际并发。
- 主要模块：`internal/agent`、`internal/config`、`cmd/agent`、`README.md`、`config.yaml`。
- 验证：`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/agent`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./internal/config`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go test ./...`；`env GOCACHE=/tmp/local-agent-go-build GOMODCACHE=/tmp/local-agent-go-mod go vet ./...`；`git diff --check`。
- 备注：`update_todos` 是当前任务的 UI 控制工具，仍先同步执行，不计入并行工具调用上限。

## 2026-06-15 - Ctrl+T 日志查看退出死锁修复

- 摘要：修复任务运行结束或进入审批请求时，若用户仍停留在 `Ctrl+T` 全量工具日志查看器中，退出查看器后可能卡在 `Ctrl+E` 工具展开区且不显示最终回答的问题；阻塞式停止快捷键监听器时不再持有 UI renderer 锁。
- 主要模块：`internal/ui`。
- 验证：`go test ./...` 通过；`go vet ./...` 通过。
- 备注：新增回归测试覆盖 `run_end` 和审批请求两个路径，确保等待日志查看器关闭时 renderer 锁仍可被查看器退出后的重绘流程获取。

## 2026-06-15 - Subagent 自动 TODO 初始化

- 摘要：子代理启动时自动初始化内部 TODO 状态，避免只读研究子代理在第一次 `read_file`、`list_files`、`find_files` 等 workspace 工具调用前因未显式调用 `update_todos` 被调度层拒绝；同时更新子代理提示词，说明只有需要修订计划时才调用 `update_todos`。
- 主要模块：`internal/agent`、`internal/tools`。
- 验证：`go test ./internal/agent ./internal/tools` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：主 Agent 的 TODO 门禁保持不变；新增回归测试覆盖子代理不先调用 `update_todos`、直接执行只读工具的路径。

## 2026-06-15 - Ctrl+T 子代理日志分组

- 摘要：`Ctrl+T` 全量工具日志改为 Main block 与 Subagent block 分组显示；每个子代理使用稳定编号和独立颜色，子代理 block 默认折叠，可用 `Ctrl+1` 到 `Ctrl+5` 或数字键切换展开；事件编号如 `[1]` 保持无色。
- 主要模块：`internal/agent`、`internal/runtimeevent`、`internal/ui`。
- 验证：`go test ./internal/agent ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：子代理编号按父 Agent 单轮 `delegate_task` tool call 顺序分配；超过 5 个子代理仍会显示 block，但没有快捷键。

## 2026-06-15 - Ctrl+T 日志颜色和 Main 折叠修正

- 摘要：调整 `Ctrl+T` 全量日志颜色规则，只给事件编号前缀如 `[1]` 着色，日志标题和正文保持终端默认颜色；Main block 增加 `Ctrl+0` 和数字 `0` 展开/折叠，默认展开。
- 主要模块：`internal/ui`。
- 验证：`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：保留子代理 block 默认折叠和 `Ctrl+1` 到 `Ctrl+5` 切换逻辑。

## 2026-06-15 - 最终回答可见性修复

- 摘要：任务结束时自动将 Tools live frame 折叠后再渲染最终回答，避免展开的大量工具日志把总结挤出视口；同时强化系统提示词，要求使用工具或子代理后最终回答必须完整综合结论，不能只用“以上分析”“如上”等短语引用隐藏日志。
- 主要模块：`internal/ui`、`internal/agent`。
- 验证：`go test ./internal/agent ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：运行中仍可用 `Ctrl+E` 展开工具日志；只在 `RunEnd` 的最后一帧自动收起，保证 final answer 优先可见。

## 2026-06-15 - Ctrl+T 滚轮输入解析修复

- 摘要：修复 `Ctrl+T` 全量工具日志查看器中大幅滚轮滚动可能退出预览界面的问题；输入解析现在会缓存被读取缓冲区截断的 ESC/CSI 序列，并支持 SGR/X10 鼠标滚轮事件，只在确认是单独 `Esc` 后关闭查看器。
- 主要模块：`internal/ui`。
- 验证：`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：单独 `Esc` 关闭会等待一次短轮询以区分它和 ESC 序列前缀；若终端长时间只发送半截转义序列，该半截输入会被丢弃而不是关闭查看器。

## 2026-06-16 - 最终回答显示修复

- 摘要：修复交互式运行结束后最终回答被前一个 Todo/Tools 实时帧挤出终端可视区的问题；最终回答渲染前会清除临时 live frame，再从原位置打印完整 Markdown 内容。
- 主要模块：`internal/ui`。
- 验证：`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：该修复不依赖缩短工具输出预览，`config.yaml` 中工具预览字符数保持默认值；全量工具日志仍可通过 `Ctrl+T` 查看。

## 2026-06-16 - 第二轮输入提示显示修复

- 摘要：修复第二轮任务开始后，用户刚提交的提示词可能被实时 UI 帧滚动或重绘挤出可视区的问题；Agent 现在发出 `user_message` 运行事件，交互式 renderer 将用户问题纳入 live frame，并在最终回答前重新打印为稳定 transcript 行。
- 主要模块：`internal/agent`、`internal/runtimeevent`、`internal/ui`。
- 验证：`go test ./internal/ui` 通过；`go test ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：`user_message` 不计入 Tools event 数量；交互式输入行提交后会清空编辑行，由 renderer 负责统一回显，避免 prompt 残留与 live frame 重绘互相覆盖。

## 2026-06-18 - Reasonix 上下文工程分析文档

- 摘要：新增 DeepSeek Reasonix Go 版本上下文工程剖析文档，说明其通过稳定 system prompt、工具 schema 规范化、append-only 会话日志、reasoning 内容克制回放、低频 compaction、子代理隔离和 cache telemetry 来提高 DeepSeek prefix cache 命中率。
- 主要模块：`docs/DEEPSEEK_REASONIX_CONTEXT_ENGINEERING.md`、`docs/WORKLOG.md`。
- 验证：`git diff --check` 通过；未运行 Go 测试，因为本次仅新增和更新文档。
- 备注：分析依据来自上游 `main-v2` Go 版本源码和 legacy 缓存基准文档；缓存命中率数字引用上游报告，未在本地独立复测。

## 2026-07-06 - TUI 复制鼠标协议串泄漏修复

- 摘要：修复 TUI 在拖拽复制正文时，终端偶发把 SGR 鼠标协议尾巴如 `[<32;26;29M` 注入到底部输入框的问题；新增一个短窗口的鼠标协议过滤器，在鼠标事件后拦截整段或分段泄漏的协议串，避免它们被 textarea 当作普通文本插入。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_update.go`、`internal/tui/mouse_protocol.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`go test ./internal/tui/...` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前过滤器只针对 SGR 鼠标协议格式 `ESC [ < ... M/m` 及其缺失 `ESC` 的尾巴泄漏做防护；如果后续某些终端还会泄漏其他鼠标上报格式，需要继续扩展匹配规则。

## 2026-07-02 - 新增 internal/tui Bubble Tea 交互界面

- 摘要：保留原始 `internal/ui` 目录不动，新增 `internal/tui` 作为新的 Bubble Tea 界面实现；TTY 交互模式默认切到新 TUI，非 TTY 继续走旧 `internal/ui`。同时把 slash 命令改成可返回文本，方便 TUI 在界面内展示 `/info`、`/model` 和未知命令输出。
- 主要模块：`internal/tui`、`cmd/agent/main.go`、`cmd/agent/slash.go`、`go.mod`、`go.sum`。
- 验证：`go mod tidy` 通过；`gofmt -w cmd/agent/main.go cmd/agent/slash.go cmd/agent/slash_test.go internal/tui/bridge.go internal/tui/helpers.go internal/tui/model.go internal/tui/model_test.go` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：新 TUI 已接入滚轮滚动、审批弹窗、运行时事件转发、slash 建议和运行中中断；旧 `internal/ui` 仍保留并作为非交互终端回退路径。由于引入 `github.com/charmbracelet/bubbles v1.0.0`，模块 `go` 版本提升为 `1.24.2`。

## 2026-07-02 - TUI 改成大 Banner + 无框内容区

- 摘要：按新的终端布局要求，去掉中间 `Session` 固定边框盒子，顶部改为大号 `ECHO DUST CODE` banner，中间内容区改成无框滚动区，底部改成独立输入框。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_test.go` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：宽终端下显示 ASCII 大字 banner，窄终端下自动回退为紧凑标题；输入框保留 slash 建议，但不再在内容区外层加固定边框。

## 2026-06-18 - Reasonix 风格 Memory

- 摘要：新增 `internal/memory` 包，实现启动时层级文档记忆加载、`@path` 导入、稳定 system prompt 拼接、Markdown 持久 fact store，以及 `memory`、`remember`、`forget` 三个原生工具；入口按配置加载 memory 并注册工具，主 agent 和 subagent 都继承同一个 memory block。
- 主要模块：`internal/memory`、`cmd/agent`、`internal/agent`、`internal/config`、`internal/approval`、`README.md`、`config.yaml`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/memory` 通过；`go test ./internal/config ./internal/agent ./internal/approval` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：`memory` 工具是只读；`remember` 和 `forget` 写入 `memory.user_dir` 下的用户记忆目录，按现有审批系统作为外部写入处理。当前搜索使用标准库的轻量文本匹配，后续可替换为更强检索而不改变工具接口。

## 2026-06-18 - 上下文裁剪与压缩

- 摘要：新增上下文维护配置和运行事件；每轮任务开始时先裁剪旧的大型 tool result 输出，保持 tool call 配对不变；接近上下文窗口阈值时执行保守 compaction，用无工具 LLM 摘要旧历史并插入 `<compaction-summary>`，失败或无收益时保留原历史。
- 主要模块：`internal/agent`、`internal/config`、`internal/runtimeevent`、`internal/ui`、`README.md`、`config.yaml`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/agent ./internal/config ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前 token 统计仍是字符数近似，compaction 没有归档被折叠原文；后续可在缓存观测和 PrefixShape 诊断完成后再做更精确的触发策略和归档。

## 2026-06-18 - 工具补全与模型注册

- 摘要：新增 `read_file_range`、`find_symbol`、`find_references`、`find_callers`、`find_callees`、`git_status`、`git_diff`、`git_log`，并为 `search_files` 增加 regex 搜索；所有新工具都已注册到主 agent，读写/搜索风险分类已接入审批系统，子代理也同步获得只读代码导航和 Git 检查能力。
- 主要模块：`internal/tools`、`internal/approval`、`internal/agent`、`README.md`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/tools ./internal/approval ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：Go 代码导航工具当前复用本地 `codegraph` 与 `gopls` CLI，运行时会落到 `/tmp` 下的私有缓存目录；`find_references` / `find_callers` / `find_callees` 目前按 Go 文件位置工作，不做跨语言语义分析。

## 2026-06-18 - 终端流式输出

- 摘要：为 OpenAI-compatible client 增加流式 chat completion 支持，Agent 在 provider 支持时优先走 streaming，把增量 assistant 文本通过新的 runtime event 发给 UI；终端 live frame 现在可在最终回答前实时显示 assistant 正在输出的内容，同时保留原有 tool call、tool result 和最终 Markdown 渲染路径。
- 主要模块：`internal/llm`、`internal/agent`、`internal/runtimeevent`、`internal/ui`、`README.md`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/llm ./internal/agent ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前流式仅对 assistant 文本增量生效；tool call 仍按单个 assistant turn 完整落地后再调度执行。若 provider 的 SSE 工具调用增量格式有差异，后续可能还需补更严格的兼容解析。

## 2026-06-18 - 运行错误日志

- 摘要：新增 `internal/logs` 包，默认把 CLI 运行日志写到工作区 `.local-agent/logs/agent.log`；主程序现在会打印日志路径，并记录 agent run 错误、LLM 普通请求错误、streaming SSE 解析错误和无最终回答的中止错误，便于复现终端异常时直接查看具体失败原因。
- 主要模块：`internal/logs`、`cmd/agent`、`internal/llm`、`internal/agent`、`README.md`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前日志仍是本地文件单点追加，不做轮转；后续如果错误量变大，可再加级别过滤、大小轮转和按运行会话分文件。

## 2026-06-18 - 输入框样式优化

- 摘要：重做终端输入框渲染，改为带淡色背景的单行输入条；空输入时显示浅灰 placeholder，输入时显示高亮提示箭头，并补齐光标回退和整行背景铺满逻辑，修复 placeholder 状态下光标跑到最右侧的问题。
- 主要模块：`internal/ui/prompt.go`、`internal/ui/prompt_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前样式仍基于 ANSI 颜色和空格填充模拟输入条，不做真实圆角；后续若继续增强视觉效果，可在不影响 raw-mode 光标定位的前提下再调色和彩色边框。

## 2026-06-18 - 终端缩放时实时面板尺寸刷新

- 摘要：修复 live frame 在终端窗口被频繁缩小和放大时继续沿用旧尺寸渲染的问题；`renderFrame()` 现在会在每次重绘前重新读取当前终端宽高，更新实时区域的行数和宽度限制，减少缩放过程中的重复叠印和错位。
- 主要模块：`internal/ui/renderer.go`、`internal/ui/renderer_frame.go`、`internal/ui/renderer_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：这次修复先解决“尺寸只在初始化时读取一次”的根因；若后续仍存在极端缩放场景的残影，需继续补针对 terminal resize 的更强清屏和 frameLines 重算策略。

## 2026-06-18 - 缩放后实时面板清理行数重算

- 摘要：继续修复终端极端缩放时 live frame 残留叠印问题；为 renderer 增加上一帧文本和宽度缓存，在清理旧 frame 与最终回答前，不再只依赖旧的 `frameLines`，而是按当前宽度重新估算上一帧的折行占用，提升缩窄后回退清屏的准确性。
- 主要模块：`internal/ui/renderer.go`、`internal/ui/renderer_frame.go`、`internal/ui/renderer_final.go`、`internal/ui/format.go`、`internal/ui/renderer_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前按可见宽度和去 ANSI 后文本宽度估算折行数，已覆盖最常见的缩放重复渲染问题；若后续发现某些终端在 resize 过程中还会主动重排历史滚动区，可能仍需结合 SIGWINCH 或强制整块重绘策略进一步兜底。

## 2026-06-19 - Responses API 接入

- 摘要：为 OpenAI-compatible client 增加 `llm.wire_api` 协议配置，支持继续使用 `/chat/completions`，也可切换到 `/responses`；Responses 模式使用扁平 function tool schema，能回放 function call 和 function_call_output，并解析 message、function_call 与 usage。默认配置切到 AnyRouter `https://anyrouter.top/v1`、`gpt-5.5`、`responses`，对齐 Codex provider 的 wire API 形态。
- 主要模块：`internal/llm`、`internal/config`、`cmd/agent`、`internal/tools`、`README.md`、`config.yaml`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/llm ./internal/config` 通过；`go test ./internal/tools` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：Responses 模式下 `ChatWithToolsStream` 暂时降级为非流式 `/responses` 请求，并把完整文本一次性发送给 UI；后续如需逐字流式输出，需要补 Responses SSE 事件解析。全量测试中发现 gopls 工具隔离环境缺少临时目录和模块缓存继承，已同步修复，避免代码导航测试依赖网络重新下载模块。

## 2026-06-19 - AnyRouter Codex Responses 适配

- 摘要：将 Responses 请求体调整为更接近 Codex CLI 的形态：system 消息提升为 `instructions`，普通消息使用 typed content 数组，工具 schema 增加 `strict:false`，并固定发送 `tool_choice`、`store`、`include`、`prompt_cache_key` 和 `client_metadata`；同时为 Responses 模式补齐真正的 SSE 流式解析，支持文本增量、完成态 usage 和 `function_call` tool call。
- 主要模块：`internal/llm`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/llm/openai.go internal/llm/openai_test.go` 完成；`go test ./internal/llm` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：尚未在本地通过真实 AnyRouter 网络请求复测；如果 AnyRouter 还要求 reasoning、service_tier 或更完整的 Codex metadata，后续需要继续按返回错误补字段。用户此前贴出的 API key 应视为已泄露，建议尽快轮换。

## 2026-06-19 - AnyRouter Reasoning 字段修复

- 摘要：通过独立最小 `/responses` 请求矩阵确认 AnyRouter 的 `gpt-5.5` Codex 路由要求携带 `reasoning` 字段；无 reasoning 的最小请求返回 `invalid_responses_request`，加入 `reasoning: {effort: "xhigh", summary: "auto"}` 和 `include: ["reasoning.encrypted_content"]` 后返回 200。随后在 Responses client 中对 `gpt-5*` / Codex 类模型自动附加该字段。
- 主要模块：`internal/llm`、`docs/WORKLOG.md`。
- 验证：`curl` 最小 no-tool 请求返回 200；`curl` 最小 function-tool 请求返回 200；`gofmt -w internal/llm/openai.go internal/llm/openai_test.go` 完成；`go test ./internal/llm` 通过；`go test ./...` 通过；`go vet ./...` 通过；使用 AnyRouter token 运行 `printf "hello\nexit\n" | AGENT_API_KEY=... go run ./cmd/agent` 成功返回 “Hello! How can I help?”。
- 备注：当前 reasoning 开关按模型名启用，避免影响普通非 reasoning Responses 模型；如后续接入新的 reasoning 模型命名，需要扩展匹配规则或做成显式配置。

## 2026-06-20 - MCP 接入与全局提示词

- 摘要：新增 `internal/mcp` 包，实现 stdio MCP server 启动、JSON-RPC `initialize`、`tools/list`、`tools/call` 和工具适配；CLI 启动时按 `mcp` 配置读取 `~/.local-agent/mcp/servers.json`，将远端 MCP 工具注册为 `mcp__<server>__<tool>` 形式的原生 function tool，并在退出时关闭 MCP 子进程。记忆加载新增用户目录 `LOCAL-AGENT.md`，作为全局提示词文件优先于用户目录中的旧 AGENTS 风格文件。
- 主要模块：`internal/mcp`、`cmd/agent`、`internal/config`、`internal/memory`、`README.md`、`config.yaml`、`docs/WORKLOG.md`。
- 验证：`gofmt -w ...` 完成；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前 MCP 实现覆盖 stdio transport 和 tools 能力，暂不支持 MCP resources/prompts、SSE transport、server 动态 tool list 变更通知，失败的 MCP server 会写入运行日志并跳过，不阻塞其他工具启动。

## 2026-06-27 - Token 消耗计数系统

- 摘要：新增 LLM token 消耗追踪，每次 `chatWithTools` 调用后累加 provider 返回的 `prompt_tokens`/`completion_tokens`/`total_tokens`，并通过新的 `TypeTokenUsage` 运行时事件把 per-call 值和累计总量推给 UI；主 agent 和子代理分别追踪，UI live frame 中用 `Tokens:` 块显示主 agent、各子代理（按任务名截断）和 total。Run 结束时在 `.local-agent/logs/agent.log` 写一条汇总日志。完全使用 provider 返回的 usage，不引入本地 tokenizer。
- 主要模块：`internal/runtimeevent`、`internal/agent`、`internal/ui`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/agent ./internal/ui` 通过；`go test ./...` 通过（`internal/tools` 中 `TestGoCodeNavigationTools` 的 gopls builtin `append` 报错是已有问题，与本次改动无关）；`go vet ./...` 通过。
- 备注：`Agent.tokenMu` 保护流式回调的并发累加；`BlockRenderer.subagentTokens` 按 `SubagentIndex` 分组，`subagentTaskMap` 记录任务名用于显示。后续如需成本估算，可在配置中加入模型单价并在 UI 增加费用列，不需要改动事件和累加链路。

## 2026-06-27 - Qwen 模型 Responses API 兼容

- 摘要：百炼平台的 Qwen 模型不支持 `/responses` 端点（返回 404），与 DeepSeek 一样需要强制走 `/chat/completions`。扩展 `usesChatCompletionsOnly()` 匹配规则，当模型名包含 `qwen` 时自动回退到 chat completions。
- 主要模块：`internal/llm`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/llm` 通过（新增 `TestOpenAICompatibleClientUsesChatCompletionsForQwenModels`）；`go vet ./...` 通过。
- 备注：当前回退规则按模型名前缀匹配，后续若百炼平台支持 `/responses`，可移除 `qwen` 匹配或改为配置化。

## 2026-06-27 - Token 汇总改为最终回答后稳定输出

- 摘要：修复 token 消耗信息在 live frame 中被立即清除、用户无法看到的问题。在 `renderFinal` 最终回答 Markdown 渲染完成后，追加一个稳定的 `Tokens:` 汇总行，直接写入终端 scrollback 不再被擦除。live frame 中的实时 token 块保持不变，运行中仍可看到累计变化。
- 主要模块：`internal/ui/renderer_final.go`、`internal/ui/renderer_frame.go`、`internal/ui/renderer_test.go`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/ui` 通过（新增 `TestBlockRendererPrintsTokenSummaryAfterFinalAnswer`）；`go vet ./...` 通过。
- 备注：修复原因——之前 token block 只在 live frame 中存在，`renderFinal` 会先 `clearLiveFrame` 再打印最终回答，导致 token 信息被清掉；现在最终回答后直接追加一行汇总，确保用户能看到。

## 2026-06-27 - Token 汇总无数据时显示 N/A

- 摘要：百炼平台的 qwen 模型在 `/chat/completions` 响应中不返回 `usage` 字段，导致 token 计数器始终为 0，最终回答后不显示 token 汇总。改为无条件输出 token 行：有数据时显示具体数值，无数据时显示 `N/A (provider did not return usage)`，让用户明确知道是 provider 未返回而非系统故障。
- 主要模块：`internal/ui/renderer_frame.go`、`internal/ui/renderer_test.go`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/ui` 通过（新增 `TestBlockRendererShowsNAWhenProviderOmitsUsage`）；`go vet ./...` 通过。
- 备注：日志中 `token usage: prompt=0 completion=0 total=0` 也印证了这一点；后续若百炼平台补上 usage 字段，会自动显示真实数值。

## 2026-06-28 - 百炼流式响应 usage 字段缺失处理

- 摘要：百炼平台的 qwen 模型在流式响应（SSE）中所有 chunk 的 `usage` 字段均为 `null`，导致 agent 无法获取 token 消耗数据。这是百炼平台的限制，非流式请求会正常返回 usage。已更新 N/A 提示信息，说明可能是流式模式或 provider 未返回 usage。
- 主要模块：`internal/ui/renderer_frame.go`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/ui` 通过；手动测试确认百炼流式响应所有 chunk 的 `usage` 均为 `null`。
- 备注：若需要准确 token 统计，可临时禁用流式（设置 `llm.parallel_tool_calls: false` 或类似配置强制非流式），但会失去实时显示优势。百炼未来可能补上流式 usage，届时会自动显示真实数值。

## 2026-06-28 - 流式无 usage 时自动降级到非流式

- 摘要：百炼平台 qwen 模型流式 SSE 所有 chunk 的 `usage` 均为 `null`，导致 token 计数器始终为 0。新增 `Agent.streamingDisabled` 标志：当流式调用返回 `resp.Usage == nil` 时自动设置该标志，后续调用切换到非流式路径（`ChatWithTools` 而非 `ChatWithToolsStream`），非流式响应正常返回 usage。降级时记录日志 `streaming returned no usage, falling back to non-streaming`。
- 主要模块：`internal/agent/agent.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 验证：`go build ./...` 通过；`go test ./internal/agent` 通过（新增 `TestRunFallsBackToNonStreamingWhenUsageOmitted`）；`go vet ./...` 通过。
- 备注：降级是 per-session 的——一旦检测到流式无 usage，当前 agent 实例剩余调用全部走非流式。代价是失去实时流式显示，换来准确的 token 统计。非流式模式下用户看不到逐字输出，但最终回答仍正常渲染。

## 2026-06-30 - 自适应 ReAct 步数预算

- 摘要：将固定 `max_steps` 升级为“初始预算 + 自适应扩展 + 绝对上限”。主 Agent 和子代理在工具仍成功、TODO 仍有未完成项且未检测到重复工具循环时，可以按批次自动扩展步数；若扩展次数耗尽、上下文接近强制压缩阈值、连续失败或重复调用，则发出预算耗尽事件并停止。
- 主要模块：`internal/agent`、`internal/config`、`internal/runtimeevent`、`internal/ui`、`cmd/agent`、`config.yaml`、`README.md`、`internal/tools/tools_test.go`。
- 验证：`gofmt -w ...` 完成；`go test ./internal/agent` 通过；`go test ./internal/config` 通过；`go test ./internal/ui` 通过；`go test ./internal/tools` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：`agent.max_steps` 和 `subagents.max_steps` 现在表示初始预算；新增 `adaptive_max_steps_enabled`、`max_step_extensions`、`step_extension_size`、`absolute_max_steps` 控制自动续跑。`AGENT_MAX_STEPS` 仍覆盖主 Agent 初始预算，若超过默认绝对上限会同步抬高上限以保持兼容。另将代码导航测试从固定行号改为按源码定位目标函数，避免后续编辑导致误报。

## 2026-06-30 - 蓝白启动 Banner

- 摘要：新增 Claude Code 风格的 `ECHO DUST CODE` 启动界面。TTY 且终端宽度足够时显示蓝白大字 Banner；非 TTY 或窄屏环境自动降级为简洁蓝白文本，避免日志和小窗口输出错乱。
- 主要模块：`internal/ui/startup.go`、`internal/ui/startup_test.go`、`cmd/agent/main.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w cmd/agent/main.go internal/ui/startup.go internal/ui/startup_test.go` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：本次只调整启动显示，不改 prompt 输入框、运行事件渲染和工具日志 UI。

## 2026-06-30 - 系统提示词分区整理

- 摘要：将主 Agent 和子代理系统提示词从单一长列表整理为按职责划分的短分区，分别覆盖角色、工具使用、委派、工作区导航和最终回答要求。保留原有 native function calling、TODO、delegate_task、路径查找和最终回答自包含等关键约束。
- 主要模块：`internal/agent/agent.go`、`internal/agent/subagent.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/agent/agent.go internal/agent/subagent.go internal/agent/agent_test.go` 完成；`go test ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：本次只重组提示词结构，不改变工具协议、工具暴露规则、子代理隔离机制或运行时调度逻辑。

## 2026-06-30 - Context 维护逻辑独立成包

- 摘要：将上下文维护核心逻辑从 `internal/agent` 拆到 `internal/context`，集中放置 tool result 剪枝、compaction 触发判断、消息压缩、token 估算和摘要输入格式化。Agent 侧只保留调用摘要模型、替换消息和发出 runtime event 的适配方法，并合并进 `agent.go`，删除 `internal/agent/context_maintenance.go` 和对应测试文件。
- 主要模块：`internal/context/maintenance.go`、`internal/context/maintenance_test.go`、`internal/agent/agent.go`、`internal/agent/agent_test.go`、`internal/agent/options.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/context/maintenance.go internal/context/maintenance_test.go internal/agent/agent.go internal/agent/agent_test.go internal/agent/options.go internal/agent/step_budget.go` 完成；`go test ./internal/context ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：本次是结构拆分，不改变剪枝阈值、压缩摘要格式、recent tail 保留策略、runtime event 类型或外部配置字段。

## 2026-06-30 - 输入框长文本粘贴单行裁剪

- 摘要：修复 Linux 终端中使用 `Ctrl+Shift+V` 粘贴长文本后输入框不断产生残留换行的问题。输入状态仍保存完整文本，渲染时按终端宽度只显示光标附近的可见窗口，避免终端自动软换行导致旧物理行无法清理。
- 主要模块：`internal/ui/prompt.go`、`internal/ui/prompt_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/ui/prompt.go internal/ui/prompt_test.go` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：本次保持输入框为单行 UI，不改变提交内容、历史记录、左右移动和回车行为。后续如需要多行编辑，应改成显式多行布局并记录/清理占用行数。

## 2026-06-30 - 输入框粘贴换行不再自动提交

- 摘要：启用终端 bracketed paste 模式，识别 `Ctrl+Shift+V` 粘贴块并作为普通文本批量插入。粘贴内容中的换行会保存在输入状态中，并在输入框内按多行显示，不再被当成回车提交给 Agent，用户可以继续编辑后再手动按 Enter 提交。
- 主要模块：`internal/ui/input.go`、`internal/ui/prompt.go`、`internal/ui/prompt_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/ui/input.go internal/ui/prompt.go internal/ui/prompt_test.go` 完成；`go test ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：输入框现在会记录上次渲染的行数并在重绘前清理旧多行区域；左右移动和退格仍按字符移动，上下键仍保留为历史导航，暂不做多行内垂直光标移动。

## 2026-07-01 - 输入框长文本自动换行

- 摘要：把输入框的渲染模型从"水平滚动 + 裁剪可视窗口"改成"按终端可用宽度自动折行"。超过终端宽度的字符现在会折到下一行显示，而不是被截掉看不见。
- 主要模块：`internal/ui/prompt.go`、`internal/ui/prompt_test.go`。
- 改动要点：
  - `promptDisplayLines()` 重构为两层：先按 `\n` 拆逻辑行，再对每个逻辑行调新加的 `wrapLogicalLine()` 按 cell width 折屏幕行。
  - `renderPromptRow()` 简化，不再调用 `visiblePromptRunes` 裁剪，`line.Runes` 已是折行后的片段。
  - 删除不再使用的 `visiblePromptRunes` 函数。
  - 光标边界行为：wrap 边界让光标落到下一行行首；逻辑行末尾光标落到最后一个 wrap 行末尾。
- 验证：`go test ./internal/ui/...` 通过（8 个测试，含 3 个新加的 wrap 行为测试：长输入折行、`\n` + 长行、光标在不同 wrap 行的 cursorUp 计算）。`go vet ./...` 通过。
- 备注：当 prompt 总占用行数超过终端可视高度时，暂未做"跟随光标滚动视图"的处理，多数场景下输入不会这么长，后续如有需要再扩展。

## 2026-07-01 - 启动详情默认隐藏，改为 /info 命令按需显示；抽取 slash.go 命令分发

- 摘要：启动时不再输出 workdir / model / wire api / mcp tools / log file 等详情，只保留大字 banner + 一行命令提示 `type /info for details, exit or quit to stop`。用户输入 `/info` 时按需打印详情。新增 `cmd/agent/slash.go` 集中管理 `/` 命令的注册与分发，当前注册 `/info` 和预留 `/model`。
- 主要模块：`cmd/agent/main.go`、`cmd/agent/slash.go`（新建）、`internal/ui/startup.go`、`internal/ui/startup_test.go`。
- 改动要点：
  - `startup.go`：`renderWideStartup` / `renderCompactStartup` 不再调 `renderStartupDetails`，改为打印 `startupHint`（居中偏移与大字对齐）。新增导出函数 `RenderStartupDetails` 供外部按需调用。`startupDetailLines` 移除 `startupQuitNotice`。
  - `main.go`：`startupInfo` 提升为包级变量供 `slash.go` 读取；输入循环里 `/info` 分支替换为 `dispatchSlash(input)`。
  - `slash.go`：`slashCommands` map 注册表 + `dispatchSlash` / `parseSlash` / `printSlashHelp`；`/model` 预留占位，无参打印当前模型，有参提示未实现。
  - `startup_test.go`：重写 3 个旧测试（验证启动时不再包含详情），新增 2 个测试 `RenderStartupDetails` 全字段输出和 MCP 禁用时隐藏 `mcp tools:`。
- 验证：`go test ./...` 全部通过（50 个 UI 测试 + 其他包）；`go vet ./...` 通过；`go build ./cmd/agent/` 通过。
- 备注：`exit` / `quit` 保留在 `main.go` 输入循环里（终止控制流，语义上不是命令）。后续新增 `/` 命令只需在 `slash.go` 的 `slashCommands` map 里加一行。

## 2026-07-01 - 输入框 / 命令建议列表

- 摘要：输入框新增 /命令 建议列表功能。用户输入 `/` 时，输入框下方自动渲染匹配的命令列表 block（命令名 + 描述），输入继续过滤（如 `/mo` 只匹配 `/model`），输入含空格时隐藏（用户在输参数）。选择命令后回车执行。
- 主要模块：`internal/ui/prompt.go`、`cmd/agent/slash.go`、`cmd/agent/main.go`、`internal/ui/prompt_test.go`。
- 改动要点：
  - `prompt.go`：新增 `CommandSuggestion` 类型、`Prompt.commands`/`suggestRows` 字段、`SetCommands()` 方法。`ReadLine` 渲染循环改用 `renderFrame()`（清旧帧 → 渲染输入行 → 渲染建议列表）。新增 `renderCommandSuggestions()` 按前缀过滤并渲染 block。`clearPrompt()` 新增清除建议列表行逻辑。
  - `slash.go`：新增导出函数 `SlashCommandList()` 返回按名称排序的 `[]ui.CommandSuggestion`，供 main 传递给 Prompt。
  - `main.go`：创建 Prompt 后调用 `prompt.SetCommands(SlashCommandList())`。
  - `prompt_test.go`：新增 5 个测试：全量匹配、前缀过滤、含空格隐藏、非 `/` 输入隐藏、清除建议行。
- 验证：`go test ./...` 全部通过（55 个 UI 测试）；`go vet ./...` 通过。
- 备注：建议列表渲染在输入行下方，用 ANSI 颜色区分命令名（light blue）和描述（muted gray）。命令名固定宽度 14 列左对齐，描述紧随其后。

## 2026-07-01 - /exit 和 /quit 命令替代裸 exit/quit 退出方式

- 摘要：新增 `/exit` 和 `/quit` 命令作为退出方式，废弃原来的裸 `exit`/`quit` 输入。通过 sentinel error `errExit` 让 handler 通知 `dispatchSlash` 返回 `shouldExit=true`，main 循环检测到后 return。
- 主要模块：`cmd/agent/slash.go`、`cmd/agent/main.go`。
- 改动要点：
  - `slash.go`：新增 `errExit` sentinel error、`slashExit` handler、注册 `exit`/`quit` 到 `slashCommands`。`dispatchSlash` 返回值从 `(handled bool)` 改为 `(handled bool, shouldExit bool)`，检测到 `errExit` 时 `shouldExit=true`。
  - `main.go`：去掉 `if input == "exit" || input == "quit"` 检查，改用 `dispatchSlash` 的 `shouldExit` 返回值。
- 验证：`go test ./...` 全部通过；`go vet ./...` 通过。
- 备注：现在退出只能通过 `/exit` 或 `/quit`，裸 `exit`/`quit` 会交给 agent 当普通输入处理（agent 会尝试理解并回复）。

## 2026-07-01 - 命令建议列表支持上下键选择

- 摘要：命令建议列表新增上下键导航和 `>` 选中标记。用户输入 `/` 后，建议列表第一行默认选中（显示 `>`），按 ↑↓ 移动选择，按回车直接执行选中命令。Tab 补全行为不变（仍补全第一个匹配）。
- 主要模块：`internal/ui/prompt.go`。
- 改动要点：
  - `Prompt` 新增 `suggestMatched []CommandSuggestion`（当前匹配列表）和 `suggestSelected int`（选中索引）字段。
  - `ReadLine` 循环在 `suggestRows > 0` 时拦截 `up`/`down`/`enter`：上下键移动 `suggestSelected` 并重绘，回车直接返回选中命令。
  - `renderCommandSuggestions` 保存匹配列表到 `p.suggestMatched`，clamp `suggestSelected` 到合法范围，选中行用 `> ` 标记（light blue），其他行用 `  `。
- 验证：`go test ./...` 全部通过（59 个 UI 测试）；`go vet ./...` 通过。
- 备注：选中索引在匹配列表变化时自动 clamp，不会越界。回车执行选中命令后直接返回，跳过 `applyKey` 处理。

## 2026-07-01 - 对话历史宽度占满终端，视觉统一

- 摘要：去掉 `MarkdownWordWrap`、`LiveFrameMaxWidth`、`SeparatorWidth` 的固定默认值（从 80/100 改为 0），让对话历史、分隔线、markdown 内容占满终端宽度。新增 `BlockRenderer.separatorWidth()` 方法动态获取终端宽度。修复 `promptPlaceholder` 为空字符串时的测试问题。
- 主要模块：`internal/ui/options.go`、`internal/ui/renderer.go`、`internal/ui/renderer_final.go`、`internal/ui/renderer_frame.go`、`internal/ui/renderer_todo.go`、`internal/ui/renderer_tools.go`、`internal/ui/prompt_test.go`。
- 改动要点：
  - `options.go`：`SeparatorWidth`、`LiveFrameMaxWidth`、`MarkdownWordWrap` 默认值改为 0（表示使用终端宽度/不限制）。`normalizeOptions` 改为 `< 0` 时才替换为默认值，允许 0 表示"自动"。
  - `renderer.go`：新增 `separatorWidth()` 方法，options 指定时用 options，否则获取终端宽度。
  - `renderer_final.go`：`newMarkdownRenderer` 在 wordWrap <= 0 时不传 `WithWordWrap` 选项，glamour 不限制宽度。
  - 所有 `separatorLine(r.options.SeparatorWidth)` 调用改为 `separatorLine(r.separatorWidth())`。
  - `prompt_test.go`：`promptPlaceholder` 为空字符串时跳过相关检查（`strings.Contains(text, "")` 总是 true）。
- 验证：`go test ./...` 全部通过；`go vet ./...` 通过。
- 备注：对话历史现在占满终端宽度，视觉更统一。banner 仍居中，对话历史和输入框左对齐从终端左边界开始。

## 2026-07-01 - UI 视觉统一：左对齐 + 4 字符统一边距

- 摘要：Banner、对话历史、输入框三者统一为左对齐 + 4 字符左边距，视觉重心一致。
- 主要模块：`internal/ui/startup.go`、`internal/ui/renderer_frame.go`、`cmd/agent/main.go`。
- 改动要点：
  - `startup.go`：`renderWideStartup` 去掉居中计算，改为固定 4 字符左边距。
  - `renderer_frame.go`：`frameOutputText` 给每行非空行加 4 字符左边距。
  - `main.go`：`ReadLine` 的 prompt 从 `› ` 改为 `    › `（4 空格 + ›）。
- 验证：`go test ./...` 全部通过；`go vet ./...` 通过。
- 备注：三者现在都从终端左边界 + 4 字符位置开始，视觉统一。

## 2026-07-02 - TUI 布局改成大 Banner 与底部输入框

- 摘要：按新的终端布局要求调整 `internal/tui`，顶部改为大号 `ECHO DUST CODE` banner，中间改为无框内容滚动区，底部改为独立输入框，并移除旧的 `Session` 固定边框感。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_test.go` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：宽终端下显示 ASCII 大字 banner，窄终端下自动回退为紧凑标题；slash 建议仍保留，但输入框现在作为底部独立区域渲染。

## 2026-07-02 - 主 Agent 缺失 todo 时自动初始化

- 摘要：主 Agent 在一轮响应包含 workspace tool、但模型没有显式调用 `update_todos` 时，会自动补一个默认 todo，并先发出 `todo_update` 事件，避免工具执行前反复撞上 “requires a todo list” 门禁。
- 主要模块：`internal/agent/agent.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/agent/agent.go internal/agent/agent_test.go` 通过；`go test ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：system prompt 同步改为“默认 todo 会自动初始化，`update_todos` 用于细化计划”；如果模型本轮已经显式调用 `update_todos`，则不会重复自动补 todo。

## 2026-07-02 - TUI 增加 subagent 折叠面板与详情视图

- 摘要：为 `internal/tui` 增加独立的 subagent 面板，默认把每个 subagent 的工具输出折叠成列表项，不再把大量子任务日志直接塞进主对话滚动区。输入框上方新增 subagent 选择框，支持 `↑/↓` 选择、`Enter` 进入详情、`Esc` 返回列表；主内容区和 subagent 详情区都支持 `End` 直达底部。
- 主要模块：`internal/tui/model.go`、`internal/tui/helpers.go`、`internal/tui/model_test.go`、`internal/agent/tool_scheduler.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `model.go`：新增 subagent 会话状态、独立 viewport、折叠列表渲染和详情渲染；把 `delegate_task` 及转发的 subagent 事件从主 transcript 中剥离，改为单独归档到 subagent 面板。
  - `model.go`：键盘交互新增列表选择与详情切换逻辑；`End` 现在会作用于当前活跃显示区，详情打开时直达 subagent 输出底部。
  - `helpers.go`：新增单行截断辅助函数，避免任务摘要在 subagent 列表里换行打乱布局。
  - `model_test.go`：新增默认折叠、上下选择/进入详情、详情区滚轮滚动与 `End` 跳底测试。
- 验证：`gofmt -w internal/tui/model.go internal/tui/helpers.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因仍是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：subagent 列表默认优先吃掉空输入状态下的 `↑/↓`，因此有 subagent 面板时，空输入状态不会再直接进入历史命令导航；需要看某个子任务详情时先选中再按 `Enter`。

## 2026-07-02 - TUI 按功能拆分文件

- 摘要：按职责把 `internal/tui/model.go` 和 `internal/tui/helpers.go` 拆成多个小文件，避免状态定义、输入更新、事件处理、布局渲染、subagent 面板、tool 文案、文本清洗和 markdown 配置全部堆在两个超大文件里。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_update.go`、`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/model_subagent.go`、`internal/tui/transcript.go`、`internal/tui/toolfmt.go`、`internal/tui/text.go`、`internal/tui/markdown.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `model.go`：仅保留 `Model` 结构、基础类型、banner 常量、构造函数和 options 归一化，作为 TUI 状态骨架。
  - `model_update.go` / `model_events.go`：把 Bubble Tea `Update` 流程、输入提交、approval 选择、运行生命周期和 runtime event 消费拆开。
  - `model_layout.go` / `model_render.go` / `model_subagent.go`：把布局计算、主界面渲染和 subagent 列表/详情逻辑分层，便于后续只改一个板块。
  - `transcript.go` / `toolfmt.go` / `text.go` / `markdown.go`：把原 `helpers.go` 里的基础 transcript 类型、tool 事件标题/详情、终端文本清洗、markdown renderer 配置拆开。
- 验证：`gofmt -w internal/tui/*.go` 通过；`go test ./internal/tui` 通过；`go test ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因仍是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：这次重构只调整文件边界，不改 TUI 对外接口和现有交互行为；后续如果继续拆分，优先考虑把 `model_subagent.go` 和 `toolfmt.go` 再细化，而不是重新引入跨文件状态耦合。

## 2026-07-02 - Subagent 面板在父 Run 完成后自动隐藏

- 摘要：修正 TUI subagent 面板生命周期。之前面板一旦收到 subagent 事件就会一直显示到下一轮 `run_start`，即使 subagent 已结束、主 Agent 已经输出最终结果，输入框上方的子任务框也不会消失。现在父 Run 产出 `final` 或结束时会自动隐藏该面板。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_update.go`、`internal/tui/model_render.go`、`internal/tui/model_subagent.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `showSubagents` 显示开关，把 subagent 面板从“只要有历史 session 就显示”改成“当前 run 期间收到 subagent 事件才显示”。
  - 在 `TypeFinal`、`TypeRunEnd` 和 `runFinishedMsg` 路径统一调用 `hideSubagentPanel()`，关闭 subagent 列表/详情视图。
  - `renderSubagentPanel()`、布局高度计算和空输入态上下键选择逻辑都改为依赖 `showSubagents`，隐藏后不会再占位或抢键盘焦点。
  - 新增回归测试：主 Agent 收到 `final` 后，subagent 面板必须从界面消失。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_events.go internal/tui/model_update.go internal/tui/model_render.go internal/tui/model_subagent.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因仍是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：这次修正是“自动隐藏”，不是“自动清空 subagent session 数据”；隐藏后的历史状态仍留在内存里，直到下一轮 `run_start` 重置。

## 2026-07-02 - 启动页改为纯 Banner，移除顶部元信息与默认提示文案

- 摘要：按新的视觉要求收紧 TUI 启动页。顶部只保留 `ECHO DUST CODE` banner，不再渲染 `cwd/model/status/todos/tool/tokens/log` 元信息行；空会话也不再预置 `Ready` / `/info` 提示和 `No conversation yet.` 占位文本。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_render.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `NewModel()` 去掉默认 `Ready` block，启动时主内容区不再自动插入提示文案。
  - `renderHeader()` 改为只返回 banner，本轮不再展示顶部状态元信息。
  - `rebuildViewportContent()` 在空内容时保留空白，不再塞入 `No conversation yet.`。
  - 新增启动页回归测试，确保 idle 视图不再出现 `cwd`、`status idle`、`Ready`、`/info` 等文字。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_render.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因仍是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：这次改动也顺带让首页更接近“空白工作台”而不是“信息看板”；如果后续还要保留运行状态信息，更合适的放置点应该是输入框附近的轻量状态位，而不是 banner 下方一整行。

## 2026-07-02 - TUI 仅显示工具调用，不显示工具结果

- 摘要：调整 `internal/tui` 的 tool event 展示规则。现在主对话区和 subagent 详情区都会显示所有 `tool_call`，但不再展示任何 `tool_result` 内容；界面只保留“调用了什么工具”这一层信息。
- 主要模块：`internal/tui/toolfmt.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `toolEventTitle()`：`TypeToolCall` 统一收敛到工具名级别展示，例如 `Tool read_file`、`Tool run_command`；不再按工具种类拼接命令详情或把 explore/edit 工具静默掉。
  - `toolEventTitle()`：`TypeToolResult` 统一返回空字符串，从渲染层彻底隐藏所有工具结果 block。
  - `toolEventDetail()`：`TypeToolCall` 与 `TypeToolResult` 都不再返回正文，界面只看标题，不再泄露工具结果、命令输出、文件预览或 explore 内容。
  - `model_test.go`：新增回归测试，确保主对话区显示 `tool_call` 但隐藏 `tool_result`；subagent 详情区也只显示工具名，不显示结果输出。
- 验证：`gofmt -w internal/tui/toolfmt.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 未完全通过，失败原因仍是 `internal/tools` 的 `TestGoCodeNavigationTools` 依赖 `gopls`，当前环境 `PATH` 中不存在该可执行文件。
- 备注：这次改动只影响 TUI 的事件可视化，不影响 runtime event 发射本身；如果后续需要更细粒度控制，可以把“主区只看 tool_call / subagent 详情看 tool_call+assistant message”单独抽成策略函数。

## 2026-07-03 - Tool Call 展示参数，继续隐藏 Tool Result

- 摘要：在 `internal/tui` 中恢复 `tool_call` 参数展示。现在主对话区和 subagent 详情区会显示“工具名 + 参数”，但仍然不展示任何 `tool_result` 内容。
- 主要模块：`internal/tui/toolfmt.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `toolEventDetail()` 对 `TypeToolCall` 恢复参数渲染：普通工具显示 compact JSON 参数，`delegate_task` 保留任务摘要格式。
  - `toolEventDetail()` 对 `TypeToolResult` 继续保持空返回，工具结果、命令输出、文件预览和搜索正文仍不会出现在 TUI 中。
  - `model_test.go`：补充主对话区与 subagent 详情区测试，确保都能看到 `tool_call` 参数，同时继续看不到 `tool_result` 输出。
- 验证：`gofmt -w internal/tui/toolfmt.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 通过。
- 备注：当前参数展示使用 compact JSON，优点是实现简单且适用于所有工具；如果后续要进一步压缩视觉噪音，可以只对 `read_file` / `search_files` / `run_command` 做定制化参数摘要。

## 2026-07-03 - Tool Call 前增加绿色状态点

- 摘要：为 TUI 中的工具调用块增加单独的视觉标记。现在 `tool_call` 会以前置绿色实心点 `●` 显示，便于在长对话中快速扫出工具调用位置。
- 主要模块：`internal/tui/transcript.go`、`internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_subagent.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `blockToolCall` transcript 类型，把工具调用从普通 info block 里单独分流。
  - 主对话区和 subagent 详情区在接收 `TypeToolCall` 时都改用 `blockToolCall`，保持视觉一致。
  - `renderBlock()` 为 `blockToolCall` 渲染 `● + 标题`，其中点为绿色，标题用独立的 tool call 标题样式。
  - 新增测试，确保工具调用块渲染时带有状态点。
- 验证：`gofmt -w internal/tui/transcript.go internal/tui/model.go internal/tui/model_events.go internal/tui/model_subagent.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 通过。
- 备注：这次只给工具调用加点，不影响 assistant/user/error 等其他块的标题样式；如果后续需要更贴近参考图，可以继续把工具标题改成更暖色的高对比度配色。

## 2026-07-03 - 审批改为贴着请求块的行内选项

- 摘要：把 `internal/tui` 的审批交互从居中 modal 改成 transcript 内联展示。现在出现审批时，不会再覆盖整屏，而是在 `Approval requested` 这条请求块下面直接展开可选项，更接近旧 CLI UI 的工作方式。
- 主要模块：`internal/tui/transcript.go`、`internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/model_update.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `blockApprovalRequest`，把审批请求从普通 info block 里单独标识出来，方便在对应位置挂载审批选项。
  - 移除 `renderApprovalScreen()` 的全屏弹窗逻辑，`View()` 恢复正常 banner + transcript + 输入框布局。
  - `rebuildViewportContent()` 现在会把审批选项直接插在最新的审批请求块下方；如果 `approvalPromptMsg` 比 `approval_request` 事件先到，还会临时补一个内联审批块，避免界面空窗。
  - 审批进行中保留原有键盘决策逻辑，同时恢复滚轮、`PgUp/PgDn/Home/End` 对主内容区的滚动能力。
  - 新增回归测试，覆盖“行内审批渲染”和“prompt 先于 runtime event 到达”的场景。
- 验证：`gofmt -w internal/tui/transcript.go internal/tui/model.go internal/tui/model_events.go internal/tui/model_render.go internal/tui/model_layout.go internal/tui/model_update.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 通过。
- 备注：这次只改审批呈现层，不改 Agent 的审批事件时序；工具真正执行前仍然先发 `approval_request`，用户确认后才会进入 `tool_call`。

## 2026-07-03 - local-agent 品牌名切换为 echo dust code

- 摘要：把项目里用户可见的 `local-agent` 品牌名切换为 `echo dust code`，同步调整默认记忆目录、MCP 目录、日志目录、全局记忆文档名和对外 client 标识；Go 模块名与 import path 保持 `local-agent` 不变，避免把这次品牌调整升级成模块迁移。
- 主要模块：`config.yaml`、`README.md`、`docs/TUI_MIGRATION_PLAN.md`、`internal/config/config.go`、`internal/logs/logger.go`、`internal/memory/doc.go`、`internal/memory/memory_test.go`、`internal/mcp/stdio.go`、`internal/llm/responses.go`、`internal/tools/code_tools.go`、`internal/ui/startup_test.go`、`internal/ui/renderer_test.go`、`internal/config/config_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 默认用户目录和 MCP 目录从 `~/.local-agent` 改为 `~/.echo-dust-code`，工作区日志目录从 `.local-agent/logs` 改为 `.echo-dust-code/logs`。
  - 全局记忆文档默认名改为 `ECHO-DUST-CODE.md`，同时保留对旧 `LOCAL-AGENT.md` 的兼容加载，避免已有用户配置失效。
  - Responses API 的 `prompt_cache_key`、`client_metadata.client` 以及 MCP `clientInfo.name` 改为 `echo-dust-code`。
  - README 和迁移文档里的品牌文案、目录示例和缓存路径示例同步更新。
  - 测试用例和临时目录命名同步改到新品牌，并新增旧 `LOCAL-AGENT.md` 兼容回归测试。
- 验证：`gofmt -w internal/config/config.go internal/logs/logger.go internal/memory/doc.go internal/memory/memory_test.go internal/mcp/stdio.go internal/llm/responses.go internal/tools/code_tools.go internal/ui/startup_test.go internal/ui/renderer_test.go internal/config/config_test.go` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次没有修改 `go.mod` 的 `module local-agent`，也没有改任何 import path；如果后续要把模块名也迁移到新品牌，需要单独做一次完整的 Go 模块重命名和下游兼容处理。

## 2026-07-03 - TUI 右下角增加 token 消耗显示

- 摘要：在 `internal/tui` 底部输入框上方增加右对齐 token footer，用来显示当前 run 的 token 消耗。主 agent 有 usage 时显示 `Tokens <total> (p<prompt> c<completion>)`；如果存在 subagent usage，会汇总成 `Tokens <total> total | main <main> | sub <sub>`。
- 主要模块：`internal/tui/model_render.go`、`internal/tui/model_layout.go`、`internal/tui/model_events.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `renderFooter()` 和 `footerSummary()`，把 token 消耗渲染到输入框上方、右对齐的位置。
  - 布局计算时为 footer 预留一行高度，避免和正文 viewport 叠在一起。
  - 新 run 开始时重置 `m.tokens`，修正之前 TUI token 会跨轮串数的问题。
  - 新增测试覆盖主 agent token footer、subagent token 汇总，以及 `run_start` 重置 token 状态。
- 验证：`gofmt -w internal/tui/model_render.go internal/tui/model_layout.go internal/tui/model_events.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/agent` 通过；`go vet ./...` 通过；`git diff --check` 通过；`go test ./...` 通过。
- 备注：这次显示的是运行时汇总，不是 provider 账单口径；数值来源仍然完全依赖 runtime event 里的 usage 字段。

## 2026-07-03 - Token usage 增加 cache hit 统计

- 摘要：把 provider 返回的 prompt cache hit 数据接进现有 token usage 链路。现在 Responses API 和 Chat Completions API 解析到的 `cached_tokens` 会进入 `llm.TokenUsage`，再通过 runtime event 传到新 TUI footer 和旧 CLI renderer，用户可以直接看到本轮命中的 cache token 数。
- 主要模块：`internal/llm/client.go`、`internal/llm/chat_completions.go`、`internal/llm/responses.go`、`internal/agent/agent.go`、`internal/runtimeevent/runtimeevent.go`、`internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_subagent.go`、`internal/tui/model_render.go`、`internal/ui/renderer.go`、`internal/ui/renderer_frame.go`、`internal/agent/agent_test.go`、`internal/llm/openai_test.go`、`internal/tui/model_test.go`、`internal/ui/renderer_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `llm.TokenUsage` 新增 `CachedTokens` 字段，统一承接不同 provider 的 cache 命中统计。
  - Responses API 兼容解析 `input_tokens_details.cached_tokens` / `prompt_tokens_details.cached_tokens` / 顶层 `cached_tokens`。
  - Chat Completions API 兼容解析 `prompt_tokens_details.cached_tokens`，并保留顶层 `cached_tokens` 兜底。
  - Agent 在发射 `TypeTokenUsage` runtime event 时追加 `CachedTokens`，同时累计到主 agent 总 usage。
  - 新 TUI footer 在有 cache 命中时显示 `cache <n>`；旧 CLI live frame 和 final summary 也同步展示 cache 统计。
- 补充解析层、Agent 事件层、TUI footer 和旧 renderer 的回归测试，确保 cache hit 不会在中途丢失。
- 验证：`gofmt -w internal/llm/client.go internal/llm/chat_completions.go internal/llm/responses.go internal/agent/agent.go internal/runtimeevent/runtimeevent.go internal/tui/model.go internal/tui/model_events.go internal/tui/model_subagent.go internal/tui/model_render.go internal/ui/renderer.go internal/ui/renderer_frame.go internal/agent/agent_test.go internal/llm/openai_test.go internal/tui/model_test.go internal/ui/renderer_test.go` 通过；`go test ./internal/llm ./internal/agent ./internal/tui ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这里展示的是 provider 返回的 cache hit token 数，不等于所有厂商统一账单口径；如果某个模型或网关不返回该字段，UI 会自动退回旧展示，不会报错。

## 2026-07-03 - TUI 恢复正文 todo 展示并压缩 token 文案

- 摘要：调整 `internal/tui` 的运行中状态展示。现在正文区会在运行期间显示一个动态 `Todo` block，直接展示当前 `update_todos` 清单；底部 token footer 同时改为 `k/m` 紧凑格式，避免大数字把右下角占满。
- 主要模块：`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `TypeRunStart` 时清空上一轮 `m.todos`，避免新任务开始后看到旧清单残留。
  - `rebuildViewportContent()` 新增动态 `Todo` block，仅在当前 run 进行中且已经收到真实 `todo_update` 后显示；像 `hello` 这样的纯文本 run 不再出现 todo 占位。
  - 审批出现时临时隐藏动态 todo，把可视优先级让给审批请求和选项。
  - token footer 统一改成紧凑格式，支持 `k/m`，例如 `34.1k`、`1.4m`。
  - 新增 TUI 回归测试，覆盖运行中 todo 渲染、纯文本 run 不显示 todo、run 结束后隐藏，以及 footer 紧凑数字格式。
- 验证：`gofmt -w internal/tui/model_events.go internal/tui/model_layout.go internal/tui/model_render.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：当前 todo 是“动态运行态块”，不会像普通 transcript block 那样把每一次更新都永久堆进正文；这样可以减少噪音，但意味着 run 结束后 todo 会消失。

## 2026-07-03 — 同步 config 默认值与 config.yaml 实际值

- 将 `internal/config/config.go` 中 `Default()` 的 13 个默认值同步为 `config.yaml` 的实际配置，使不携带 config 文件时的行为与当前使用习惯一致。
- 改动模块：`internal/config/config.go`。
  - LLM：`base_url` → `anyrouter.top/v1`、`model` → `gpt-5.5`、`wire_api` → `responses`、`request_timeout_seconds` → 300。
  - Agent：`max_steps` 20→30、`max_step_extensions` 3→5。
  - Subagents：`max_concurrent` 2→5、`max_steps` 8→30、`result_max_bytes` 12288→16888。
  - Context：`window_tokens` 128000→256000。
- UI：`separator_width` 0→80、`live_frame_max_width` 0→100、`markdown_word_wrap` 0→100。
- 验证：`go test ./...` 全部通过；`go vet ./...` 无报错。
- 备注：同步后可仅通过 `AGENT_API_KEY` 环境变量启动，无需携带 config.yaml；LLM 提供商信息（base_url/model/wire_api）已固化进默认值，若切换提供商仍需 config 或新增环境变量覆盖。

## 2026-07-03 - TUI todo 固定在当前轮工具日志之前

- 摘要：修正正文区 live todo 的插入位置。运行中 todo 不再永远追加在正文末尾，而是固定插入到“当前这轮用户/助手文本之后、工具日志之前”，避免工具调用越多，todo 越被挤到下面。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `TypeRunStart` 时记录当前 run 的 block 起点，用于区分历史正文和本轮正文。
  - `rebuildViewportContent()` 按当前 run 的 block 边界计算 todo 插入点，不再简单把 live todo 追加到最后。
  - 新增回归测试，明确要求正文顺序为“当前用户消息 < Todo < Tool call”。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_events.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只调整展示顺序，没有改变主 Agent 自动补默认 todo 的门禁策略；也就是说，模型没显式调用 `update_todos` 时，workspace 工具前的自动 todo 仍然存在，只是位置更稳定。

## 2026-07-03 - TUI todo 改成清单方框样式

- 摘要：把正文区 live todo 从带标题的信息块，改成直接嵌入正文的清单方框样式，更接近任务列表而不是状态面板，减少“Todo 标题 + 缩进块”带来的视觉重量。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 todo 文本样式和已完成样式，运行中 todo 统一走专门的 checklist renderer。
  - 取消 live todo 的 `Todo` 标题，正文里直接输出 `□` / `■` 风格的任务项。
  - 保持之前修复过的插入顺序：当前轮用户消息之后、工具日志之前。
  - 更新测试，覆盖 checklist 样式和顺序约束。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只改 TUI 展示样式，不改 todo 的生成与门禁逻辑；自动补 todo 的策略保持不变。

## 2026-07-03 - delegate_task 改为异步 subagent

- 摘要：把 `delegate_task` 从“启动并同步等待子代理返回”改成“后台启动、父 Agent 继续工作、最终收尾时再自动汇总结果”的异步模型。主 Agent 现在可以在子代理运行期间继续执行自己的独立工具调用，不会一发起委托就整轮卡住。
- 主要模块：`internal/agent/agent.go`、`internal/agent/subagent.go`、`internal/agent/agent_test.go`、`internal/tui/model_subagent.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `delegate_task` 现在立即返回 `subagent started`，后台 goroutine 再真正运行子代理。
  - 父 Agent 维护单轮 subagent 任务表；每轮开始前会先收集已完成子代理，把结论注入为合成 `system` message，供下一轮推理直接使用。
  - 当父 Agent 准备无工具结束时，如果仍有未收集的子代理，会先等待这些后台任务完成，再继续下一轮综合，不直接提前 final。
  - `Run()` 改为使用 run-scope context，确保父 run 结束时能取消仍在后台的异步子代理，避免悬挂 goroutine 继续输出事件。
  - TUI 子代理列表把 `delegate_task` 的即时成功结果视为 `running`，只有后台终态 `subagent completed` 才切到 `done`。
  - 测试重写为父/子独立脚本客户端，覆盖异步继续工作、结果回注、子代理隔离、安全门禁与事件转发。
- 验证：`gofmt -w internal/agent/agent.go internal/agent/subagent.go internal/agent/agent_test.go internal/tui/model_subagent.go` 通过；`go test ./internal/agent ./internal/tui ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：当前 join 仍是“隐式 join”而不是显式 `await_subagent` 工具；也就是父 Agent 可异步推进，但最终综合阶段仍由 runtime 自动等完后台子代理再继续。

## 2026-07-03 - TUI 增加 cache hit rate 展示

- 摘要：在新 TUI 的 token 统计里补上 `cache hit rate` 计算，并把结果直接显示到 footer 和子代理列表中。现在不再只看到 `cache <n>`，还能看到按 `cached / prompt` 计算出来的命中比例。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_render.go`、`internal/tui/model_subagent.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 子代理会额外累计 `PromptTokens`，不再只保留 `TokenTotal` 和 `CachedTokens`；这样主 Agent 和 subagent 可以用同一口径计算命中率。
  - footer 的全局统计改为在有 cache 命中时显示 `cache <n> | hit <rate>`；主 Agent 独立运行时也会在原来的 `Tokens ... (p... c...)` 摘要里追加 `hit`。
- 子代理列表仍然保持低噪音：每一行默认只显示 token 总量；只有当前选中的 subagent 才额外展开 `cache` 和 `hit`。
- `cache hit rate` 的分母明确使用 `PromptTokens`，不再拿 `TotalTokens` 充数，避免长回答把命中率稀释掉。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_render.go internal/tui/model_subagent.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这里仍然是 provider usage 字段驱动的“best effort”统计；如果某次调用没有返回 `PromptTokens` 或 `CachedTokens`，TUI 会退回只显示已有数字，不会硬算一个不可靠的命中率。

## 2026-07-03 - TUI 对话改成提问框加无标题回答

- 摘要：调整主对话区的视觉结构。用户提问不再显示 `You` 标签，而是渲染成一个高对比度的横向深色提问框；assistant 回复也去掉 `Agent` 标签，直接排在提问框下方，减少噪音，让一问一答的层级更明确。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增用户提问框样式，边框结构沿用输入框的语言，但增加深色背景和更亮的边框，和正文图片区分开。
  - `renderBlock()` 现在对 `blockUser` 和 `blockAssistant` 走专门分支：用户消息输出为 question box，assistant 消息只输出正文。
  - markdown assistant 回复仍然保留 markdown 渲染能力，只是不再附带 `Agent` 标题行。
- 增加 TUI 测试，覆盖“去掉 You/Agent 标签”和“用户消息必须渲染成带边框的提问框”这两个约束。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只调整主 transcript 的视觉结构；工具调用、审批块、todo 清单和子代理列表保持原有语义与顺序，不混入新的问答框样式。

## 2026-07-03 - TUI 用户提问框收敛为黄色星号提示

- 摘要：回退掉上一版过重的整条深色提问框，把用户消息改成左侧黄色 `*` 标记加正文。assistant 回复仍然保持无标题正文，这样问答层级还在，但视觉重量明显降低。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 删除用户提问专用 box 样式，改成 `* ` 前缀的轻量提示行。
  - 保留多行换行对齐，第二行开始与正文左边界对齐，不和星号重叠。
- 更新测试，明确要求用户块存在 `*` 标记、隐藏 `You` 标签，并且不再渲染边框字符。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只收敛用户消息样式；assistant、tool、approval、todo 和 subagent 的展示逻辑都没改。

## 2026-07-03 - 复用 session 审批时不再重复刷 Approval 日志

- 摘要：修正审批链路的日志噪音。之前即使 session 级别的 `Always` 审批已经命中缓存，Agent 仍然会继续发 `Approval requested` / `Approval always` 事件，导致正文里反复出现没有信息增量的审批块。现在命中缓存时直接放行，不再额外产生日志事件。
- 主要模块：`internal/approval/types.go`、`internal/approval/memory.go`、`internal/approval/approval_test.go`、`internal/agent/tool_approval.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `approval.DecisionCache` 接口，让 approver 可以在真正发起审批前暴露“是否已有缓存决策”。
  - `MemoryApprover` 增加 `CachedDecision()`，目前只对 session 级别的 `Always` 复用生效；loop 级审批仍按原来的单轮逻辑处理。
  - `approveTool()` 在发 `TypeApprovalRequest` 事件前先探测缓存；如果命中，直接放行，不再发 request/decision 这对事件。
  - 补充测试，明确要求同一 session 内第二次 workspace write 不再出现新的审批事件。
- 验证：`gofmt -w internal/approval/types.go internal/approval/memory.go internal/approval/approval_test.go internal/agent/tool_approval.go internal/agent/agent_test.go` 通过；`go test ./internal/approval ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只压掉“缓存命中后的重复审批日志”；第一次真实审批，以及 loop 级 external write 的按轮审批行为没有变化。

## 2026-07-03 - 新增 session 持久化与 `/resume`

- 摘要：为当前 workspace 增加基于 JSON 的 session 持久化，并接入 `/resume`。现在每轮对话结束后会把会话保存到 `~/.echo-dust-code/session`，之后可以列出最近 session、恢复最新 session，或按 session id 前缀恢复指定会话。
- 主要模块：`internal/session/session.go`、`cmd/agent/session_runtime.go`、`cmd/agent/slash.go`、`cmd/agent/main.go`、`internal/agent/agent.go`、`internal/tui/session.go`、`internal/config/config.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `internal/session`，按 `projects/<workspace-slug>/<session-id>/meta.json + state.json` 保存会话；持久化内容包含 conversation history 和可选 TUI snapshot。
  - `Agent` 新增 `ConversationMessages()` 和 `RestoreConversation()`，恢复时保留当前进程生成的 system prompt，只替换 system 之后的历史消息，并重置 token 统计。
  - `TUI` 新增 session snapshot 导出/导入；恢复后会替换当前 transcript，并追加一条 resumed 提示块。
  - slash 入口改成有状态 router，支持 `/resume`、当前 session id 展示，以及运行中拒绝恢复。
  - 新增 `session.enabled` / `session.dir` 配置项；默认启用，目录默认 `~/.echo-dust-code/session`。
- 验证：`gofmt -w cmd/agent/main.go cmd/agent/session_runtime.go cmd/agent/slash.go cmd/agent/slash_test.go internal/agent/agent.go internal/agent/session_test.go internal/config/config.go internal/config/config_test.go internal/config/yaml.go internal/session/session.go internal/session/session_test.go internal/tui/model.go internal/tui/model_events.go internal/tui/model_update.go internal/tui/session.go internal/tui/session_test.go internal/ui/startup.go internal/ui/startup_test.go` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 已知限制/后续风险：
  - 当前只支持按 workspace 维度列出和恢复 session，不做跨项目全局搜索。
  - classic UI 只恢复对话历史，不恢复旧 TUI transcript；TUI 才会加载 UI snapshot。
  - 当前没有 `/new`、删除 session、重命名 session 等额外管理命令。

## 2026-07-04 - `/resume` 在 TUI 中改为可选列表

- 摘要：把 TUI 里的 `/resume` 从“输出一段最近 session 文本”改成“打开可导航的 session 选择列表”。现在在 TUI 中输入 `/resume` 后，可以直接用 `↑/↓` 或 `j/k` 选择目标 session，按 `Enter` 恢复，按 `Esc` 取消；classic UI 仍保持原来的文本列表行为。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_update.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/resume_picker.go`、`internal/tui/session_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - TUI 新增 resume picker 状态和回调接口，不再把 `/resume` 无参路径直接交给 slash 文本输出。
  - picker 会把最近 session 渲染到正文区，提供选择高亮和操作提示；slash 自动补全提示在 picker 打开时隐藏，避免视觉冲突。
  - 运行中的 `/resume` 仍然不允许打开 picker，会继续走原 slash 错误路径。
  - 选择确认后，仍然复用已有 session 恢复逻辑；也就是说，恢复后的 system prompt、历史会话和 TUI snapshot 语义没有变化，只是交互方式更直接。
- 验证：`gofmt -w cmd/agent/main.go internal/tui/model.go internal/tui/model_events.go internal/tui/model_layout.go internal/tui/model_render.go internal/tui/model_update.go internal/tui/resume_picker.go internal/tui/session.go internal/tui/session_test.go` 通过；`go test ./internal/tui ./cmd/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前 picker 只接在 TUI 的 `/resume` 无参场景；`/resume latest` 和 `/resume <id>` 仍走原来的命令路径，classic UI 也继续输出文本，不额外实现交互列表。

## 2026-07-04 - `/resume` 不再恢复历史 tool 调用块

- 摘要：收敛 session 恢复时的 TUI 内容。现在 resume 只恢复对话和必要信息块，不再把历史 `Tool ...` 调用块重新展示出来，避免恢复后的页面被旧工具操作噪音占满。
- 主要模块：`internal/tui/session.go`、`internal/tui/session_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - session snapshot 导出时会过滤 `blockToolCall`，不再把 tool 调用块写入持久化 UI 快照。
  - session 恢复时也会再次过滤一次；这样即使磁盘上已有旧 snapshot 带着 tool 调用块，恢复时也不会再显示出来。
  - Agent 的真实消息历史没有改动；只收敛 TUI resume 展示层，不影响后续上下文连续性。
- 验证：`gofmt -w internal/tui/session.go internal/tui/session_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前只过滤 `blockToolCall`；普通 assistant/user/error/info 块仍然会按原样恢复。

## 2026-07-04 - `/resume` 兼容历史内部 system 消息

- 摘要：修复 session 恢复失败问题。之前异步 `delegate_task` 完成后会把结论注入为内部 `system` message，session 保存时把这类消息原样落盘，但恢复时又严格拒绝任何历史 `system` message，导致 `/resume` 直接报 `restored conversation must not contain system messages`。现在保存和恢复都会把“首条 prompt 之外的 system 历史”降级为普通用户上下文，旧 session 也能继续恢复。
- 主要模块：`internal/agent/agent.go`、`internal/agent/session_test.go`、`cmd/agent/slash_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `ConversationMessages()` 改为统一走可恢复消息克隆逻辑，不再把中途插入的 `system` 历史原样写入 session。
  - `RestoreConversation()` 改为兼容清洗旧存档；恢复时仍然只保留当前进程生成的首条 system prompt，其余历史 `system` message 会被转成普通 `user` 上下文。
  - 新增 agent 级回归测试，覆盖保存与恢复时的 system-message 规范化。
- 新增 `/resume` 集成测试，覆盖磁盘上已有旧格式 session 时的恢复行为。
- 验证：`gofmt -w internal/agent/agent.go internal/agent/session_test.go cmd/agent/slash_test.go` 通过；`go test ./internal/agent ./cmd/agent ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只修复历史消息角色规范化，不改变 session 的目录结构、JSON 存储格式，也不恢复历史 tool 调用块。

## 2026-07-04 - `/resume` 不再展示历史 subagent 信息

- 摘要：继续收敛 session 恢复后的 TUI 展示。现在 resume 只恢复主对话 transcript 和主 agent token 快照，不再保存或恢复历史 subagent 列表、子代理输出块和对应的底部统计；即使旧 session 文件里已经带有 `subagents` 字段，恢复时也会主动忽略。
- 主要模块：`internal/tui/session.go`、`internal/tui/session_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `SessionSnapshot()` 不再把 `m.subagentOrder` / `m.subagents` 写入 session UI 快照。
  - `LoadSessionSnapshot()` 在恢复主 transcript 前先清空 subagent 状态，并忽略旧快照里的 `snapshot.Subagents`，避免 resume 后重新打开 Subagents 面板。
  - 新增 TUI 回归测试，分别覆盖“新快照不持久化 subagent”和“旧快照里的 subagent 会被忽略”。
- 验证：`gofmt -w internal/tui/session.go internal/tui/session_test.go` 通过；`go test ./internal/tui ./cmd/agent ./internal/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次只影响 `/resume` 的恢复展示；运行中的 subagent 面板、实时事件和 token 统计逻辑不变。

## 2026-07-04 - 停止启动时自动加载 REASONIX.md / AGENTS.md / CLAUDE.md

- 摘要：按用户要求移除项目/祖先作用域和用户全局作用域对 `REASONIX.md`、`AGENTS.md`、`CLAUDE.md` 的自动扫描与加载。启动后不再把这些文件注入到系统提示词中。
- 主要模块：`internal/memory/doc.go`、`internal/memory/memory_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `docNames` 清空（项目/祖先作用域不再扫描任何候选名）。
  - `userDocNames` 移除三个文件名，仅保留 `ECHO-DUST-CODE.md` 与兼容别名 `LOCAL-AGENT.md`。
  - `localNames`（`.claude/` 作用域的 `*.local.md` 变体）按字面未动，因为这些是不同文件名；如需也移除请告知。
- `defaultDocName` 仍为 `AGENTS.md`：`DocPath(ScopeProject)` 仍返回该规范路径，仅供工具写作用，启动时不再自动读取。
- 相应单元测试改为通过 `AGENTS.local.md` 验证 import 机制，并通过 user 作用域候选顺序验证 `DocPath` 偏好。
- 验证：`go test ./...` 全部通过；`go vet ./...` 无告警。
- 备注：如仍希望 `.claude/` 作用域也不加载 `*.local.md` 变体，或对 `DocPath` 的默认返回值有其它偏好，再告诉我做下一轮收敛。

## 2026-07-04 - TODO 改为复杂任务的可选规划工具

- 摘要：收敛主 Agent 的 TODO 机制。现在不再为每次 workspace 工具调用自动生成一条复述用户问题的默认 todo，也不再把“未先创建 todo”作为所有工具调用的硬门禁；改为通过系统提示词明确要求模型只在复杂、多步、跨文件、调试或代码修改任务中主动调用 `update_todos`，简单单步查询则可以直接执行。
- 主要模块：`internal/agent/agent.go`、`internal/agent/tool_scheduler.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - 主 Agent 系统提示词改为“复杂任务优先拉起 `update_todos`，简单读查类任务不需要 todo”。
  - 删除主 Agent 在工具调用前自动注入 `Handle request: ...` 单条 todo 的逻辑，避免 TUI 中长期出现无信息增量的占位任务。
  - 删除“所有 workspace tools 都必须先有 todo list”这条运行时硬阻断；`update_todos` 继续保留为原生 tool，但改成显式规划能力，而不是强制门禁。
  - 更新回归测试：简单读取任务应当直接执行且不产生 synthetic todo；显式 `update_todos` 的执行顺序和历史清理语义保持不变。
- 验证：`gofmt -w internal/agent/agent.go internal/agent/tool_scheduler.go internal/agent/agent_test.go` 通过；`go test ./internal/agent ./internal/tui ./cmd/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过；`git diff --check` 通过。
- 备注：这次没有加入“复杂任务识别后强制拦截并要求先建 todo”的硬策略；是否真正出现多条 todo，当前仍主要取决于模型是否按提示词主动调用 `update_todos`。

## 2026-07-04 - TUI 实时 Diff 展示

- 摘要：为 `write_file`、`replace_in_file` 和 `apply_patch` 统一生成真实 unified diff；主 TUI 在每次成功修改文件后立即插入 diff block，新增内容显示为绿色、删除内容显示为红色，并在 session resume 后保留这些 diff 记录。
- 主要模块：`internal/tools`、`internal/tui`、`go.mod`。
- 验证：`go test ./internal/tools` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前 transcript 中展示的是按 `FileChangePreviewLines` 截断后的 diff 预览，而 `tools.FileChange.Diff` 保留完整 diff，可继续复用于其他日志或导出场景。

## 2026-07-04 - git_diff 结构化输出与 TUI 行号 Diff

- 摘要：把现有 `git_diff` 工具从纯文本输出升级为结构化 unified diff 结果，并把主 TUI 的 diff block 渲染改成带 old/new 行号的 inline diff。现在无论是主动调用 `git_diff`，还是执行 `write_file`、`replace_in_file`、`apply_patch`，都能在 transcript 中看到红绿高亮且带行号的改动内容。
- 主要模块：`internal/tools/git_tools.go`、`internal/tools/builtin.go`、`internal/tools/tools_test.go`、`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tools/git_tools.go internal/tools/builtin.go internal/tools/tools_test.go internal/tui/model_events.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tools` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：`git_diff` 在输出超出 `CommandOutputMaxBytes` 时仍受截断保护；此时 TUI 会尽量展示已保留下来的 diff 片段，并在 tool summary 中标明已截断。若后续需要更接近 GitHub 的整行背景色或左右双栏布局，可在当前行号渲染基础上继续扩展。

## 2026-07-04 - TUI Diff 改为编辑器式 Inline 视图

- 摘要：把主 TUI 的 diff block 从 raw patch 文本视图进一步收敛为更接近编辑器的 inline diff。现在默认隐藏 `diff --git`、`index`、`---/+++`、`@@` 等 patch 头部，只展示代码行本身；删除行显示红色背景，新增行显示绿色背景，并使用单列行号和 `+/-` 标记，更接近代码审阅场景而不是补丁文本阅读。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前仍未做语法高亮，代码文本会沿用统一前景色，仅通过背景色和行号/标记表达 diff；若后续要完全贴近截图中的 IDE 风格，可以继续引入按语言分词的高亮层。

## 2026-07-04 - TUI Diff 接入多语言语法高亮

- 摘要：在主 TUI 的 diff 渲染链中接入 `Chroma` 词法高亮。现在 diff block 会先根据 patch 路径匹配语言 lexer，在缺少路径时再尝试基于内容分析与轻量 heuristic 回退；代码行在保留新增/删除背景色与行号的同时，关键字、函数名、字符串、数字、注释等 token 也会使用统一主题配色显示。
- 主要模块：`internal/tui/diff_syntax.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`go.mod`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tui/diff_syntax.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过；`go mod tidy` 通过。
- 备注：当前高亮主题是统一的跨语言 token 颜色映射，不追求逐语言 1:1 复刻 IDE 官方主题；多数带扩展名的源码 diff 会通过文件路径准确命中 lexer，无扩展名场景目前仅补了 JSON 这类常见格式的 heuristic 回退。

## 2026-07-04 - TUI Diff 调整整行铺色与删除色深度

- 摘要：继续微调主 TUI 的 diff 视觉表现。删除行背景色加深到更接近审阅工具中的深红底色，同时移除 diff block 的左侧额外缩进，让新增/删除行的背景从可视区域左边缘开始连续铺满，形成更完整的横向色带。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 验证：`gofmt -w internal/tui/model.go internal/tui/model_layout.go internal/tui/model_test.go` 通过；`go test ./internal/tui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前行号仍保持右对齐，因此左侧可见的空白主要来自行号列本身，而不再是额外的 transcript 缩进；如果后续希望连行号列的前导空格也视觉更弱，可以再单独调整行号 gutter 的样式。

## 2026-07-04 - TUI 滚动卡顿与“内存泄漏”问题修复

- 摘要：修复主 TUI 在滚轮滚动时无条件重建整份 transcript 的问题。现在 `syncLayout()` 会基于布局、主 viewport 和 subagent viewport 三类脏标记按需重建，滚轮滚动只更新 `YOffset`，不再触发整页 diff 重渲染和全文 `SetContent`。同时新增专门文档记录这次被感知为“内存泄漏”的根因、修复边界与后续建议。
- 主要模块：`internal/tui/model_dirty.go`、`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_events.go`、`internal/tui/model_subagent.go`、`internal/tui/model_update.go`、`internal/tui/resume_picker.go`、`internal/tui/session.go`、`internal/tui/model_test.go`、`docs/TUI_SCROLL_MEMORY_ISSUE.md`。
- 验证：`gofmt -w internal/tui/model_dirty.go internal/tui/model.go internal/tui/model_layout.go internal/tui/model_events.go internal/tui/model_subagent.go internal/tui/model_update.go internal/tui/session.go internal/tui/resume_picker.go internal/tui/model_test.go` 通过；`go test ./internal/tui ./internal/ui` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：这次主要消除了滚动路径上的高频全文重建与重复分配，显著缓解了卡顿和滚动失真；但 transcript 常驻内存仍会随会话增长，diff 语法高亮在内容真正重建时仍会继续执行，后续如有需要可继续做 transcript 虚拟化与 diff 渲染缓存。

## 2026-07-04 - README 改为中英双语并同步 TUI 现状

- 摘要：将根目录 `README.md` 从单语英文改为中英双语版本，并同步修正文档中已过时的项目描述。重点补充了当前默认维护的是 Bubble Tea TUI、`git_diff` 与文件写入的 inline diff 展示、`/resume` 会话恢复、subagent 行为、贡献约束，以及 `internal/tui` / `internal/ui` 的角色划分。
- 主要模块：`README.md`、`docs/WORKLOG.md`。
- 验证：未运行 Go 测试；本次仅修改文档。
- 备注：移除了旧 README 中不再准确的 “2 个直接依赖”“旧 UI 主描述”“过时统计数字” 等内容，避免继续把历史状态当成当前实现对外展示。

## 2026-07-04 - 接入按需加载 Skill Registry 与 invoke_skill

- 摘要：实现一套 metadata-first 的 skill 接入链路。现在启动时只扫描并注册 `skill.json` 元数据；每次用户请求会基于输入检索 top-k skill，把技能摘要、输入 schema、权限和触发场景临时注入模型上下文，并只在模型真正调用 `invoke_skill` 时才读取对应 `SKILL.md`，再用受限工具集启动隔离的内部 agent 执行 skill。
- 主要模块：`internal/skill`、`internal/agent/skill.go`、`internal/agent/agent.go`、`internal/agent/options.go`、`internal/agent/tool_specs.go`、`internal/config`、`cmd/agent/main.go`、`internal/approval`、`config.yaml`、`README.md`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增 `internal/skill` 包，负责 skill manifest 加载、用户级/项目级目录扫描、top-k 检索、输入 schema 校验和懒加载 `SKILL.md`。
  - 新增原生工具 `invoke_skill`，仅在当前请求检索到候选 skill 时暴露给模型；参数里的 `name` 会动态限定到当前激活 skill 列表。
  - skill 执行路径复用内部 agent，但默认禁用 `delegate_task` 和 skill 递归调用，并按 `permissions.tools` 构造受限工具 registry。
  - 主 agent 在 `Run()` 开始时为当前请求激活 skill 候选，并通过瞬时 system message 注入上下文，不写回会话历史，避免跨请求泄漏。
  - `invoke_skill` 在调度层被视为未知写目标的工作区级锁，避免 skill 内部写操作与同轮其它写工具并发踩踏。
  - 配置新增 `skills.enabled`、`skills.user_dir`、`skills.project_dir`、`skills.top_k`、`skills.min_score`，README 也补充了 skill 目录结构和 `skill.json` 示例。
- 验证：`gofmt -w cmd/agent/main.go internal/agent/agent.go internal/agent/options.go internal/agent/skill.go internal/agent/tool_specs.go internal/agent/agent_test.go internal/approval/classifier.go internal/approval/write_targets.go internal/config/config.go internal/config/config_test.go internal/config/yaml.go internal/skill/skill.go internal/skill/registry.go internal/skill/schema.go internal/skill/skill_test.go internal/tools/todo.go` 通过；`go test ./internal/skill ./internal/config ./internal/agent` 通过；`go vet ./internal/skill ./internal/config ./internal/agent ./cmd/agent` 通过；`go test ./...` 通过；`go vet ./...` 通过。
- 备注：当前 skill 检索是轻量关键字/短语匹配，不是向量检索；`permissions` 在 v1 里只实现了 `permissions.tools` 白名单，底层审批仍由现有工具审批链负责，后续如果需要更细粒度的 capability/approval 语义，可以在此基础上继续扩展。

## 2026-07-04 - Skill 元数据改为根级 registry.json 为主

- 摘要：把上一版“每个 skill 目录都要带一个 `skill.json`”的约束改成“根级 `registry.json` 为主、目录内 `skill.json` 可选覆盖”。现在 `~/.echo-dust-code/skills/registry.json` 或 `<workspace>/skills/registry.json` 可以统一声明所有 skill 的名称、描述、输入 schema、权限和触发场景；目录内 `skill.json` 仍保留兼容，但只作为局部覆盖层。只有 `SKILL.md` 的 skill 目录也会被注册为最小元数据版本，以保持对旧 skill 目录的容忍度。
- 主要模块：`internal/skill/registry.go`、`internal/skill/skill.go`、`internal/skill/skill_test.go`、`internal/agent/agent_test.go`、`README.md`、`config.yaml`、`docs/WORKLOG.md`。
- 改动要点：
  - 新增根级 `registry.json` 解析，格式为 top-level `skills` 数组，每个条目通过 `path` 指向 skill 目录或 `SKILL.md`。
  - 保留根级 `skill.json` 作为兼容别名，便于已有目录约定平滑迁移到集中式配置。
  - 目录内 `skill.json` 不再是必需文件，只在存在时覆盖根级 registry 中的对应字段。
  - 对只有 `SKILL.md` 的目录，loader 会生成基于目录名的最小元数据占位，但不会在启动期读取 skill 正文，继续保持 `SKILL.md` 懒加载。
  - 更新测试覆盖：根级 registry 加载、根级 registry 与局部覆盖合并、仅 `SKILL.md` 目录兜底注册、根级 `skill.json` 别名兼容。
- 验证：`gofmt -w internal/skill/skill.go internal/skill/registry.go internal/skill/skill_test.go internal/agent/agent_test.go` 通过；`go test ./internal/skill ./internal/agent` 通过；全量验证见本次最终回复。
- 备注：这次刻意没有在启动期解析 `SKILL.md` 内容来推断描述，以避免破坏“用到时才加载”这条设计约束；如果后续确实需要更好的无元数据召回质量，建议优先增强根级 registry，而不是回到启动期读取 skill 正文。

## 2026-07-05 - Agent.Run 计时系统：step 级 + run 级

- 摘要：修复之前 `TypeStepTiming` 只接入 legacy UI（`internal/ui/`）而未接入 TUI（`internal/tui/`）的问题；新增 `TypeRunTiming` 事件展示 run 总耗时；将 step 循环体抽成 `executeStep` 方法，用 defer 保证每个 step 无论成功/失败都稳定产出一次 timing 事件；修复 `formatDuration` 可能输出 `1m60.0s` 的浮点边界问题；Step 编号改为 1-based 人类可读。
- 主要模块：`internal/runtimeevent/runtimeevent.go`、`internal/agent/agent.go`、`internal/agent/agent_test.go`、`internal/tui/model_events.go`、`internal/tui/model_subagent.go`、`internal/tui/toolfmt.go`、`internal/ui/renderer_tools.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `runtimeevent.go`：新增 `TypeRunTiming = "run_timing"` 常量。
  - `agent.go`：`Run()` 开头 defer 发射 `TypeRunTiming`（run 总耗时）；step 循环体抽成 `executeStep()` 方法，内部用 defer 保证每次退出（含 `chatWithTools` error、`awaitOutstandingSubagents` error）都发射一次 `TypeStepTiming`。新增 `stepOutcomeKind`/`stepOutcome` 类型。
  - `agent_test.go`：`TestRunEmitsToolAndFinalEvents` 和 `TestRunDeniesHighRiskToolWithoutExecuting` 的事件序列断言新增 `TypeStepTiming` 和 `TypeRunTiming`，并验证 duration 非负。
  - `tui/model_events.go`：主视图 switch 新增 `TypeStepTiming`/`TypeRunTiming` 分支，作为 info block 追加到 transcript。
  - `tui/model_subagent.go`：subagent 视图 `appendSubagentBlock` 同步新增 `TypeStepTiming`/`TypeRunTiming`。
  - `tui/toolfmt.go`：`toolEventTitle` 新增 timing 展示（`Step N · 1.2s` / `Total · 3.5s`），Step 编号 `event.Step+1`；新增 `formatDuration`（从 legacy UI 复制并修复浮点边界）；`toolEventDetail` 对 timing 事件返回空字符串。
  - `ui/renderer_tools.go`：legacy UI 同步修正 Step 编号为 1-based，新增 `TypeRunTiming` 展示，`formatDuration` 改用整数分钟除法避免 `1m60.0s`。
- 验证：`go test ./...` 通过；`go vet ./...` 通过。
- 备注：计时策略最终选择 step 级 + run 级两者都有——TUI 中每个 step 结束显示 `Step N · duration`，run 结束时显示 `Total · duration`。`executeStep` 的 defer 模式确保即使 `chatWithTools` 或 `awaitOutstandingSubagents` 返回 error 也不会漏发 step timing。

## 2026-07-05 - TUI 输入区实时总计时

- 摘要：将运行中的总耗时从正文区块挪到 TUI 输入区右上角，改为在 agent 运行期间持续刷新显示；run 结束后输入区实时计时消失，正文末尾继续保留 `Total · duration` 最终耗时记录。同时修正了一条因 `executeStep` 重构导致过时的 code navigation 测试断言。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/model_events.go`、`internal/tui/model_update.go`、`internal/tui/model_test.go`、`internal/tools/tools_test.go`。
- 改动要点：
  - 新增 TUI 本地 `runTimerTickMsg` 和运行态字段，在 `startRun()` 后以 `tea.Tick` 周期刷新输入区总耗时。
  - 输入区布局改为“实时总计时行 + 输入框”组合，计时文本固定右对齐显示在输入区顶部。
  - `TypeRunEnd` 后隐藏实时计时；`TypeRunTiming` 正文块仍保留最终总耗时。
  - 新增测试覆盖运行中计时显示位置、结束后仅保留正文总耗时。
  - 放宽 `internal/tools/tools_test.go` 中对 `chatWithTools` caller 的断言，兼容 `Run` 与 `executeStep` 两种结构。
- 验证：`go test ./...` 通过；`go vet ./...` 通过。
- 备注：实时总计时使用本地 TUI 时钟刷新，最终正文里的 `Total · duration` 仍以 runtime event 的实际运行耗时为准。

## 2026-07-05 - Step 耗时改为可配置开关

- 摘要：将 `step timing` 从默认总是输出改为可配置开关，默认关闭；`run timing` 总耗时保持默认开启。新增 `agent.step_timing_enabled` 配置项和 `AGENT_STEP_TIMING_ENABLED` 环境变量，用于按需开启每一步耗时事件。
- 主要模块：`internal/agent/options.go`、`internal/agent/agent.go`、`internal/agent/agent_test.go`、`internal/config/config.go`、`internal/config/yaml.go`、`internal/config/config_test.go`、`cmd/agent/main.go`。
- 改动要点：
  - `agent.Options` / `config.AgentConfig` 新增 `StepTimingEnabled`。
  - 默认配置中 `step timing` 关闭，只有显式开启时才发出 `TypeStepTiming`。
  - 新增 YAML 配置键 `agent.step_timing_enabled` 和环境变量 `AGENT_STEP_TIMING_ENABLED`。
  - 更新 agent 测试：默认不再期待 `TypeStepTiming`，新增一条开启开关后会发出 step timing 的测试。
- 验证：`gofmt -w` 通过；全量验证见本次最终回复。
- 备注：当前 UI 行为变为“默认只展示总耗时；需要逐 step 耗时时再开启配置”。

## 2026-07-07 - Slash 命令建议列表滚动修复

- 摘要：修复 TUI 中 `/` 命令建议列表只显示前 5 项且不会跟随当前选中项滚动的问题。现在当用户用方向键向下移动到第 6 项及以后时，可视窗口会随选中项一起滚动，保证末尾命令也能被看到和选中。
- 主要模块：`internal/tui/model.go`、`internal/tui/model_layout.go`、`internal/tui/model_render.go`、`internal/tui/model_test.go`。
- 改动要点：
  - 抽出 `maxVisibleSlashSuggestions` 常量，统一建议列表最大可见条数。
  - `renderSuggestions()` 改为根据 `slashSuggest` 计算可视窗口，而不是固定截取前 5 条。
  - 新增测试覆盖：下移到第 7 个 slash 命令时，列表窗口会滚动，前面的命令会退出可视区域。
- 验证：`gofmt -w` 通过；全量验证见本次最终回复。
- 备注：当前行为仍保留方向键循环选择，只修复“选中项不可见”的滚动问题。

## 2026-07-05 - 修正 /init 作用域并补齐 ECHODUST 注入回归测试

- 摘要：修复新加 `/init` 命令把 `ECHODUST.md` 写到 git 根目录的问题，改为与 `memory.ScopeProject` 一致，始终写到当前 workspace；同时修正 `.gitignore` 对 `cmd/agent/` / `internal/agent/` 新文件的误伤，并恢复对旧 `AGENTS.md` 的忽略；补齐 `/init` 和 `ECHODUST.md` 自动加载的回归测试。
- 主要模块：`cmd/agent/slash.go`、`cmd/agent/slash_test.go`、`internal/memory/memory_test.go`、`.gitignore`、`docs/WORKLOG.md`。
- 改动要点：
  - `slash.go`：删除“向上寻找 git 根目录后写文件”的逻辑，`/init` 现在直接在 `startup.Workdir` 下生成 `ECHODUST.md`，与 `discoverDocs()`/`DocPath(ScopeProject)` 的项目作用域定义保持一致。
  - `slash_test.go`：新增测试覆盖“在嵌套 workspace 中执行 `/init` 时，文件写入当前 workspace 而不是仓库根目录”以及“目标文件已存在时返回错误提示”。
  - `memory_test.go`：把项目级注入测试改为真实覆盖 `ECHODUST.md` 的发现与 `@import` 展开，并校验 `DocPath(ScopeProject)` 默认名。
  - `.gitignore`：将根目录二进制忽略规则从 `agent` 收紧为 `/agent`，避免误伤 `cmd/agent/`、`internal/agent/` 下的新文件；同时恢复忽略旧 `AGENTS.md`，避免已有本地说明文件因为品牌迁移而意外出现在版本控制中。
- 验证：
  - `go test ./...` 通过。
  - `go vet ./...` 通过。
- 已知限制或后续风险：
  - `README.md` 里关于 `/init` 和 `ECHODUST.md` 的公开说明仍未同步；当前修复先保证运行时行为和测试正确，文档可以在下一次整理时一并收敛。

## 2026-07-05 - 强化系统提示词的工程审查约束

- 摘要：将“结论前核实、跨模块不变量检查、副作用扫描、测试语义覆盖、review 先列 findings”等工程规则直接加入 agent 基础系统提示词，降低把文档滞后误判成实现缺失、或改动后遗漏仓库副作用检查的概率。
- 主要模块：`internal/agent/agent.go`、`internal/agent/agent_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `agent.go`：新增 `# Engineering Discipline` 段落，要求模型在声称“功能缺失”前先核实注册入口、实现函数、加载/注入链路，并区分实现缺失、语义不一致、文档过时、缺少测试、迁移未完成等问题类型。
  - `agent.go`：明确要求涉及路径、文件名、作用域、配置键、提示词注入文件的改动时，必须检查写入路径与读取路径是否一致，`workspace/project/repo root` 语义是否一致，以及测试、文档、兼容名、ignore 规则是否同步。
  - `agent.go`：把 `git status --short -uall` 与 `git diff --check` 提升为系统提示词里的默认副作用检查项，并要求验证成功路径、冲突路径、边界路径和兼容路径。
  - `agent_test.go`：扩展系统提示词断言，防止后续重构时静默丢失这些工程约束。
- 验证：
  - `go test ./...`
  - `go vet ./...`
- 已知限制或后续风险：
  - 系统提示词只能提高默认行为，不会替代具体任务里的项目规则；对于非常大的任务，仍然需要代码侧测试和人工 review 兜底。

## 2026-07-05 - TUI Todo 区块固定到内容末尾

- 摘要：调整 TUI 的 live todo 渲染顺序，不再把 todo 清单插到“当前用户消息”和后续 tool/agent 内容之间，而是固定追加到内容区末尾。这样运行中的 transcript、tool log 和 streaming 内容会一直显示在 todo 清单上方，todo 作为当前 run 的尾部状态区出现。
- 主要模块：`internal/tui/model_layout.go`、`internal/tui/model_test.go`、`docs/WORKLOG.md`。
- 改动要点：
  - `model_layout.go`：移除 `todoInsertBlockIndex()` 的中间插入逻辑，改为在完成 transcript、resume picker、inline approval、assistant draft 的内容拼接后，再把 `renderLiveTodoBlock()` 追加到末尾。
  - `model_layout.go`：补充注释，明确 todo 清单现在是“内容尾部状态区”，正文和工具流永远在它上方。
  - `model_test.go`：把顺序断言从“todo 位于 user 与 tool 之间”改为“todo 位于 tool 之后”，防止后续回退到旧布局。
- 验证：
  - `go test ./...`
  - `go vet ./...`
- 已知限制或后续风险：
  - 当前 todo 仍属于主 content viewport，而不是独立固定面板；如果后续需要“始终可见、不随正文滚动”的 todo 区，还需要再做单独布局分区。

## 2026-07-05 - Session 持久化后端从 JSON 目录迁移到 SQLite

- 摘要：将 `/resume` 会话持久化后端从每个 session 一个目录（`meta.json` + `state.json`）改为 SQLite 单文件存储。外部行为不变，`OpenStore`、`Save`、`Load`、`List` 接口语义保持一致。使用纯 Go 的 `modernc.org/sqlite` 驱动，避免 CGO 依赖。数据库文件位于 `<session root>/projects/<workspace-slug>/sessions.db`，保持按 workspace 隔离。首次打开时自动检测并幂等导入遗留 JSON session 目录，不删除原始文件。
- 主要模块：
  - `internal/session/session.go`：新增 `database/sql` + SQLite 初始化、WAL 模式、建表、`insertSession`、`migrateLegacyJSON`、`Close()`；`List` 改为 SQL `ORDER BY updated_at DESC, session_id DESC`；`Load` 缺失 session 返回 `os.ErrNotExist`；删除 `writeJSONAtomic`、`safeJoin`、`slug` 等不再使用的函数。
  - `internal/session/session_test.go`：新增 `TestStoreAutoMigratesLegacyJSON`（遗留 JSON → SQLite 自动迁移）、`TestStoreIdempotentMigration`（幂等迁移）、`TestStoreSaveOverwrite`（同 ID 覆盖写）；已有测试补充 `defer store.Close()`。
  - `cmd/agent/session_runtime.go`：新增 `sessionRuntime.Close()` 方法，释放 SQLite 连接。
  - `go.mod` / `go.sum`：新增 `modernc.org/sqlite v1.53.0` 作为直接依赖（纯 Go 实现，无 CGO）。
- 验证命令和结果：
  - `go test ./internal/session -v`：8 个测试全部通过（Save/Load roundtrip、List 排序、missing session、broken meta 跳过、home 展开、自动迁移、覆盖写、幂等迁移）。
  - `go test ./cmd/agent`：通过（包含 `/resume` 相关测试）。
  - `go test ./...`：除 `internal/tools` 的 `TestGoCodeNavigationTools` 因环境缺少 `gopls` 失败外（与本次改动无关），其余全部通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - `modernc.org/sqlite` 是纯 Go 编译的 SQLite 实现，首次编译时间较长且二进制体积增大（约增加 10MB），但避免了 CGO 交叉编译复杂度。
  - 遗留 JSON 目录在迁移后不会被删除，作为安全备份保留；迁移通过 `SELECT COUNT(*)` 检查保证幂等。
  - `conversation` 和 `ui_snapshot` 仍以 JSON blob 存储在 `state_json` 列中，未做拆表。
  - `meta.json` / `state.json` 常量和 `ProjectDir()` 方法保留，因为迁移逻辑仍需要读取遗留目录结构。

## 2026-07-05 - SQLite 迁移后续修正与 `/resume` 端到端回归覆盖

- 摘要：修正 SQLite 会话迁移收尾中的 4 个问题：`OpenStore` 不再吞掉 legacy JSON 迁移失败；主进程补上 session store 关闭；将 SQLite 驱动版本降到兼容仓库原始 `go 1.24.2` 的 `modernc.org/sqlite v1.29.0`；补齐从 legacy JSON 目录一路恢复到 `/resume` 与 UI snapshot 的端到端测试。
- 主要模块：
  - `internal/session/session.go`：legacy 迁移失败时立即关闭 DB 并返回错误，避免半迁移状态被静默放行。
  - `cmd/agent/main.go`：成功创建 `sessionRuntime` 后统一 `defer sessions.Close()`。
  - `cmd/agent/slash_test.go`：新增 legacy `meta.json/state.json` → `/resume latest` 恢复测试，校验 conversation 恢复、legacy synthetic system message 清洗、UI snapshot 加载、Session info block 追加。
  - `go.mod` / `go.sum`：将 SQLite 驱动及其间接依赖压回兼容 `go 1.24.2` 的版本集合。
- 验证命令和结果：
  - `GOMODCACHE=/home/lqy/ai-workspace/local-agent/.gomodcache GOCACHE=/home/lqy/ai-workspace/local-agent/.gocache GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org go test ./...`：通过。
  - `GOMODCACHE=/home/lqy/ai-workspace/local-agent/.gomodcache GOCACHE=/home/lqy/ai-workspace/local-agent/.gocache GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org go vet ./...`：通过。
- 已知限制或后续风险：
  - 本次验证为了绕过系统只读模块缓存与代理限制，使用了工作区内本地 `GOMODCACHE/GOCACHE` 并补齐缺失依赖；如果后续要在干净环境复现，需要确保可下载这些模块或预热缓存。

## 2026-07-05 - 为 chat 过程增加超时重试机制

- 摘要：为 agent 的 chat 过程增加有限重试，避免单次 LLM 请求因 `context deadline exceeded` 或其他超时型传输错误而直接终止整个 ReAct loop。默认对单次 chat 失败自动重试 1 次，重试间隔 2000ms；如果 streaming 已经向 UI 输出过可见 delta，则不再重试，避免重复打印半截回答。
- 主要模块：
  - `internal/agent/agent.go`：将 `chatWithTools` 改为“尝试 + 重试”包装；新增超时错误判定、context-aware backoff 等待，以及“streaming 失败但尚未输出内容时切回 non-streaming 再试”的逻辑。
  - `internal/agent/options.go`：新增 `ChatRetryOptions`，定义重试次数和退避间隔。
  - `cmd/agent/main.go`：把 LLM 重试配置映射到 agent options。
  - `internal/config/config.go`、`internal/config/yaml.go`、`internal/config/config_test.go`、`config.yaml`、`README.md`：新增 `llm.max_retries`、`llm.retry_backoff_milliseconds` 及对应环境变量说明。
  - `internal/agent/agent_test.go`：新增两条回归测试，覆盖“non-streaming 超时后成功重试”与“streaming 已输出 delta 后失败时不重试”。
- 验证命令和结果：
  - `go test ./internal/agent ./internal/config`：通过。
  - 全量验证见本次最终回复。
- 已知限制或后续风险：
  - 当前仅对超时类传输错误自动重试，未把 429/5xx HTTP 状态码纳入默认重试集合；如果后续提供商经常返回这类状态，可以再补一层基于状态码的可重试判定。

## 2026-07-05 - TUI 增加 chat 重试中的正文状态提示

- 摘要：把 chat 自动重试从“仅写日志”补齐到 TUI 运行态展示。现在主 agent 在等待下一次重试时，会在正文末尾追加一个临时状态块，显示当前重试次数、剩余等待时间，以及简短原因说明；新输出一旦到来，该状态块会自动消失，不会写入 resume 历史。
- 主要模块：
  - `internal/runtimeevent/runtimeevent.go`：新增 `chat_retry` 运行时事件类型。
  - `internal/agent/agent.go`：在计划重试时发出 `chat_retry` 事件，携带次数、backoff 和简短原因说明。
  - `internal/tui/model.go`、`internal/tui/model_events.go`、`internal/tui/model_layout.go`、`internal/tui/model_update.go`、`internal/tui/session.go`：新增 live retry 状态、正文末尾渲染、倒计时刷新，以及 run/session 结束时的清理逻辑。
  - `internal/agent/agent_test.go`、`internal/tui/model_test.go`、`internal/tui/session_test.go`：补充 retry 事件发射、TUI 排序/清理、倒计时刷新和 session 恢复清理的回归测试。
- 验证命令和结果：
  - `go test ./...`
  - `go vet ./...`
- 已知限制或后续风险：
  - 当前只有主 content viewport 会显示 live retry block；subagent 面板里的 retry 仍未单独可视化。
  - 重试倒计时按当前 TUI 的 200ms run tick 刷新，短 backoff（例如几毫秒）通常只会闪现或直接被后续输出覆盖。

## 2026-07-05 - 根据复制任务失败模式强化系统提示词闭环约束

- 摘要：针对“写了交互草稿但没有接入真实用户入口、功能文件未纳入 Git、残留 `.orig` 备份文件、没有形成最小完整解”的失败模式，补强主 agent 的系统提示词，让它在处理 TUI/CLI 交互需求时更强调入口闭环、交互出入口、Git 交付卫生和最小完整方案。
- 主要模块：
  - `internal/agent/agent.go`：在 `# Engineering Discipline` 段落新增 4 条规则，要求校验真实触发入口、交互进入/退出路径、核心实现文件是否被 Git 跟踪，以及优先选择最小完整修复。
  - `internal/agent/agent_test.go`：扩展系统提示词断言，防止后续重构时静默丢失这些约束。
- 验证命令和结果：
  - `go test ./...`
  - `go vet ./...`
- 已知限制或后续风险：
  - 系统提示词只能提高默认行为，不能替代具体功能的测试、review 和人工验收；对于复杂 TUI 交互，仍然需要代码侧回归测试兜底。

## 2026-07-05 - 收敛系统提示词为通用工程代理规范

- 摘要：在用户扩展版提示词基础上做最终收敛，保留“指令优先级、请求模式、验证标准、命令安全”等通用结构，同时删减重复表述并修正与当前工具能力不一致的 todo blocked 规则，使提示词更适合泛化到各类代码任务。
- 主要模块：
  - `internal/agent/agent.go`：重整主 agent 系统提示词，统一 `Tool Use`、`Delegation`、`Review And Editing`、`Verification` 等段落；保留真实入口闭环、Git 交付卫生、最小完整修复等关键约束；将 blocked todo 规则改为当前支持的 `pending/in_progress/completed` 状态表达。
  - `internal/agent/agent_test.go`：更新系统提示词断言，改为覆盖最终提示词中的稳定能力点和关键风险约束。
- 验证命令和结果：
  - `go test ./...`
  - `go vet ./...`
- 已知限制或后续风险：
  - 该提示词仍是硬编码在 agent 代码中的英文模板；如果后续希望按项目或用户偏好定制，需要再设计外部化配置与覆盖优先级。

## 2026-07-05 - 修正 TUI 中 todo 正文重复与 think 标签泄漏

- 摘要：修复 TUI 运行态里 todo 同时出现在正文和底部 checklist 的重复展示问题，并清理模型输出里泄漏到正文的 `<think>` / `</think>` 标签。现在当前 run 的 assistant 正文会去掉与 live todo 区重复的 checklist 行，推理标签和被标签包裹的内容也不会再出现在主内容区。
- 主要模块：
  - `internal/tui/assistant_text.go`：新增 assistant 文本净化逻辑，负责剥离 `<think>...</think>` 片段、孤立 think 标签，以及识别并移除与当前 todo 列表重复的 checklist 行。
  - `internal/tui/model_layout.go`：为当前 run 的 assistant block 增加渲染前预处理，仅对当前运行中的正文启用 todo 去重，保留历史 transcript 不变。
  - `internal/tui/text.go`：把通用终端文本清洗接入新的 assistant 文本净化逻辑。
  - `internal/tui/model_test.go`、`internal/tui/text_test.go`：补充 todo 去重和 think 标签清理的回归测试。
  - `internal/agent/agent_test.go`：同步修正一条系统提示词断言，避免与当前 prompt 文案不一致导致全量测试误报。
- 验证命令和结果：
  - `go test ./internal/tui`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前 todo 去重基于“assistant 正文中的 checklist 行”和 live todo 文本精确归一化匹配；如果模型用完全不同的表述复述计划，仍会保留在正文中。
  - think 标签清理当前只针对 `<think>` / `</think>` 这一类明文标签；若供应商改用其他私有推理标记，需再扩展过滤规则。

## 2026-07-05 - 增加 npm 全局分发壳与 release 发布链路

- 摘要：为项目补齐 npm 全局安装方案，使 `echo-dust-code` 可以作为一个轻量 npm 包对外发布，并在安装时按平台自动下载对应的 Go 二进制。同时新增 GitHub Actions release 工作流，统一完成 tag 校验、Go 交叉编译、GitHub Releases 上传与 npm trusted publishing。为保证全局安装后的可用性，还补充了配置文件查找顺序：显式环境变量路径优先，其次当前工作目录，再次用户级 `~/.echo-dust-code/config.yaml`。
- 主要模块：
  - `package.json`、`bin/echo-dust-code.js`、`lib/npm-platform.js`、`scripts/postinstall.js`：新增 npm 包壳、全局 bin 启动器、平台识别逻辑，以及按 tag/version 从 GitHub Releases 下载平台二进制的安装脚本。
  - `scripts/build-release-artifacts.sh`、`.github/workflows/release.yml`：新增 release 构建脚本和 GitHub Actions 工作流，覆盖版本校验、Go 测试/vet、Node 脚本语法检查、npm pack dry-run、跨平台归档、GitHub Release 发布与 npm trusted publishing。
  - `internal/config/config.go`、`internal/config/config_test.go`：新增 `AGENT_CONFIG_FILE`、用户级默认配置 `~/.echo-dust-code/config.yaml` 的解析逻辑，并补充显式配置、home 回退、workspace 优先级和缺失显式路径的回归测试。
  - `README.md`、`config.yaml`、`.gitignore`：补充 npm 全局安装/更新说明、配置文件查找顺序、发布说明，以及 npm 本地缓存和二进制下载目录的忽略规则。
- 验证命令和结果：
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
  - `npm run check`：通过。
  - `HOME=/home/lqy/ai-workspace/local-agent/.tmp-home NPM_CONFIG_CACHE=/home/lqy/ai-workspace/local-agent/.npm-cache ECHODUST_CODE_SKIP_DOWNLOAD=1 npm pack --dry-run`：通过，确认 npm 包内容与入口文件正确。
  - `./scripts/build-release-artifacts.sh linux amd64 /tmp/echodust-dist`：通过，成功生成 `echo-dust-code-linux-amd64.tar.gz`，归档内包含单个 `echo-dust-code` 可执行文件。
  - `git diff --check`：通过。
- 已知限制或后续风险：
  - npm `postinstall` 依赖系统可用的 `tar` 命令来解压 GitHub Release 归档；在极少数缺少 `tar` 的环境中需要额外安装或改成纯 JS 解压实现。
  - 当前 npm 包名使用 `@hongchengxianyue/echo-dust-code` 作用域；如果正式发布时希望改成其他组织名或无作用域名称，需要同步调整 `package.json`、README 和下载提示文案。
  - trusted publishing 仍需要先在 npm 后台为这个 GitHub 仓库配置 trusted publisher，且要求推送的 `vX.Y.Z` tag 与 `package.json` 版本完全一致。

## 2026-07-06 - 去掉 find_symbol 对 codegraph 的运行依赖并放宽外部工具测试

- 摘要：将 `find_symbol` 从依赖外部 `codegraph` 命令改为纯 Go 标准库实现，直接扫描 workspace 下的 Go AST 做符号匹配，避免在运行时和 CI 中把 codegraph 当成必需依赖。同时将 `gopls` 相关测试改为“命令不存在则 skip”，让 release 验证在精简环境下也能通过。
- 主要模块：
  - `internal/tools/code_tools.go`：删除 `find_symbol` 对 `codegraph query` 的外部命令依赖，改为遍历 workspace Go 文件并收集 `function/method/type/var/const` 符号；按精确匹配、前缀匹配、子串匹配排序输出。
  - `internal/tools/tools_test.go`：为 `gopls` 依赖型导航测试加入 `requireCommand` 检查，在命令缺失时自动 skip，而不是让整套 `go test ./...` 失败。
- 验证命令和结果：
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - `find_symbol` 现在只扫描 workspace 内 `.go` 文件中的显式声明，不提供像 codegraph 那样的跨语言或更丰富的图关系信息。
  - `find_references` / `find_callers` / `find_callees` 仍然依赖外部 `gopls`；只是测试层面在缺少命令时不再强制失败。

## 2026-07-06 - 将 gopls 作为 npm 发行物的内置强依赖一起打包

- 摘要：把 `gopls` 从“依赖用户本机 PATH”改为“随 npm release 资产一并分发”。现在每个 GitHub Release 归档都会同时包含 `echo-dust-code` 和 `gopls`，npm `postinstall` 会一起安装并强校验两者都存在；运行时 Node 启动器会把内置 `gopls` 路径注入环境，Go 侧优先使用该路径，从而让全局 npm 安装后的 `find_references` / `find_callers` / `find_callees` 能直接工作。
- 主要模块：
  - `scripts/build-release-artifacts.sh`：release 构建阶段除主程序外额外交叉编译 `gopls`，并将两个可执行文件一起打进平台归档。
  - `scripts/postinstall.js`、`lib/npm-platform.js`、`bin/echo-dust-code.js`：npm 安装阶段同时验证和落地 `echo-dust-code` 与 `gopls`，启动时通过 `ECHODUST_CODE_GOPLS` 把内置 `gopls` 路径传给主程序。
  - `internal/tools/code_tools.go`、`internal/tools/code_tools_test.go`：导航工具运行时优先使用环境变量指定的 `gopls`，其次检查主程序同目录下的 bundled `gopls`，最后才回退到系统 PATH；补充 env override 回归测试。
  - `README.md`：补充 npm 发行物内置 `gopls` 的说明。
- 验证命令和结果：
  - `go test ./...`
  - `go vet ./...`
  - `npm run check`
  - `ECHODUST_CODE_SKIP_DOWNLOAD=1 npm pack --dry-run`
- 已知限制或后续风险：
  - 当前 release 构建已固定使用 `golang.org/x/tools/gopls@v0.22.0`，可复现性更好；但该版本在构建时会触发 Go toolchain 自动切换到 `go1.26.4`，因此 GitHub Actions 需要保留外网下载 toolchain 的能力。
  - 如果未来要继续升级 `gopls` 版本，需要同步回归验证其最低 Go 版本要求，避免 release workflow 因 toolchain 不匹配而失败。

## 2026-07-06 - 修复 0.1.1 release 跨平台构建失败并准备重发

- 摘要：`v0.1.1` 在 GitHub Actions 的 release matrix 阶段暴露出两个真实问题：一是 `internal/ui` 的历史非阻塞输入逻辑直接把 `int` 传给 Windows 版 `syscall.Handle` API，导致 Windows 交叉编译失败；二是 release 脚本在交叉编译 `gopls` 时同时设置了 `GOBIN`，触发 `go install` 的跨平台限制。为避免复用失败 tag，本次将 npm 版本提升到 `0.1.2`，修复后重新发布。
- 主要模块：
  - `internal/ui/nonblock_io_unix.go`、`internal/ui/nonblock_io_windows.go`：为历史 UI 的非阻塞输入读写增加按平台分发的 syscall 封装，让 Windows 编译使用 `syscall.Handle`，Unix 继续使用 `int` fd。
  - `internal/ui/toggle_watcher.go`、`internal/ui/full_log_viewer.go`：改为复用平台封装，而不是在共享文件里直接写死 Unix 风格 `syscall.Read` / `SetNonblock`。
  - `scripts/build-release-artifacts.sh`：交叉编译 `gopls` 时改用临时 `GOPATH` 安装，再从对应 `bin` 目录拷贝到 release 归档，避开 `go install` 在 cross-compile 场景下对 `GOBIN` 的限制。
  - `package.json`：版本从 `0.1.1` 提升到 `0.1.2`，为新的 release tag 做准备。
- 验证命令和结果：
  - `go test ./...`
  - `go vet ./...`
  - `GOOS=windows GOARCH=amd64 go build ./cmd/agent`
  - `GOOS=darwin GOARCH=amd64 go build ./cmd/agent`
  - `GOOS=linux GOARCH=arm64 go build ./cmd/agent`
  - `npm run check`
  - `NPM_CONFIG_CACHE=/home/lqy/ai-workspace/local-agent/.npm-cache ECHODUST_CODE_SKIP_DOWNLOAD=1 npm pack --dry-run`
  - `env -u HTTPS_PROXY -u HTTP_PROXY -u ALL_PROXY GOPROXY=https://proxy.golang.org,direct ./scripts/build-release-artifacts.sh darwin amd64 /tmp/edc-release-smoke`
- 已知限制或后续风险：
  - `internal/ui` 是历史实现，当前只修到“可跨平台编译”的程度；其 Windows 运行时交互体验没有单独做实机回归，主维护面仍然是 `internal/tui`。
  - `gopls@v0.22.0` 仍会触发 Go toolchain 自动升级到 `go1.26.4`，因此 release workflow 依赖外网可正常下载该 toolchain。

## 2026-07-06 - 新增 /new 会话命令并支持 Esc 中断当前运行

- 摘要：为 slash 命令新增 `/new`，可以在当前 workspace 内立即开启一段全新的会话，对旧会话内容先做一次持久化保存，再清空内存中的 conversation 和 TUI transcript。同时为 TUI 补上 `Esc` 中断当前 `agent.Run` 的行为，并让审批提示打开时的 `Esc`/`Ctrl+C` 也能真正结束本次运行，而不是仅仅拒绝当前审批。
- 主要模块：
  - `cmd/agent/slash.go`、`cmd/agent/session_runtime.go`：新增 `/new` 命令和 `StartNewSession` 运行时入口，复用现有 session 存储、agent 恢复与 UI snapshot 重置逻辑。
  - `internal/tui/model_update.go`：把主输入态的 `Esc` 映射到 `interruptRun()`，同步修正审批态和信号态的取消路径，并更新运行中提示文案。
  - `cmd/agent/slash_test.go`、`internal/tui/model_test.go`：补充 `/new` 保存并清空会话、运行态 `Esc` 中断、审批态 `Esc` 中断的回归测试。
- 验证命令和结果：
  - `go test ./cmd/agent ./internal/tui`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - `/new` 会先尝试保存当前会话；如果当前会话还没有任何 conversation 消息，则不会生成新的历史 session 记录。
  - `Esc` 现在在 TUI 运行态优先用于中断当前 run；如果用户正停留在 subagent 详情视图，需要等 run 结束后再用 `Esc` 返回列表视图。

## 2026-07-06 - 调整 TUI 默认鼠标策略以恢复正文复制

- 摘要：修复 TUI 启动后正文难以直接鼠标选中复制的问题。原因是程序启动时默认开启了 Bubble Tea 鼠标捕获，终端拖拽会优先变成程序内鼠标事件。现在改为默认关闭鼠标捕获，让正文和 diff 可以直接框选复制；同时新增 `F2` 切换鼠标滚轮模式，用户需要滚动时再临时开启，切回后继续正常复制。
- 主要模块：
  - `cmd/agent/main.go`：移除 TUI 启动时默认的 `tea.WithMouseCellMotion()`，改成按需开启鼠标捕获。
  - `internal/tui/model.go`、`internal/tui/model_update.go`：新增鼠标模式状态、`F2` 切换逻辑，以及基于当前模式的输入框提示文案；鼠标关闭时忽略 TUI 内部滚轮事件，避免与终端原生选区冲突。
  - `internal/tui/model_test.go`：补充默认复制模式、`F2` 切换、鼠标关闭时忽略滚轮、开启后主 viewport 和 subagent viewport 仍可滚动的回归测试。
- 验证命令和结果：
  - `go test ./internal/tui ./cmd/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 默认复制模式下，鼠标滚轮不会驱动 TUI viewport；需要按一次 `F2` 进入鼠标滚轮模式，再按一次切回可复制模式。
  - 当前只做了“终端原生选区优先”和“F2 切换鼠标捕获”，还没有实现块级复制、OSC52 直接复制到剪贴板等更强的内建复制能力。

## 2026-07-06 - 将 TUI 默认交互改为鼠标滚轮优先并保留 F2 复制模式

- 摘要：修正上一版默认禁用鼠标捕获带来的交互回退。现在 TUI 启动后重新默认开启鼠标滚轮模式，正文可以直接滚动，避免部分终端把滚轮解释成历史导航并误带出 `/resume`。需要复制正文时，按 `F2` 临时切到文本复制模式，关闭鼠标捕获后再用终端原生框选复制；再按一次 `F2` 恢复滚轮模式。
- 主要模块：
  - `cmd/agent/main.go`：恢复 TUI 启动时默认 `tea.WithMouseCellMotion()`。
  - `internal/tui/model.go`、`internal/tui/model_update.go`：将默认鼠标状态改回开启，并保留 `F2` 在“滚轮模式”和“复制模式”之间切换。
  - `internal/tui/model_test.go`：更新默认模式、滚轮行为和 `F2` 切换的回归测试，确保主 viewport 和 subagent viewport 在默认状态下可滚动，复制模式下滚轮会被忽略。
- 验证命令和结果：
  - `go test ./internal/tui ./cmd/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 复制正文时仍需要显式按一次 `F2` 切到复制模式；这是终端鼠标协议本身对“滚轮/拖拽选区”二选一的限制，不是本项目独有。
  - 当前仍未提供块级复制或一键复制到系统剪贴板；后续如果频繁需要复制内容，最好补一个显式的 copy action。

## 2026-07-06 - 让 TUI 输入框支持多行粘贴并按内容自动增高

- 摘要：把 TUI 主输入从单行 `textinput` 改成多行 `textarea`，长文本或包含换行的粘贴内容不再被压成一行显示，输入框会按当前宽度自动增高，方便在发送前检查完整正文。同时保留原来的 Enter 提交语义，并补上 slash 命令候选的手动选择与补全，避免替换输入组件后交互退化。
- 主要模块：
  - `internal/tui/model.go`：将主输入模型切换为 `textarea`，补齐多行提示、slash 候选状态和与现有样式一致的输入外观。
  - `internal/tui/model_layout.go`、`internal/tui/model_render.go`：根据输入内容和终端宽度动态计算输入框高度，并让 slash 候选继续跟随输入框渲染。
  - `internal/tui/model_update.go`：保持 Enter 提交不变，新增 Tab 接受 slash 候选，并在多行输入时避免把上下方向键误当成历史切换。
  - `internal/tui/model_test.go`：补充长文本自动增高、多行粘贴显示、Tab 补全 slash 命令等回归测试。
- 验证命令和结果：
  - `gofmt -w internal/tui/model.go internal/tui/model_render.go internal/tui/model_layout.go internal/tui/model_update.go internal/tui/model_test.go`
  - `go test ./internal/tui ./cmd/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 输入框当前会按内容自动增高，但有一个上限，避免在小终端里占满整个视口；超出上限后会改为输入框内部滚动。
  - slash 候选仍然只看第一行且只在输入 slash 命令名时触发；如果后续要支持多行 prompt 模板或更复杂的命令参数提示，还需要继续扩展这一层逻辑。

## 2026-07-06 - 为 TUI 正文新增内建拖拽复制

- 摘要：为主 TUI 正文区域新增内部选区复制能力，默认鼠标模式下可以直接拖拽正文完成复制，不再必须先按 `F2` 在“滚动”和“复制”之间切换。实现方式不是依赖终端原生选区，而是基于 viewport 已渲染后的逐行文本做坐标映射、选区高亮和剪贴板写入；`F2` 仍然保留，用作切回终端原生框选的 fallback。
- 主要模块：
  - `internal/tui/copy.go`：新增正文渲染缓冲、鼠标拖拽选区、可见文本切片和剪贴板写入逻辑；优先走系统剪贴板，失败时回退到 OSC52。
  - `internal/tui/model_update.go`、`internal/tui/model_render.go`、`internal/tui/model_layout.go`：接入鼠标事件分发、复制结果提示、选区高亮渲染，以及在 viewport 内容重建时清理失效选区。
  - `internal/tui/model.go`、`internal/tui/session.go`：为模型增加复制状态、提示状态和渲染缓存，并在会话恢复时清空这些瞬态 UI 状态。
  - `internal/tui/model_test.go`：补充鼠标拖拽复制、单击不复制、宽字符切片等回归测试。
- 验证命令和结果：
  - `gofmt -w internal/tui/model.go internal/tui/copy.go internal/tui/model_update.go internal/tui/model_layout.go internal/tui/model_render.go internal/tui/session.go internal/tui/model_test.go`
  - `go test ./internal/tui ./cmd/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前内建选区只覆盖主 transcript viewport，尚未扩展到 subagent 详情面板。
  - 复制内容基于“已经渲染并换行后的可见文本”，因此它追求的是“接近终端原生框选”的体验，而不是保留 assistant 原始 markdown/source 的无折行版本。
  - 当正文因新事件流入、窗口宽度变化或重新排版而触发 viewport 重建时，现有选区会被主动清空，避免旧坐标错误映射到新文本。

## 2026-07-06 - 对话开始后自动收起顶部大字 Banner

- 摘要：为了让正文区域更接近原生终端的阅读和拖拽复制体验，启动空白页仍然保留 `ECHO DUST CODE` 大字 banner，但只要进入会话态，就自动收起成单行 header，把更多垂直空间让给 transcript viewport。
- 主要模块：
  - `internal/tui/model_render.go`：新增启动态/会话态 header 分支，空白页显示多行 banner，对话态切换为单行 slim header。
  - `internal/tui/model_test.go`：补充“启动页保留大字”“对话开始后自动收起”的回归测试，并更新已有 transcript 渲染断言。
- 验证命令和结果：
  - `gofmt -w internal/tui/model_render.go internal/tui/model_test.go`
  - `go test ./internal/tui ./cmd/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前收起条件按“进入会话态”判断，包括已有 transcript、resume picker、运行态和 subagent 面板；如果后续要做“对话后完全隐藏 header”，可以继续沿用这层分支逻辑扩展。

## 2026-07-06 - 强化主 Agent 对 skill 与 MCP 安装配置的系统提示词

- 摘要：增强主 Agent 的系统提示词，明确要求当用户说“给当前项目安装/接入 skill 或 MCP”但没有额外说明目标 runtime 时，默认理解为“给当前 workspace 的这个 agent 配置”，不能只停留在下载仓库或拉取脚本。提示词新增项目级 skill 与 MCP 的落点规则，以及两段 few-shot 示例，教模型把文件放到正确目录、同步修改 `config.yaml`、并验证最终加载路径。
- 主要模块：
  - `internal/agent/agent.go`：新增 `# Skill And MCP Setup` 和 `# Examples` 段落，写清项目级 skill 默认落点 `<workspace>/skills/`、项目级 MCP 需要 workspace-local `servers.json` 与 `config.yaml` `mcp.dir` 联动配置，并补充 project skill / project MCP 两个 few-shot。
  - `internal/agent/agent_test.go`：扩展系统提示词断言，锁定新增的 skill/MCP 安装配置规则和示例文案，防止后续回归。
- 验证命令和结果：
  - `gofmt -w internal/agent/agent.go internal/agent/agent_test.go`
  - `go test ./internal/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前改动是提示词增强，不是硬编码工作流；如果未来需要更强约束，可能还要补专门的安装/配置工具或 project bootstrap helper。
  - few-shot 中默认把“当前项目”解释为 workspace-local 配置；如果用户明确要求全局安装，模型仍应改走 `~/.echo-dust-code/skills` 或 `~/.echo-dust-code/mcp/servers.json`。

## 2026-07-06 - 校正 skill 与 MCP 安装提示词以匹配真实加载路径

- 摘要：继续修正上一版 skill/MCP 安装提示词，使其严格对齐当前代码和 npm 分发后的真实运行方式。新的提示词不再把项目根 `config.yaml` 当成唯一配置入口，而是先强调配置解析优先级：`AGENT_CONFIG_FILE`、workspace `./config.yaml`、`~/.echo-dust-code/config.yaml`、最后才是内建默认值。基于这个事实，skill 安装提示改为“默认项目 skill 根目录就是 `<workspace>/skills`，多数情况下不需要额外改配置”；MCP 安装提示改为“runtime 只读取一个 `mcp.dir`，默认是 `~/.echo-dust-code/mcp`，workspace-local `servers.json` 只有在活动配置真的把 `mcp.dir` 指过去时才会生效”。同时新增一个反例 few-shot，明确“随便 clone 到某个目录但 loader 根本不会扫描”不算安装完成。
- 主要模块：
  - `internal/agent/agent.go`：重写 `# Skill And MCP Setup` 和 `# Examples`，加入配置源优先级、skill 双根目录加载、MCP 单目录加载，以及“不要凭空发明 workspace config 改动”的约束。
  - `internal/agent/agent_test.go`：同步更新系统提示词断言，锁定新的加载路径事实、few-shot 文案和错误模式说明。
  - `docs/WORKLOG.md`：追加本次修正记录。
- 验证命令和结果：
  - `gofmt -w internal/agent/agent.go internal/agent/agent_test.go`
  - `go test ./internal/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前仍然是提示词层面的工程约束，不是硬编码安装向导；模型是否总能一步到位，仍取决于上下文是否给出了足够明确的 repo/path、目标 scope 和依赖要求。
  - MCP 的“当前项目”语义在现有实现里天然弱于 skill，因为默认 `mcp.dir` 是用户级单目录；要做到真正项目隔离，仍需要显式使用一个活动配置源把 `mcp.dir` 改到 workspace-local 目录。

## 2026-07-06 - 为 skill 与 MCP 提示词补充 JSON 结构样例

- 摘要：在上一版真实加载路径提示的基础上，继续补充 `skills/registry.json` 和 `mcp/servers.json` 的规范写法，避免模型只知道“该写哪个文件”，却不知道文件内容应该长什么样。系统提示词现在会要求优先参考 workspace 或 `~/.echo-dust-code` 下已有样例；若没有现成样例，则按内置的 canonical JSON 形状生成。`skills/registry.json` 明确为根对象 `{"skills":[...]}`，每项包含 `name`、`path`、`description`、`summary`、`input_schema`、`permissions.tools`、`triggers`；`mcp/servers.json` 明确优先使用对象形式 `{"servers":{"name":{...}}}`，字段包含 `command`、`args`、`env`、`cwd`、`enabled`，同时说明 array 形式也兼容但不是新文件首选。
- 主要模块：
  - `internal/agent/agent.go`：在 `# Skill And MCP Setup` 段落中补充“优先检查现有样例”和两个 canonical JSON 结构提示。
  - `internal/agent/agent_test.go`：扩展系统提示词断言，锁定新的 JSON 结构说明。
  - `docs/WORKLOG.md`：追加本次提示词增强记录。
- 验证命令和结果：
  - `gofmt -w internal/agent/agent.go internal/agent/agent_test.go`
  - `go test ./internal/agent`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 提示词里给的是“规范形状”和常用字段，不替代运行时校验；模型仍然应在写入前优先读取现有示例或现有配置，避免覆盖用户已有风格或遗漏 repo 特定要求。

## 2026-07-06 - 为主 Agent 与 Subagent 增加步数耗尽后的无工具总结轮

- 摘要：调整 agent 的 step budget 收尾策略。当主 Agent 或 Subagent 走到最后一步且无法继续扩步时，不再直接以 `without a final response` 结束，而是追加一轮“禁用全部工具、仅允许总结”的 LLM 调用，要求模型基于当前已收集的信息向用户输出最终总结。这样即使因为循环检测、扩步次数耗尽或其他 budget stop reason 停机，也尽量给用户一个可读结果，而不是空手退出。
- 主要模块：
  - `internal/agent/agent.go`：新增预算耗尽后的 summary-only 收尾逻辑；把通用聊天调用抽成“指定 messages + 指定 tools”的通路，复用现有的流式输出、重试和 token usage 统计；为最终总结轮注入临时 system 提示词。
  - `internal/agent/step_budget.go`：`maybeExtendStepBudget` 现在会把停止原因回传给 `Run`，用于决定是否触发最后总结轮，并把 stop reason 透传给总结提示词。
  - `internal/agent/agent_test.go`：把原本“重复工具循环后直接报错”的回归，改成“触发 step budget stop 后再做一次无工具总结”的回归，并断言最后一轮不再暴露 tools。
  - `internal/tools/tools_test.go`：同步更新 Go 代码导航测试的调用图期望，匹配 `chatWithTools -> chat` 的新结构。
- 验证命令和结果：
  - `gofmt -w internal/agent/agent.go internal/agent/step_budget.go internal/agent/agent_test.go internal/tools/tools_test.go`
  - `go test ./internal/agent/...`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 这次新增的是“一次 summary-only 的额外 LLM 收尾轮”，不是额外的工具行动轮；它会超出原 action step budget 一次聊天请求，但不会再允许执行工具。
  - 如果 stop reason 是上下文已取消（例如外部主动终止），当前实现不会再补总结轮，仍保持原有终止语义。

## 2026-07-07 - 强化主 Agent 的通用工程完成标准

- 摘要：撤销上一轮未提交的 approval/accept-all TUI 实现后，增强主 Agent 系统提示词，但不写入某个具体问题的专用修法。新的 `Complete Change Discipline` 段落把失败经验抽象为通用工程行为：用户可见行为变更必须按端到端 workflow 处理，修改前识别状态归属、入口、决策逻辑、输出路径、副作用、资源约束和测试；新增状态、模式、配置、命令或可见输出时，必须覆盖初始化、变更、持久化或重置、用户表现、运行时副作用和异常路径。同时新增 transient `engineering_checklist` 工具，把工程建模从纯 prompt 约束升级为可调用的程序级引导：非平凡代码变更前，模型需要先明确任务、变更类型、入口、预期行为和风险区域，再获得对应 checklist。
- 主要模块：
  - `internal/agent/agent.go`：新增 `# Complete Change Discipline` 段落，并要求非平凡代码变更前调用 `engineering_checklist`。
  - `internal/tools/engineering_checklist.go`：新增无副作用工程引导工具，按 bugfix、feature、UI 交互、API、配置安装、工具 I/O、重构等类型返回 checklist。
  - `internal/agent/tool_specs.go`、`internal/agent/tool_scheduler.go`、`internal/agent/tool_engineering.go`、`internal/agent/tool_todo.go`：接入 checklist 工具，使其不占普通并行工具额度、不走审批，并像 `update_todos` 一样从长期 conversation 中裁剪。
  - `internal/agent/agent_test.go`、`internal/tools/engineering_checklist_test.go`：补充系统提示词、工具输出、工具暴露和 transient 历史裁剪测试。
  - `README.md`：同步内置工具数量、工具分类和并行调度规则。
- 验证命令和结果：
  - `gofmt -w internal/agent/agent.go internal/agent/agent_test.go internal/agent/skill.go internal/agent/subagent.go internal/agent/tool_engineering.go internal/agent/tool_scheduler.go internal/agent/tool_specs.go internal/agent/tool_todo.go internal/tools/engineering_checklist.go internal/tools/engineering_checklist_test.go internal/tools/todo.go`
  - `go test ./internal/agent ./internal/tools`：通过。
  - `go test ./internal/tui`：通过。
  - `go test ./...`：通过。
  - `go vet ./...`：通过。
- 已知限制或后续风险：
  - 当前优化是“程序级引导”，不是硬阻断 gate。它能显著提高模型执行前的检查质量，但仍不能完全替代针对具体功能的回归测试和代码 review。
  - 如果后续仍希望进一步提升一致性，可以把关键完成标准继续下沉为 test helper、lint 或 harness gate，让代码层面强制失败，而不是依赖模型自觉遵守提示词。
