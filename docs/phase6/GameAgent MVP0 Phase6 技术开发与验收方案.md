# GameAgent MVP0 Phase6 技术开发与验收方案

> **Status:** Implementation Baseline Draft
> **Date:** 2026-08-28
> **Scope:** Turn Completion, Interaction Guard, Async Action Lifecycle and AgentTurn Resume
> **Architecture Baseline:** GameAgent Runtime Architecture v0.4
> **Roadmap Baseline:** GameAgent Phase3-Phase8 阶段规划 v0.6
> **Entry ADR:** [Async Action Protocol Strategy ADR](GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR.md)
> **Protocol Baseline:** gameagent.protocol.v1alpha2 after Phase5.6
> **Previous Phase Gate:** Phase5.6 Stardew Dialogue Interaction Surface Accepted or Accepted with Known Limitations

---

# 1. 阶段目标

Phase5 已经证明一个 AgentTurn 可以包含多个有界 AgentStep，同一 Step 可以执行 ordered ToolCall batch，并把 ToolResult 回灌给模型。

Phase5.5 已经证明 Stardew Adapter 可以通过 `Observation.state.stardew` 提供成熟的当前事实，Runtime 不需要理解 Stardew 字段。

Phase5.6 已经证明 Stardew Adapter 可以维护跨 Turn conversation，并通过 `present_dialogue` / `player_said_to_npc` / `ContextFact` 打通 NPC 台词、玩家回复、Runtime AgentTurn 和 Recent Memory。

Phase6 要证明：

> **长时间运行的 Environment Action 不等于同步函数；Runtime 可以启动异步 Action，等待 terminal result，刷新 Observation，并恢复同一个 AgentTurn 继续推进。**

Phase6 还要补齐 Phase5.6 暴露出的交互生命周期缺口：

> **Adapter 可以知道一个已接受 GameEvent 对应的 Turn 已经终结，并释放等待态交互上下文。**

目标链路：

```text
GameEvent(player_interacted_with_npc / player_said_to_npc)
  -> EventAck(ACCEPTED)
  -> Adapter records interaction context snapshot
  -> Observe
  -> AgentStep #1
  -> ModelDecision(move_to or present_dialogue or settle)
  -> ActionRequest(move_to)
  -> ActionStatusUpdate(ACCEPTED / RUNNING)
  -> AgentTurn suspended
  -> ActionResult(SUCCEEDED / FAILED / REJECTED / CANCELLED / INTERRUPTED)
  -> re-observe target entity
  -> AgentTurn resumed
  -> AgentStep #2
  -> ToolResult transcript visible to model
  -> settle
  -> TurnCompletion(COMPLETED / FAILED / CANCELLED)
  -> Adapter releases interaction context
```

---

# 2. 阶段结论

Phase6 做这些工作：

```text
1. 接受 Async Action Protocol Strategy ADR。
2. Protocol additive 增加 Runtime -> Adapter 的 TurnCompletion 终态信号。
3. Runtime Gateway 在每个 accepted GameEvent 对应 Turn 终结时发送 TurnCompletion。
4. Stardew Adapter 使用 TurnCompletion 释放 pending interaction context。
5. Stardew Adapter 在 present_dialogue 显示前执行 Interaction Context Guard。
6. Runtime Gateway 分发 ActionStatusUpdate。
7. Runtime Environment Port 支持 async action start / wait / cancel。
8. Tool Registry 暴露 ASYNC capability，并保存 execution mode metadata。
9. Tool Scheduler 支持单 async ToolCall 的 start -> wait -> terminal result。
10. AgentLoop 支持 waiting / suspended / resumed trace，并在 async terminal result 后 re-observe。
11. Context transcript 继续只接收 terminal ToolResult，不把 ActionStatusUpdate 当作 ToolResult。
12. Memory 沿用 Phase5.6 的 SourceContextFacts + visible outcome 投影；本阶段只新增 terminal SUCCEEDED async action outcome。
13. Stardew Adapter 增加一个真实异步 Environment Tool：move_to。
14. 确定性测试夹具覆盖 TurnCompletion、status update、延迟 terminal result、timeout cancel、late result、resume。
```

Phase6 不做这些工作：

```text
ActionBatchRequest / ActionBatchResult
多个并发长 Action
一个 Step 内混合同步和异步 ToolCall
Runtime 崩溃后的 continuation 恢复
跨 Environment reconnect 恢复 pending async action
Workflow Engine
复杂行为树
路径规划进入 Runtime
事务回滚
同一 Turn 内等待玩家输入
长期 conversation persistence
AgentDefinition store
canonical dialogue retrieval
long-term event memory persistence
玩家输入 ContextFact / Recent Memory projection 重新设计
```

等待 LLM 或等待异步 Action 期间，游戏世界继续运行。Phase6 不把“冻结玩家或 NPC”作为 Runtime 能力；Stardew Adapter 通过 interaction context snapshot 和执行前 guard 保证 UI 与 Action 不落到过期上下文。

---

# 3. 架构边界

