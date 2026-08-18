# GameAgent Runtime：Agent Harness 架构借鉴与改进设计

> Status: Design Draft
> Scope: Runtime Architecture
> Related: `GameAgent Protocol v1alpha1`

------

# 1. 文档目的

GameAgent 当前已经完成第一层核心抽象：

```text
Game
 ↓
Game Adapter
 ↓
GameAgent Protocol
 ↓
Runtime
```

`GameAgent Protocol v1alpha1` 已经定义：

```text
Environment
Entity
GameEvent
Observation
Capability
Action
Environment Session
```

并使用：

```text
gRPC Bidirectional Streaming
```

建立 Adapter 与 Runtime 之间的持续 Environment Session。

但 Protocol 只解决：

> **Runtime 如何连接并操作一个游戏环境。**

它并没有回答完整 Agent Runtime 的问题：

```text
一个 Agent 如何运行？

一次 Agent 调用如何开始和结束？

上下文如何构造？

Tool 如何注册和执行？

正在执行 Action 时来了新事件怎么办？

Agent 状态如何持久化？

如何支持中断？

如何 Trace？

如何切换模型 Provider？

如何避免 Runtime 逐渐变成一个巨大的 Agent 类？
```

因此下一阶段的设计目标不是继续扩展 Protocol，而是：

> **在 GameAgent Protocol 上构建一个结构清晰、可扩展的 Game-native Agent Harness。**

------

# 2. 什么是 Agent Harness

这里的 Harness 不等于 Agent 本身。

可以简单理解为：

```text
              Agent Harness

             ┌─────────────┐
             │     LLM     │
             └──────┬──────┘
                    │
        ┌───────────┼───────────┐
        ↓           ↓           ↓
     Context       Tools      Control
        │           │           │
        ↓           ↓           ↓
     Memory      Execution    Agent Loop
        │                       │
        └───────────┬───────────┘
                    ↓
                 Session
```

LLM 只是其中一个组件。

Harness 负责让模型能够：

```text
Observe
↓
Reason
↓
Act
↓
Observe result
↓
Reason again
```

同时解决：

```text
Session
Context
Tool
State
Interrupt
Retry
Persistence
Trace
Permission
Lifecycle
```

等工程问题。

因此对于 GameAgent：

```text
GameAgent Protocol
```

应该被看作 Harness 的：

> **Environment Interface / Environment Gateway Protocol**

而不是 Harness 本身。

------

# 3. 参考项目

当前主要参考：

```text
Pi Agent Harness
Hermes Agent
```

二者都不是游戏 Agent，因此不能直接复制它们的架构。

我们需要借鉴的是：

> **成熟 Agent Harness 如何组织 Runtime。**

而不是复制它们的 Coding Agent 功能。

------

# 4. Pi Agent Harness 值得借鉴的设计

Pi 官方目前直接将自己定义为一个 **minimal terminal coding harness**，并强调保持核心较小，将扩展能力放到 Extensions、Skills、Prompt Templates 和 Packages 中。

这一原则非常适合 GameAgent。

------

## 4.1 Minimal Core

Pi 最值得借鉴的不是功能数量，而是：

> **Core 应该保持最小。**

GameAgent 不应该逐渐演化成：

```text
DialogueAgent
BehaviorAgent
PlannerAgent
MemoryAgent
QuestAgent
EmotionAgent
SupervisorAgent
...
```

这种大量 Agent 互相调用的结构。

第一版更应该保持：

```text
Agent Harness Core

├── AgentLoop
├── AgentSession
├── ContextEngine
├── ToolRuntime
├── ModelRuntime
└── RunControl
```

其他能力放到外围：

```text
Memory
Permission
Skills
Trace
Environment
Extensions
```

形成：

```text
        ┌─────────────────┐
        │    Small Core   │
        └────────┬────────┘
                 │
       capability at edges
                 │
 ┌───────────────┼────────────────┐
 ↓               ↓                ↓
Environment    Memory            Skills
Tools          Hooks             Providers
```

------

# 5. Pi 的异步 RPC 模型

Pi 的 RPC 模式使用 JSONL stdin/stdout。

它区分：

