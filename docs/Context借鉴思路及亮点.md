可以把目前 GameAgent 的 Context 设计理解成：**分别从 Pi、Hermes、DeepSeek Harness 吸收了一部分关键思想，但最终形成的是一套不同于三者的、面向“持久游戏世界 + 多 Agent Instance”的 Context Architecture。**

最简洁的总结是：

> **Pi 教我们区分 History 和当前 Context；Hermes 教我们区分 Definition、Core Memory、Session History 和 Context Engine；DeepSeek Harness 教我们把事实日志、Projection 和 Storage Backend 解耦；GameAgent 在此之上增加了 WorldScope、AgentScope、Authority 和 Current Environment Ground Truth。**

你现在原始 Context 设计已经明确把静态 Game/Agent Definition 与 world-scoped Agent State/Memory 分开，并用 `game_id / world_id / entity_id` 规定不同数据的 Scope。

后续多游戏兼容性评估进一步把 `Agent Definition` 拆成：

```text
Agent Definition / Archetype = game_id + definition_id
Agent Instance Descriptor    = game_id + world_id + entity_id
```

这样 Stardew 的固定 NPC 仍可以 `definition_id == entity_id`，而动态生成 NPC / Pawn / Villager 可以共享可复用模板。

------

# 一、先看四套系统最根本的区别

| 维度           | Pi                              | Hermes                                 | DeepSeek Harness                  | GameAgent                                        |
| -------------- | ------------------------------- | -------------------------------------- | --------------------------------- | ------------------------------------------------ |
| 中心对象       | Coding Session                  | Personal Agent / Session               | Event-sourced Session             | **World + Agent Instance**                       |
| 静态定义       | AGENTS.md / SYSTEM.md           | SOUL / USER / AGENTS                   | System Prompt / Workspace         | **Game Definition / Agent Definition**           |
| 完整历史       | JSONL Session Tree              | SQLite Session History                 | `SessionEvent` Log                | **Experience / History**                         |
| 当前 Context   | Session branch + compaction     | Context Engine                         | Event Log Projection + Compaction | **Context Engine 从多 Scope Sources 构建**       |
| Memory         | 较弱，主要靠 history/compaction | Core Memory + external Memory Provider | 核心并不强绑定长期 Memory         | **Agent-scoped Memory**                          |
| 世界事实       | 文件/工具结果                   | 用户/项目环境                          | Workspace / events                | **Game/Adapter authoritative Environment State** |
| 多 World       | 非核心                          | 非核心                                 | 非核心                            | **一等概念**                                     |
| 多实体长期身份 | 非核心                          | 非核心                                 | Agent/session scope               | **`game + world + entity`**                      |
| Authority 冲突 | 简单                            | 相对简单                               | Event log 为事实源                | **显式 Authority/Freshness Policy**              |

所以三者实际上都是：

```text
session-centric
```

而我们逐渐变成：

```text
world-and-agent-centric
```

这是最核心的区别。

------

# 二、Pi：我们主要借了“History ≠ Model Context”

## Pi 本身怎么做

Pi 的中心非常简单：

```text
Session
   ↓
JSONL
```

Session 以 JSONL 保存，每条 entry 有 `id / parentId`，因此一个 session 文件本身可以形成分支树。([GitHub](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/session.md?utm_source=chatgpt.com))

稳定的项目 Context 则来自：

```text
AGENTS.md
CLAUDE.md
SYSTEM.md
APPEND_SYSTEM.md
```

这些内容在启动时加载。([GitHub](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/usage.md?utm_source=chatgpt.com))

当 Session 过长以后，Pi 不删除完整历史，而是：

```text
完整 Session History
       │
       ├── old messages
       │       ↓
       │    summary
       │
       └── recent messages
               ↓
        当前 Model Context
```

它会找到一个 cut point，把旧内容 summarization，然后让模型看到：

```text
system
+
summary
+
recent messages
```

完整 Session entry 仍然保留。([GitHub](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/compaction.md))

### Pi 最大的亮点

我认为就是一句话：

> **记录过什么，和这次模型应该看到什么，不是一回事。**

也就是：

```text
History
≠
Current Context
```

这是非常基础但非常重要的 Context Engineering 原则。

------

## GameAgent 从 Pi 借了什么

主要有三个。

### 1. Experience / History 与 Model Context 分离

我们后来给 GameAgent 增加：

```text
Experience / History
```

这一层，很大程度就是这个思想。

例如：