## 3.1 TurnCompletion 是 Turn 终态，不是 Action 结果

`EventAck(ACCEPTED)` 只表示 Runtime 接受了 GameEvent，不表示 Turn 已经完成。

`ActionResult` 只表示某个 Action 的终态，不表示整个 AgentTurn 已经完成。

Phase6 新增 Runtime -> Adapter 的 `TurnCompletion`：

```text
TurnCompletion
  turn_id
  event_id
  world_id
  entity_id
  status = COMPLETED / FAILED / CANCELLED
  error
```

语义：

```text
- 每个 accepted GameEvent 最多对应一个 TurnCompletion；
- TurnCompletion 必须在 Runtime terminal outcome 已确定后、唯一 terminal trace 之前发送；
- TurnCompletion 是 Adapter 释放 interaction context / pending lock 的正式信号；
- TurnCompletion 不进入 model transcript；
- TurnCompletion 不替代 ActionResult。
```

## 3.2 Action 不是同步函数

整体架构已定义：

```text
Action = Runtime 请求 Environment 执行的一次具有独立业务身份和生命周期的副作用操作。
```

Phase6 将当前同步假设拆开：

```text
Phase5:
    SubmitAction(ctx, req) -> terminal ActionResult

Phase6:
    StartAction(ctx, req) -> accepted / running / fast terminal
    WaitActionResult(ctx, action_id) -> terminal ActionResult
    CancelAction(action_id, reason) -> best-effort cancellation
```

`action_id` 是 Runtime 与 Adapter 之间关联异步生命周期的唯一业务 ID。

## 3.3 AgentStep 仍然不进入 Protocol

`AgentStep` 是 Runtime 内部推理推进单位。Adapter 不需要知道：

```text
step_index
ToolResult transcript
settle control
multi-step budget
resume 后是不是下一次模型调用
```

Adapter 只处理：

```text
ActionRequest
ActionStatusUpdate
ActionResult
CancelActionRequest
TurnCompletion
```

## 3.4 Runtime 不接管路径规划

`move_to` 的目标解析、可达性判断、寻路、主线程执行、中断和失败原因都属于 Stardew Adapter。

Runtime 只负责：

```text
ToolCall envelope validation
ActionRequest routing
action lifecycle correlation
timeout / cancel
ToolResult transcript
AgentTurn continuation
TurnCompletion
trace
```

## 3.5 Current Observation 在 async resume 后刷新

长 Action 可能改变游戏状态。Phase6 规定：

```text
收到 async terminal ActionResult 后，
Runtime 必须重新 Observe 当前 target entity，
再构建下一步 model request。
```

这样模型看到的是 action 后的当前事实，而不是 action 启动前的旧位置、旧场景或旧 schedule。

## 3.6 Status Update 是 trace，不是 ToolResult

`ActionStatusUpdate(ACCEPTED / RUNNING)` 表示 Adapter 已接管异步 Action 或正在执行。

它进入 trace：

```text
action_status_update_received
turn_suspended
turn_resumed
```

它不进入 model transcript。模型只看到 terminal `ToolResult`。

---

# 4. Protocol 设计

## 4.1 Additive Proto

修改范围：

```text
protocol/proto/gameagent.proto
protocol/tests/check-protocol-static.ps1
protocol/gen/go/...
adapters/stardew/src/Generated/...
```

新增：

```protobuf
enum TurnCompletionStatus {
  TURN_COMPLETION_STATUS_UNSPECIFIED = 0;
  TURN_COMPLETION_STATUS_COMPLETED = 1;
  TURN_COMPLETION_STATUS_FAILED = 2;
  TURN_COMPLETION_STATUS_CANCELLED = 3;
}

message TurnCompletion {
  string turn_id = 1;
  string event_id = 2;
  string world_id = 3;
  string entity_id = 4;
  TurnCompletionStatus status = 5;
  Error error = 6;
}
```

`RuntimeMessage.oneof payload` 增加：

```protobuf
TurnCompletion turn_completion = 17;
```

字段策略：

```text
- 使用 RuntimeMessage 当前 oneof 的下一个可用字段号；
- 不修改 AdapterMessage；
- 不修改 ActionRequest / ActionResult；
- 不新增 ActionBatchRequest / ActionBatchResult；
- 不引入 TurnStep / transcript / Runtime internal 字段。
```

## 4.2 TurnCompletion Semantics

映射规则：

```text
Runtime turn_completed      -> TURN_COMPLETION_STATUS_COMPLETED
Runtime turn_failed         -> TURN_COMPLETION_STATUS_FAILED
Runtime turn_cancelled      -> TURN_COMPLETION_STATUS_CANCELLED
```

发送规则：

```text
- 仅对 EventAck(ACCEPTED) 后真实创建的 AgentTurn 发送；
- Duplicate / rejected GameEvent 不发送；
- TurnCompletion 与原 GameEvent 使用相同 event_id；
- TurnCompletion.world_id / entity_id 来自 Turn target；
- TurnCompletion.error 仅在 failed / cancelled 时携带；
- Adapter 使用 event_id 释放 EventAck(ACCEPTED) 后记录的 interaction context；
- TurnCompletion.turn_id 只作为诊断字段，不作为 Adapter context matching 主键；
- Adapter 必须能接受未知 event_id 或已释放 context 的 TurnCompletion，并安全忽略。
```