```text
Command
Response
Event
```

一个请求被接受以后，可以先返回 response，而 Agent 后续执行状态继续通过异步事件流输出。RPC 还支持 request `id` 做关联。

这和 GameAgent 当前设计：

```text
ActionRequest
      ↓
ActionStatusUpdate(ACCEPTED)
      ↓
ActionStatusUpdate(RUNNING)
      ↓
ActionResult(SUCCEEDED)
```

属于非常类似的语义模式。

区别只是 Transport：

```text
Pi

JSONL
stdin/stdout
```

而 GameAgent：

```text
Protobuf
gRPC Bidirectional Stream
```

GameAgent 不需要改成 JSONL。

真正应该借鉴的是：

> **Command Acceptance 与 Execution Completion 分离。**

这一点现有 GameAgent Protocol 已经做对。

------

# 6. Pi 的生命周期事件与扩展机制

Pi Extensions 可以：

```text
监听 Agent 生命周期
监听 Tool 生命周期
注册 Tool
拦截 Tool Call
修改 Context
保存 Session State
```

官方 Extension API 明确提供生命周期事件、工具注册和事件拦截能力。

GameAgent 不需要复制 Pi 的 TypeScript Plugin API。

但应该借鉴这种架构思想：

```text
AgentLoop
   ↓
Lifecycle Hook
   ↓
Extension / Policy / Trace
```

例如：

```go
type RunHook interface {
    BeforeContext(...)
    AfterContext(...)

    BeforeModel(...)
    AfterModel(...)

    BeforeTool(...)
    AfterTool(...)

    AfterRun(...)
}
```

这样：

```text
Permission
Trace
Metrics
Audit
Rate Limit
Debug
```

都不需要硬编码到：

```text
agent_loop.go
```

------

# 7. Pi 的 Steering / Follow-up 思想

Pi 对 Agent 正在执行时到来的新输入区分两类：

```text
steer
followUp
```

`steer` 会等待当前 Tool Call 完成，然后影响下一轮模型调用；`followUp` 则等待当前 Agent 工作结束后再执行。

这个机制对 GameAgent 非常有价值。

游戏中经常出现：

```text
Agent 正在执行：

move_to(Beach)

        ↓

突然发生：

player_interacted_with_npc
```

如果此时直接启动第二个 Agent Run：

```text
Run A
+
Run B
```

同时控制 Abigail，就很容易产生竞态。

因此 GameAgent 应设计：

```text
AgentSession
      │
      ├── Active Run
      │
      └── Pending Triggers
```

并为 Event 定义 Runtime 内部处理策略：

```text
INTERRUPT
QUEUE
COALESCE
DROP
```

例如：

```text
player_interacted_with_npc
→ INTERRUPT / STEER

important_goal_due
→ QUEUE

time_changed
→ COALESCE

position_changed
→ DROP
```

v1 不需要实现复杂优先级调度器。

但应该遵守：

> **同一个 AgentSession 同一时刻最多只有一个 Active AgentRun。**

------

# 8. Hermes 值得借鉴的设计

Hermes 当前的系统架构将多个入口统一到同一个 Agent Core，例如 CLI、Gateway、API 等入口最终进入共享 Agent Runtime。

Hermes 的 Gateway 则是一个长期运行的外部平台接入层，它管理不同平台 Adapter、Session、消息分发和生命周期。

这和 GameAgent 非常接近。

------

# 9. 借鉴 Hermes：Environment Gateway

Hermes：

```text
Telegram ─┐
Discord ──┼→ Messaging Gateway
Slack ────┘
               ↓
            Session
               ↓
             Agent
```

GameAgent 可以对应为：

```text
Stardew ────┐
Minecraft ──┼→ Environment Gateway
MiniWorld ──┘
                  ↓
        Environment Session
                  ↓
              Runtime
```

因此应该正式在 Runtime 中引入：

```text
EnvironmentGateway
```

职责只包括：

```text
gRPC Stream

EnvironmentSession 管理

Adapter 生命周期

Capability Registry

GameEvent Ingress

Observation Request/Response

Action Egress

Heartbeat

Reconnect
```

它不负责：

