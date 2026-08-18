# GameAgent Runtime MVP0 Harness 技术开发方案

> Status: Implementation Plan
> Date: 2026-08-11
> Scope: MVP0 Runtime + Adapter vertical slice
> Related:
> - `0811-GameAgent Runtime 架构设计规范.md`
> - `0811-GameAgent Protocol v1alpha1 设计规范.md`
> - `0811Agent Harness 架构借鉴与改进设计.md`

------

# 1. 目标

MVP0 的目标不是一次性做出完整 Agent Harness。

MVP0 的目标是：

> **用最小 Runtime server 拉通 Stardew Adapter 与 GameAgent Protocol 的第一条完整链路。**

验收链路固定为：

```text
玩家点击 Linus
        ↓
Stardew Adapter 捕获交互
        ↓
Adapter 发送 GameEvent
        ↓
Runtime 可靠接纳事件并返回 EventAck
        ↓
Runtime 请求 CapabilityList
        ↓
Adapter 返回 speak capability
        ↓
Runtime 请求 Observation
        ↓
Adapter 返回 Linus 当前观察上下文
        ↓
Runtime 产生最小决策
        ↓
Runtime 发送 ActionRequest(speak)
        ↓
Adapter 执行 SpeakCapability
        ↓
游戏中显示 Runtime 返回文本
        ↓
Adapter 返回 ActionResult
        ↓
Runtime 记录 Trace
```

MVP0 用一个非常薄的 `HardcodedDecider` 代替真实 LLM。

这样先验证：

```text
Protocol
gRPC stream
EnvironmentGateway
Capability → Tool
Observe
Action
Trace
```

全部成立。

真实 LLM Provider、Memory、复杂 AgentLoop 在 MVP0 之后接入。

------

# 2. 核心原则

MVP0 必须遵守：

```text
Runtime owns cognition.
Protocol owns contracts.
Adapter owns translation.
```

含义：

```text
Adapter 只负责把 Stardew 翻译成 GameAgent Protocol。

Runtime 只依赖 protocol 生成包，不依赖 Stardew / SMAPI / Game1 / Farmer / NPC 类型。

Agent 决策逻辑只存在于 Runtime 内部。
```

MVP0 要采用 harness 形状，但不展开完整 harness。

也就是：

```text
保留边界
压薄实现
拉通链路
避免大抽象
```

------

# 3. 不做什么

MVP0 不实现：

```text
真实 LLM Provider
长期 Memory
Vector Database
Goal Scheduler
Context Compression
Skill System
Sub-agent
Supervisor Agent
Multi-Agent Collaboration
复杂 Planner
复杂 Hook / Middleware
复杂 Interrupt 策略
复杂 Session Resume
Web Debug UI
Distributed Runtime
Kafka / Redis / Event Sourcing
```

MVP0 不把 Runtime 做成完整 Harness 产品。

MVP0 只做：

> **一个具备正确 harness 边界的最小可运行 vertical slice。**

------

# 4. 当前已有基础

已经具备：

```text
Stardew Adapter Probe
  - SMAPI Mod 可加载
  - 可捕获 Linus 交互
  - 可读取 location / time / player / friendship
  - 可显示动态文本

GameAgent Protocol v1alpha1
  - gameagent.proto
  - Go protobuf 生成入口
  - Go gRPC 生成代码

Runtime 目录骨架
  - runtime/cmd/server
  - runtime/internal/*
```

因此 MVP0 不再验证 SMAPI 是否可行，也不再继续扩展 Protocol。

MVP0 的主要工程任务是：

```text
Go Runtime server
Stardew Adapter gRPC client
两端通过 protocol messages 拉通
```

------

# 5. MVP0 系统边界

最终运行形态：

```text
┌─────────────────────────────┐
│        Stardew Valley        │
│             SMAPI            │
│     Stardew Adapter Mod      │
└──────────────┬──────────────┘
               │
               │ gRPC bidirectional stream
               │ GameAgent Protocol v1alpha1
               ↓
┌─────────────────────────────┐
│       Go Runtime Server      │
│                             │
│  EnvironmentGateway          │
│  CapabilityRegistry          │
│  EventInbox                  │
│  Minimal AgentRun            │
│  HardcodedDecider            │
│  ActionDispatcher            │
│  TraceRecorder               │
└─────────────────────────────┘
```

