# GameAgent MVP0 Phase6 技术开发与验收方案

> **Status:** Implementation Baseline Draft
> **Date:** 2026-08-27
> **Scope:** Async Action Lifecycle and AgentTurn Resume
> **Architecture Baseline:** GameAgent Runtime Architecture v0.3
> **Roadmap Baseline:** GameAgent Phase3-Phase8 阶段规划 v0.5
> **Entry ADR:** [Async Action Protocol Strategy ADR](GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR.md)
> **Protocol Baseline:** gameagent.protocol.v1alpha2 after Phase5
> **Previous Phase:** Phase5.5 Stardew Adapter Context Enrichment Accepted

---

# 1. 阶段目标

Phase5 已经证明一个 AgentTurn 可以包含多个有界 AgentStep，同一 Step 可以执行 ordered ToolCall batch，并把 ToolResult 回灌给模型。

Phase5.5 已经证明 Stardew Adapter 可以通过 `Observation.state.stardew` 提供更成熟的当前事实，Runtime 不需要理解 Stardew 字段。

Phase6 要证明：

> **长时间运行的 Environment Action 不等于同步函数；Runtime 可以启动异步 Action，等待 terminal result，刷新 Observation，并恢复同一个 AgentTurn 继续推进。**

目标链路：

```text
GameEvent
  -> Observe
  -> AgentStep #1
  -> ModelDecision(move_to)
  -> ActionRequest(move_to)
  -> ActionStatusUpdate(ACCEPTED / RUNNING)
  -> AgentTurn waiting
  -> ActionResult(SUCCEEDED / FAILED / REJECTED / CANCELLED / INTERRUPTED)
  -> re-observe target entity
  -> AgentTurn resume
  -> AgentStep #2
  -> ToolResult transcript visible to model
  -> settle or bounded continuation
  -> unique terminal trace event
```

---

# 2. 阶段结论

Phase6 做这些工作：

```text
1. 接受 Async Action Protocol Strategy ADR。
2. Runtime Gateway 分发 ActionStatusUpdate。
3. Runtime Environment Port 支持 async action start / wait / cancel。
4. Tool Registry 暴露 ASYNC capability，并保存 execution mode metadata。
5. Tool Scheduler 支持单 async ToolCall 的 start -> wait -> terminal result。
6. AgentLoop 支持 waiting / suspended / resumed trace，并在 async terminal result 后 re-observe。
7. Context transcript 继续只接收 terminal ToolResult，不把 ActionStatusUpdate 当作 ToolResult。
8. Memory 只记录 terminal SUCCEEDED 的 async action outcome。
9. Stardew Adapter 增加一个真实异步 Environment Tool：move_to。
10. 确定性测试夹具覆盖 status update、延迟 terminal result、timeout cancel、late result、resume。
```

Phase6 不做这些工作：

```text
Protocol 字段变更
ActionBatchRequest / ActionBatchResult
多个并发长 Action
一个 Step 内混合同步和异步 ToolCall
Runtime 崩溃后的 continuation 恢复
跨 Environment reconnect 恢复 pending async action
Workflow Engine
复杂行为树
路径规划进入 Runtime
事务回滚
AgentDefinition store
canonical dialogue retrieval
long-term event memory persistence
```

---

# 3. 架构边界

## 3.1 Action 不是同步函数

整体架构已定义：

```text
Action = Runtime 请求 Environment 执行的一次具有独立业务身份和生命周期的副作用操作。
```

Phase6 需要把当前同步假设拆开：

```text
Phase5:
    SubmitAction(ctx, req) -> terminal ActionResult

Phase6:
    StartAction(ctx, req) -> accepted / running / fast terminal
    WaitActionResult(ctx, action_id) -> terminal ActionResult
    CancelAction(action_id, reason) -> best-effort cancellation
```

`action_id` 是 Runtime 与 Adapter 之间关联异步生命周期的唯一业务 ID。

## 3.2 AgentStep 仍然不进入 Protocol

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
```

## 3.3 Runtime 不接管路径规划

`move_to` 的目标解析、可达性判断、寻路、主线程执行、中断和失败原因都属于 Stardew Adapter。

Runtime 只负责：

```text
ToolCall envelope validation
ActionRequest routing
action lifecycle correlation
timeout / cancel
ToolResult transcript
AgentTurn continuation
trace
```

## 3.4 Current Observation 在 async resume 后刷新

长 Action 可能改变游戏状态。Phase6 规定：

```text
收到 async terminal ActionResult 后，
Runtime 必须重新 Observe 当前 target entity，
再构建下一步 model request。
```

这样模型看到的是 action 后的当前事实，而不是 action 启动前的旧位置、旧场景或旧 schedule。

## 3.5 Status Update 是 trace，不是 ToolResult

`ActionStatusUpdate(ACCEPTED / RUNNING)` 表示 Adapter 已接管异步 Action 或正在执行。

它进入 trace：

```text
action_status_update_received
turn_suspended
turn_resumed
```

它不进入 model transcript。模型只看到 terminal `ToolResult`。

---

# 4. Runtime 设计

## 4.1 Environment Port

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
```

`streamEnvironment` 内部 pending action 建议统一为 lifecycle waiter：

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

## 4.2 Tool Registry Execution Metadata

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

