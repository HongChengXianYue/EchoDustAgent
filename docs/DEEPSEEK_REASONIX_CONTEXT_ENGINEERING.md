# DeepSeek Reasonix Go 版本上下文工程剖析

本文分析 `esengine/deepseek-reasonix` 的 Go 重写版本，也就是上游 `main-v2` 分支。结论先说清楚：Reasonix 并没有在客户端实现 DeepSeek 的 KV cache。DeepSeek 的 prefix cache 是服务端能力，Reasonix 做的是上下文工程：让每一轮请求尽可能复用上一轮请求已经出现过的、字节级稳定的前缀，从而把服务端缓存命中率推到很高。

上游旧版基准文档给出过一个单用户单日数据：

| 指标 | Token 数 |
| --- | ---: |
| Input cache hit | 435,033,856 |
| Input cache miss | 767,616 |
| Output | 179,763 |

命中率计算为 `435,033,856 / (435,033,856 + 767,616) = 99.82%`。这个数字来自上游项目自述和基准材料，本文没有独立复现该线上数据，重点是剖析代码里支撑这种命中率的机制。

## 1. DeepSeek prefix cache 的实际问题

DeepSeek API 默认提供 prefix caching。对 agent 来说，关键不是“有没有缓存”，而是下一次请求的开头能不能和上一次请求的开头保持一致。

一次 agent 请求大致由这些部分组成：

```text
model / request options
+ system prompt
+ tool schema list
+ conversation messages already accumulated
+ this turn's new user / assistant / tool suffix
```

如果前面的大块内容保持完全一致，新请求只是在尾部追加新消息，那么 DeepSeek 可以把旧请求的前缀作为缓存命中。反过来，下面这些看似无害的变化都会破坏前缀：

- 每轮重新拼 system prompt，顺序或空白发生变化。
- 工具 schema 每轮重新序列化，map 顺序或工具顺序漂移。
- 中途把 plan、memory、background job 状态塞回 system prompt。
- 修改、重排或清理历史消息。
- 把模型的 reasoning 内容原样回灌到下一轮 prompt。
- 太频繁地压缩历史，导致旧前缀被 summary 替换。

Reasonix 的上下文工程就是围绕这些风险做约束：稳定前缀，尾部追加，少改历史，必要改写时可观测。

## 2. 启动阶段：系统前缀只构造一次

Go 版启动流程在 `internal/boot/boot.go` 中完成系统提示词组装，然后创建 executor session：

```text
base prompt
+ output style
+ language policy
+ optional token economy prompt
+ memory block
+ skill index
=> agent.NewSession(sysPrompt)
```

关键设计点是：这些内容在 boot 时一次性合成，进入 session 后作为稳定的 system prompt 使用。这样每一轮主 agent 请求都能复用同一段 system 前缀。

### Memory 的位置

`internal/memory/memory.go` 的 `Compose(base, set)` 会把 memory block 拼到 base prompt 后面，并明确保留 base 在最前面：

```text
base system prompt

persistent memory block
```

这有两个好处：

1. 同一个 session 内，memory 不会每轮重新扫描再改写 system prompt。
2. 跨 session 即使 memory 变化，最稳定的 base prompt 仍然留在最前面，能保住一部分 prefix cache。

如果会话中发生 memory 更新，Reasonix 不会直接改 system prompt。`internal/control/input.go` 的 controller 会把更新作为 `<memory-update>` 注入下一条 user message，也就是放在请求尾部，而不是污染最前面的 system prefix。

## 3. 工具 schema：排序、规范化、缓存

工具列表是 OpenAI-compatible function calling 请求里很大的稳定前缀。很多 agent 的缓存命中率低，不是因为对话历史变了，而是工具 schema 每轮序列化出的字节不一样。

Reasonix 在几层做稳定化。

### 工具名稳定排序

`internal/tool/tool.go` 中：

- `Builtins()` 按工具名排序返回内置工具。
- `Registry.Schemas()` 再次按工具名排序导出给 provider。

这保证工具数组顺序不会受注册顺序、插件加载时机或 Go map 遍历顺序影响。

### JSON Schema 规范化

