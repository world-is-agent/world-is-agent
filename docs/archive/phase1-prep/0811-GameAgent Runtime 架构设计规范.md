# GameAgent Runtime 架构设计规范

> Version: v0.1  
> Status: Architecture Baseline  
> Purpose: 本文档用于约束 AI Coding Agent 和开发者在 GameAgent 项目中的代码设计、目录组织、模块边界和依赖关系。  
> 本文档中的 `MUST / MUST NOT / SHOULD / MAY` 为规范性关键词。

---

# 1. 项目定义

GameAgent Runtime 是一个：

> **面向游戏 NPC 的、与具体游戏解耦的 LLM Agent Runtime。**

系统目标是让 NPC 具备：

```text
Observe
→ Retrieve Memory
→ Build Context
→ LLM Decision
→ Tool Call
→ Permission Check
→ Game Action
→ Action Result
→ Trace / Memory Update
```

GameAgent Runtime 不属于某一个具体游戏。

具体游戏必须通过：

```text
Game Adapter
```

接入 Runtime。

第一阶段真实游戏：

```text
Stardew Valley
```

对应：

```text
Stardew Adapter
```

技术：

```text
C# + SMAPI
```

Runtime：

```text
Go
```

Runtime 与 Stardew Adapter：

```text
Environment Protocol
+
gRPC
```

通信。

---

# 2. 最核心架构原则

系统必须遵循：

```text
LLM 决定 WHAT
游戏决定 HOW
```

例如：

```text
Agent:

move_to("lake")
```

Runtime 不负责：

```text
A*
地图寻路
碰撞
动画
NPC 每帧移动
```

这些属于：

```text
Game / Adapter
```

Runtime 只关心：

```text
Action Submitted
↓
Accepted
↓
Running
↓
Succeeded / Failed
```

---

# 3. 总体架构

系统固定划分为：

```text
┌────────────────────────────────────┐
│         GameAgent Runtime          │
│                 Go                 │
│                                    │
│ Agent                              │
│ Context                            │
│ Memory                             │
│ Goal                               │
│ Tool                               │
│ Permission                         │
│ Trace                              │
│ Evaluation                         │
│ LLM Provider                       │
└─────────────────┬──────────────────┘
                  │
                  │ Environment Protocol
                  │
                  │ gRPC / In-Process
                  │
┌─────────────────▼──────────────────┐
│            Game Adapter            │
│                                    │
│ State Reader                       │
│ Event Collector                    │
│ Capability Provider                │
│ Action Executor                    │
└─────────────────┬──────────────────┘
                  │
                  │ Game API / Mod API
                  │
┌─────────────────▼──────────────────┐
│              Game                  │
└────────────────────────────────────┘
```

Stardew：

```text
GameAgent Runtime
        ↕
Environment Protocol
        ↕
Stardew Adapter
C# + SMAPI
        ↕
Stardew Valley
```

未来：

```text
                     GameAgent Runtime
                            │
                  Environment Protocol
                            │
            ┌───────────────┼───────────────┐
            ↓               ↓               ↓
      Stardew Adapter   MiniWorld       Future Adapter
            ↓               ↓               ↓
      Stardew Valley   Simulator       Other Game
```

---

# 4. 顶层目录规范

项目采用 Monorepo。

目录 MUST 保持：

```text
gameagent/
│
├── runtime/
│
├── protocol/
│
├── adapters/
│   ├── stardew/
│   └── miniworld/
│
├── scenarios/
│
├── console/
│
├── docs/
│
├── deploy/
│
├── scripts/
│
├── Makefile
│
├── docker-compose.yml
│
└── README.md
```

各目录职责：

```text
runtime/
    通用 Agent Runtime

protocol/
    Runtime 与 Game Adapter 之间的公共协议

adapters/
    不同游戏的领域适配

scenarios/
    Agent Evaluation 场景

console/
    Agent Trace / Debug Web UI

docs/
    产品、架构、协议与 Adapter 文档

deploy/
    部署文件

scripts/
    开发辅助脚本
```

---

# 5. 依赖方向

这是项目最重要的架构规则。

合法依赖：