Adapter 和 Runtime 之间只有一个物理接口：

```protobuf
service GameAgentGateway {
  rpc Connect(stream AdapterMessage) returns (stream RuntimeMessage);
}
```

所有业务请求都通过 stream 内消息表达。

------

# 6. MVP0 模块边界

## 6.1 Runtime 模块

建议目录：

```text
runtime/
├── cmd/
│   └── server/
│       └── main.go
│
└── internal/
    ├── gateway/
    │   ├── server.go
    │   ├── stream.go
    │   └── session.go
    │
    ├── environment/
    │   ├── inbox.go
    │   ├── capabilities.go
    │   └── pending.go
    │
    ├── agent/
    │   ├── run.go
    │   └── hardcoded_decider.go
    │
    ├── tool/
    │   ├── registry.go
    │   └── environment_tool.go
    │
    ├── permission/
    │   └── basic.go
    │
    └── trace/
        └── recorder.go
```

说明：

```text
gateway
  只处理 gRPC stream、AdapterMessage、RuntimeMessage、EnvironmentSession 生命周期。

environment
  管理 event inbox、capability cache、pending observe/action correlation。

agent
  只实现最小 AgentRun 和 HardcodedDecider，不接真实 LLM。

tool
  把 Adapter Capability 包装成 Runtime 可调用的 EnvironmentTool。

permission
  MVP0 只允许 speak，拒绝未知 capability。

trace
  MVP0 先输出 stdout 或 JSONL。
```

不使用 `internal/context` 目录名，避免和 Go 标准库 `context` 混淆。后续需要时使用：

```text
internal/contextengine
```

------

## 6.2 Adapter 模块

Stardew Adapter 继续保持：

```text
adapters/stardew/
├── ModEntry.cs
├── Events/
├── State/
├── Capabilities/
└── RuntimeClient/
```

新增 `RuntimeClient`：

```text
RuntimeClient
  - 连接 Go Runtime
  - 发送 AdapterHello
  - 发送 GameEvent
  - 接收 RuntimeMessage
  - 响应 CapabilityRequest
  - 响应 ObserveRequest
  - 执行 ActionRequest
  - 返回 ActionResult
```

Adapter 不实现：

```text
Agent Decision
LLM Call
Memory
Permission
Prompt
```

------

# 7. EnvironmentSession 状态机

MVP0 Runtime 中的 `EnvironmentSession` 状态：

```text
CREATED
  ↓
WAITING_HELLO
  ↓
READY
  ↓
DISCONNECTED
```

状态说明：

```text
CREATED
  gRPC Connect 已建立，但尚未收到 AdapterHello。

WAITING_HELLO
  Runtime 等待第一条有效业务消息。

READY
  AdapterHello 校验通过，Runtime 已发送 EnvironmentReady。

DISCONNECTED
  gRPC stream 结束或发生不可恢复错误。
```

MVP0 约束：

```text
Adapter 第一条业务消息必须是 AdapterHello。

Runtime 只接受 protocol_version == "v1alpha1"。

Runtime MVP0 只接受 game_id == "stardew-valley" 或配置允许的 game_id。

READY 之后才能处理 GameEvent / Observation / CapabilityList / ActionResult。
```

------

# 8. Stream 读写模型

MVP0 不应把 stream 写成严格同步调用：

```text
Recv
Send
Recv
Send
```

因为同一条 stream 中可能出现：

```text
GameEvent
Observation
CapabilityList
ActionStatusUpdate
ActionResult
Heartbeat
Error
```

推荐模型：

```text
gRPC stream
    │
    ├── readLoop
    │       ↓
    │   incoming channel
    │
    └── writeLoop
            ↑
        outgoing channel
```

Runtime 内部维护：

```text
pendingRequests:
  message_id -> response waiter

pendingActions:
  action_id -> AgentRun

capabilityCache:
  entity_id -> CapabilityList

eventInbox:
  event_id -> accepted / duplicate / rejected
```

MVP0 可以先只支持一个 Adapter stream，但内部结构不要假设永远只有一个游戏环境。

