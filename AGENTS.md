# GameAgent Agent Instructions

本文件是 `world-is-agent` 仓库的项目级常驻指令。所有 agent 在本仓库工作时应遵守这些边界。

## 基本工作方式

- 默认使用中文沟通。
- 先读现有代码与阶段文档，再修改实现。
- 使用 `rg` / `rg --files` 查找文件和文本。
- 手工编辑文件使用 `apply_patch`。
- 不还原用户或其它 agent 已经做出的无关改动。
- 未经用户明确允许，不执行 `git push`。
- 只有用户明确要求时才提交 git；如需保存阶段性结果，最多执行本地 `git commit`。

## 范围与交付

- 只完成用户明确要求的内容，以及让这些内容正常成立所必需的配套工作。
- 范围模糊时按最小必要集合处理，并在聊天中说明可选扩展，不自行扩大产品或协议范围。
- 最终交付物只描述当前确认后的结果，不记录被放弃的方案、调试过程或修改痕迹。
- 文档和代码注释只解释当前系统真实存在的约束、规则、风险或兼容逻辑。

## 架构边界

GameAgent 的长期边界是：

```text
Agent owns intent.
Runtime owns cognition.
Protocol owns contracts.
Adapter owns translation.
Game owns execution.
```

Runtime Core 必须保持 game-agnostic：

- 不依赖 Stardew / SMAPI 类型。
- 不解析 game-specific `Observation.state` 字段。
- 不按 game-specific `event_type` 写 Trigger / Memory / Context 分支。
- 不按 game-specific capability name 写执行策略分支。
- 不接管路径规划、UI 展示、游戏主线程调度或具体动作实现。

Adapter 是游戏事实来源：

- 负责读取游戏 API。
- 负责构建 namespaced Observation。
- 负责 capability 的 schema、description、extensions、执行和失败原因。
- 负责 UI、pathfinding、主线程执行和游戏侧状态检查。

Protocol 是 Runtime 与 Adapter 的契约，不承载单个游戏的私有模型。

## Capability 与 Tool Policy

`Capability.description` 是 model-facing 自然语言说明，用于告诉模型工具用途、参数语义和游戏侧效果。

`Capability.extensions.gameagent.tool_policy` 是 Runtime-facing 结构化执行策略，用于表达通用调度约束。

Runtime 执行逻辑只能依赖结构化 metadata：

```text
Capability.execution_mode
Capability.concurrency_mode
Capability.extensions.gameagent.tool_policy
```

Runtime 不得从以下来源推断执行策略：

```text
capability name
Capability.description
game-specific event_type
game-specific Observation.state
```

同类能力在不同游戏中可以有不同名称。例如 Stardew 可以叫 `present_dialogue`，其它游戏可以叫 `ask_player` 或 `show_choices`。Runtime 应通过 `exclusive_per_step`、`settle_after_success` 等通用 policy 理解执行语义。

后续玩家输入或环境进展由新的 GameEvent 驱动这一事实，属于 capability description 与 event contract，不作为 Runtime-facing policy。

## Prompt 配置边界

- Runtime 默认 prompt 必须保持通用，不写死 Stardew capability name。
- 游戏特定 prompt profile 如有需要，应放在 Runtime 配置树，例如 `runtime/config/profiles/<game>.json`。
- 不把 Runtime prompt 配置放入 Adapter 目录。
- Prompt 只能引导模型选择工具，不能作为 Runtime 执行约束的唯一来源。
- 需要强制执行的规则必须进入结构化 metadata、Registry、Scheduler 或 Protocol。

## Context 与 Memory

- Observation 是 narrow waist，不是跨游戏统一状态 schema。
- `Observation.state.<game>` 可以承载 Adapter namespaced 当前事实，Runtime Core 不直接解析其游戏私有结构。
- `ContextFact` 表示 model-visible 的通用事件事实，kind 必须保持跨游戏语义。
- Runtime Memory projection 不得以 `player_said_to_npc` 等 game-specific event type 作为核心分支条件。
- Recent Memory 只表示 previous turns；Current Turn Transcript 只表示当前 Turn 内 earlier steps。
- 过滤或排序 Memory 时使用通用时间、sequence 和 AgentSession 边界，不依赖具体游戏字段。

## Protocol 变更原则

- 优先 additive 变更。
- 不为单个游戏字段扩展 Protocol。
- 不引入双来源字段；同一语义应有唯一权威来源。
- `definition_id` 的协议来源是 `EntityRef.definition_id`，不得重新放入 `Observation`。
- `ActionRequest.source_event_id` / `source_turn_id` 是 Runtime 写入的来源关联，不由模型或 Adapter 猜测。
- `TurnCompletion` 是 Runtime -> Adapter 的 Turn 终态信号，不替代 `ActionResult`。
- 尚未稳定为协议核心的 capability policy 优先使用 `Capability.extensions` 验证。

## Async Action 边界

- Action 不等于同步函数。
- Runtime 可以管理 action lifecycle、timeout、cancel、suspend、resume 和 trace。
- Adapter / Game 负责真实动作执行、可达性判断、路径规划和中断原因。
- Phase6 的 continuation 是 in-memory；Runtime 崩溃恢复、Adapter reconnect 恢复 pending action 和长期持久化属于 Phase7 之后。

## 阶段文档边界

- 已 Accepted 的阶段不因后续发现的架构前置问题而随意重开。
- 当前阶段需要的架构收敛应进入当前阶段的技术方案或 ADR。
- 技术方案必须写清 milestone、修改范围、验收命令和通过标准。
- Roadmap 只描述阶段目标，不替代具体技术开发方案。
- Architecture 文档只写长期边界，不承诺尚未实现的细节已经通过验收。

## 验证要求

- 修改 Protocol 时同步更新 static check、生成代码和相关 Runtime / Adapter 测试。
- 修改 Runtime Tool / Scheduler / Loop 时覆盖通用 fake capability，避免只用 Stardew 工具名证明行为。
- 修改 Adapter capability 时同步更新 `CapabilityCatalog` 测试和 static check。
- 文档修改至少运行 `git diff --check`。
- 代码完成前运行与改动范围匹配的测试；不能运行时在最终回复中说明原因。