---

# 5. Runtime 设计

## 5.1 Environment Port

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/agent/scheduler.go
runtime/internal/gateway/stream_environment.go
runtime/internal/gateway/gateway.go
runtime/internal/gateway/stream_environment_test.go
runtime/internal/gateway/gateway_integration_test.go
```

目标接口形态：

```go
type ActionStart struct {
    ActionID string
    Status   protocolv1alpha2.ActionStatus
    Update   *protocolv1alpha2.ActionStatusUpdate
    Result   *protocolv1alpha2.ActionResult
}

type Environment interface {
    Observe(ctx context.Context, worldID string, entityID string) (*protocolv1alpha2.Observation, error)
    SubmitAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (*protocolv1alpha2.ActionResult, error)
    StartAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (ActionStart, error)
    WaitActionResult(ctx context.Context, actionID string) (*protocolv1alpha2.ActionResult, error)
    CancelAction(actionID string, reason string)
    SendTurnCompletion(ctx context.Context, completion *protocolv1alpha2.TurnCompletion) error
}
```

语义：

```text
SubmitAction
    保留给 SYNC capability，等待 terminal ActionResult。

StartAction
    注册 pending action，发送 ActionRequest，等待第一条 ACCEPTED / RUNNING status update。
    如果 terminal ActionResult 比 status update 更早到达，返回 fast terminal Result。

WaitActionResult
    等待同一 action_id 的 terminal ActionResult。

CancelAction
    发送 best-effort CancelActionRequest。

SendTurnCompletion
    在 AgentTurn 进入唯一终态后发送 Runtime -> Adapter terminal Turn signal。
```

`streamEnvironment` 内部 pending action 统一为 lifecycle waiter：

```text
pendingActions[action_id]
  updates chan *ActionStatusUpdate
  results chan actionResult
```

`recvLoop` 必须分发：

```text
AdapterMessage_ActionStatus -> resolveActionStatusUpdate(action_id, update)
AdapterMessage_ActionResult -> resolveActionResult(action_id, result)
```

## 5.2 Tool Registry Execution Metadata

修改范围：

```text
runtime/internal/tool/registry.go
runtime/internal/tool/registry_test.go
```

`Entry` 增加 execution mode：

```go
type ExecutionMode string

const (
    ExecutionSync  ExecutionMode = "sync"
    ExecutionAsync ExecutionMode = "async"
)

type Entry struct {
    Definition  model.ToolDefinition
    Kind        Kind
    Concurrency ConcurrencyMode
    Execution   ExecutionMode
}
```

映射规则：

```text
Capability.execution_mode = SYNC         -> ExecutionSync
Capability.execution_mode = ASYNC        -> ExecutionAsync
Capability.execution_mode = UNSPECIFIED  -> ExecutionSync
```

Phase6 开始，`RegisterEnvironmentCapabilities` 不再排除 `EXECUTION_MODE_ASYNC`。

`Available()` 仍只返回 `[]model.ToolDefinition`，不把 execution metadata 暴露到 provider-specific contract。

## 5.3 Scheduler Async Path

修改范围：

```text
runtime/internal/agent/scheduler.go
runtime/internal/agent/scheduler_test.go
runtime/internal/tool/environment_tool.go
runtime/internal/tool/environment_tool_test.go
```

Phase6 支持的 async 调度规则：

```text
- async ToolCall 必须单独占据当前 AgentStep；
- async ToolCall 不与其它 ToolCall 组成 batch；
- async action 使用 ActionStartTimeout 等待 ACCEPTED / RUNNING；
- async action 使用 AsyncActionTimeout 等待 terminal ActionResult；
- timeout 时 Runtime 发送 CancelActionRequest；
- late ActionResult 不恢复已经 failed 的 AgentTurn；
- terminal ActionResult 转成普通 model.ToolResult，按既有 transcript 规则回灌。
```

新增 ToolResult code：

```text
async_batch_unsupported
async_action_limit_exceeded
action_start_timeout
async_action_timeout
action_start_rejected
```

终态映射沿用 Phase5：

```text
SUCCEEDED     -> ToolResult.status = succeeded
REJECTED      -> ToolResult.status = rejected
FAILED        -> ToolResult.status = failed
CANCELLED     -> ToolResult.status = cancelled
INTERRUPTED   -> ToolResult.status = interrupted
```

## 5.4 AgentLoop Resume

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/agent/loop_test.go
runtime/internal/context/builder_test.go
runtime/internal/memory/projector_test.go
runtime/internal/trace/trace.go
runtime/internal/trace/turn_test.go
```

Loop 行为：

