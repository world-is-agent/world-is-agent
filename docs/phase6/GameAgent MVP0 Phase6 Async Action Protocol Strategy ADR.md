# GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR

> **Status:** Proposed for Phase6 Implementation
> **Date:** 2026-08-28
> **Scope:** Async Action Lifecycle and Turn Completion Protocol Strategy
> **Architecture Baseline:** GameAgent Runtime Architecture v0.4
> **Roadmap Baseline:** GameAgent Phase3-Phase8 阶段规划 v0.6
> **Protocol Baseline:** gameagent.protocol.v1alpha2 after Phase5.6

---

# 1. Decision

Phase6 继续复用现有 `ActionRequest / ActionStatusUpdate / ActionResult / CancelActionRequest` 表达异步 Action lifecycle。

Phase6 additive 增加 Runtime -> Adapter 的 `TurnCompletion` 终态信号，用于表达已接受 GameEvent 对应的 AgentTurn 已经结束：

```text
RuntimeMessage.turn_completion
  turn_id
  event_id
  world_id
  entity_id
  status = COMPLETED / FAILED / CANCELLED
  error
```

Phase6 不新增 Action batch protocol，不把 AgentStep 暴露给 Adapter，不把 model transcript 暴露给 Protocol。

---

# 2. Rationale

现有 v1alpha2 已经表达 Phase6 需要的 Action 生命周期：

```text
- Capability 能声明 sync / async execution mode；
- Action 有独立 action_id；
- Adapter 能发送非终态 ActionStatusUpdate；
- Adapter 能发送终态 ActionResult；
- Runtime 能发送 best-effort CancelActionRequest。
```

Phase5.6 引入了跨 Turn conversation 和同步 Dialogue UI。Adapter 在 `EventAck(ACCEPTED)` 后会进入等待态，但现有协议没有 Runtime -> Adapter 的 Turn 终态信号：

```text
EventAck
    只表示 Runtime 接受事件，不表示 Turn 已完成。

ActionResult
    只表示某个 Environment Action 完成，不覆盖 settle-only / no-action Turn。

Heartbeat / Error
    不承载 Turn lifecycle 语义。
```

因此 Phase6 需要一个小的 terminal Turn signal，让 Adapter 可以释放 interaction context、清理 pending lock，并让真实 UI 行为与 Runtime Turn 终态对齐。

---

# 3. Protocol Additive Change

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
- 不修改 ActionStatusUpdate / ActionResult；
- 不新增 target_definition_id、Observation.definition_id 或 AgentDefinition store 字段；
- 不新增 ActionBatchRequest / ActionBatchResult。
```

---

# 4. Protocol Semantics

## 4.1 Capability.execution_mode

`Capability.execution_mode` 表示 Adapter 对该能力执行生命周期的承诺。

```text
EXECUTION_MODE_SYNC
    Adapter 会在短时间内返回 terminal ActionResult。

EXECUTION_MODE_ASYNC
    Adapter 接收 ActionRequest 后，可以先返回 ActionStatusUpdate，
    并在未来某个时间返回 terminal ActionResult。

EXECUTION_MODE_UNSPECIFIED
    MVP0 按 sync 处理，保留旧 fixture 和未显式声明能力的兼容行为。
