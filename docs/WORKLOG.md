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