```text
1. Step 返回 sync ToolCalls：
   走 Phase5 scheduler 逻辑。

2. Step 返回一个 async ToolCall：
   Loop 检查 MaxAsyncActionsPerTurn；
   scheduler start action；
   Loop emit turn_suspended；
   scheduler wait terminal result；
   Loop emit turn_resumed；
   Loop re-observe target entity；
   terminal ToolResult 进入 transcript；
   继续下一 AgentStep。

3. Step 返回 async ToolCall + 任意其它 ToolCall：
   不发送 ActionRequest；
   生成 model-visible invalid/skipped ToolResult；
   继续下一 AgentStep。

4. 当前 Turn 已经达到 MaxAsyncActionsPerTurn 后再次返回 async ToolCall：
   不发送 ActionRequest；
   生成 model-visible async_action_limit_exceeded ToolResult；
   继续下一 AgentStep。

5. async terminal success：
   completed Turn 写入 Memory outcome。

6. async terminal rejected / failed / cancelled / interrupted：
   ToolResult 对模型可见；
   模型可在剩余 step budget 内修正或 settle；
   settle 只有在当前 step 没有 model-visible failure 时才完成 Turn。

7. 任意 accepted GameEvent 对应的 Turn 进入 completed / failed / cancelled：
   Runtime 发送 TurnCompletion。
```

## 5.5 Config Budgets

修改范围：

```text
runtime/internal/agent/config.go
runtime/internal/agent/config_test.go
runtime/config/agent.json
```

新增配置：

```json
{
  "action_start_timeout_ms": 3000,
  "async_action_timeout_ms": 45000,
  "max_async_actions_per_turn": 1,
  "turn_timeout_ms": 90000
}
```

默认值：

```text
ActionStartTimeout = 3s
AsyncActionTimeout = 45s
MaxAsyncActionsPerTurn = 1
TurnTimeout = 90s
```

`ActionTimeout = 3s` 继续表示同步 Action terminal wait 上限。

Phase6 的 TurnTimeout 仍是 global hard bound。异步等待不能绕过 TurnTimeout。

## 5.6 Trace

修改范围：

```text
runtime/internal/trace/trace.go
runtime/internal/trace/turn_test.go
runtime/internal/agent/loop_test.go
runtime/internal/gateway/gateway_integration_test.go
```

新增事件：

```text
action_status_update_received
turn_suspended
turn_resumed
turn_completion_sent
turn_completion_send_failed
```

字段：

```text
step_index
tool_call_id
action_id
tool
action_status
wait_ms
turn_completion_status
reason
```

不变量：

```text
- turn_suspended / turn_resumed / turn_completion_sent / turn_completion_send_failed 是非终态事件；
- turn_completed / turn_failed / turn_cancelled 仍然唯一且最后；
- ActionStatusUpdate 不改变 Memory；
- TurnCompletion 不改变 Memory；
- trace 不成为 action lifecycle source of truth。
```

---

# 6. Adapter 设计

## 6.1 Interaction Context Guard

修改范围：

```text
adapters/stardew/src/Dialogue/InteractionContextStore.cs
adapters/stardew/src/Dialogue/ConversationStateStore.cs
adapters/stardew/src/Capabilities/PresentDialogueCapability.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/tests/ProtocolMapper.Tests/Program.cs
adapters/stardew/tests/check-context-static.ps1
```

Adapter 在 `player_interacted_with_npc` 与 `player_said_to_npc` EventAck(ACCEPTED) 后记录 snapshot：

```text
event_id
conversation_id
world_id
npc entity_id
player entity_id
location
npc tile
player tile
max interaction distance
```

`present_dialogue` 显示前 guard：

```text
- world_id 不一致 -> REJECTED / interaction_context_changed
- conversation_id 不匹配 -> REJECTED / interaction_context_changed
- NPC 或 player 不在原 location -> REJECTED / interaction_context_changed
- 当前距离超过 max interaction distance -> REJECTED / interaction_context_changed
- guard 失败时关闭匹配 conversation；
- guard 成功后按 Phase5.6 语义显示 UI。
```

TurnCompletion 处理：

```text
COMPLETED / FAILED / CANCELLED
  -> release interaction context matched by event_id
```

等待 LLM 期间：

```text
- 不冻结玩家；
- 不冻结 NPC schedule；
- 不阻塞游戏时间；
- Adapter 在 effect time 用 guard 决定是否展示 UI 或执行 action。
```

## 6.2 Stardew move_to Capability

修改范围：

```text
adapters/stardew/src/Runtime/CapabilityCatalog.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/Runtime/ActionCancellationRegistry.cs
adapters/stardew/src/Capabilities/MoveToCapability.cs
adapters/stardew/tests/ProtocolMapper.Tests/Program.cs
adapters/stardew/tests/ActionCancellationRegistry.Tests/Program.cs
adapters/stardew/tests/check-context-static.ps1
```

Capability：

```text
name = move_to
version = 0.1.0
execution_mode = EXECUTION_MODE_ASYNC
concurrency_mode = CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL
description = Moves the NPC toward a reachable tile in the current location.
```

Input schema：