```text
             protocol
             ↑      ↑
            /        \
       runtime      adapter
                      ↓
                    game
```

即：

```text
runtime
    → protocol

adapter
    → protocol

adapter
    → concrete game APIs
```

Runtime MUST NOT：

```text
runtime
    → stardew
```

Runtime 中 MUST NOT 出现：

```text
SMAPI
Game1
NPC
Farmer
Abigail
PelicanTown
StardewValley
Minecraft
Unity
Godot
```

等具体游戏概念。

## 5.1 Architecture Enforcement

上述依赖方向 MUST 通过自动检查执行，而不只依赖人工自觉。

项目 SHOULD 提供脚本：

```text
scripts/check-architecture.ps1
```

并在 CI 中执行。

检查项 MUST 至少包含：

```text
runtime/ 中不得出现 Stardew / SMAPI / Game1 / Farmer / Abigail / PelicanTown / StardewValley 等具体游戏符号

runtime/ 不得 import / reference adapters/

runtime/ 只能依赖 protocol/ 和 runtime/ 内部包

adapters/stardew/ 不得依赖 runtime/internal/

protocol/gen/ 为生成代码，不得手工修改
```

如果检查失败：

```text
MUST block merge
```

这些检查是架构规范的一部分，而不是可选 lint。

---

# 6. Runtime 目录规范

建议：

```text
runtime/
│
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   │
│   ├── agent/
│   │   ├── agent.go
│   │   ├── runtime.go
│   │   ├── loop.go
│   │   └── trigger.go
│   │
│   ├── prompt/
│   │   ├── builder.go
│   │   └── prompt.go
│   │
│   ├── memory/
│   │   ├── memory.go
│   │   ├── store.go
│   │   └── retriever.go
│   │
│   ├── goal/
│   │   ├── goal.go
│   │   └── scheduler.go
│   │
│   ├── tool/
│   │   ├── tool.go
│   │   ├── registry.go
│   │   ├── executor.go
│   │   └── call.go
│   │
│   ├── permission/
│   │   ├── policy.go
│   │   └── validator.go
│   │
│   ├── environment/
│   │   ├── environment.go
│   │   ├── session.go
│   │   └── transport.go
│   │
│   ├── llm/
│   │   ├── provider.go
│   │   ├── request.go
│   │   └── response.go
│   │
│   ├── trace/
│   │   ├── trace.go
│   │   └── recorder.go
│   │
│   └── eval/
│       ├── evaluator.go
│       └── metrics.go
│
├── migrations/
│
├── go.mod
└── go.sum
```

该目录是长期目标结构。

MVP 不要求所有模块立即实现。

---

# 7. Runtime 模块职责

## 7.1 agent

负责：

```text
Agent 生命周期
Agent Step
GameEvent 触发
各 Runtime 模块编排
```

核心入口 SHOULD 为：

```go
HandleEvent(ctx, event)
```

禁止使用：

```go
StartAgent()
```

表达每次事件。

Agent 是长期存在的逻辑实体。

GameEvent 只是：

```text
wake agent
```

而不是创建一个新 Agent。

---

## 7.2 environment

定义 Runtime 视角下的游戏环境。

核心接口：

```go
type Environment interface {
    Observe(
        ctx context.Context,
        agentID AgentID,
    ) (Observation, error)

    Capabilities(
        ctx context.Context,
        agentID AgentID,
    ) ([]Capability, error)

    SubmitAction(
        ctx context.Context,
        action Action,
    ) (ActionID, error)

    GetActionStatus(
        ctx context.Context,
        actionID ActionID,
    ) (ActionStatus, error)
}
```

后续 MAY 增加：

```go
CancelAction()
```

Runtime MUST 依赖：

```text
Environment
```

而不是：

```text
StardewClient
```

---

# 8. Environment Protocol

Environment Protocol 是：

> Runtime 与所有 Game Adapter 的公共契约。

Protocol MUST NOT 属于 Stardew Adapter。

目录：

```text
protocol/
│
├── proto/
│   └── gameagent.proto
│
├── gen/
│   ├── go/
│   └── csharp/
│
└── README.md
```

初期优先使用一个：

```text
gameagent.proto
```

不要过早拆多个 proto 文件。