```text
Prompt
Memory
LLM
Agent Reasoning
```

形成：

```text
Game Adapter
     ↓
Game Protocol
     ↓
EnvironmentGateway
     ↓
Agent Harness
```

这样 gRPC 不会污染 Agent Core。

------

# 10. 借鉴 Hermes：Narrow Waist

Hermes 的开发原则中特别强调：

> Core 应形成一个 narrow waist，能力尽量存在于边缘，而不是持续扩大核心。

这一原则非常适合 GameAgent。

GameAgent 的 narrow waist 应该是：

```text
              Agent Harness

                   │
          Tool + Context + Run

────────────────────────────────

      Environment Abstraction

────────────────────────────────

                   │
            Game Protocol
```

任何新游戏：

```text
Stardew
Minecraft
RimWorld
```

只增加：

```text
Adapter
```

而不是修改 AgentLoop。

任何新模型：

```text
OpenAI
Anthropic
Local Model
```

只增加：

```text
ModelProvider
```

任何新的 Agent 能力：

```text
Memory
Schedule
Skill
```

通过外围模块扩展。

------

# 11. 借鉴 Hermes：ContextEngine

Hermes 已经将：

```text
Prompt Builder
Context Engine
Context Compressor
Prompt Caching
```

从 Agent Loop 中拆开。

GameAgent 应采用类似边界。

不要：

```go
func AgentLoop() {
    prompt := profile +
              memory +
              observation +
              goals +
              history +
              tools
}
```

而应该：

```text
AgentLoop
    ↓
ContextEngine
    ↓
AgentContext
    ↓
PromptBuilder
    ↓
ModelRuntime
```

例如：

```go
type AgentContext struct {
    Profile      Profile
    Observation  Observation
    Memories     []Memory
    Goals        []Goal
    RecentEvents []Event
    History      []Message
    Tools        []Tool
}
```

------

# 12. Context 应进一步分层

GameAgent 的 Context 可以分成：

```text
Stable Context
Semi-Stable Context
Volatile Context
```

### Stable

```text
NPC Personality
NPC Background
World Rules
Behavior Policy
```

### Semi-Stable

```text
Relationship
Relevant Memory
Current Goal
Long-term State
```

### Volatile

```text
Current Observation
Current GameEvent
Current Action
Current Time
Nearby Entities
```

这样以后更容易实现：

```text
Prompt Cache
Context Budget
Context Compression
Memory Retrieval
```

第一版无需优化 Prompt Cache。

但结构应该提前正确。

------

# 13. 借鉴 Hermes：Tool Registry

Hermes 使用中心化 Tool Registry 管理 Tool schema、dispatch、availability 和错误处理。

GameAgent 也应该建立：

```text
ToolRegistry
```

而不是：

```go
switch tool.Name {
case "speak":
case "move_to":
case "give_item":
}
```

GameAgent Tool 分两类。

------

## Runtime Tools

Runtime 自己提供：

```text
memory_search

memory_write

goal_create

goal_update
```

------

## Environment Tools

由 Adapter Capability 动态生成：

```text
speak

move_to

give_item

inspect_nearby
```

形成：

```text
                  Tool Registry

          ┌─────────────┴─────────────┐
          ↓                           ↓

    Runtime Tools              Environment Tools

    memory_search              speak
    memory_write               move_to
    goal_update                give_item
                               inspect
```

------

# 14. GameAgent 最有特色的设计：Capability → Tool

这是 GameAgent 不应该照搬 Pi/Hermes、而应该自己重点做好的部分。

当前 Protocol 已经定义 Capability。

完整链路应该是：

```text
Game Adapter
     ↓
CapabilityList
     ↓
Environment Gateway
     ↓
Capability Registry
     ↓
Tool Factory
     ↓
Runtime Permission
     ↓
Tool Registry
     ↓
Model Tool Schema
     ↓
LLM
```

LLM 返回：

```text
ToolCall
```

随后：

```text
ToolRuntime
     ↓
EnvironmentTool.Execute()
     ↓
ActionRequest
     ↓
Game Protocol
     ↓
Adapter
```

因此可以抽象：