```json
{
  "type": "object",
  "properties": {
    "location": { "type": "string" },
    "tile": {
      "type": "object",
      "properties": {
        "x": { "type": "number" },
        "y": { "type": "number" }
      },
      "required": ["x", "y"],
      "additionalProperties": false
    }
  },
  "required": ["location", "tile"],
  "additionalProperties": false
}
```

Phase6 vertical slice 约束：

```text
- location 必须等于 NPC 当前 location；
- tile 必须在当前 location map bounds 内；
- tile 可达性由 Stardew Adapter 判断；
- Runtime 不生成路径，不读取地图，不判断坐标语义；
- 跨 location movement 留到后续阶段。
```

## 6.3 Async Adapter Lifecycle

Adapter 行为：

```text
收到 move_to ActionRequest
  -> world_id mismatch: ActionResult(REJECTED, world_mismatch)
  -> cancel marker already exists: ActionResult(CANCELLED)
  -> input invalid: ActionResult(REJECTED, invalid_move_target)
  -> accepted: send ActionStatusUpdate(ACCEPTED)
  -> main thread starts movement: send ActionStatusUpdate(RUNNING)
  -> reached target: send ActionResult(SUCCEEDED, output current location/tile)
  -> path failed: send ActionResult(FAILED, move_failed)
  -> cancel received while running: stop movement, send ActionResult(CANCELLED)
```

Active async action state 属于 Stardew Adapter：

```text
action_id
entity_id
world_id
target location
target tile
started_at game time / tick
terminal sent flag
```

Runtime 不读取这些内部字段。

---

# 7. Milestones And Acceptance

## M0：Protocol + ADR

目标：

```text
冻结 Phase6 协议策略，并把 TurnCompletion 作为 Runtime -> Adapter 的正式终态信号。
```

修改范围：

```text
docs/phase6/GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR.md
docs/phase6/GameAgent MVP0 Phase6 技术开发与验收方案.md
protocol/proto/gameagent.proto
protocol/tests/check-protocol-static.ps1
protocol/gen/go/...
adapters/stardew/src/Generated/...
```

验收命令：

```powershell
powershell -ExecutionPolicy Bypass -File protocol/tests/check-protocol-static.ps1
powershell -ExecutionPolicy Bypass -File protocol/tests/check-go-generation.ps1
go test ./protocol/gen/go/...
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

```text
- ADR 明确 TurnCompletion 是 terminal Turn signal；
- protocol/proto/gameagent.proto additive 增加 TurnCompletion；
- RuntimeMessage.oneof 增加 turn_completion = 17；
- Go / C# generated code 与 proto 一致；
- ActionStatusUpdate / ActionResult / CancelActionRequest 字段保持不变；
- 非目标包含 ActionBatchRequest、persistent continuation、多个并发长 Action、Runtime pathfinding。
```

## M1：Runtime TurnCompletion Plumbing

目标：

```text
Runtime 能在 accepted GameEvent 的 AgentTurn 进入终态时，向 Adapter 发送 TurnCompletion。
```

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/gateway/gateway.go
runtime/internal/gateway/stream_environment.go
runtime/internal/gateway/stream_environment_test.go
runtime/internal/gateway/gateway_integration_test.go
runtime/internal/trace/trace.go
```

验收命令：

```powershell
go test ./runtime/internal/gateway ./runtime/internal/agent ./runtime/internal/trace
```

通过标准：

```text
- completed Turn 发送 TURN_COMPLETION_STATUS_COMPLETED；
- failed Turn 发送 TURN_COMPLETION_STATUS_FAILED 并携带 error；
- cancelled Turn 发送 TURN_COMPLETION_STATUS_CANCELLED；
- settle-only Turn 也发送 TurnCompletion；
- TurnCompletion 与原 GameEvent event_id 绑定；
- duplicate / rejected GameEvent 不发送 TurnCompletion；
- TurnCompletion 发送发生在唯一 terminal trace 之前；
- TurnCompletion 发送失败会进入非终态 trace，不生成第二个 Turn terminal event；
- turn_completion_sent 或 turn_completion_send_failed trace 最多出现一次；
- turn_completed / turn_failed / turn_cancelled 仍是最后一条 Turn trace。
```

建议测试：

```text
TestHandleEventSendsTurnCompletionOnSettle
TestHandleEventSendsTurnCompletionOnFailure
TestHandleEventDoesNotSendTurnCompletionForRejectedEvent
TestConnectSendsTurnCompletionBeforeTerminalTrace
TestTurnCompletionSendFailureDoesNotCreateSecondTerminalTrace
```

## M2：Adapter Interaction Context Guard

目标：

```text
Stardew Adapter 能记录交互上下文，并在 TurnCompletion 后释放。
```

修改范围：