## 8.1 Protocol Versioning

Protocol 是跨 Runtime 与 Adapter 的稳定契约。

v0.1 MUST 遵守：

```text
新增字段优先使用 optional / extensions

Breaking Change 必须提升 protocol version

Stardew 专有信息必须进入 extensions

新增通用字段前必须说明它的跨游戏意义

未经至少一个真实 Adapter 或 MiniWorld 验证的字段 SHOULD 标记 experimental
```

Protocol 变更必须同时说明：

```text
影响哪些 message

是否需要重新生成 Go / C# 代码

旧 Adapter 是否还能运行
```

---

# 9. Protocol 核心领域对象

v0.1 MUST 至少包含：

```text
GameEvent

Observation

Capability

ActionRequest

ActionResult
```

后续 MAY 包含：

```text
Heartbeat

ActionStatusUpdate

EnvironmentMetadata

GameSession
```

---

# 10. GameEvent

GameEvent 表示：

> 游戏世界中发生了一件可能需要 Runtime 响应的事情。

例如：

```text
player_interact_npc

player_give_item

player_enter_location

day_started

game_time_changed

action_completed
```

通用结构：

```protobuf
message GameEvent {
    string event_id = 1;

    string game_id = 2;

    string save_id = 3;

    string agent_id = 4;

    string type = 5;

    string payload_json = 6;
}
```

Runtime SHOULD 通过：

```text
TriggerPolicy
```

决定事件是否需要触发 Agent Step。

不是所有 Event 都调用 LLM。

---

# 11. Observation

Observation 表示：

> Agent 当前被允许看到的世界。

Observation 不等于完整 Game State。

原则：

```text
Game State
    ↓
Adapter
    ↓
filtered / normalized
    ↓
Observation
```

Runtime MUST NOT 默认访问完整游戏世界。

初期可以包含：

```text
agent_id
game_time
location
nearby_entities
world_context
extensions
```

Stardew 特有字段 SHOULD 放入：

```text
extensions
```

而不是污染通用协议。

例如禁止：

```protobuf
string season = ...
int friendship_hearts = ...
string festival = ...
```

直接成为所有游戏必填字段。

---

# 12. Capability

Capability 表示：

> 当前 Environment 允许 Agent 执行的动作类型。

例如：

```text
speak

move_to

face_to

inspect_nearby

give_item
```

Environment 必须显式声明 Capability。

Agent MUST NOT 自己假设某个 Tool 存在。

流程：

```text
Environment
      ↓
Capabilities
      ↓
Tool Registry
      ↓
LLM Context
```

---

# 13. Tool 与 Capability 的关系

Runtime 中存在：

```text
Tool
```

Game 中存在：

```text
Capability
```

Game Capability 可以映射成 Environment Tool。

例如：

```text
Stardew Capability
"speak"
      ↓
Runtime Environment Tool
"speak"
```

但 Runtime 还可以存在内部 Tool，例如：

```text
remember

create_goal

schedule_goal
```

这些不会进入 Game Adapter。

因此 Tool 分两类：

```text
Tool
├── Runtime Tool
│
└── Environment Tool
```

---

# 14. Runtime Tool

Runtime Tool 修改 Agent Runtime 自身。

例如：

```text
remember()

create_goal()

schedule_goal()

cancel_goal()
```

调用链：

```text
LLM
 ↓
Runtime Tool
 ↓
Runtime State
```

不会调用 Adapter。

---

# 15. Environment Tool

Environment Tool 最终影响真实游戏。

例如：

```text
speak()

move_to()

give_item()
```

调用链：

```text
LLM
 ↓
Tool Call
 ↓
Tool Registry
 ↓
Permission
 ↓
Environment
 ↓
Adapter
 ↓
Game
```

---

# 16. Tool 接口

Runtime SHOULD 使用类似：

```go
type Tool interface {
    Name() string

    Description() string

    Schema() JSONSchema

    Execute(
        ctx context.Context,
        call ToolCall,
    ) (ToolResult, error)
}
```

Tool MUST NOT：

```text
自己调用 LLM
```

Tool SHOULD 只负责：

```text
validate
execute
return result
```

---