------

# 9. EventAck 可靠接纳

Protocol 已规定：

```text
EVENT_ACK_STATUS_ACCEPTED
表示 Runtime 已经可靠接纳该事件，并记录 event_id 用于后续幂等处理。
```

MVP0 实现：

```text
收到 GameEvent
  ↓
检查 event_id 是否已存在
  ↓
如果重复：发送 EventAck(DUPLICATE)
  ↓
如果新事件：写入 EventInbox
  ↓
写入成功后：发送 EventAck(ACCEPTED)
  ↓
进入 TriggerRouter
```

MVP0 的 `EventInbox` 可以先用：

```text
内存 map + JSONL append-only log
```

JSONL 文件建议：

```text
runtime/.local/events.jsonl
```

每行至少包含：

```json
{
  "event_id": "evt_...",
  "event_type": "player_interacted_with_npc",
  "sequence": 1,
  "accepted_at_unix_ms": 1786420000000
}
```

如果 JSONL 写入失败：

```text
Runtime 不得发送 EventAck(ACCEPTED)
```

可以返回：

```text
EventAck(REJECTED)
Error(code = "EVENT_INBOX_WRITE_FAILED")
```

------

# 10. Capability → Tool

MVP0 支持一条动态能力链：

```text
CapabilityList
    ↓
CapabilityRegistry
    ↓
EnvironmentTool
    ↓
ToolRegistry
    ↓
HardcodedDecider / future LLM
```

Adapter 对 Linus 返回：

```text
Capability:
  name = "speak"
  version = "0.1.0"
  execution_mode = SYNC
  input_schema_json = JSON Schema for { text: string }
```

Runtime 注册：

```text
entity_id: npc:Linus
capability: speak
```

MVP0 校验：

```text
ActionRequest capability 必须存在于 capabilityCache。

speak.arguments.text 必须存在、非空、长度不超过 300。

Permission 必须允许 entity_id 执行 speak。
```

注意：

```text
Capability 不是 Permission。
Capability 不是 Runtime Tool。
Capability 只是 Environment 声明的可执行能力。
```

------

# 11. AgentSession 与 AgentRun 的 MVP0 定义

MVP0 需要保留两个概念，但实现要薄。

## AgentSession

表示一个 Runtime 内部 Agent 与游戏 Entity 的绑定：

```text
agent_session_id = save_id + entity_id
entity_id = npc:Linus
environment_id = env_...
```

MVP0 不持久化完整 AgentSession，只在内存维护。

MVP0 不实现：

```text
Profile
Long-term Memory
Goal
Conversation History
```

## AgentRun

表示一次事件触发后的最小决策执行：

```text
CREATED
  ↓
OBSERVING
  ↓
DECIDING
  ↓
EXECUTING_ACTION
  ↓
SETTLED
```

MVP0 不实现复杂 suspend/resume。

但数据结构中应保留：

```text
run_id
agent_session_id
trigger_event_id
status
pending_action_id
```

这样后续接异步 `move_to` 时可以扩展为：

```text
WAITING_ACTION
```

------

# 12. Trigger 策略

MVP0 只处理：

```text
event_type == "player_interacted_with_npc"
```

处理规则：

```text
如果事件中包含 npc Entity：
  解析 entity_id
  创建或获取 AgentSession
  创建 AgentRun

如果没有 npc Entity：
  记录 Trace
  不创建 AgentRun
```

同一 AgentSession 并发规则：

```text
同一 AgentSession 同一时刻最多一个 Active AgentRun。
```

MVP0 策略：

```text
如果没有 Active Run：
  start run

如果已有 Active Run：
  queue latest trigger
```

MVP0 不做：

```text
interrupt
coalesce
priority scheduling
```

------

# 13. HardcodedDecider

MVP0 的 decider 不调用 LLM。

接口形状可以先定义为：

```go
type Decider interface {
    Decide(ctx context.Context, input DecisionInput) (Decision, error)
}
```

MVP0 实现：

```text
HardcodedDecider
  输入：GameEvent + Observation + available tools
  输出：ToolCall(name = "speak", arguments.text = "Hello from GameAgent Runtime")
```