`Registry.Add()` 注册工具时会调用 `provider.CanonicalizeSchema()`，把 schema 规范化后缓存下来。`internal/provider/schema_canonicalize.go` 会递归处理 JSON Schema：

- 规范化 `properties`、`$defs`、`definitions` 等命名 schema。
- 对 `required`、`enum`、`type` 等数组做稳定排序。
- 递归处理嵌套对象和数组。

这样“语义一样但字段顺序不同”的 schema 最终会变成稳定 JSON。

### MCP / plugin schema 缓存

`internal/plugin/cache.go` 为 MCP 握手结果和工具 schema 做缓存，缓存 key 来自 `SpecFingerprint`。fingerprint 会稳定地哈希 command、args、env、headers 等会影响插件行为的字段，其中 map key 会排序。

这解决两个问题：

1. 减少冷启动时反复握手和拉取 schema 的成本。
2. 避免插件 schema 因枚举顺序、map 顺序、握手时机产生漂移。

### Token economy 降低工具前缀体积

`internal/boot/token_profile.go` 提供 token economy 模式：默认只暴露核心工具，把 skills、MCP、LSP、web_fetch、install_source、task 等可选工具藏在 `connect_tool_source` 后面，需要时再启用。

这不只是省 token。工具 schema 越大，稳定前缀越重，一旦工具面变化，cache miss 代价也越大。把可选工具延迟连接，可以同时降低首轮 prompt 成本和 prefix 漂移风险。

## 4. 对话日志：append-only 是高命中的核心

`internal/agent/session.go` 里 session 的主要行为很简单：

- `Add(message)` 只追加消息。
- `Replace(messages)` 只在压缩、裁剪等历史改写场景使用。
- `RewriteVersion()` 记录历史被改写的版本。

正常 run loop 下，下一轮请求几乎就是上一轮请求再追加新的 user / assistant / tool 消息。这正好符合 prefix cache 的理想形态：

```text
request N:
  stable prefix + messages[0:N]

request N+1:
  stable prefix + messages[0:N] + messages[N+1]
```

如果中途修改 messages[0:N] 的任何内容，缓存前缀就会断。Reasonix 把“改历史”限制在少数明确路径里，并通过 rewrite version 做诊断。

## 5. Reasoning 内容：不要把高成本 scratch 变成下一轮 prompt

DeepSeek thinking mode 有一个容易踩的点：模型输出的 reasoning 内容如果全部塞回下一轮 prompt，会增加 prompt 成本，也会把每轮高度变化的 scratch 写进缓存前缀，降低后续命中稳定性。

`internal/provider/openai/openai.go` 的处理比较克制：

- 对 DeepSeek 请求始终启用 `thinking: { type: "enabled" }`，并使用 `reasoning_effort` 控制深度。
- 普通 assistant turn 不把 reasoning 内容当作下一轮可见正文回放。
- 只有当 assistant turn 带有 tool calls 时，才回传 `reasoning_content`，因为 DeepSeek thinking mode 对工具调用回放有协议要求，丢掉会导致 400。

也就是说，Reasonix 把 reasoning 当作每轮运行时 scratch，而不是长期对话记忆。只有协议要求必须保留的工具调用 reasoning 才进入下一轮请求。

## 6. 运行时遥测：每轮都知道缓存有没有漂移

Reasonix 不是盲目相信缓存会命中。它显式读取 provider usage，并做 prefix 形状诊断。

### DeepSeek usage 归一化

`internal/provider/openai/openai.go` 的 `normaliseUsage()` 会读取两类生态格式：

- DeepSeek 顶层字段：`prompt_cache_hit_tokens`、`prompt_cache_miss_tokens`。
- OpenAI / MiMo 风格嵌套字段：`prompt_tokens_details.cached_tokens`。

归一化后进入统一的 `provider.Usage`：

```text
PromptTokens
CompletionTokens
CacheHitTokens
CacheMissTokens
ReasoningTokens
```

主 agent 会累计 session 级 cache hit / miss，从而得到会话级命中率，而不是只看单次请求。

### PrefixShape 诊断

`internal/agent/cache_shape.go` 会对影响缓存前缀的关键部分做 hash：