```go
type EnvironmentTool struct {
    capability Capability
    environment Environment
}
```

Adapter 增加：

```text
fish
attack
trade
craft
```

Runtime 不应该增加：

```go
case "fish"
case "attack"
case "trade"
```

而应该动态注册。

这是 GameAgent 非常重要的架构卖点。

------

# 15. 四种不同生命周期

后续 Runtime 最容易混淆的就是 Session 和 Run。

建议正式区分四个概念。

## EnvironmentSession

生命周期：

```text
Adapter Connect
↓
EnvironmentReady
↓
Online
↓
Disconnect
```

通常持续：

```text
几十分钟 ～ 几小时
```

表示：

> 当前正在运行的游戏环境。

------

## AgentSession

生命周期远长于游戏连接。

例如：

```text
Farm001 / Abigail
```

包含：

```text
Profile
Memory
Conversation
Goal
Persistent State
```

即使关闭游戏再打开：

```text
AgentSession
```

仍然存在。

------

## AgentRun

表示一次：

> Agent 被唤醒后执行到稳定状态。

例如：

```text
玩家点击 Abigail
↓
Run #1001
```

或者：

```text
Goal 到期
↓
Run #1002
```

------

## Action

表示：

> Agent 对环境发起的一次具体操作。

例如：

```text
Action #2001

speak(...)
```

或者：

```text
Action #2002

move_to(Beach)
```

关系：

```text
EnvironmentSession

       │
       ├──────── AgentSession
       │             │
       │             ├── AgentRun #1
       │             │       │
       │             │       ├── Action #1
       │             │       └── Action #2
       │             │
       │             └── AgentRun #2
       │
       └──────── another AgentSession
```

------

# 16. AgentRun 应成为 Harness 的核心执行单位

AgentRun 推荐生命周期：

```text
CREATED
   ↓
OBSERVING
   ↓
BUILDING_CONTEXT
   ↓
THINKING
   ↓
EXECUTING_TOOL
   ↓
WAITING_ACTION
   ↓
THINKING
   ↓
SETTLED
```

这里：

```text
WAITING_ACTION
```

是 GameAgent 特别重要的状态。

------

# 17. GameAgent 与 Coding Agent 最大差异之一：异步世界 Action

Coding Agent 执行：

```text
read_file
```

通常：

```text
调用
↓
马上返回
```

而 GameAgent：

```text
move_to(Beach)
```

可能：

```text
ActionRequest
↓
ACCEPTED
↓
RUNNING

AgentRun suspend

      ...

NPC 到达 Beach

      ↓
ActionResult
SUCCEEDED

      ↓
AgentRun resume
```

因此 Harness 必须原生支持：

> **Suspend / Resume**

而不是：

```text
Tool Execute()
↓
一直阻塞 goroutine
↓
等几十秒
```

建议模型：

```text
AgentRun
   ↓
ToolCall
   ↓
Environment Action
   ↓
WAITING_ACTION
   ↓
persist continuation
   ↓
ActionResult arrives
   ↓
Resume AgentRun
```

这会是 GameAgent 相比普通 Agent Framework 很有特色的设计。

------

# 18. Trigger Router / Run Scheduler

Environment Gateway 收到：

```text
GameEvent
```

不应该直接：

```text
go agent.Run(...)
```

而应该：

```text
GameEvent
   ↓
Trigger Router
   ↓
AgentSession Resolver
   ↓
Run Scheduler
   ↓
AgentRun
```

Run Scheduler 至少负责：

```text
Active Run Guard

Queue

Interrupt

Coalesce

Drop
```

第一版只需要非常简单：

```text
一个 AgentSession
最多一个 Active Run
```

新 Trigger 根据类型：

```text
queue / interrupt / drop
```

即可。

------

# 19. ModelRuntime

Agent Harness 不应该直接依赖：

```text
OpenAI SDK
```

应该：

```go
type ModelProvider interface {
    Complete(
        ctx context.Context,
        req ModelRequest,
    ) (ModelResponse, error)
}
```

内部：

```text
ModelRuntime
    │
    ├── OpenAIProvider
    ├── AnthropicProvider
    └── OpenAICompatibleProvider
```