```text
玩家送 Abigail 紫水晶
Abigail speak(...)
ActionResult succeeded
```

这些可以作为真实 Experience 保存。

但不意味着以后 Abigail 每轮都必须看到全部：

```text
1000 条历史
```

而是：

```text
Experience
    ↓
Context Engine
    ↓
Recent / Relevant subset
```

------

### 2. Recent Context 与长期 History 分离

以后我们也可以自然拥有：

```text
Recent Experience
+
Retrieved Memory
```

而不是：

```text
全部历史 → LLM
```

这跟 Pi：

```text
summary + recent
```

是同一类原则。

------

### 3. Static context files 的简单主义

Game Definition / Agent Definition 未来完全可以像 Pi 的 AGENTS.md 一样：

```text
games/stardew-valley/game.yaml
agents/abigail.yaml
```

这些是人工可读、版本化、确定性的 Definition，不需要全部丢数据库或向量检索。

------

## 我们没有照抄 Pi 的部分

Pi 是典型：

```text
Session → Context
```

但 GameAgent 不能仅仅这样。

因为 Abigail 今天被唤醒时，最重要的信息可能不是：

```text
上一条聊天消息
```

而是：

```text
当前天气
当前地点
当前 relationship
当前游戏事件
昨天的经历
很久以前的重要 Memory
```

所以我们最终不是：

```text
Conversation History
→ Context
```

而是：

```text
Multiple Context Sources
→ Context Engine
→ Model Context
```

这是一个很大的差别。

------

# 三、Hermes：我们主要借了“不同类型知识必须分层”

Hermes 是三个参考项目里，在 **Memory/Context 分层** 上最值得 GameAgent 学的。

## Hermes 本身怎么做

它至少区分了：

```text
SOUL.md
USER.md
MEMORY.md
AGENTS.md / .hermes.md
Session History
```

其中：

- `SOUL.md`：Agent 是谁；
- `USER.md`：用户是谁；
- `MEMORY.md`：Agent 长期记住的重要事实；
- `AGENTS.md`：项目上下文和规则。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/which-file-does-what.md?utm_source=chatgpt.com))

Hermes 的 `MEMORY.md / USER.md` 刻意做得很小，MEMORY 大约限制 2200 chars，USER 约 1375 chars，并在 session 开始时作为 frozen snapshot 注入 system prompt。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/memory.md?utm_source=chatgpt.com))

与此同时，完整 Session 并不塞进 MEMORY.md，而是进入：

```text
SQLite state.db
+
FTS5
```

保存完整消息历史，需要的时候再 search。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/sessions.md?utm_source=chatgpt.com))

所以 Hermes 实际是：

```text
              Agent Context
                    │
       ┌────────────┼────────────┐
       ↓            ↓            ↓
     SOUL       Core Memory   Project Context

                    +
               Active Session

                    ↓

        Complete Session History
               SQLite / FTS
```

------

## Hermes 还有一个特别值得借的地方

它现在明确把：

```text
Memory Provider
```

和：

```text
Context Engine
```

分成两种 plugin。

Memory Provider 负责 persistent recall、prefetch、sync turn；

Context Engine 则负责 context selection / compression。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/developer-guide/memory-provider-plugin.md?utm_source=chatgpt.com))

也就是说：

> **记忆系统不是 Context Engine。**

反过来也一样：

> **Context Engine 不应该成为 Memory Database。**

这和我们当前的三层设计非常一致。

------

# 四、GameAgent 从 Hermes 借了什么

### 1. Definition 和 Memory 必须分离

我们现在的：

```text
Agent Definition
```

类似 Hermes：

```text
SOUL
```

回答：

> 我是谁？

例如：

```text
Abigail
喜欢冒险
喜欢紫水晶
性格活泼
```

而：

```text
Agent Memory
```

回答：

> 我经历过什么？

这正是 Hermes 给出的一个非常成熟的分层思想。

------

### 2. Always-on Context 和 Retrieved Context 分离

以后 GameAgent 很可能也是：

```text
Always-on:
    Agent Definition
    少量 Core Memory

On-demand:
    Episodic Memory
    World Experience
```

而不是所有信息都：

```text
vector_search()
```

Hermes 的 MEMORY.md 和 Session Search 正好说明了这个区别。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/memory.md?utm_source=chatgpt.com))

------

### 3. Memory 和 Context Engine 分离

我们现在：

```text
Context Sources
      ↓
Context Engine
      ↓
Model Context
```

其中 Memory 只是：

```text
Context Sources.Memory
```

这就比：

