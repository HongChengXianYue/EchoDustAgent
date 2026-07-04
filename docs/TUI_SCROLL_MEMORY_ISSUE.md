# TUI 滚动卡顿与“内存泄漏”问题记录

## 1. 背景

在主 TUI 引入实时 diff 展示、整行铺色和多语言语法高亮后，滚轮滚动开始出现以下问题：

- 滚动明显卡顿，画面一顿一顿。
- 会话内容越长、diff 越多，卡顿越明显。
- 个别终端里会看到类似 `[<64;98;27M` 的鼠标滚轮转义序列残留在界面底部。
- 从用户体感上看，像是 “TUI 内存泄漏 + 滚动失真”。

## 2. 结论

这次问题并不是单一的典型内存泄漏，而是两类问题叠加：

1. 常驻内存占用真实存在：
   - 主 transcript 会持续保留在 `Model.blocks` 中。
   - 子代理 transcript 会持续保留在 `subagentSession.Blocks` 中。
   - diff block 会把完整 diff 文本保留在 block body 里。

2. 滚动路径存在高频全量重建：
   - 每次 `View()` 都会调用 `syncLayout()`。
   - 旧实现的 `syncLayout()` 每次都会重建主 viewport 和 subagent viewport 的全部内容。
   - 滚轮本来只该移动 `YOffset`，却被放大成 “整份 transcript 重新 render + Join + SetContent + Split”。
   - diff block 在重建时还会重新做语法高亮，进一步放大 CPU 和内存分配成本。

用户感知到的 “内存泄漏” 主要来自第 2 类问题：滚动期间产生了大量瞬时分配和频繁 GC，表现为卡顿、掉帧、失真和输入区异常残留。

## 3. 根因拆解

### 3.1 常驻内存主要在哪

- `internal/tui/model.go`
  - `Model.blocks`
  - `Model.assistantDraft`
  - `Model.subagents`
- `internal/tui/model_subagent.go`
  - `subagentSession.Blocks`

这部分内存会随会话增长而增加，属于当前设计下的“保留完整 transcript”成本，不是 bug 本身。

### 3.2 滚动为什么会卡

旧链路如下：

1. Bubble Tea 收到滚轮事件。
2. viewport 只改 `YOffset`。
3. 但下一帧 `View()` 又调用 `syncLayout()`。
4. `syncLayout()` 无条件执行：
   - `rebuildViewportContent()`
   - `rebuildSubagentViewportContent()`
5. `rebuildViewportContent()` 会：
   - 遍历全部 transcript block
   - 重新 render 所有 block
   - `strings.Join(...)` 拼出完整 transcript 字符串
   - `viewport.SetContent(...)` 再把整份文本拆回行数组
6. 如果 block 里包含 diff：
   - 重新跑 unified diff 行渲染
   - 重新跑 Chroma 语法高亮

结果是：一次滚轮动作触发了整页重建，时间复杂度接近 O(整份 transcript 大小)，而不是 O(可视窗口大小)。

### 3.3 为什么会看到鼠标转义序列

截图里的 `[<64;...M` 是 SGR 鼠标滚轮序列。

本次定位下来，主问题仍是滚动路径过重导致的渲染阻塞。UI 阻塞后，终端输入序列更容易以肉眼可见的形式残留在底部区域。当前主 TUI 没有像旧 `full_log_viewer` 那样做一层自定义鼠标转义序列兜底解析，所以这类现象在高负载时更容易暴露。

本次修复没有改动鼠标协议解析器，而是优先消掉滚动路径上的整页重建，先把主要卡顿源拿掉。

## 4. 本次修复

### 4.1 改动目标

把滚动路径从“整页重建”改成“仅 viewport 偏移变化”，让 `syncLayout()` 只在真正需要时才重建内容。

### 4.2 具体做法

在 `internal/tui/` 增加了三类脏标记：

- `layoutDirty`
- `viewportDirty`
- `subagentViewportDirty`

关键调整如下：

1. `View()` 仍然调用 `syncLayout()`，但 `syncLayout()` 现在会先判断脏标记。
2. 当没有布局变化、没有主 transcript 变化、没有 subagent 内容变化时，`syncLayout()` 直接返回。
3. 只有以下场景才会重建主 viewport 内容：
   - transcript block 新增或删除
   - assistant draft 变化
   - todo 变化
   - approval inline 内容变化
   - diff block 新增
   - viewport 宽度变化，导致换行结果可能变化
4. 只有以下场景才会重建 subagent viewport 内容：
   - subagent detail transcript 变化
   - 进入 subagent detail
   - subagent viewport 宽度变化
5. 仅高度变化但内容未变时：
   - 不再重建全文
   - 如果原本就在底部，则继续贴底，避免高度变化后视口偏一行

### 4.3 这次修掉了什么

- 修掉了滚轮滚动触发整份 transcript 重建的问题。
- 修掉了许多“内容没变却重复 `SetContent`”的无效分配。
- 保留了原本的展示效果、slash suggestions、resume picker、approval inline、subagent panel 与 diff block 行为。

## 5. 验证

本次实际执行的验证命令：

- `gofmt -w internal/tui/model_dirty.go internal/tui/model.go internal/tui/model_layout.go internal/tui/model_events.go internal/tui/model_subagent.go internal/tui/model_update.go internal/tui/session.go internal/tui/resume_picker.go internal/tui/model_test.go`
- `go test ./internal/tui ./internal/ui`
- `go test ./...`
- `go vet ./...`

另外新增了两条回归测试：

- `TestMouseWheelDoesNotDirtyViewportContent`
  - 确保滚轮不会把主 viewport 重新标记为 dirty。
- `TestTypingSlashCommandMarksLayoutDirtyAndRendersSuggestions`
  - 确保输入变化仍然能正确触发布局刷新，不会因为脏标记优化而让 suggestions 丢失。

## 6. 仍然存在的限制

### 6.1 transcript 常驻内存仍然会增长

这次没有做 transcript 虚拟化，也没有做历史 block 淘汰。会话越长，`Model.blocks` 和 `subagentSession.Blocks` 仍然会继续增长。

这属于当前产品策略，不是本次修复的范围。

### 6.2 diff 高亮仍然会在内容重建时重新执行

虽然滚动不再触发全文重建，但当 transcript 本身继续增长时，diff block 仍然会在 rebuild 时重新 render 和高亮。

后续如果要继续降耗，可以考虑：

- 给 diff block 增加按 body hash 的渲染缓存
- 对超长 transcript 做窗口化或分页
- 对历史 block 做折叠或延迟展开

### 6.3 鼠标转义序列问题只被间接缓解

本次没有直接修改 Bubble Tea 层的鼠标协议处理逻辑，只是把滚动路径大幅减重。正常情况下，这已经足以显著缓解滚动时的失真和底部残留；但如果后续仍偶发看到原始鼠标序列，需要再单独补一层输入协议兜底。

## 7. 后续建议

优先级建议如下：

1. 观察实际终端中的滚轮体验是否已恢复流畅。
2. 若仍有长会话场景下的内存压力，优先做 transcript 虚拟化或历史折叠。
3. 若仍偶发看到 `[<64;...M` 一类输入残留，再单独处理鼠标转义序列的兜底解析。

## 8. 相关文件

- `internal/tui/model_dirty.go`
- `internal/tui/model_layout.go`
- `internal/tui/model_update.go`
- `internal/tui/model_events.go`
- `internal/tui/model_subagent.go`
- `internal/tui/resume_picker.go`
- `internal/tui/session.go`
- `internal/tui/model_test.go`