这不是最终 Agent。

它只是为了验证：

```text
Event
Observation
Capability
Tool
Permission
Action
Trace
```

这条 harness 管线可以端到端跑通。

LLM 接入阶段只替换：

```text
HardcodedDecider
```

不改变：

```text
EnvironmentGateway
CapabilityRegistry
ActionDispatcher
TraceRecorder
```

------

# 14. Runtime 主流程

Runtime 启动：

```text
cmd/server/main.go
  ↓
load config
  ↓
create EventInbox
  ↓
create TraceRecorder
  ↓
create GatewayServer
  ↓
listen localhost:50051
```

Adapter 连接：

```text
Adapter Connect
  ↓
Runtime creates EnvironmentSession
  ↓
AdapterMessage(hello)
  ↓
Runtime validates hello
  ↓
RuntimeMessage(environment_ready)
  ↓
RuntimeMessage(capability_request)
  ↓
AdapterMessage(capabilities)
```

玩家交互：

```text
AdapterMessage(event)
  ↓
EventInbox.Accept(event)
  ↓
RuntimeMessage(event_ack)
  ↓
TriggerRouter
  ↓
AgentRun.CREATED
  ↓
RuntimeMessage(observe)
  ↓
AdapterMessage(observation)
  ↓
HardcodedDecider
  ↓
ToolRegistry.Resolve("speak")
  ↓
Permission.Allow
  ↓
RuntimeMessage(action)
  ↓
AdapterMessage(action_result)
  ↓
AgentRun.SETTLED
  ↓
TraceRecorder
```

------

# 15. Adapter 主流程

Adapter 启动：

```text
SMAPI Mod loaded
  ↓
RuntimeClient.Connect(localhost:50051)
  ↓
send AdapterHello
  ↓
receive EnvironmentReady
  ↓
ready
```

收到 RuntimeMessage：

```text
capability_request
  ↓
return CapabilityList(speak)

observe
  ↓
ObservationBuilder.Build(entity_id)
  ↓
return Observation

action
  ↓
if capability == speak
  ↓
SpeakCapability.Speak(entity_id, text)
  ↓
return ActionResult(SUCCEEDED)
```

玩家点击 Linus：

```text
PlayerInteractProbe
  ↓
build GameEvent(player_interacted_with_npc)
  ↓
RuntimeClient.Send(event)
```

Adapter 本地仍可以保留 probe 日志，但最终对话文本应来自 Runtime 的 `ActionRequest`。

------

# 16. Trace MVP

Trace MVP 先使用 stdout 或 JSONL。

推荐 JSONL：

```text
runtime/.local/traces.jsonl
```

每个 AgentRun 至少记录：

```text
run_started
event_accepted
capability_requested
capability_received
observation_requested
observation_received
decision_created
tool_validated
action_submitted
action_result_received
run_settled
run_failed
```

Trace event 基础字段：

```json
{
  "trace_id": "trace_...",
  "run_id": "run_...",
  "event": "action_submitted",
  "entity_id": "npc:Linus",
  "timestamp_unix_ms": 1786420000000
}
```

Trace 不进入 Protocol。

Trace 是 Runtime 内部 harness observability。

------

# 17. 错误处理

MVP0 必须处理：

```text
AdapterHello 缺失或协议版本不匹配
EventInbox 写入失败
CapabilityRequest 超时
ObserveRequest 超时
Capability 不存在
Permission 拒绝
ActionResult FAILED
gRPC stream 断开
```

推荐超时：

```text
CapabilityRequest: 2s
ObserveRequest: 2s
ActionResult for speak: 3s
```

失败策略：

```text
失败必须写 Trace。

失败不得让 Runtime panic。

失败不得让 Adapter 执行未校验 Action。
```

------

# 18. 配置

Runtime MVP0 配置：

```text
GAMEAGENT_RUNTIME_ADDR=127.0.0.1:50051
GAMEAGENT_EVENT_LOG=runtime/.local/events.jsonl
GAMEAGENT_TRACE_LOG=runtime/.local/traces.jsonl
GAMEAGENT_ALLOWED_GAME_ID=stardew-valley
```

Adapter MVP0 配置：

