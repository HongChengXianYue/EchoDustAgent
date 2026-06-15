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