```text
Memory System = Prompt Builder
```

更干净。

这一点和 Hermes 当前的 Context Engine / Memory Provider 分离高度一致。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/plugins.md?utm_source=chatgpt.com))

------

### 4. Core Memory / Episodic Memory 的未来分层

我们现在把完整 Memory Architecture 预留为：

```text
Memory
├── Recent Memory
├── Episodic Memory
└── Core / Semantic Memory
```

这也明显吸收了 Hermes 这种：

```text
小而常驻
+
大而可搜索
```

的思想。

------

## 我们没有照抄 Hermes 的地方

Hermes 的中心身份更接近：

```text
Agent
+
User
+
Project
```

而我们必须解决：

```text
Abigail Definition
       │
       ├── World A Abigail Instance
       │
       └── World B Abigail Instance
```

所以不能简单：

```text
Abigail/MEMORY.md
```

因为 Memory 必须是：

```text
game_id
+
world_id
+
entity_id
```

scoped。

这也是我们那张 Context Scope Contract 比 Hermes 多出来的重要一层。

------

# 五、DeepSeek Harness：我们主要借的是“真源、Projection 和 Storage Seam”

如果从 Runtime Architecture 的角度，我认为 DeepSeek Harness 对我们影响应该最大。

## DeepSeek Harness 怎么做

它的核心 Session 不是：

```text
messages[]
```

而是：

```text
append-only SessionEvent log
```

而且官方明确把这份 log 定义为：

> Session 全部 interaction history 的 single source of truth。

LLM 看到的 `Message[]` 则由：

```text
SessionEvent
    ↓
deriveMessages()
    ↓
Message[]
```

投影出来，而不是单独再维护一份消息历史。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/session.md?utm_source=chatgpt.com))

这点特别重要。

------

## Persistence 也是独立 Seam

SessionEvent 是：

```text
Domain Model
```

至于保存成：

```text
JSONL
```

还是：

```text
SQLite
```

是另外一个 backend 问题。

DeepSeek Harness 已经同时提供 JSONL / SQLite 的 session persistence backend。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/persistence.zh.md?utm_source=chatgpt.com))

而非 Session 数据，又走独立 Storage subsystem：

```text
Storage Domain
       ↓
Storage Backend
       ├── JSON
       └── SQLite
```

Product/domain package 不直接接触具体 backend。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/storage.md?utm_source=chatgpt.com))

所以它真正贯彻了：

> **Data semantics ≠ Physical storage.**

------

## DeepSeek 的 Projection 也值得学

它现在甚至把：

```text
Session Projection
```

独立成 capability seam。

Domain 提供 pure projection unit，从 event log 折叠出：

```text
current derived state
```

给其他组件使用。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/session-projection.md?utm_source=chatgpt.com))

Compaction 也不是硬编码进 Agent Loop，而是 optional capability seam，并把 compaction 的结果本身记录成 session events。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/compaction.md?utm_source=chatgpt.com))

所以它非常强调：

```text
raw facts
    ↓
projection
    ↓
consumer view
```

------

# 六、GameAgent 从 DeepSeek 借了什么

### 1. Experience 应该比 Memory 更接近真源

我们后来给 Context Architecture 新增：

```text
Experience / History
```

其实就是在吸收这种思想。

例如：

```text
event
observation
tool call
action result
turn completed
```

是真实发生过的东西。

Memory：

```text
The player has repeatedly been kind to me.
```

只是从 Experience 中提炼出来的 representation。

所以：

```text
Experience
    >
Memory
```

在事实可靠性上类似：

```text
SessionEvent
    >
derived Message / compaction summary
```

------

### 2. Context 是 Projection，不是真源

我们现在三层：

```text
Context Sources
      ↓
Context Engine
      ↓
Model Context
```

本质也是：

```text
Canonical / semi-canonical sources
      ↓
projection
      ↓
model view
```

这个思想和 DeepSeek Harness 很接近。

------

### 3. Storage Architecture 与 Context Architecture 分离

例如：

```text
Agent Memory
```

是 domain concept。

至于以后：

```text
SQLite
Postgres
JSON
```

都不应该改变：

```text
AgentMemory
```

本身的语义。

同理：

```text
Experience
```

未来可以存：

```text
JSONL
```

也可以：

```text
SQLite
```

这跟 DeepSeek 的 SessionPersistence/Storage seam 思想是一致的。

------

### 4. Projection/derived state 思想

GameAgent 以后很可能有：

```text
Game Events / Observations
        ↓
World State Projection
```