```text
RuntimeAddress = "http://127.0.0.1:50051"
TargetNpc = "Linus"
```

MVP0 默认只测试 Linus，因为 Linus 更容易在游戏外部场景稳定触发。

后续可把 `TargetNpc` 改成配置项。

------

# 19. 测试计划

## Runtime 单元测试

必须覆盖：

```text
AdapterHello valid -> EnvironmentReady
AdapterHello invalid protocol_version -> Error
GameEvent first seen -> EventAck(ACCEPTED)
GameEvent duplicate -> EventAck(DUPLICATE)
player_interacted_with_npc -> creates AgentRun
CapabilityList registers speak
Observation response resumes run
HardcodedDecider emits speak ToolCall
missing speak capability rejects action
permission rejects unknown capability
ActionResult(SUCCEEDED) settles run
```

## Runtime 集成测试

使用 fake in-memory stream 或 bufconn：

```text
fake Adapter connects
send AdapterHello
receive EnvironmentReady
receive CapabilityRequest
send CapabilityList
send GameEvent
receive EventAck
receive ObserveRequest
send Observation
receive ActionRequest(speak)
send ActionResult(SUCCEEDED)
assert trace contains run_settled
```

## Protocol / Generated Code 验证

继续运行：

```powershell
powershell -ExecutionPolicy Bypass -File npcore\protocol\tests\check-protocol-static.ps1
powershell -ExecutionPolicy Bypass -File npcore\protocol\tests\check-go-generation.ps1
go test ./protocol/gen/go/...
```

## Adapter 手工测试

```text
1. 启动 Go Runtime。
2. 启动 Stardew Valley + SMAPI。
3. 加载包含 Linus 的存档。
4. 玩家点击 Linus。
5. 游戏显示来自 Runtime 的文本。
6. Runtime trace 出现 event、observation、action、result。
```

------

# 20. 实施顺序

推荐顺序：

```text
1. Runtime server skeleton
2. Gateway Connect + AdapterHello
3. EventInbox + EventAck
4. CapabilityRequest / CapabilityList
5. ObserveRequest / Observation
6. HardcodedDecider
7. ToolRegistry + Permission
8. ActionRequest / ActionResult
9. TraceRecorder
10. Stardew Adapter gRPC client
11. End-to-end manual smoke test
```

每一步都必须有可运行验证。

不要等到最后才测试游戏内链路。

------

# 21. MVP0 验收标准

MVP0 完成时必须满足：

```text
Go Runtime 可以启动并监听 localhost gRPC。

Stardew Adapter 可以主动连接 Runtime。

Runtime 可以完成 AdapterHello -> EnvironmentReady。

Runtime 可以请求并缓存 speak Capability。

玩家点击 Linus 后，Adapter 发送 GameEvent。

Runtime 可靠接纳事件并发送 EventAck(ACCEPTED)。

Runtime 请求 Linus Observation。

Adapter 返回 Observation。

Runtime 通过 HardcodedDecider 生成 speak ToolCall。

Runtime 校验 Capability 和 Permission。

Runtime 发送 ActionRequest(speak)。

Adapter 在游戏中显示 Runtime 文本。

Adapter 返回 ActionResult(SUCCEEDED)。

Runtime 记录完整 Trace。

Runtime 代码不引用 Stardew / SMAPI / Game1 / Farmer / NPC 类型。
```

------

# 22. MVP0 之后

MVP0 通过后，再进入：

```text
MVP1: 替换 HardcodedDecider 为真实 LLM Provider
MVP2: 引入最小 MemoryStore
MVP3: 支持异步 move_to 与 AgentRun WAITING_ACTION / resume
MVP4: 增加更完整 TriggerRouter / RunScheduler
MVP5: Debug UI / Eval / Replay
```

也就是说：

```text
MVP0 证明链路。
MVP1 才证明智能。
MVP2+ 才证明长期 harness 能力。
```

------

# 23. 最终一句话

MVP0 的正确姿势是：

> **用最小代码把 GameAgent Protocol、Stardew Adapter、Go Runtime、Capability → Tool、Observation、Action、Trace 连成一条真实可运行链路。**

不要在 MVP0 试图完成整个 Harness。