AgentLoop 只依赖：

```text
ModelRuntime
```

Hermes 同样将 Provider Resolution 作为共享 Runtime subsystem，而不是散落在不同入口中。

------

# 20. Runtime Event Bus / Trace

Pi RPC 暴露了大量 Agent、Turn、Message、Tool 生命周期事件。

GameAgent 应借鉴这种 Observability 思路。

Runtime 内部定义：

```text
RunStarted

ObservationRequested
ObservationReceived

ContextBuilt

ModelStarted
ModelCompleted

ToolSelected
ToolStarted

ActionSubmitted
ActionStatusChanged
ActionCompleted

ToolCompleted

RunSettled
RunFailed
```

然后：

```text
RuntimeEventBus
       │
       ├── TraceRecorder
       ├── Metrics
       ├── DebugConsole
       └── EvalCollector
```

AgentLoop 不应该自己写大量：

```go
log.Printf(...)
```

Trace 应该是一等能力。

------

# 21. Middleware / Hook

Runtime 内可以提供少量生命周期 Hook：

```text
BeforeContext

AfterContext

BeforeModel

AfterModel

BeforeTool

AfterTool

AfterRun
```

例如 Permission：

```text
ToolCall
   ↓
BeforeTool
   ↓
Permission Policy
   ↓
Allowed
   ↓
Execute
```

Trace：

```text
ToolCall
   ↓
BeforeTool
   ↓
Trace Hook
```

这样避免：

```text
AgentLoop
=
Context
+
Permission
+
Trace
+
Metrics
+
Audit
+
Tool
+
Model
+
Memory
```

最终变成一个几千行类。

------

# 22. Memory 应属于 Harness Peripheral

Memory 不应该成为：

```text
Memory Agent
```

第一版只需要：

```go
type MemoryStore interface {
    Save(...)
    Search(...)
}
```

ContextEngine：

```text
Observation
↓
MemoryRetriever
↓
Relevant Memories
↓
AgentContext
```

这样即可。

不要第一版引入：

```text
Memory Reflection Agent
Memory Consolidation Agent
Knowledge Graph Agent
```

------

# 23. Skill 可以作为未来扩展机制

Pi 支持按需加载 Skills，Skill 是自包含的能力/工作流包。

这个思想以后很适合 GameAgent：

```text
skills/

shopkeeper/
festival/
romance/
fishing/
quest-giver/
```

例如：

```text
Abigail
+
festival_behavior Skill
```

但 Skill 不属于 MVP。

第一阶段甚至不需要实现：

```text
SkillRegistry
```

只需要架构上不要阻止以后增加即可。

------

# 24. 不建议照搬的设计

参考 Pi / Hermes 并不意味着复制全部功能。

当前 GameAgent 不应该实现：

```text
Supervisor Agent

Sub-agent

Dialogue Agent + Behavior Agent

Multi-Agent Collaboration

Agent-to-Agent Protocol

复杂 Planner

自动 Skill Generation

复杂 Plugin Marketplace

Distributed Runtime

Kafka

Redis Cluster

Event Sourcing

复杂 Session Branch

复杂 Context Fork

几十种 Tool
```

原因不是这些技术没价值。

而是：

> **它们目前没有真实 GameAgent Requirement 支撑。**

过早引入只会掩盖项目真正的核心。

------

# 25. 对现有 GameAgent 架构的改进

现有架构：

```text
Adapter
    ↓
Game Protocol
    ↓
Runtime
    ↓
LLM
```

应该升级为：

