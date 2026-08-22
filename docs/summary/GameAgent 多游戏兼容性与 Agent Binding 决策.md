# GameAgent 多游戏兼容性与 Agent Binding 决策

> **Status:** Accepted Architecture Decision
> **Date:** 2026-08-22
> **Applies To:** Phase5 前置架构语义；不要求立即修改 Protocol / Runtime 代码
> **Related:** Runtime Architecture v0.3, Context Architecture v0.2, Roadmap v0.4

---

# 1. 背景

Phase3 已经把 AgentSession identity 收敛为：

```text
AgentSessionKey
=
game_id
+
world_id
+
entity_id
```

Phase4 在这个 key 上实现了短期 Memory，并验证：

```text
同一 AgentSessionKey 的后续 Turn 可以读取最近 Memory；
不同 world / entity 的 Memory 不串线；
Memory 不绑定 EnvironmentSession / gRPC session_id。
```

这条 identity 对 Memory / State 仍然正确。

但在评估多游戏兼容性时发现一个隐含 Stardew 假设：

```text
Agent Definition = game_id + entity_id
```

对 Stardew 的固定 NPC 很自然：

```text
entity_id     = npc:Linus
definition_id = npc:Linus
```

但对 Minecraft、RimWorld、迷你世界或其他动态生成实体不成立：

```text
entity_id     = villager:uuid-123
definition_id = villager/farmer

entity_id     = pawn:723491
definition_id = human_pawn
```

因此必须在 Phase5 前把“具体游戏实体”和“可复用 Agent 定义”拆开。

---

# 2. 决策

GameAgent 长期模型正式区分：

```text
Game Entity
    ↓
Agent Binding
    ↓
Agent Instance
    ├── Agent Definition / Archetype
    ├── Agent Instance Descriptor
    ├── Agent State
    └── Agent Memory
```

## 2.1 Entity 不是 Agent Definition

`entity_id` 表示当前 world 中的稳定游戏实体身份。

它用于：

```text
AgentSessionKey
Agent State scope
Agent Memory scope
Trace / routing diagnostics
```

它不再被理解为 Agent Definition 的天然 key。

## 2.2 Agent Definition / Archetype

Agent Definition 表示可复用的行为模板、角色模板或 archetype。

Scope：

```text
game_id + definition_id
```

示例：

```text
stardew-valley + npc:Linus
minecraft + villager/farmer
rimworld + human_pawn
mini-world + merchant_npc
```

Stardew 只是一个特殊情况：`definition_id` 可以等于 `entity_id`。

## 2.3 Agent Instance Descriptor

Agent Instance Descriptor 表示当前 world 中这个具体实体的描述性事实。

Scope：

```text
game_id + world_id + entity_id
```

示例：

```text
display_name
definition_id
traits
profession
faction
relationship hints
adapter-provided metadata
```

Descriptor 是事实，不是完整 prompt。

Descriptor facts MAY come from：

```text
Adapter-provided data
static binding / config
current Observation-derived facts
```

当前 MVP 不要求新增独立 descriptor protocol message。只要 Context Source 能明确区分 `entity_id`、`definition_id` 和 instance facts，即可满足本 ADR。

## 2.4 Agent Binding

Agent Binding 负责回答：

```text
这个 Game Entity 是否应该成为 Agent？
如果是，它使用哪个 definition_id？
它有哪些 instance descriptor facts？
```

逻辑结果：

```text
AgentBinding {
    eligible: bool
    entity_id: string
    definition_id: string
    descriptor: AgentInstanceDescriptor
}
```

当前 MVP 可以先不创建独立 Runtime 类型，但后续设计不得继续假设：

```text
definition_id == entity_id
```

---

# 3. Adapter 与 Runtime 职责

## Adapter 提供事实

Adapter / Environment 可以按需提供：

```text
entity_id
definition_id
display_name
entity_type
traits / attributes / metadata
current state facts
available capabilities
```

Adapter 不应该成为 Agent framework，也不应该直接决定完整 system prompt。

并非所有 Descriptor facts 都必须由 Adapter 主动返回。Runtime 也 MAY 从静态 binding / config 或当前 Observation 中派生 Descriptor facts，但不得把这些事实误当作 Agent Definition 本身。

## Runtime 组合 Agent Context

Runtime / Context Sources 根据：

```text
definition_id
Agent Instance Descriptor
Agent State
Agent Memory
Current Observation
Current Event
Available Tools
```

组合本轮模型上下文。

换句话说：

```text
Adapter owns game facts.
Runtime owns cognition and context projection.
```

---

# 4. Resolve 策略

长期推荐：

```text
Lazy Resolve 为主，主动注册为可选优化。
```

也就是第一次真正需要运行某个 Agent 时：

```text
GameEvent.target_entity_id
    ↓
Agent Resolver / Binding
    ↓
Adapter-provided descriptor or local binding rule
    ↓
definition_id
    ↓
Agent Definition Source
    ↓
AgentSession
```