```

## 4.2 ActionStatusUpdate

`ActionStatusUpdate` 表示异步 Action 的非终态进展。

Phase6 只把以下状态视为有效 start progress：

```text
ACTION_STATUS_ACCEPTED
ACTION_STATUS_RUNNING
```

`ACTION_STATUS_PENDING` 可以记录 trace，但不得被 Runtime 当作 Adapter 已接管 action 的承诺。

终态必须通过 `ActionResult` 表达：

```text
ACTION_STATUS_SUCCEEDED
ACTION_STATUS_FAILED
ACTION_STATUS_REJECTED
ACTION_STATUS_CANCELLED
ACTION_STATUS_INTERRUPTED
```

## 4.3 ActionResult

`ActionResult` 是 Action terminal truth。

Runtime 只在收到 terminal `ActionResult` 后生成 model-visible `ToolResult`。

`ActionStatusUpdate` 不进入 model transcript；它属于 lifecycle trace，不属于模型下一步决策需要直接消费的工具结果。

## 4.4 CancelActionRequest

Cancel 保持 best-effort 语义：

```text
如果 Action 尚未执行或仍可中断，Adapter 尽量停止它；
如果 Action 已经产生游戏副作用，Runtime 不要求回滚。
```

Runtime 对超时或 Turn cancellation 发出 `CancelActionRequest` 后，当前等待失败；之后到达的 late terminal result 不恢复已失败的 Turn。

## 4.5 TurnCompletion

`TurnCompletion` 表示 Runtime 已经结束某个 accepted GameEvent 对应的 AgentTurn。

发送规则：

```text
- 每个 accepted GameEvent 最多发送一次；
- Duplicate / rejected GameEvent 不发送；
- settle-only / no-action Turn 也发送；
- TurnCompletion.event_id 与原 GameEvent.event_id 一致；
- TurnCompletion.turn_id 是 Runtime 内部 Turn ID 的 protocol projection；
- TurnCompletion.world_id / entity_id 来自 Turn target；
- failed / cancelled status 携带 Error；
- Adapter 使用 event_id 释放 EventAck(ACCEPTED) 后记录的 interaction context；
- TurnCompletion.turn_id 只作为诊断字段，不作为 Adapter context matching 主键；
- Adapter 收到未知 event_id 或已释放 context 的 TurnCompletion 时安全忽略。
```

映射规则：

```text
turn_completed  -> TURN_COMPLETION_STATUS_COMPLETED
turn_failed     -> TURN_COMPLETION_STATUS_FAILED
turn_cancelled  -> TURN_COMPLETION_STATUS_CANCELLED
```

TurnCompletion 不进入 model transcript，不写 Memory，不替代 ActionResult。

---

# 5. Phase6 Limits

Phase6 只支持：

```text
- 单个 AgentTurn 内最多一个 async ToolCall；
- async ToolCall 必须单独占据一个 AgentStep；
- async ToolCall 与其它 ToolCall 不在同一个 batch 中执行；
- async terminal result 到达后，Runtime re-observe 当前目标实体，再进入下一 AgentStep；
- Adapter 用 TurnCompletion 释放本地 interaction context；
- Adapter 在 effect time 执行 Interaction Context Guard。
```

Phase6 不支持：

```text
多个并发长 Action
一个 Step 内混合同步和异步 ToolCall
Runtime 崩溃后的 continuation 恢复
跨 Environment reconnect 恢复 pending async action
ActionBatchRequest / ActionBatchResult
路径规划进入 Runtime
事务回滚
Workflow Engine
同一 Turn 内等待玩家输入
```

---

# 6. Acceptance

ADR 通过标准：

```text
1. protocol/proto/gameagent.proto additive 增加 TurnCompletion。
2. RuntimeMessage.oneof 增加 turn_completion = 17。
3. Go / C# generated code 与 proto 一致。
4. Runtime tests 证明 TurnCompletion 会在 accepted GameEvent 的 terminal outcome 确定后发送。
5. Runtime tests 证明 ActionStatusUpdate 会被接收和 trace。
6. Runtime tests 证明 ASYNC capability 可以进入 tool view。
7. Runtime tests 证明 async action terminal result 能恢复原 AgentTurn。
8. Runtime tests 证明 async timeout 会发送 CancelActionRequest，并且 late result 不恢复已失败 Turn。
9. Stardew adapter tests / build 证明 move_to 使用 EXECUTION_MODE_ASYNC。
10. Stardew adapter tests 证明 TurnCompletion 可以释放 interaction context。
```

验收命令：

```powershell
powershell -ExecutionPolicy Bypass -File protocol/tests/check-protocol-static.ps1
powershell -ExecutionPolicy Bypass -File protocol/tests/check-go-generation.ps1
go test ./runtime/...
go test ./protocol/gen/go/...
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet build adapters/stardew/GameAgent.Stardew.csproj
git diff --check
```