## 4.3 Scheduler Async Path

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

## 4.4 AgentLoop Resume

修改范围：

```text
runtime/internal/agent/loop.go
runtime/internal/agent/loop_test.go
runtime/internal/context/builder_test.go
runtime/internal/memory/projector_test.go
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
```

## 4.5 Config Budgets

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

## 4.6 Trace

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
```

字段：

```text
step_index
tool_call_id
action_id
tool
action_status
wait_ms
reason
```

不变量：

```text
- turn_suspended / turn_resumed 是非终态事件；
- turn_completed / turn_failed 仍然唯一且最后；
- ActionStatusUpdate 不改变 Memory；
- trace 不成为 action lifecycle source of truth。
```

---

# 5. Adapter 设计

## 5.1 Stardew move_to Capability

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

## 5.2 Async Adapter Lifecycle

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

# 6. Milestones And Acceptance

## M0：Async Action Protocol Strategy ADR

目标：

```text
冻结 Phase6 协议策略，确认是否需要 proto 变更。
```

修改范围：

```text
docs/phase6/GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR.md
docs/phase6/GameAgent MVP0 Phase6 技术开发与验收方案.md
```

验收命令：

```powershell
powershell -ExecutionPolicy Bypass -File protocol/tests/check-protocol-static.ps1
powershell -ExecutionPolicy Bypass -File protocol/tests/check-go-generation.ps1
git diff -- protocol/proto/gameagent.proto
```

通过标准：

```text
- ADR 明确复用 v1alpha2；
- proto 无 Phase6 字段变更；
- 开发方案明确 Runtime / Adapter 改动面；
- 非目标包含 ActionBatchRequest、persistent continuation、多个并发长 Action、Runtime pathfinding。
```

## M1：Runtime Action Lifecycle Plumbing

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

## M2：Tool Registry Execution Mode Metadata

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

## M3：Scheduler Async Single Action Path

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

## M4：AgentLoop Suspend / Resume

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
- terminal SUCCEEDED 的 async action 在 completed Turn 后写入 Memory；
- terminal failed / rejected / cancelled / interrupted 进入 transcript，模型可在剩余 step 内修正；
- settle 仍只能在当前 step 无 model-visible failure 时完成 Turn；
- turn_completed / turn_failed 仍唯一且最后。
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

## M5：Stardew Adapter move_to Vertical Slice

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

## M6：Gateway Integration And Regression

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
- async timeout 会发送 CancelActionRequest；
- late ActionResult 不生成第二个 terminal trace；
- sync speak / emote 多步链路保持通过；
- non-Stardew trigger fixture 保持通过。
```

建议测试：

```text
TestConnectRunsAsyncActionLifecycleAndResumesTurn
TestConnectKeepsRecvLoopAvailableWhileAsyncActionIsWaiting
TestConnectAsyncActionTimeoutSendsCancelAndKeepsSingleTerminalTrace
TestConnectIgnoresLateAsyncResultAfterTimeout
```

## M7：Full Acceptance

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
- protocol/proto/gameagent.proto 无 Phase6 字段变更；
- Runtime 不引用 Stardew / SMAPI / Adapter 项目；
- Adapter 不依赖 runtime/internal；
- Tool Registry 能暴露 sync + async capabilities；
- AgentTurn 能 suspend / resume 并保持唯一终态；
- async terminal result 后会 re-observe；
- successful async action 可以进入 Memory；
- move_to 的寻路与可达性判断完全位于 Stardew Adapter；
- 真实 Stardew trace 可以看到 move_to 的 status update、suspend、resume 和 completed / failed terminal。
```

---

# 7. 开发顺序

```text
1. M0：先接受 ADR，确认不改 proto。
2. M1：先做 Gateway / Environment Port lifecycle plumbing。
3. M2：再让 Tool Registry 暴露 async capability。
4. M3：再接 Scheduler 的单 async ToolCall 路径。
5. M4：再接 AgentLoop suspend / resume / re-observe。
6. M5：最后实现 Stardew move_to vertical slice。
7. M6-M7：做 integration hardening 和全量验收。
```

不要把 `move_to` Adapter 实现和 Runtime lifecycle plumbing 混在同一个提交里。

建议提交：

```text
docs: add phase6 async action plan
feat: add runtime async action lifecycle plumbing
feat: expose async tool execution metadata
feat: resume agent turns after async actions
feat: add stardew move_to async action
test: harden phase6 async action integration
```

---

# 8. 阶段验收状态

Phase6 可以验收为 `Accepted` 的最低条件：

```text
1. Async Action Protocol Strategy ADR 被接受。
2. Runtime 复用 v1alpha2 完成 async lifecycle，不修改 proto。
3. AgentTurn 可以等待 async action terminal result，并恢复同一 Turn。
4. resume 后会重新 Observe 当前目标实体。
5. ToolResult transcript 只使用 terminal ActionResult。
6. ActionStatusUpdate 进入 trace，不进入 Memory / model transcript。
7. timeout / cancel / late result 有确定性测试。
8. Stardew move_to 作为真实长 Action vertical slice 跑通。
9. Runtime / Adapter 架构边界检查通过。
```

---

# 9. 后续进入 Phase7 的边界

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
- move_to 是否暴露出需要升级 protocol 的真实缺口。
```