```text
adapters/stardew/src/Dialogue/InteractionContextStore.cs
adapters/stardew/src/Dialogue/ConversationStateStore.cs
adapters/stardew/src/Capabilities/PresentDialogueCapability.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/tests/ProtocolMapper.Tests/Program.cs
adapters/stardew/tests/check-context-static.ps1
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
powershell -ExecutionPolicy Bypass -File adapters/stardew/tests/check-context-static.ps1
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

```text
- EventAck(ACCEPTED) 后记录以 event_id 为主键的 interaction context snapshot；
- TurnCompletion 后按 event_id 释放匹配 snapshot；
- world / conversation / location / distance 变化会让 present_dialogue 返回 REJECTED / interaction_context_changed；
- guard 失败时关闭匹配 conversation；
- guard 成功时沿用 Phase5.6 的 UI 展示语义；
- 等待 LLM 期间玩家和 NPC 不被 Runtime 冻结；
- Adapter 不新增 runtime/internal 依赖。
```

建议测试：

```text
TestInteractionContextCommittedAfterAcceptedAck
TestInteractionContextReleasedByTurnCompletion
TestPresentDialogueRejectsWhenConversationContextChanged
TestPresentDialogueRejectsWhenNpcMovedAwayBeforeDisplay
TestPresentDialogueRejectsWhenPlayerMovedAwayBeforeDisplay
TestPresentDialogueGuardFailureClosesMatchingConversation
```

## M3：Runtime Action Lifecycle Plumbing

目标：

```text
Gateway 能分发 ActionStatusUpdate，streamEnvironment 能关联同一 action_id 的 status update 和 terminal result。
```

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/gateway/gateway.go
runtime/internal/gateway/stream_environment.go
runtime/internal/gateway/stream_environment_test.go
runtime/internal/gateway/gateway_integration_test.go
```

验收命令：

```powershell
go test ./runtime/internal/gateway
go test ./runtime/internal/agent
```

通过标准：

```text
- AdapterMessage_ActionStatus 会进入 resolveActionStatusUpdate；
- StartAction 在收到 ACCEPTED / RUNNING 后返回 ActionStart；
- terminal ActionResult 比 status update 更早到达时，StartAction 返回 fast terminal Result；
- WaitActionResult 只由 terminal ActionResult 完成；
- StartAction / WaitActionResult 超时会发送 CancelActionRequest；
- unknown action_id 的 ActionStatusUpdate / ActionResult 被忽略；
- disconnect 会唤醒 pending async action waiter；
- 现有 sync SubmitAction 测试继续通过。
```

建议测试：

```text
TestStreamEnvironmentStartActionReceivesAcceptedStatus
TestStreamEnvironmentStartActionReturnsFastTerminalResult
TestStreamEnvironmentWaitActionResultReceivesTerminalResult
TestStreamEnvironmentAsyncTimeoutSendsCancelAction
TestStreamEnvironmentLateAsyncResultAfterTimeoutIsIgnored
TestConnectRoutesActionStatusUpdateToPendingAction
```

## M4：Tool Registry Execution Mode Metadata

目标：

```text
Runtime Tool Registry 能保存并暴露 ASYNC capability 的 execution mode。
```

修改范围：

```text
runtime/internal/tool/registry.go
runtime/internal/tool/registry_test.go
```

验收命令：

```powershell
go test ./runtime/internal/tool
```

通过标准：

```text
- Entry.Execution 能区分 sync / async；
- ExecutionMode_UNSPECIFIED 映射为 sync；
- ExecutionMode_SYNC 映射为 sync；
- ExecutionMode_ASYNC 映射为 async；
- ASYNC capability 进入 Available()；
- Available() 仍只返回 provider-facing ToolDefinition；
- Available() 仍按 Name 升序稳定输出。
```

建议测试：

```text
TestRegistryMapsSyncExecutionMode
TestRegistryMapsUnspecifiedExecutionModeToSync
TestRegistryIncludesAsyncCapabilitiesInPhase6ToolView
TestRegistryLookupReturnsExecutionModeMetadata
```

## M5：Scheduler Async Single Action Path

目标：

```text
Tool Scheduler 支持单 async ToolCall 的 start / wait / terminal ToolResult。
```

修改范围：

```text
runtime/internal/agent/scheduler.go
runtime/internal/agent/scheduler_test.go
runtime/internal/agent/config.go
runtime/internal/agent/config_test.go
```

验收命令：

```powershell
go test ./runtime/internal/agent
```

通过标准：

```text
- 单 async ToolCall 会调用 StartAction；
- 收到 ACCEPTED / RUNNING 后等待 terminal ActionResult；
- terminal SUCCEEDED 生成 succeeded ToolResult；
- terminal REJECTED / FAILED / CANCELLED / INTERRUPTED 生成 model-visible ToolResult；
- async ToolCall 与其它 ToolCall 同 step 出现时，preflight 失败且不发送 ActionRequest；
- action start timeout 发送 CancelActionRequest 并 fail turn；
- async action timeout 发送 CancelActionRequest 并 fail turn；
- late terminal result 不恢复已失败 Turn；
- sync scheduler 行为保持 Phase5 语义。
```

建议测试：