不把“所有 entity 必须启动时主动注册”作为 correctness 前提，因为动态游戏里会遇到：

```text
entity spawn / despawn
Runtime reconnect
大量临时 entity
遗漏注册
缓存失效
```

主动注册可以用于 cache warmup 或调试，但不能成为唯一语义。

---

# 5. 多游戏边界

## 5.1 Stable Entity Identity

Adapter MUST 为可 Agent 化实体提供 world 内稳定的 `entity_id`。

如果游戏内部 ID 只是临时 runtime handle，Adapter 必须建立稳定映射。

Memory / State 仍然绑定：

```text
game_id + world_id + entity_id
```

而不是：

```text
game_id + definition_id
```

因为 Memory 属于具体实体，不属于模板。

## 5.2 World Namespace

`world_id` 表示 Agent State / Memory / Entity Identity 的持久化命名空间。

它不等价于某个具体游戏术语。

示例：

```text
Stardew    -> save / farm
Minecraft  -> world / server world
RimWorld   -> colony save
Roguelike  -> run
MOBA / FPS -> match
```

换 `world_id` 默认表示另一套 Agent Instance State / Memory。

## 5.3 Observation Narrow Waist

Observation MUST 保持：

```text
small common envelope
+
game-specific structured state / extensions
```

Runtime Core 不应因为某个游戏需要就持续增加：

```text
season
weather
friendship
biome
hunger
mood
job
block
mana
```

这些应由 Adapter 放入 game-specific state / extensions，并由 Context projection 按需使用。

## 5.4 Dynamic Capability View

Available Tools 表示当前 AgentTurn 的 capability view。

它可能由以下因素共同决定：

```text
Environment capabilities
entity capabilities
current observation/state
runtime policy / permission
turn policy
```

不应理解为某个 entity type 的永久固定工具列表。

## 5.5 Trigger Admission

Trigger admission MUST NOT hardcode game-specific `event_type` in AgentLoop / Gateway core.

Stardew 当前的：

```text
player_interacted_with_npc
```

只是 Phase3 / Phase4 的最小 trigger，不是长期 Runtime contract。

第二个游戏接入前，Runtime 必须具备可配置或可扩展的 Trigger Admission / Trigger Router，使非 Stardew trigger 可以进入相同 AgentTurn 链路，只要它满足：

```text
world_id
target_entity_id
stable entity identity
Agent Binding / eligibility
```

具体 `event_type` 的业务语义应由 Adapter、game-specific config 或 Trigger Policy 解释。

---

# 6. Phase5 前置要求

进入 Phase5 有界 Multi-step AgentTurn 之前，必须把本 ADR 作为架构前置条件。

Phase5 不要求立即实现：

```text
AgentBinding runtime package
AgentDescriptor protocol message
definition_id proto field
Agent Definition storage
第二个真实 Adapter
```

但 Phase5 的技术方案和代码不得新增依赖以下假设：

```text
Agent Definition = game_id + entity_id
所有 entity 都应该 Agent 化
Observation 会被统一成万能游戏状态 schema
Tool 是 Environment / Entity 的永久固定列表
Adapter 决定完整 Agent prompt
AgentLoop / Gateway core 写死 game-specific event_type
```

Phase5 Entry Gate MUST include a minimal non-Stardew fixture / contract test.

该 fixture 不需要实现第二个真实 Adapter，也不需要新增 Protocol 字段。

它只需要证明一个非 Stardew 语义的 trigger 可以通过 Runtime core：

```text
game_id = test-survival
event_type = damage_received
entity_id = creature:uuid-1
observation.state = game-specific data
capability = test capability
```

并且不要求为了这个 fixture 修改：

```text
runtime/internal/agent
runtime/internal/context
runtime/internal/tool
runtime/internal/model
runtime/internal/session
```

如果 Phase5 或后续 FakeGame contract test 证明需要 protocol 字段，再单独设计 `v1alpha3` 或 additive extension。

---

# 7. 本次不修改

本 ADR 不修改：

```text
protocol/proto/gameagent.proto
AgentSessionKey
MemoryStore key
Phase4 Runtime code
Stardew Adapter behavior
```

Phase4 的 Memory scope 仍然成立：

```text
game_id + world_id + entity_id
```

变化的是 Agent Definition 的长期解释方式：

```text
旧：Agent Definition = game_id + entity_id
新：Agent Definition / Archetype = game_id + definition_id
```

---

# 8. Decision Summary

```text
Entity != Agent Definition

Entity -> Agent Binding

Agent Definition / Archetype = game_id + definition_id

Agent Instance Descriptor = game_id + world_id + entity_id

Agent State / Memory = game_id + world_id + entity_id

Trigger admission is not hardcoded to one game-specific event_type.

Available Tools = current AgentTurn dynamic capability view.

Adapter provides facts.

Runtime owns cognition and context projection.
```