# 17. LLM 不允许直接调用 Adapter

禁止：

```text
LLM
 ↓
Stardew Adapter
```

必须：

```text
LLM
 ↓
Tool Call
 ↓
Tool Registry
 ↓
Permission
 ↓
Environment
 ↓
Adapter
```

即使 MVP 只有：

```text
speak
```

也 MUST 遵守此链路。

禁止为了 Demo 写：

```go
text := llm.Generate(...)
env.Speak(text)
```

应该是：

```text
LLM Response
↓
ToolCall(speak)
↓
Tool Executor
```

---

# 18. Permission

所有 Environment Tool MUST 经过 Permission。

链路：

```text
Tool Call
   ↓
Schema Validation
   ↓
Capability Validation
   ↓
Permission
   ↓
Environment Action
```

第一版 Permission 可以简单。

例如：

```text
Abigail

allowed:
    speak
```

后续：

```text
move_to
give_item
create_quest
```

再增加约束。

---

# 19. Action

Action 表示：

> Runtime 请求游戏执行一个行为。

例如：

```json
{
  "agent_id": "abigail",
  "capability": "speak",
  "arguments": {
    "text": "..."
  }
}
```

Action MUST 拥有唯一：

```text
action_id
```

---

# 20. Action Lifecycle

Runtime 架构必须允许异步 Action。

标准状态：

```text
PENDING

ACCEPTED

RUNNING

SUCCEEDED

FAILED

INTERRUPTED

CANCELLED
```

即使第一版：

```text
speak
```

可以近似同步执行，也不能把整个 Action 模型设计成：

```text
Execute() -> bool
```

因为未来：

```text
move_to
```

是典型异步行为。

v0.1 MUST 提供 Action 状态反馈机制。

允许两种实现方式：

```text
GetActionStatus(action_id)
```

或：

```text
ActionResult / ActionStatusUpdate stream
```

但 Runtime 的领域模型中 MUST 能表达：

```text
submitted action
current status
final result
failure reason
```

---

# 21. Memory

Memory MUST 属于 Runtime。

Adapter MUST NOT 实现 Agent Memory。

Adapter 只发送：

```text
GameEvent
Observation
ActionResult
```

Runtime 根据这些内容决定：

```text
是否写 Memory
```

Memory 初期分：

```text
Working Memory

Episodic Memory
```

第一阶段 MAY 使用：

```text
InMemory Store
```

后续升级：

```text
PostgreSQL
pgvector
```

不要让数据库成为第一个 Vertical Slice 的阻塞项。

---

# 22. Goal

Goal 属于 Runtime。

例如：

```text
meet_player_at_lake

talk_to_player

give_item_to_player
```

Goal 与 Action 必须区分：

```text
Goal
=
希望达到的状态

Action
=
当前执行的一个游戏行为
```

例如：

```text
Goal:
meet_player

Action:
move_to(lake)
```

---

# 23. Scheduled Goal

Scheduled Goal 表示：

> 未来某个 Game Time 重新唤醒 Agent 对某目标进行判断。

例如：

```text
Tomorrow 14:50

Goal:
meet_player_at_lake
```

到期流程：

```text
Scheduled Goal
      ↓
Wake Agent
      ↓
Observe
      ↓
Memory Retrieval
      ↓
LLM
      ↓
Action
```

Scheduled Goal MUST NOT 默认等价于：

```text
未来直接执行 Action
```

---

# 24. Scheduled Action

后续 MAY 支持：

```text
Scheduled Action
```

表示：

> 一个已经确定的未来游戏行为。

例如：

```text
每天 22:00 回家
```

可以下沉给游戏原生 Schedule。

区别：

```text
Scheduled Goal
    Runtime 管
    到期重新 Think

Scheduled Action
    Game / Adapter 管
    到期直接 Execute
```

MVP 优先实现：

```text
Scheduled Goal
```

而不是 Scheduled Action。

---

# 25. Trigger Policy

Agent MUST 使用 Event Driven 模型。

禁止：

```text
每帧调用 LLM
```

禁止：

```text
固定高频轮询全部 NPC → LLM
```

GameEvent 进入 Runtime 后：