- system prompt hash
- tool schema hash
- combined prefix hash
- log rewrite version
- tool schema token estimate

每次请求前 capture shape，请求后和上一轮比较。如果 cache miss 变高，诊断能指出大致原因：

- `system`
- `tools`
- `log_rewrite`

这是一种很实用的工程手段。高缓存不是靠猜，而是把可能破坏 prefix 的维度变成运行时事件。

## 7. 压缩策略：宁可晚压缩，也不频繁重写前缀

上下文压缩天然会破坏 prefix cache，因为它会把旧历史替换成 summary。Reasonix 的策略不是“不压缩”，而是把压缩做成低频、可解释、尽量少损伤的操作。

`internal/agent/compact.go` 的默认阈值：

| 参数 | 默认值 | 意义 |
| --- | ---: | --- |
| soft ratio | 0.5 | 只提示上下文变大，不改写历史 |
| compact ratio | 0.8 | 到达该比例才自动压缩 |
| force ratio | 0.9 | 高水位强制压缩 |
| target ratio | 0.5 | 压缩后保留尾部不超过窗口一半 |
| recent tail | 16384 tokens | 最近上下文原样保留预算 |

最关键的是 soft threshold：到 50% 时只发 notice，不压缩。代码注释里明确写了原因：这个阶段压缩会不必要地击穿缓存。

### 先 prune，再 summarize

自动压缩前会先调用 `PruneStaleToolResults()`：

- 只处理 protected recent tail 之外的旧 tool result。
- 大工具输出会被替换成 marker。
- 原始内容先 archive 到 JSONL。
- 保留工具调用配对关系和必要 assistant content。

如果 prune 后 prompt 已经回到阈值以下，就跳过真正的 summary 压缩。这个顺序很重要：裁掉旧工具输出通常比让模型总结更便宜，也更少改变语义。

### 真正 compaction 时保留哪些东西

压缩时，Reasonix 不粗暴地把旧消息全部总结掉，而是分区处理：

- system prompt 保留。
- 首个小型 user turn 可以 pin 住。
- 小型 user turn 和 prior summary 尽量原样保留。
- 近期 tail 原样保留。
- assistant / tool 工作过程折叠进 summary。
- 被折叠的原始消息先 archive。
- summary 作为 user message 插入，并包在 `<compaction-summary>` 中。

这种策略的取舍很明确：用户事实和最近上下文尽量不改，机器工作过程可以折叠。压缩会让后续请求对新增 summary 段产生一次冷 miss，但不会把整个会话的认知都变成不可追溯的黑盒。

### 防止压缩循环

如果 context window 太小，压缩后仍然超过阈值，下一轮可能继续压缩，持续击穿缓存。Reasonix 有 `compactStuck` 保护：检测到连续压缩没有把 prompt 拉回安全区，就暂停自动压缩并提示用户调大 context window 或缩小工具输出。

## 8. Planner、subagent 和主上下文隔离

Reasonix 不是把所有认知都塞进一个主会话。

`internal/boot/boot.go` 中 executor session 和 planner session 是分开的：

- executor 用主系统提示词和主工具面。
- planner 用 `PlannerPromptWithContext(mem.Block())` 创建独立 session。
- 两个模型各自保持自己的 prefix cache 稳定。

subagent / task 也使用独立 session。子代理可以读大量文件、做探索、跑检查，但父 agent 只接收最终结果作为 tool result，而不是把子代理完整历史并入父上下文。

这对缓存很关键：探索型任务会产生大量高噪声消息。如果全部写进主上下文，主 session 的可缓存前缀会快速膨胀，并且更容易触发压缩。隔离 session 让主上下文只承担决策和最终事实，不承担所有过程日志。

## 9. 为什么这些机制能堆出高命中率

把上面的设计合起来，Reasonix 的缓存命中路径是这样的：

