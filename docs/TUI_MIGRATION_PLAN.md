# local-agent TUI 迁移方案

> 基于 2025 年对话分析整理。当前 UI 使用手写 ANSI 转义码（事件驱动 + 实时帧覆盖 + 折叠输出），本文件记录引入 Bubble Tea TUI 的全套方案。

---

## 1. 背景与动机

### 当前 UI 方案（legacy）

- 事件驱动：Agent 发射 `runtimeevent.Event`，`BlockRenderer` 消费
- 运行时使用 ANSI 转义实现可覆盖的 Live Frame（`\x1b[N A` 上移 + `\x1b[J` 清屏）
- 非运行时使用一次性的 Block 输出
- 键盘通过后台 goroutine + `syscall.SetNonblock` + 轮询读取
- 全屏日志查看器通过 alternate screen（`\x1b[?1049h`）+ 手写滚动/分页/鼠标支持

### 现有问题

1. **帧定位健壮性不足** — 终端自动折行后 `countLines()` 按 `\n` 计数，与实际显示行数不一致，光标定位会偏移
2. **Mutex 持有期间做 I/O** — `HandleEvent()` 加锁后写 `os.Stdout`，写阻塞会连带阻塞 Agent 的事件发射
3. **键盘轮询浪费** — 每 30-50ms 唤醒一次检查按键，有响应延迟
4. **手写了一个终端分页器** — `full_log_viewer.go` ~400 行，包含 ANSI 序列解析、滚动、鼠标、分页，维护成本高
5. **审批两路通信不够优雅** — 审批时 `stopKeyWatcher()` 暂停键盘监听，审批结束后恢复，有竞态风险
6. **测试盲区** — 1000+ 行测试只覆盖事件组合逻辑，无法验证视觉输出、ANSI 序列是否正确

### 引入 TUI 能解决什么

| 问题 | 解决方式 |
|---|---|
| 帧定位偏移 | Bubble Tea 的 Model/Update 自动管理重绘区域，不需要手动计数 |
| Mutex + I/O 阻塞 | Agent 通过 channel 发事件到 TUI 的独立 goroutine，彻底解耦 |
| 键盘轮询 | `tea.KeyMsg` 是框架一等公民，原生阻塞读取 |
| 手写分页器 | `bubbles/viewport` 替代，支持滚动、按键绑定、鼠标开箱即用 |
| 审批交互 | `tea.Msg` 路由 + channel 双向通信 |
| 视觉测试 | 可以直接 assert Model 的内部状态（todos、events、mode） |
| 输入框 | `bubbles/textinput` 替代手写的 raw mode 输入处理 |

### 代价

- `go.mod` 会增加 ~20 个间接依赖（bubbletea + bubbles + lipgloss + 其依赖）
- 但项目已依赖同一生态的 `charmbracelet/glamour`，边际成本较低
- 约 800-900 行手写代码可被 ~200 行 Bubble Tea 胶水代码替代

---

## 2. 架构设计：事件桥接模式

核心思路：**不改 Agent 的 `emit()` 机制**，只在渲染侧替换。

```
┌─────────────────────┐
│       Agent         │
│  emit(Event)        │────► runtimeevent.Handler 接口（已存在）
└─────────────────────┘          │
                                 │
                    ┌────────────┴────────────┐
                    ▼                         ▼
            BlockRenderer(旧)        TUIEventBridge(新)
            os.Stdout 直接写          eventChan chan Event
                                                 │
                                    ┌────────────┘
                                    ▼
                          TUIProgram (bubbletea)
                          ┌─────────────────────┐
                          │  Model (状态)        │
                          │  ├─ todos           │
                          │  ├─ toolEvents[]    │
                          │  ├─ assistantText   │
                          │  ├─ userInput       │
                          │  └─ mode: run/idle  │
                          │                     │
                          │  Update(msg)         │◄── tea.Msg 包括：
                          │  ├─ EventMsg(Event)  │    事件桥发来的 Event
                          │  ├─ tea.KeyMsg      │    键盘输入
                          │  └─ ApprovalResult   │    审批结果
                          │                     │
                          │  View() → string    │──► lipgloss 布局
                          └─────────────────────┘
```

### 关键类型定义

```go
// internal/ui/tui/bridge.go

// EventMsg 包装 runtimeevent.Event 使其实现 tea.Msg
type EventMsg struct {
    Event runtimeevent.Event
}

// TUIEventBridge 实现 runtimeevent.Handler
// 通过 channel 将事件转发给 Bubble Tea 程序
type TUIEventBridge struct {
    eventChan chan runtimeevent.Event
}

func (b *TUIEventBridge) HandleEvent(event runtimeevent.Event) {
    b.eventChan <- event
}
```