```text
GameEvent
 ↓
TriggerPolicy
 ↓
ShouldThink?
```

只有满足条件才进入：

```text
Agent Step
```

---

# 26. Agent Step 标准流程

Runtime 中所有真正的 Agent 决策 SHOULD 遵循：

```text
Game Event / Scheduled Goal
           ↓
      Trigger Policy
           ↓
        Observe
           ↓
    Memory Retrieval
           ↓
      Current Goal
           ↓
    Context Builder
           ↓
       LLM Decision
           ↓
        Tool Call
           ↓
      Tool Registry
           ↓
   Permission Check
           ↓
     Tool Execution
           ↓
     Action / Result
           ↓
        Trace
           ↓
   Memory / Goal Update
```

这是核心业务链。

不得在 Adapter 内实现其中任何 Agent Decision 逻辑。

---

# 27. LLM Provider

Runtime MUST 抽象模型 Provider。

例如：

```go
type LLMProvider interface {
    Generate(
        ctx context.Context,
        request Request,
    ) (Response, error)
}
```

Agent Core MUST NOT 直接依赖：

```text
OpenAI SDK
Anthropic SDK
DeepSeek SDK
```

具体 Provider 放：

```text
llm/
```

MVP 只实现一个 Provider 即可。

---

# 28. Prompt Builder

Prompt Builder 负责组合 LLM 输入上下文：

```text
Agent Profile

Observation

Retrieved Memories

Current Goal

Available Tools

Recent Action Results
```

Prompt Builder MUST NOT：

```text
执行 Tool

读取 Stardew 对象

直接操作数据库之外的 Game State
```

代码目录 MUST 使用：

```text
runtime/internal/prompt/
```

不要使用：

```text
runtime/internal/context/
```

避免与 Go 标准库 `context` 混淆。

---

# 29. Trace

每次 Agent Step SHOULD 创建：

```text
trace_id
```

Trace 至少记录：

```text
event

observation

retrieved memory

goal

LLM request metadata

LLM result

tool call

permission result

action

action result

latency

token usage
```

第一版可以：

```text
structured log
```

后续升级数据库和 Console。

---

# 30. Evaluation

Eval MUST 使用真实 Runtime。

禁止单独实现：

```text
TestAgentRuntime
```

Eval Environment SHOULD 使用：

```text
MiniWorld Adapter
```

结构：

```text
Scenario
   ↓
MiniWorld
   ↓
Environment Protocol
   ↓
Real GameAgent Runtime
```

这样测试：

```text
Tool Selection

Memory Recall

Permission

Task Completion
```

---

# 31. Stardew Adapter 定位

Stardew Adapter 是：

> Stardew Valley 与 Environment Protocol 之间的 Game Driver。

技术：

```text
C#

SMAPI
```

它负责：

```text
State Reading

Event Collection

Capability Declaration

Action Execution

Transport
```

它 MUST NOT 负责：

```text
LLM

Prompt

Memory Retrieval

Goal Planning

Agent Decision

Evaluation
```

---

# 32. Stardew Adapter 目录

必须遵守：

```text
adapters/
└── stardew/
    │
    ├── src/
    │   │
    │   ├── ModEntry.cs
    │   ├── StardewAdapter.cs
    │   │
    │   ├── State/
    │   │   ├── ObservationBuilder.cs
    │   │   ├── NpcStateReader.cs
    │   │   └── WorldStateReader.cs
    │   │
    │   ├── Events/
    │   │   ├── EventCollector.cs
    │   │   └── PlayerInteractHandler.cs
    │   │
    │   ├── Capabilities/
    │   │   ├── CapabilityRegistry.cs
    │   │   └── SpeakCapability.cs
    │   │
    │   ├── Actions/
    │   │   └── ActionExecutor.cs
    │   │
    │   └── Transport/
    │       └── GrpcTransport.cs
    │
    ├── manifest.json
    │
    └── GameAgent.Stardew.csproj
```

---

# 33. Stardew Adapter 内部职责

## State

只负责：

```text
读取 Stardew
↓
转成 Observation
```

不得修改游戏状态。

---

## Events

只负责：

```text
监听 SMAPI / Stardew Event
↓
转成 GameEvent
```