```text
TestSchedulerStartsAndWaitsForSingleAsyncAction
TestSchedulerRejectsAsyncMixedWithSyncBatchBeforeExecution
TestSchedulerRejectsMultipleAsyncCallsBeforeExecution
TestSchedulerAsyncTerminalFailureIsModelVisible
TestSchedulerAsyncStartTimeoutCancelsAction
TestSchedulerAsyncWaitTimeoutCancelsAction
TestConfigLoadsPhase6AsyncBudgets
TestConfigDefaultsPhase6AsyncBudgetsWhenMissingZeroOrNegative
TestConfigPhase6DefaultTurnTimeoutCoversAsyncBudget
```

## M6：AgentLoop Suspend / Resume

目标：

```text
AgentLoop 可以在 async action terminal result 后恢复同一个 Turn，并进入下一 AgentStep。
```

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/agent/loop_test.go
runtime/internal/context/builder_test.go
runtime/internal/memory/projector_test.go
runtime/internal/trace/trace.go
runtime/internal/trace/turn_test.go
```

验收命令：

```powershell
go test ./runtime/internal/agent ./runtime/internal/context ./runtime/internal/memory ./runtime/internal/trace
```

通过标准：

```text
- async ToolCall 后 emit turn_suspended；
- 一个 Turn 内第二个 async ToolCall 产生 async_action_limit_exceeded；
- terminal ActionResult 后 emit turn_resumed；
- resume 后重新 Observe target entity；
- 下一次 model request 包含 terminal ToolResult transcript；
- terminal SUCCEEDED 的 async action 在 completed Turn 后作为 visible outcome 写入 Memory；
- terminal failed / rejected / cancelled / interrupted 进入 transcript，模型可在剩余 step 内修正；
- settle 仍只能在当前 step 无 model-visible failure 时完成 Turn；
- TurnCompletion 在 terminal outcome 确定后发送，唯一 Turn terminal trace 仍保持最后。
```

建议测试：

```text
TestHandleEventSuspendsAndResumesAfterAsyncAction
TestHandleEventReobservesAfterAsyncTerminalResultBeforeNextStep
TestHandleEventPassesAsyncToolResultTranscriptToNextStep
TestHandleEventWritesAsyncSuccessfulOutcomeToMemoryOnCompletion
TestHandleEventRetriesAfterAsyncTerminalFailureWithinStepBudget
TestAsyncTurnTerminalEventIsUniqueAndLast
```

## M7：Stardew Adapter move_to Vertical Slice

目标：

```text
Stardew Adapter 提供一个真实异步 Environment Tool，用于验证长 Action lifecycle。
```

修改范围：

```text
adapters/stardew/src/Runtime/CapabilityCatalog.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/Runtime/ActionCancellationRegistry.cs
adapters/stardew/src/Capabilities/MoveToCapability.cs
adapters/stardew/tests/ProtocolMapper.Tests/Program.cs
adapters/stardew/tests/ActionCancellationRegistry.Tests/Program.cs
adapters/stardew/tests/check-context-static.ps1
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet run --project adapters/stardew/tests/ActionCancellationRegistry.Tests/ActionCancellationRegistry.Tests.csproj
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

```text
- CapabilityCatalog 注册 move_to；
- move_to 使用 EXECUTION_MODE_ASYNC；
- move_to 使用 CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL；
- move_to schema 要求 location 和 tile；
- world mismatch 返回 ActionResult(REJECTED, world_mismatch)；
- invalid target 返回 ActionResult(REJECTED, invalid_move_target)；
- accepted 后发送 ActionStatusUpdate(ACCEPTED)；
- movement running 后发送 ActionStatusUpdate(RUNNING)；
- 到达目标后发送 ActionResult(SUCCEEDED)；
- cancel before start 返回 ActionResult(CANCELLED)；
- cancel while running 停止移动并返回 ActionResult(CANCELLED)；
- Adapter 不引入 runtime/internal 依赖。
```

## M8：Gateway Integration And Regression

目标：

```text
bufconn / fake adapter 覆盖完整 async turn lifecycle。
```

修改范围：

```text
runtime/internal/gateway/gateway_integration_test.go
runtime/internal/gateway/stream_environment_test.go
runtime/internal/agent/loop_test.go
runtime/internal/trace/turn_test.go
```

验收命令：

```powershell
go test ./runtime/internal/gateway ./runtime/internal/agent ./runtime/internal/trace
go test ./runtime/...
```

通过标准：

```text
- 一个 EventAck 后出现 ActionRequest(move_to)；
- fake adapter 发送 ACCEPTED / RUNNING status update；
- Runtime trace 记录 action_status_update_received；
- Runtime trace 记录 turn_suspended / turn_resumed；
- fake adapter 延迟 terminal ActionResult 后，Runtime 发起下一 step model request；
- 下一 step 能 settle，Turn completed；
- Runtime 发送 TurnCompletion；
- Adapter 收到 TurnCompletion 后释放 interaction context；
- async timeout 会发送 CancelActionRequest；
- late ActionResult 不生成第二个 terminal trace；
- sync speak / emote / present_dialogue 多步链路保持通过；
- non-Stardew trigger fixture 保持通过。
```