```go
// internal/ui/tui/model.go

type Model struct {
    // 状态
    mode          Mode  // run / idle / approval
    todos         []runtimeevent.TodoItem
    toolEvents    []runtimeevent.Event
    assistantText string
    userMessage   string

    // 子组件
    assistantView  viewport.Model
    todoPanel      lipgloss.Style
    toolsPanel     lipgloss.Style
    input          textinput.Model

    // 审批
    approvalResult chan approval.Decision
    approvalEvent  *runtimeevent.Event

    // 事件通道（由外部注入）
    eventChan      <-chan runtimeevent.Event
    bridge         *TUIEventBridge

    // 配置
    options        Options
}
```

---

## 3. Model 状态机

```
┌─────────┐    RunStart     ┌─────────┐
│  idle   │ ──────────────► │   run   │
│         │                 │         │
│ 等待输入│                 │ 正在运行│
└─────────┘                 └────┬────┘
      ▲                          │
      │     RunEnd / Final       │
      └──────────────────────────┘

  运行中遇到审批：
  ┌─────────┐   ApprovalRequest   ┌────────────┐
  │   run   │ ──────────────────► │  approval  │
  └─────────┘                     │            │
                                  │ 底部弹出   │
                                  │ 选择器     │
                                  └──────┬─────┘
                                         │
                          approvalDecision
                          ───────────────► run (继续)
```

---

## 4. 布局方案

```
┌─ 终端窗口 ───────────────────────────────────────┐
│                                                  │
│  ┌────────── Assistant (streaming / final) ────┐ │
│  │ 这是 Agent 的回复内容，使用 glamour          │ │  ← viewport.Model
│  │ 渲染 Markdown，自动滚动到底部                │ │     流式更新
│  │                                              │ │
│  └──────────────────────────────────────────────┘ │
│                                                  │
│  ┌─── Todo ────┐  ┌─── Tools (Ctrl+E toggles) ─┐ │
│  │ [✓] 分析    │  │ 🔄 go test ./...           │ │  ← lipgloss.JoinHorizontal
│  │ [>] 实现    │  │ ✓ list_files (3 files)     │ │     可折叠右侧面板
│  │ [ ] 测试    │  │ ✓ write_file foo.go (+15)  │ │
│  └─────────────┘  └───────────────────────────-┘ │
│                                                  │
│  ┌──────────────────────────────────────────────┐ │
│  │ › 请输入你的需求...                    Ctrl  │ │  ← textinput.Model
│  └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### 面板详细定义

| 面板 | 组件 | 说明 |
|---|---|---|
| **Assistant** | `bubbles/viewport` | Markdown 渲染（glamour），流式追加，自动滚动到底 |
| **Todo** | 自定义 lipgloss 块 | 绿色 `[✓]/[>]/[ ]` 标记，无滚动（短内容） |
| **Tools** | 自定义 lipgloss 块 + viewport | 可折叠（Ctrl+E），展开后用 viewport 支持滚动 |
| **Input** | `bubbles/textinput` | 底部输入栏，支持历史记录、多行 |
| **审批弹窗** | 自定义 overlay | 模式化，覆盖在其他面板之上，完成后消失 |

### 全屏日志查看器的替代方案

在 Bubble Tea 中，不再需要单独的 `\x1b[?1049h` 全屏查看器：
- Tools 面板展开后自带 viewport 滚动
- 如果想查看所有历史，可以在 Tools 面板中显示全部事件（不折叠）
- 或者用一个 `tea.WindowSizeMsg` 触发的替代屏幕模式（`tea.WithAltScreen()`）

---

## 5. 审批两路通信

### 当前方案的痛点

```go
// 当前：Agent 阻塞等待 approver
func (a *Agent) approveTool(...) bool {
    decision, err := a.approver.Approve(ctx, ...)
    // 此时 renderer 的 keyWatcher 必须先 stop
    // 审批结束后再 restart
    return decision.Approved
}
```

### TUI 方案

```go
// Agent 侧（在 tool_scheduler.go 中修改）
func (a *Agent) approveTool(ctx, step, tool, category, args, writeImpact, loopApprovals) bool {
    if a.tuiMode {
        result := make(chan approval.Decision, 1)
        a.emit(Event{
            Type: TypeApprovalRequest,
            ExtraData: ApprovalRequestData{
                Decisions: d,
                ResultCh:  result,  // 两路通道
            },
        })
        select {
        case decision := <-result:
            return decision.Approved
        case <-ctx.Done():
            return false
        }
    }
    // legacy 路径
    return a.approver.Approve(...)
}
```

```go
// TUI Model 侧
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case EventMsg:
        if msg.Event.Type == TypeApprovalRequest {
            data := msg.Event.ExtraData.(ApprovalRequestData)
            m.mode = ModeApproval
            m.approvalResult = data.ResultCh
            m.approvalEvent = &msg.Event
            return m, nil
        }
    case tea.KeyMsg:
        if m.mode == ModeApproval {
            switch msg.String() {
            case "y", "Y":
                m.approvalResult <- approval.Decision{Approved: true}
                m.mode = ModeRun
            case "n", "N":
                m.approvalResult <- approval.Decision{Approved: false}
                m.mode = ModeRun
            }
            close(m.approvalResult)
            return m, nil
        }
    }
}
```

这样 Agent 不再直接操作终端，TUI 框架天然处理键盘输入，不存在竞态。

---

## 6. 事件桥实现

```go
// internal/ui/tui/bridge.go
package tui