不得执行 Agent Decision。

---

## Capabilities

描述当前游戏支持：

```text
speak
move_to
...
```

以及必要的输入 Schema。

---

## Actions

负责：

```text
ActionRequest
↓
调用 Stardew / SMAPI
↓
ActionResult
```

不得调用 LLM。

---

## Transport

只负责：

```text
Adapter
↔
Runtime
```

的网络通信。

不得承载业务决策。

---

# 34. Stardew API 使用优先级

Adapter 实现游戏行为时，应遵守：

```text
1. SMAPI API

2. Stardew Public API

3. SMAPI Reflection

4. Harmony Patch
```

Harmony SHOULD 作为最后手段。

不要默认 Patch 游戏逻辑。

---

# 35. MiniWorld Adapter

MiniWorld MUST 遵守和 Stardew Adapter 相同的 Environment Protocol。

禁止：

```text
Runtime 特殊判断：
if env == miniworld
```

MiniWorld 是正常 Environment。

用途：

```text
Unit Test

Integration Test

Scenario Eval

CI
```

---

# 36. Transport 与 Protocol 分离

必须区分：

```text
Environment Protocol
```

和：

```text
gRPC
```

Environment Protocol 是：

```text
领域契约
```

gRPC 是：

```text
Transport
```

未来允许：

```text
Environment Protocol
├── gRPC transport
├── in-process transport
└── future transport
```

MiniWorld SHOULD 可以：

```text
in-process
```

运行，不依赖真实 gRPC Server。

---

# 37. Runtime 部署

Runtime MUST 设计成独立进程。

MVP：

```text
Stardew
 ↓
SMAPI Mod
 ↓
localhost gRPC
 ↓
Local Go Runtime
 ↓
Cloud LLM API
```

未来 MAY 支持：

```text
Remote Runtime
```

Adapter 不应该因为 Runtime 是：

```text
localhost
```

还是：

```text
remote
```

改变领域逻辑。

---

# 38. 第一阶段 Capability Mapping

在正式扩展 Protocol 前，必须结合：

```text
SMAPI

StarDojo

Stardew Valley Game API

ILSpy
```

整理三份能力：

```text
Observation

Event

Capability
```

例如：

```text
Observation
────────────
time
weather
location
nearby entities
friendship

Event
────────────
player interact
gift received
day started
time changed
action completed

Capability
────────────
speak
face_to
move_to
give_item
```

未经真实 Stardew Capability Spike 验证的能力：

```text
不得作为 Runtime 核心假设。
```

---

# 39. 第一条 Vertical Slice

项目第一条完整业务链固定为：

> 玩家与 Abigail 交互 → Runtime 生成动态对话 → 游戏显示。

链路：

```text
Player clicks Abigail
        ↓
Stardew Event
        ↓
PlayerInteractHandler
        ↓
GameEvent
        ↓
gRPC
        ↓
Runtime.HandleEvent
        ↓
TriggerPolicy
        ↓
Environment.Observe
        ↓
ObservationBuilder
        ↓
Observation
        ↓
Context Builder
        ↓
LLM Provider
        ↓
ToolCall:
speak
        ↓
Tool Registry
        ↓
Permission
        ↓
Environment.SubmitAction
        ↓
gRPC
        ↓
SpeakCapability
        ↓
Stardew Dialogue
        ↓
ActionResult
        ↓
Trace
```

---

# 40. 第一阶段实现范围

MVP 0 MUST：

```text
SMAPI Mod 加载

Adapter ↔ Runtime 通信

player_interact Event

Observation

LLM Provider

speak Tool

Permission basic implementation

ActionResult

Trace
```

暂时 MAY 不实现：

```text
PostgreSQL

Vector Database

Advanced Memory

Goal Scheduler

move_to

Eval Dashboard

Multi-Agent
```

---

# 41. 第二阶段

加入 Memory：

```text
Game Event
↓
Memory Write
↓
Future Event
↓
Memory Retrieval
↓
Context
↓
Response
```

目标 Demo：

```text
Day 1
Player gives Abigail an amethyst.

Day 3
Player talks to Abigail.

Abigail recalls the event.
```