建议测试：

```text
TestConnectRunsAsyncActionLifecycleAndResumesTurn
TestConnectKeepsRecvLoopAvailableWhileAsyncActionIsWaiting
TestConnectAsyncActionTimeoutSendsCancelAndKeepsSingleTerminalTrace
TestConnectIgnoresLateAsyncResultAfterTimeout
TestConnectSendsTurnCompletionForSettleOnlyDialogueTurn
```

## M9：Full Acceptance

目标：

```text
确认 Phase6 的 Protocol、Runtime、Adapter、Trace、Memory 和 Stardew vertical slice 全部满足阶段验收标准。
```

修改范围：

```text
protocol/proto/gameagent.proto
protocol/gen/go/...
adapters/stardew/src/Generated/...
runtime/...
adapters/stardew/...
docs/phase6/...
docs/summary/...
```

验收命令：

```powershell
go test ./runtime/... ./protocol/gen/go/...
go test ./...
powershell -ExecutionPolicy Bypass -File protocol/tests/check-protocol-static.ps1
powershell -ExecutionPolicy Bypass -File protocol/tests/check-go-generation.ps1
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet run --project adapters/stardew/tests/ActionCancellationRegistry.Tests/ActionCancellationRegistry.Tests.csproj
dotnet run --project adapters/stardew/tests/PlayerInteractProbe.Tests/PlayerInteractProbe.Tests.csproj
dotnet build adapters/stardew/GameAgent.Stardew.csproj
git diff --check
```

通过标准：

```text
- 全部 PASS；
- Protocol additive TurnCompletion 已生成到 Go / C#；
- Runtime 不引用 Stardew / SMAPI / Adapter 项目；
- Adapter 不依赖 runtime/internal；
- Tool Registry 能暴露 sync + async capabilities；
- AgentTurn 能 suspend / resume 并保持唯一终态；
- async terminal result 后会 re-observe；
- successful async action 可以作为 visible outcome 进入 Memory；
- TurnCompletion 能释放 Adapter interaction context；
- Interaction Context Guard 能拒绝过期 dialogue display；
- move_to 的寻路与可达性判断完全位于 Stardew Adapter；
- 真实 Stardew trace 可以看到 move_to 的 status update、suspend、resume、TurnCompletion 和 completed / failed terminal。
```

---

# 8. 开发顺序

```text
1. M0：先接受 ADR，完成 TurnCompletion proto additive 与生成代码。
2. M1：接 Runtime TurnCompletion plumbing。
3. M2：接 Stardew Adapter Interaction Context Guard。
4. M3：做 Gateway / Environment Port async action lifecycle plumbing。
5. M4：让 Tool Registry 暴露 async capability。
6. M5：接 Scheduler 的单 async ToolCall 路径。
7. M6：接 AgentLoop suspend / resume / re-observe。
8. M7：实现 Stardew move_to vertical slice。
9. M8-M9：做 integration hardening 和全量验收。
```

不要把 `move_to` Adapter 实现和 Runtime lifecycle plumbing 混在同一个提交里。

建议提交：

```text
docs: add phase6 async action plan
feat: add turn completion protocol signal
feat: release stardew interaction contexts on turn completion
feat: add runtime async action lifecycle plumbing
feat: expose async tool execution metadata
feat: resume agent turns after async actions
feat: add stardew move_to async action
test: harden phase6 async action integration
```

---

# 9. 阶段验收状态

Phase6 可以验收为 `Accepted` 的最低条件：

```text
1. Async Action Protocol Strategy ADR 被接受。
2. Protocol additive 增加 TurnCompletion，并完成 Go / C# 生成。
3. Runtime 对每个 accepted GameEvent Turn 发送唯一 TurnCompletion。
4. Adapter 能用 TurnCompletion 释放 interaction context。
5. Interaction Context Guard 能在 effect time 拒绝过期 UI 展示。
6. AgentTurn 可以等待 async action terminal result，并恢复同一 Turn。
7. resume 后会重新 Observe 当前目标实体。
8. ToolResult transcript 只使用 terminal ActionResult。
9. ActionStatusUpdate 进入 trace，不进入 Memory / model transcript。
10. timeout / cancel / late result 有确定性测试。
11. Stardew move_to 作为真实长 Action vertical slice 跑通。
```

---

# 10. 后续进入 Phase7 的边界

Phase7 聚焦 Environment Recovery 与持久 Agent State。

Phase6 的 in-memory continuation 不处理：

```text
Runtime crash recovery
Adapter reconnect 后恢复 pending async action
durable pending action store
event replay
long-term memory persistence
```

阶段结束 Review 重点确认：

```text
- pending async action 是否需要持久化；
- reconnect 后 ActionResult 如何关联原 Turn；
- Action lifecycle 是否需要独立子系统；
- 当前 single async action per Turn 是否足够进入 Phase7；
- TurnCompletion 是否足以支撑 Adapter interaction lifecycle；
- move_to 是否暴露出需要升级 protocol 的真实缺口。
```