import (
    "local-agent/internal/runtimeevent"
    tea "github.com/charmbracelet/bubbletea"
)

// eventBridge 实现 runtimeevent.Handler
// 接收 Agent 事件并通过 tea.Cmd 发送到 TUI 的事件循环
type eventBridge struct {
    events chan runtimeevent.Event
}

func (b *eventBridge) HandleEvent(event runtimeevent.Event) {
    b.events <- event
}

// waitForEvent 返回一个 tea.Cmd，从 channel 中消费事件
func waitForEvent(events <-chan runtimeevent.Event) tea.Cmd {
    return func() tea.Msg {
        event, ok := <-events
        if !ok {
            return nil
        }
        return EventMsg{Event: event}
    }
}

// StartTUI 是入口函数，创建 TUI 并返回 bridge 和 run 函数
func StartTUI(options Options) (*eventBridge, func() error) {
    events := make(chan runtimeevent.Event, 64)
    bridge := &eventBridge{events: events}
    // ... 创建 Model，启动 tea.NewProgram
    return bridge, func() error {
        return program.Start()
    }
}
```

---

## 7. 增量迁移路径

### Phase 1 — 并行实现（不改现有代码）

```
internal/ui/
├── renderer.go              ← 不动
├── renderer_frame.go        ← 不动
├── renderer_tools.go        ← 不动
├── ...
├── tui/                     ← 新增
│   ├── model.go             ← Bubble Tea Model/Update/View
│   ├── bridge.go            ← 事件桥 + tea.Cmd
│   ├── panels.go            ← 各面板的 View() 逻辑
│   ├── approval.go          ← 审批交互
│   └── options.go           ← TUI 配置
```

- Model 内部只维护状态，不写 `os.Stdout`
- 写测试验证状态转换：`model.Update(EventMsg{...})` → assert `model.todos`
- 不接入 main.go，只写单元测试验证逻辑

### Phase 2 — 可切换

```yaml
# config.yaml
ui:
  mode: tui  # legacy | tui
```

```go
// cmd/agent/main.go
var renderer runtimeevent.Handler
if cfg.UI.Mode == "tui" {
    bridge, runTUI := tui.StartTUI(tuiOptions(cfg.UI))
    renderer = bridge
    go runTUI()  // 在后台 goroutine 中运行 TUI 事件循环
} else {
    renderer = ui.NewInteractiveBlockRendererWithOptions(os.Stdin, os.Stdout, ...)
}
codingAgent.SetRenderer(renderer)
```

- 两个渲染器并行存在
- 用户可通过配置文件切换
- 旧功能不受影响

### Phase 3 — 稳定后删除旧代码

删除以下文件：

| 文件 | 原因 |
|---|---|
| `renderer_frame.go` | Bubble Tea 自动管理帧 |
| `toggle_watcher.go` | tea.KeyMsg 替代 syscall 轮询 |
| `full_log_viewer.go` | viewport + 展开面板替代 |
| `prompt.go` | textinput 替代 |
| `input.go` | raw mode 管理由框架处理 |
| `format.go` 中部分函数 | lipgloss 替代手写 ANSI |

保留文件（逻辑可复用）：

| 文件 | 可复用内容 |
|---|---|
| `renderer_todo.go` | TODO 标记 `[✓]/[>]/[ ]`、`greenText` |
| `renderer_tools.go` | 事件标题生成 `toolEventTitle`、`fileChangeTitle` |
| `renderer_changes.go` | 文件变更摘要格式化 |
| `renderer_final.go` | Markdown 渲染（glamour） |
| `renderer_approval.go` | 审批详情格式化 |
| `options.go` | 通用配置 |

---

## 8. 关键实现细节

### 8.1 流式 Assistant 文本

```go
func (m Model) handleAssistantDelta(delta string) {
    m.assistantText += delta
    m.assistantView.SetContent(renderMarkdown(m.assistantText))
    m.assistantView.GotoBottom()
}
```

`viewport.GotoBottom()` 确保流式输出时自动滚动到底部。

### 8.2 工具折叠状态

```go
type Model struct {
    toolsExpanded bool
    // ...
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if msg.String() == "ctrl+e" {
            m.toolsExpanded = !m.toolsExpanded
        }
    }
}

