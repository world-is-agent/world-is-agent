# GameAgent MVP0 Phase6 Async Action Protocol Strategy ADR

> **Status:** Proposed for Phase6 Implementation
> **Date:** 2026-08-27
> **Scope:** Async Action Lifecycle Protocol Strategy
> **Architecture Baseline:** GameAgent Runtime Architecture v0.3
> **Roadmap Baseline:** GameAgent Phase3-Phase8 阶段规划 v0.5
> **Protocol Baseline:** gameagent.protocol.v1alpha2 after Phase5

---

# 1. Decision

Phase6 复用现有 `gameagent.protocol.v1alpha2`，不新增 proto 字段，不做 Go / C# protocol regeneration。

Phase6 异步 Action lifecycle 使用现有协议元素：

```text
Capability.execution_mode = EXECUTION_MODE_ASYNC
ActionRequest.action_id
ActionStatusUpdate(action_id, status, metadata)
ActionResult(action_id, status, output, error)
CancelActionRequest(action_id, reason)
```

Runtime 在 Phase6 中补齐这些字段的执行语义：

```text
ActionRequest
  -> ActionStatusUpdate(ACCEPTED / RUNNING)
  -> AgentTurn waiting
  -> ActionResult(SUCCEEDED / FAILED / REJECTED / CANCELLED / INTERRUPTED)
  -> AgentTurn resume
  -> next AgentStep
```

---

# 2. Rationale

现有 v1alpha2 已经表达 Phase6 需要的最小异步生命周期：

```text
- Capability 能声明 sync / async execution mode；
- Action 有独立 action_id；
- Adapter 能发送非终态 ActionStatusUpdate；
- Adapter 能发送终态 ActionResult；
- Runtime 能发送 best-effort CancelActionRequest。
```

Phase6 的真实缺口在 Runtime / Adapter 执行模型，不在协议字段：

```text
- Runtime 当前只把 SubmitAction 当作同步等待 terminal result；
- Gateway 当前没有把 ActionStatusUpdate 分发到 pending action；
- Tool Registry 当前不暴露 ASYNC capability；
- AgentLoop 当前没有显式 waiting / suspended / resumed trace；
- Stardew Adapter 当前只有 speak / emote 两个同步 capability。
```

因此 Phase6 不通过 proto 变更解决 Runtime 同步假设，而是把现有协议语义接起来。

---

# 3. Protocol Semantics

## 3.1 Capability.execution_mode

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

## 3.2 ActionStatusUpdate

`ActionStatusUpdate` 表示异步 Action 的非终态进展。

Phase6 只把以下状态视为有效进展：

```text
ACTION_STATUS_ACCEPTED
ACTION_STATUS_RUNNING
```

`ACTION_STATUS_PENDING` 不作为 Adapter 已接管 action 的承诺。Runtime 可以记录 trace，但不得把它当成 start 成功。

终态必须通过 `ActionResult` 表达：

```text
ACTION_STATUS_SUCCEEDED
ACTION_STATUS_FAILED
ACTION_STATUS_REJECTED
ACTION_STATUS_CANCELLED
ACTION_STATUS_INTERRUPTED
```

## 3.3 ActionResult

`ActionResult` 是 Action terminal truth。

Runtime 只在收到 terminal `ActionResult` 后生成 model-visible `ToolResult`。

`ActionStatusUpdate` 不进入 model transcript；它属于 lifecycle trace，不属于模型下一步决策需要直接消费的工具结果。

## 3.4 CancelActionRequest

Cancel 继续保持 best-effort 语义：

```text
如果 Action 尚未执行或仍可中断，Adapter 尽量停止它；
如果 Action 已经产生游戏副作用，Runtime 不要求回滚。
```

Runtime 对超时或 Turn cancellation 发出 `CancelActionRequest` 后，当前等待失败；之后到达的 late terminal result 不恢复已失败的 Turn。

---

# 4. Phase6 Limits

Phase6 只支持：

```text
- 单个 AgentTurn 内最多一个 async ToolCall；
- async ToolCall 必须单独占据一个 AgentStep；
- async ToolCall 与其它 ToolCall 不在同一个 batch 中执行；
- async terminal result 到达后，Runtime re-observe 当前目标实体，再进入下一 AgentStep。
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
```

这些限制让 Phase6 先证明核心生命周期：

```text
start async action
receive lifecycle update
suspend current turn
receive terminal result
refresh observation
resume same turn
continue bounded multi-step
```

---

# 5. Acceptance

ADR 通过标准：

```text
1. protocol/proto/gameagent.proto 无 Phase6 字段变更。
2. Runtime tests 证明 ActionStatusUpdate 会被接收和 trace。
3. Runtime tests 证明 ASYNC capability 可以进入 tool view。
4. Runtime tests 证明 async action terminal result 能恢复原 AgentTurn。
5. Runtime tests 证明 async timeout 会发送 CancelActionRequest，并且 late result 不恢复已失败 Turn。
6. Stardew adapter tests / build 证明 move_to 使用 EXECUTION_MODE_ASYNC。
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