---

# 42. 第三阶段

加入：

```text
move_to
```

并验证：

```text
ACCEPTED
↓
RUNNING
↓
SUCCEEDED / FAILED
```

这一步用于验证：

```text
Asynchronous Action Lifecycle
```

---

# 43. 第四阶段

加入：

```text
Scheduled Goal

Game Time Scheduler
```

支持：

```text
未来时间
↓
Wake Agent
↓
Observe
↓
重新 Decision
```

---

# 44. 第五阶段

加入：

```text
Persistent Memory

Advanced Permission

Trace Storage

Scenario Evaluation
```

---

# 45. 禁止事项

AI Coding Agent MUST NOT：

### 禁止 1

在 Runtime 中：

```text
import Stardew
```

---

### 禁止 2

在 Adapter 中：

```text
调用 LLM
```

---

### 禁止 3

让 Adapter：

```text
Retrieve Memory
```

---

### 禁止 4

LLM 直接调用 Game API。

---

### 禁止 5

为了第一个 Demo 绕过：

```text
Tool Registry
Permission
Environment
```

---

### 禁止 6

将 Stardew 专有概念作为所有 Environment 的强制字段。

---

### 禁止 7

第一阶段创建：

```text
AbstractToolFactory
PluginManager
CQRS
Event Sourcing
Workflow Engine
复杂 DDD
```

等尚无真实需求的抽象。

---

### 禁止 8

每帧调用 LLM。

---

### 禁止 9

将真实世界时间用于：

```text
NPC Scheduled Goal
```

必须基于：

```text
Game Time
```

---

### 禁止 10

假设：

```text
Action = synchronous function
```

Action 模型必须允许未来异步执行。

---

# 46. 编码原则

## YAGNI

没有至少两个真实使用场景的抽象：

```text
SHOULD NOT
```

提前实现。

---

## Interface First, Not Framework First

需要提前定义：

```text
Environment

LLMProvider

MemoryStore

Tool
```

但不要提前定义大量无真实调用方的 interface。

---

## Vertical Slice First

每次开发优先跑通：

```text
真实 Event
↓
Runtime
↓
真实 Tool
↓
真实 Game
```

而不是按模块分别开发半年。

---

## Game-Agnostic Core

如果一段逻辑只对 Stardew 有意义：

```text
必须进入 Stardew Adapter。
```

如果一段逻辑对所有 Agent 有意义：

```text
应该进入 Runtime。
```

---

# 47. 判断代码应该放哪里的规则

AI Coding Agent 遇到新功能时必须依次判断：

### 问题 1

这个逻辑是否依赖具体游戏内部对象？

如果：

```text
YES
```

放：

```text
Adapter
```

例如：

```text
NPC.currentLocation
Game1.timeOfDay
SMAPI Event
```

---

### 问题 2

这个逻辑是否属于 Agent 思考/状态？

如果：

```text
YES
```

放：

```text
Runtime
```

例如：

```text
Memory
Goal
Prompt
Tool Decision
Permission
```

---

### 问题 3

这个逻辑是否定义 Runtime 与 Game 如何交流？

如果：

```text
YES
```

放：

```text
Protocol
```

例如：

```text
Observation
ActionResult
Capability
```

---

# 48. 一个重要例子

需求：

> 获取 Abigail 当前地点。

实现位置：

```text
Stardew Adapter / State
```

因为：

```text
Abigail
currentLocation
```

属于 Stardew。

Adapter 转成：

```text
Observation.location
```

Runtime 只看到：

```text
location
```

---

需求：

> Abigail 根据过去记忆决定是否提起紫水晶。

实现位置：

```text
Runtime
```

因为：

```text
Memory Retrieval
Context Builder
LLM Decision
```

是 Agent 能力。

---

需求：

> 让 Abigail 说话。

Runtime：

```text
ToolCall(speak)
```

Protocol：

```text
ActionRequest
```

Adapter：

```text
将 speak 转成 Stardew Dialogue
```

三个层次都有参与，但职责完全不同。

---

# 49. 新游戏 Adapter 接入标准

未来新增：

```text
Minecraft Adapter
```

原则上 MUST NOT 修改：