```text
                         GAME
                          │
                          ↓
                  Stardew Adapter
                          │
                          │
               GameAgent Protocol
               gRPC Bidirectional
                          │
                          ↓
┌───────────────────────────────────────────────────────┐
│                  GameAgent Runtime                    │
│                                                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │             Environment Gateway                 │  │
│  │                                                 │  │
│  │ Stream Manager                                  │  │
│  │ EnvironmentSession Registry                     │  │
│  │ Capability Registry                             │  │
│  │ Event Ingress                                   │  │
│  │ Action Egress                                   │  │
│  │ Heartbeat / Connection Lifecycle                │  │
│  └────────────────────┬────────────────────────────┘  │
│                       │                               │
│                       ↓                               │
│  ┌─────────────────────────────────────────────────┐  │
│  │          Trigger Router / Run Scheduler         │  │
│  │                                                 │  │
│  │ Active Run Guard                                │  │
│  │ Queue / Interrupt / Coalesce / Drop             │  │
│  └────────────────────┬────────────────────────────┘  │
│                       │                               │
│                       ↓                               │
│  ┌─────────────────────────────────────────────────┐  │
│  │              Agent Harness Core                 │  │
│  │                                                 │  │
│  │ AgentSession                                    │  │
│  │ AgentRun                                        │  │
│  │ AgentLoop                                       │  │
│  │ ContextEngine                                   │  │
│  │ ToolRuntime                                     │  │
│  │ ModelRuntime                                    │  │
│  │ Middleware / Hooks                              │  │
│  │ RuntimeEventBus                                 │  │
│  └──────────────┬─────────────────┬────────────────┘  │
│                 │                 │                   │
│                 ↓                 ↓                   │
│          Runtime Tools      Environment Tools         │
│                                                       │
│          memory_search       speak                    │
│          memory_write        move_to                  │
│          goal_update         give_item                │
│                                                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │                 Persistence                     │  │
│  │                                                 │  │
│  │ AgentSessionStore                               │  │
│  │ MemoryStore                                     │  │
│  │ TraceStore                                      │  │
│  └─────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────┘
                          │
                          ↓
                    Model Provider
```

------

# 26. 推荐模块边界

Go Runtime 可以逐渐形成：

```text
internal/

gateway/
    environment.go
    session.go
    stream.go
    capability.go

trigger/
    router.go
    scheduler.go
    policy.go

agent/
    session.go
    run.go
    loop.go

context/
    engine.go
    builder.go
    budget.go

tool/
    registry.go
    runtime.go
    environment_tool.go

model/
    runtime.go
    provider.go

memory/
    store.go
    retriever.go

policy/
    permission.go

events/
    bus.go
    types.go

trace/
    recorder.go

store/
    session.go
```

注意：

```text
grpc/
```

相关实现应该主要存在：

```text
gateway
```

而不是：

```text
agent
```

------

# 27. MVP 应真正实现哪些模块

虽然最终架构包含很多抽象，但 v0.1 不需要一次写完。

第一阶段真正实现：

```text
EnvironmentGateway

EnvironmentSession

AgentSession

AgentRun

AgentLoop

ContextEngine

ToolRegistry

EnvironmentTool

ModelProvider

RuntimeEventBus

MemoryStore
```

其中很多都可以很薄。

------

# 28. 第一版 Harness 执行链路

例如玩家点击 Abigail：

```text
GameEvent
player_interacted_with_npc
        ↓
EnvironmentGateway
        ↓
TriggerRouter
        ↓
Resolve AgentSession
Farm001 / Abigail
        ↓
RunScheduler
        ↓
Create AgentRun
        ↓
Observe
        ↓
ContextEngine
        │
        ├── Profile
        ├── Observation
        ├── Memory
        └── Tools
        ↓
ModelRuntime
        ↓
ToolCall
speak(...)
        ↓
ToolRuntime
        ↓
Permission
        ↓
EnvironmentTool
        ↓
ActionRequest
        ↓
Adapter
        ↓
Stardew Dialogue
        ↓
ActionResult
        ↓
AgentRun
        ↓
SETTLED
```

------

# 29. 异步行为链路

例如：

```text
Goal:
15:00 去湖边
```

Trigger：

```text
goal_due
↓
AgentRun
↓
LLM
↓
move_to("Lake")
```

之后：

```text
EnvironmentTool
↓
ActionRequest
↓
Adapter

ActionStatusUpdate(ACCEPTED)
↓
AgentRun → WAITING_ACTION
```

游戏继续运行。

之后：

```text
ActionResult(SUCCEEDED)
↓
EnvironmentGateway
↓
RunScheduler
↓
Resume AgentRun
↓
LLM / settle
```

这条链路应该成为 GameAgent Runtime 的核心 Demo 之一。