这个 World State Projection 不是 Game 本身，而是：

> Runtime 最近已知、由 Game-derived facts 投影出来的 representation。

这个概念和 DeepSeek 的 log-derived projection 非常接近。

------

# 七、我们真正独有的第一大亮点：Context Scope Contract

这应该是目前 GameAgent Context Architecture 最明显的自有设计。

我们没有只说：

```text
memory
history
profile
```

而是先问：

> **这条 Context 属于谁？**

你最早那张表就是这个思想的核心。

现在可以进一步形成：

| Context Source          | Scope                            |
| ----------------------- | -------------------------------- |
| Runtime Policy          | Runtime                          |
| Game Definition         | `game_id + game_version`         |
| World Environment State | `game_id + world_id`             |
| Agent Definition / Archetype | `game_id + definition_id`        |
| Agent Instance Descriptor | `game_id + world_id + entity_id` |
| Agent Environment State | `game_id + world_id + entity_id` |
| Agent Cognitive State   | `game_id + world_id + entity_id` |
| World Experience        | `game_id + world_id`             |
| Agent Experience        | `game_id + world_id + entity_id` |
| Agent Memory            | `game_id + world_id + entity_id` |
| Event / Observation     | Current AgentTurn                |
| Tools                   | Current AgentTurn capability view |

这不是 Pi/Hermes/DeepSeek 的直接设计。

它来自游戏 Agent 的特殊要求。

------

# 八、第二个自有亮点：Definition 与 Instance 真正分离

我们一开始的：

```text
Agent Definition
=
game_id + entity_id
```

例如：

```text
Stardew + Abigail
```

回答：

> Abigail 是谁。

多游戏兼容性评估后，这个模型进一步收敛为：

```text
Agent Definition / Archetype
=
game_id + definition_id
```

回答：

> 这一类 Agent 该如何说话、思考和行动。

例如：

```text
Stardew + npc:Abigail
Minecraft + villager/farmer
RimWorld + human_pawn
```

而：

```text
Agent Instance Descriptor / State / Memory
=
game_id + world_id + entity_id
```

回答：

> Farm001 里的 Abigail / world_001 里的 villager:uuid-123 现在是谁。

于是：

```text
                Abigail Definition

               /                  \

Farm001 Abigail                     Farm002 Abigail

2 hearts                             8 hearts
memory A                             memory B
cognitive state A                    cognitive state B
experience A                         experience B
```

这种“同一角色 Definition 派生多个 World Instance Agent”的设计，是典型 game-native architecture。

你原始 Context 文档已经明确提出了“基础角色定义 + 世界实例级角色状态”，而不是每个存档复制完整 Profile。

------

# 九、第三个亮点：Environment State 和 Cognitive State 分离

这也是我认为特别重要的一步。

例如：

```text
Game Fact:
friendship = 2 hearts
```

属于：

```text
Agent Environment State
```

而：

```text
Abigail feels:
I increasingly trust the player.
```

属于：

```text
Agent Cognitive State
```

两者 Scope 相同：

```text
game + world + entity
```

但 authority 完全不同：

```text
Environment State
    authority = Game

Cognitive State
    authority = Runtime Agent
```

所以 Agent 可以：

```text
主观上觉得很信任玩家
```

但不能：

```text
主观把 friendship 从 2 改成 8
```

这使得 LLM 的“心理世界”和真实 Game State 可以同时存在而不互相污染。

这对角色 Agent 很重要，也是 Coding Harness 通常不需要显式建模的东西。

------

# 十、第四个亮点：我们显式建立 Authority Contract

Pi/Hermes 更多主要是在解决：

```text
什么要进上下文
```

GameAgent 还必须解决：

> **信息冲突时谁说了算？**

例如：

```text
Observation:
friendship = 2

Memory:
we seem very close

LLM inference:
maybe friendship is 8
```

我们可以明确：

```text
对于游戏事实：

Current Observation
        >
fresh Game-derived State Projection
        >
Experience
        >
Memory
        >
LLM inference
```

所以：

```text
friendship = 2
```

仍然是事实。

而：

```text
we seem very close
```

可以作为 Cognitive/Memory context 使用。

这就是：

```text
Scope
```

之外的第二个核心 Contract：

```text
Authority
```

我认为这是 GameAgent Context Architecture 很重要的特色。

------

# 十一、第五个亮点：Game 永远是 Environment Ground Truth

这其实是整个体系最关键的安全阀。

我们不会：