func (m Model) View() string {
    if m.toolsExpanded {
        // 完整的事件列表（用 viewport 支持滚动）
    } else {
        // 一行摘要：N event(s), latest: xxx
    }
}
```

### 8.3 Agent 结束时

```go
func (m Model) handleRunEnd() {
    m.mode = ModeIdle
    // 清空实时流式状态，保留最终内容用于 View
}
```

### 8.4 信号处理

```go
// main.go
// Ctrl+C 由 Bubble Tea 的 tea.Quit 处理
// 不再需要单独的 signal goroutine
```

但需要确保 Agent 的 context cancel 与 TUI 的 quit 联动：

```go
type quitMsg struct{}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    case tea.KeyMsg:
        if msg.String() == "ctrl+c" {
            if m.cancel != nil {
                m.cancel()  // 取消当前 Agent run
            }
            return m, tea.Quit
        }
}
```

---

## 9. 测试策略

### TUI 的优点：可以直接 assert Model 状态

```go
func TestModelReceivesTodoEvent(t *testing.T) {
    m := NewModel()
    m, _ = m.Update(EventMsg{Event: runtimeevent.Event{
        Type:  runtimeevent.TypeTodoUpdate,
        Todos: []runtimeevent.TodoItem{
            {Text: "分析需求", Status: runtimeevent.TodoInProgress},
        },
    }})
    assert.Equal(t, 1, len(m.todos))
    assert.Equal(t, "分析需求", m.todos[0].Text)
}

func TestModelModeTransitions(t *testing.T) {
    m := NewModel()
    assert.Equal(t, ModeIdle, m.mode)

    m, _ = m.Update(EventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
    assert.Equal(t, ModeRun, m.mode)

    m, _ = m.Update(EventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunEnd}})
    assert.Equal(t, ModeIdle, m.mode)
}

func TestApprovalFlow(t *testing.T) {
    resultCh := make(chan approval.Decision, 1)
    m := NewModel()
    m, cmd := m.Update(EventMsg{Event: runtimeevent.Event{
        Type: runtimeevent.TypeApprovalRequest,
        ExtraData: ApprovalRequestData{
            ResultCh: resultCh,
        },
    }})
    assert.Equal(t, ModeApproval, m.mode)

    // 模拟键盘事件
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
    decision := <-resultCh
    assert.True(t, decision.Approved)
    assert.Equal(t, ModeRun, m.mode)
}
```

### 不测试的事情

- View() 的视觉输出不 assert（lipgloss 样式、边框对齐等）
- 这些可以通过手动审查确认

---

## 10. 实施顺序建议

```
Step 1:  创建 internal/ui/tui/ 包，定义 Model 结构体和基本 Update/View
Step 2:  实现事件桥 bridge.go，写单元测试验证 Agent 事件驱动 Model 状态变化
Step 3:  实现 assistant 面板（viewport + glamour），处理流式 delta
Step 4:  实现 todo 面板
Step 5:  实现 tools 面板（折叠/展开 + viewport 滚动）
Step 6:  实现审批交互（overlay + channel 两路通信）
Step 7:  实现输入栏（textinput + 历史记录）
Step 8:  接入 main.go，添加 config.yaml 切换选项
Step 9:  功能测试（手动验证各场景：流式输出、审批、工具调用、中断）
Step 10: 稳定后清理旧代码（Phase 3）
```

---

## 11. 验证清单

- [ ] Agent 启动后 TUI 正常显示，输入栏可交互
- [ ] 用户输入后 TUI 切换到 run 模式，显示 TODO 列表
- [ ] 工具调用实时显示在 Tools 面板，Ctrl+E 折叠/展开
- [ ] 文件变更摘要正确显示（write_file、replace_in_file、apply_patch）
- [ ] Assistant 流式文本实时渲染，自动滚动到底部
- [ ] 审批请求在 TUI 中弹窗，y/n 选择后恢复运行
- [ ] Ctrl+C 中断正常（取消 Agent + 退出 TUI）
- [ ] 最终答案正确显示（glamour Markdown 渲染）
- [ ] 终端恢复干净（退出后无残留 ANSI 转义）
- [ ] 非 TTY 模式（管道/重定向）回退到 BlockRenderer
- [ ] `mode: legacy` 配置项正常工作，旧 UI 不受影响