------

# 30. GameAgent 与普通 Coding Harness 的差异

GameAgent 不应该只是：

```text
Pi
+
Game Tools
```

它真正不同的地方是：

### 1. Persistent Environment

```text
游戏世界一直运行。
```

### 2. Environment主动产生事件

```text
Agent 不只是等待 User Prompt。
```

### 3. Action 可以长时间运行

```text
move_to
schedule
wait
interaction
```

### 4. 世界可能在 Agent 思考期间继续变化

Observation 具有：

```text
revision
```

非常重要。

### 5. AgentRun 可以 Suspend / Resume

因为 Action 并非即时完成。

### 6. Capability 来自 Environment

不是 Runtime 静态写死 Tool。

因此 GameAgent 真正的特色应该定义成：

> **Event-driven, capability-driven, asynchronous environment agent harness.**

------

# 31. 项目核心创新点

最终 README 不应该把核心卖点写成：

```text
Use LLM to talk with Stardew NPC.
```

应该强调：

```text
1. Game-native Environment Protocol

2. Dynamic Capability → Tool Registration

3. Event-driven Agent Runtime

4. Persistent AgentSession

5. Suspendable / Resumable AgentRun

6. Asynchronous World Action Lifecycle

7. Game-independent Environment Gateway

8. Pluggable Model Provider

9. Observable Agent Execution
```

Stardew 只是：

> 第一套真实 Adapter 和验证环境。

------

# 32. 最终 Harness 定位

GameAgent 可以定义为：

> **GameAgent is a game-native agent harness that connects intelligent runtimes to live game environments through a capability-driven environment protocol.**

核心关键词：

```text
Game-native

Agent Harness

Environment Protocol

Capability-driven Tools

Event-driven Runtime

Persistent Sessions

Asynchronous Actions

Observable Execution
```

------

# 33. 当前架构决策

现有 GameAgent Protocol：

```text
KEEP
```

不需要因为参考 Pi / Hermes 修改整体方向。

重点新增 Runtime 层：

```text
EnvironmentGateway

AgentSession

AgentRun

AgentLoop

ContextEngine

ToolRuntime

TriggerRouter

RunScheduler

ModelRuntime

RuntimeEventBus
```

------

# 34. 当前不做

```text
Multi-Agent collaboration

Supervisor

Sub-agent

Dialogue/Behavior 双 Agent

复杂 Planner

Distributed Runtime

复杂 Skill System
```

坚持：

> **一个通用 Agent Harness + 多份独立 AgentSession。**

------

# 35. 推荐下一步

Protocol 已经进入：

```text
v1alpha1 Design Baseline
```

下一份正式架构文档应该是：

```text
docs/runtime-architecture.md
```

重点敲定：

```text
EnvironmentGateway

EnvironmentSession

AgentSession

AgentRun

AgentLoop

TriggerRouter

RunScheduler

ContextEngine

ToolRuntime

RuntimeEventBus
```

然后进入代码实现。

不再继续做纯理论上的 Agent 类型拆分。

------

# 36. 最终原则

GameAgent 应吸收 Pi 的：

```text
Minimal Core

Extension-friendly

Lifecycle Events

Session

Tool Runtime

Steering / Queue 思想
```

Pi 的核心保持精简、通过 extensions 和 skills 扩展的设计，是 GameAgent 控制复杂度的重要参考。

吸收 Hermes 的：

```text
Gateway

Session Routing

Context Engine

Provider Runtime

Tool Registry

Persistence

Narrow Waist
```

Hermes 已经将 Gateway、Agent Loop、Provider、Tool Registry 和 Session Persistence 拆成相对明确的系统边界，这些边界很适合作为 GameAgent Runtime 的工程参考。

但保留 GameAgent 自己的核心：

```text
Environment Protocol

Environment Session

Dynamic Capability → Tool

Game Events

Async Action Lifecycle

Suspend / Resume AgentRun
```

最终目标不是：

> **复制一个现有 Agent Framework。**

而是：

> **吸收成熟 Harness 的工程模式，构建一个真正适合实时游戏环境的 Agent Harness。**