```text
Agent Loop
Memory
Goal
Permission
Trace
Eval
```

只需要实现：

```text
Observation Translation

Event Translation

Capability Declaration

Action Execution

Transport
```

如果新增一个游戏必须大量修改 Runtime：

> 应重新检查 Environment Protocol 或 Runtime 是否泄露了具体游戏语义。

---

# 50. 项目成功标准

GameAgent Runtime 的核心成功标准不是：

```text
能调用 LLM
```

而是：

> 新增一个游戏时，Agent 的 Memory、Goal、Tool、Permission、Trace、Eval 能够复用，仅通过新增 Adapter 即可让 NPC 进入相同 Agent Loop。

第一阶段使用 Stardew Valley 验证这一设计。

---

# 51. AI Coding Agent 工作规则

任何 AI 在修改本项目代码前 MUST：

1. 阅读本文档。
2. 判断需求属于 Runtime / Protocol / Adapter 哪一层。
3. 检查是否违反依赖方向。
4. 优先修改最少模块。
5. 不为了当前功能破坏 Game-Agnostic Runtime。
6. 不提前实现 Roadmap 功能。
7. 新增 Protocol 字段前说明为什么它具有跨游戏通用意义。
8. 新增 Adapter Capability 前确认 Stardew 实际可行。
9. 新增 Tool 时明确它属于 Runtime Tool 还是 Environment Tool。
10. 对异步游戏行为使用 Action Lifecycle，而不是同步 bool。
11. 新增 Agent 行为时保留 Trace。
12. 优先完成可以运行的 Vertical Slice。

## 51.1 Definition of Done

每个开发任务完成前 MUST 检查：

```text
Architecture Classification 已写明

没有 runtime → adapter 依赖

没有具体游戏概念进入 runtime/

新增 Tool 已明确为 Runtime Tool 或 Environment Tool

Environment Tool 已经过 Schema Validation / Capability Validation / Permission

Agent Step 产生 Trace

异步游戏行为使用 Action Lifecycle

Protocol 变更说明了跨游戏意义和兼容性影响

至少有一个 MiniWorld scenario、contract test 或等价测试覆盖核心链路

没有为了 Demo 绕过 Tool Registry / Permission / Environment
```

如果以上任一项无法满足，任务输出 MUST 明确说明原因和后续补救计划。

---

# 52. AI 实现任务输出要求

当 AI 被要求实现功能时，应该先输出：

```text
Architecture Classification
```

包含：

```text
Layer:
Runtime / Protocol / Adapter

Modules affected:

Protocol changes:

New Capability:

New Tool:

Dependency impact:
```

然后再修改代码。

例如：

```text
Feature:
Dynamic NPC dialogue

Layer:
Runtime + Protocol + Stardew Adapter

Runtime:
PromptBuilder
LLMProvider
SpeakTool

Protocol:
ActionRequest

Adapter:
SpeakCapability

New Capability:
speak

Dependency violation:
None
```

---

# 53. 当前最高优先级

当前禁止优先做：

```text
Multi-Agent

Complex Planner

Dynamic Quest

Social Graph

Vector DB Optimization

Cloud Platform
```

当前最高优先级：

```text
PlayerInteract
      ↓
Observation
      ↓
LLM
      ↓
speak Tool
      ↓
Stardew Dialogue
```

完成第一个真实 Vertical Slice。

---

# 54. 当前项目架构一句话

```text
GameAgent Runtime
负责 NPC 如何思考

Environment Protocol
负责 Runtime 与游戏如何交流

Game Adapter
负责如何把一个具体游戏翻译成 Runtime 能理解和控制的环境

Game
负责底层行为真正如何执行
```

任何代码设计都必须保持这个边界。

---

# 55. 最终架构原则

整个项目长期遵循：

> **Agent owns intent.**

> **Runtime owns cognition.**

> **Protocol owns contracts.**

> **Adapter owns translation.**

> **Game owns execution.**

中文：

> **Agent 负责意图。**

> **Runtime 负责认知。**

> **Protocol 负责契约。**

> **Adapter 负责翻译。**

> **Game 负责执行。**

这是本项目最高级别的架构约束。