1. boot 阶段构造一次稳定 system prompt。
2. tool schema 按名称排序并规范化，插件 schema 缓存后复用。
3. 正常对话只 append，不重排、不编辑旧消息。
4. 动态状态放到 user turn 尾部，不改 system prompt。
5. reasoning 内容不作为普通历史回灌，避免 volatile scratch 污染前缀。
6. 大工具输出先 prune，历史压缩到高水位才发生。
7. planner / subagent 隔离，减少主上下文噪声。
8. 每轮读取 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`，并用 PrefixShape 解释漂移。

这会让第 N+1 次请求的开头大概率等于第 N 次请求的完整内容。长会话越长，上一轮已缓存前缀占本轮 prompt 的比例越大，会话级命中率自然向高位爬升。第一次请求、工具面变化、system prompt 变化、compaction 后的新 summary 段仍然会 miss，但这些 miss 被压缩到少数时刻。

## 10. 对当前项目可迁移的设计清单

如果要把 Reasonix 的高缓存上下文工程迁移到本项目，优先级建议如下。

| 优先级 | 设计 | 目的 |
| --- | --- | --- |
| P0 | 保持 `Run(ctx, input) (string, error)` 外部 API，但内部 session message append-only | 最大化跨轮 prefix 复用 |
| P0 | 工具 schema 按名称排序，JSON Schema canonicalize 后缓存 | 避免工具前缀随机漂移 |
| P0 | DeepSeek usage 解析 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` | 让缓存效果可观测 |
| P1 | system prompt 只在 session 创建时合成，memory / mode 变化进入 user tail | 避免中途击穿 system prefix |
| P1 | prefix shape 诊断：system hash、tool hash、rewrite version | cache miss 时能定位原因 |
| P1 | 对旧大 tool result 做 prune，再考虑 summary compaction | 少改历史，少花模型总结成本 |
| P2 | planner / subagent 独立 session，只把结论回传父上下文 | 控制主上下文噪声 |
| P2 | token economy 模式，按需启用可选工具源 | 降低工具 schema 前缀体积 |

本项目已有一些相近基础，例如原生 OpenAI-compatible function calling、runtime events、工具分类和子代理能力。真正缺口在于：还没有把“prefix 稳定性”作为一等工程目标来度量和保护。

## 11. 局限和风险

- 99.82% 是上游项目报告的真实 workload 数据，不是本文复测结果。
- DeepSeek prefix cache 是服务端行为，其他 provider 不一定有同样字段或同样缓存粒度。
- 首轮请求、session 重启、工具面变化、system prompt 变化、memory block 变化后，都会出现冷 miss。
- compaction 是必要的上下文维护，但它本质上会改写历史，因此应低频、可观测、可归档。
- MCP 和插件工具如果 schema 不稳定，会严重拖累缓存，必须做 canonicalization 和 fingerprint。
- 如果用户一次粘贴超大内容，它可能成为很重的前缀。Reasonix 的压缩策略会尽量保护小型 user turn，但大型粘贴内容仍需要单独的文件化或引用化策略。

## 12. 上游源码参考

| 主题 | 文件 |
| --- | --- |
| Go 重写版本说明 | `README.md` |
| 真实缓存基准 | legacy `benchmarks/real-world-cache/README.md` |
| DeepSeek 请求构造和 usage 归一化 | `internal/provider/openai/openai.go` |
| usage 数据结构 | `internal/provider/provider.go` |
| session append-only 和 rewrite version | `internal/agent/session.go` |
| 主 run loop 的 cache telemetry | `internal/agent/agent.go` |
| PrefixShape 诊断 | `internal/agent/cache_shape.go` |
| compaction 策略 | `internal/agent/compact.go` |
| 旧工具结果裁剪 | `internal/agent/prune.go` |
| memory 进入 system prefix | `internal/memory/memory.go` |
| 动态状态注入 user turn | `internal/control/input.go` |
| 工具 registry 和 schema 排序 | `internal/tool/tool.go` |
| JSON Schema canonicalization | `internal/provider/schema_canonicalize.go` |
| MCP / plugin schema 缓存 | `internal/plugin/cache.go`、`internal/plugin/canonicalize.go` |
| token economy | `internal/boot/token_profile.go` |
| planner / executor session 分离 | `internal/boot/boot.go`、`internal/agent/coordinator.go` |
| subagent session 隔离 | `internal/agent/task.go` |
| cache 回归测试 | `internal/agent/cachehit_e2e_test.go`、`internal/agent/cache_diagnostics_test.go` |