```text
LLM：
“现在 Community Center 完成了。”

↓

Runtime DB:
completed=true
```

正确链路永远是：

```text
LLM
↓
Action intention
↓
Adapter
↓
Game
↓
Game State changes
↓
Observation / Event
↓
Runtime learns new fact
```

也就是一句非常好的架构原则：

> **LLM 可以提出改变世界，但不能宣布世界已经改变。**

Game 才有 authority。

你原来的 Context 初步设计已经强调 Runtime 的持久 World Context 只是面向 Agent Context 的 representation，而不是第二套游戏 World State。

------

# 十二、第六个亮点：Context Engine 不是 PromptBuilder，而是 Projection Engine

最终我们现在的 Context Engine 是：

```text
Current AgentTurn
       ↓
Scope Resolution
       ↓
Source Loading
       ↓
Authority Resolution
       ↓
Freshness
       ↓
Retrieval
       ↓
Selection
       ↓
Budget
       ↓
Projection / Rendering
       ↓
Model Context
```

这比普通：

```go
prompt =
gamePrompt +
agentPrompt +
memory +
observation
```

要完整得多。

特别是：

```text
Scope Resolution
Authority
Freshness
```

这三步是游戏 Agent 场景真正多出来的东西。

------

# 十三、所以四者的“贡献关系”可以总结成这样

```text
                            GameAgent Context Architecture


Pi
│
├── Session History ≠ Model Context
├── JSONL history
├── Recent + Compaction
└── Static Context Files
             │
             │
             ▼

Hermes ─────────────────────────────────────────┐
│                                              │
├── Definition / Core Memory 分层               │
├── Small Always-on Memory                      │
├── Session DB / Searchable History             │
└── Memory Provider ≠ Context Engine            │
                                               │
                                               ▼
                                  Context Sources
                                         │
                                         ▼
                                  Context Engine
                                         │
                                         ▼
                                   Model Context
                                         ▲
                                              
DeepSeek Harness ───────────────────────────────┤
│                                              │
├── Append-only Event Source                    │
├── History → Projection                        │
├── Domain Semantics ≠ Storage Backend          │
├── JSONL / SQLite interchangeable persistence  │
└── Compaction / Projection as capability seams │
                                               │
                                               │
GameAgent 自有设计 ──────────────────────────────┘
│
├── GameScope / WorldScope / AgentScope
├── Context Scope Contract
├── Definition ≠ World-scoped Instance
├── Environment State ≠ Cognitive State
├── Experience ≠ Memory
├── Game-owned Ground Truth
├── Context Authority Contract
└── Scope → Authority → Freshness → Selection → Model Context
```

------

# 十四、如果各选一个最值得我们继承的思想

我会这样概括：

**Pi：**

> **History is not Context.**

完整历史应该保留，但模型只看当前值得看的 projection。([GitHub](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/compaction.md))

**Hermes：**

> **Not all memory belongs in the same tier.**

Identity、Project Context、Core Memory、Session History、Retrieved Memory 是不同东西。([GitHub](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/which-file-does-what.md?utm_source=chatgpt.com))

**DeepSeek Harness：**

> **Facts first, projections second, storage backend last.**

Event Log 是语义事实；LLM history 是 projection；JSONL/SQLite 只是 persistence implementation。([GitHub](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/core.md?utm_source=chatgpt.com))

**GameAgent 自己：**

> **Every Context Source has a Scope and an Authority.**

同时：

> **The Game owns reality; the Agent owns cognition.**

我认为后面这两句已经可以作为 GameAgent Context Architecture 最核心的设计原则。

------

## 最终评价

我们现在并不是：

> “把 Pi + Hermes + DeepSeek 拼起来。”

更准确的是：

```text
Pi
→ 教我们如何管理 History / Context Window

Hermes
→ 教我们如何管理不同层次的 Persistent Knowledge

DeepSeek Harness
→ 教我们如何管理 Facts / Projection / Persistence Boundary

GameAgent
→ 把这些原则迁移到
   Game → World → Agent Instance
   这个新的 Scope 模型中
```

所以真正属于 GameAgent 自己的核心竞争力不是“Memory 用 SQLite 还是 Vector”，而是：

```text
World-scoped Agent identity

+
Context Scope Contract

+
Environment / Cognitive State separation

+
Experience / Memory separation

+
Authority / Freshness policy

+
Context Engine projection
```

这些东西组合起来，才构成了一套真正 **game-native Context Architecture**，而不是给 Coding Agent 换一套 NPC Prompt。
