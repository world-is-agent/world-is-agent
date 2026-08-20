# GameAgent Phase3–Phase8 阶段规划

> **Version:** v0.3  
> **Status:** Roadmap Baseline  
> **Date:** 2026-08-20  
> **Architecture Baseline:** GameAgent Runtime Architecture v0.2  
> **Current Baseline:** Phase1 Accepted + Phase2 Accepted
> **Revision Source:** [评审意见](./评审意见.md)（Roadmap Review，2026-08-18）；[Phase3 评估](../phase3/评估.md)（Protocol v1alpha2 Decision，2026-08-20）

---

# 1. 文档定位

本文档用于初步划分 Phase3 及后续阶段的核心目标和边界。

它只回答：

> **后续每个阶段主要验证哪一项新的 GameAgent 架构能力？**

本文档不提前规定具体文件、接口、协议字段、测试命令、技术选型或阶段内部开发顺序。这些内容应在进入对应阶段前，由独立的《PhaseN 技术开发与验收方案》重新设计和确认。

每个阶段完成并验收后，必须重新 Review：

- 当前实现事实是否仍符合 Architecture v0.2；
- 下一阶段是否仍是最高优先级；
- 后续阶段是否需要合并、拆分或调整顺序；
- 是否需要形成新的 Architecture Decision。

因此，本 Roadmap 是阶段方向，不是不可修改的长期排期承诺。

---

# 2. 当前基线

## Phase1：真实 One-Turn Vertical Slice

状态：`Accepted`

已经跑通：

```text
真实 Stardew GameEvent
→ Runtime Observe
→ 真实 LLM ToolCall
→ ActionRequest
→ 真实游戏 speak
→ ActionResult
```

Phase1 确立了 Runtime、Protocol、Adapter、Provider 和 Tool 的基础边界。

## Phase2：最小 AgentTurn Runtime 工程化

状态：`Accepted`

已经完成：

```text
Turn observability
Prompt / timeout config
Failure convergence
Dynamic Capability → Tool
speak + emote
Action timeout + best-effort cancel
```

Phase2 将系统从“能跑通一轮”升级为一个可观察、可配置、失败可收敛、可扩展简单 Tool 的最小 AgentTurn Runtime。

## Baseline Evidence

Accepted 状态的依据不在本文重复展开，以下文档作为当前 Roadmap 的证据入口：

- [GameAgent MVP0 Phase1 技术开发与验收方案](../phase1/GameAgent MVP0 Phase1 技术开发与验收方案.md)
- [GameAgent MVP0 Phase1 工程设计规范](../phase1/GameAgent MVP0 Phase1 工程设计规范.md)
- [GameAgent MVP0 Phase2 技术开发与验收方案](../phase2/GameAgent MVP0 Phase2 技术开发与验收方案.md)
- [GameAgent MVP0 Phase2 Trace 链路观测设计](../phase2/GameAgent MVP0 Phase2 Trace 链路观测设计.md)
- [GameAgent Runtime 整体架构设计规范](./GameAgent Runtime 整体架构设计规范.md)

---

# 3. 后续阶段划分原则

## 3.1 每阶段只证明一个主要架构跃迁

后续阶段不应同时引入 Memory、Multi-step、Reconnect、异步 movement、Scheduler 和产品化工具等多个高耦合能力。

## 3.2 新 Capability 只作为架构验证手段

功能数量不是阶段目标。每阶段只增加验证该阶段架构所需的最小 Capability，例如：

```text
Phase3：用简单 Action 验证 Adapter 泛化。
Phase6：用 move_to 验证异步 Action lifecycle。
```

## 3.3 每阶段结束后重新规划

Phase4–Phase8 都属于初始范围。上一阶段结束后，可以根据实际代码和验收结果调整后续阶段，但不得静默破坏 Architecture v0.2 的核心边界。

---

# 4. 阶段总览

| 阶段 | 核心主题 | 主要验证目标 |
| --- | --- | --- |
| Phase3 | Agent Identity 与 Adapter 泛化 | 同一实体具有稳定身份；Stardew Adapter 不再局限于单 NPC |
| Phase4 | Context 与短期 Memory | Agent 可以在多个 Turn 之间保留隔离的上下文，并形成可复用确定性测试底座 |
| Phase5 | 有界 Multi-step AgentTurn | 一个 Turn 可以包含多个 Model → Tool / Result Step |
| Phase6 | 异步 Action 与 Turn Resume | 长时间 Action 不被建模为同步函数；Turn 可以等待并恢复 |
| Phase7 | Environment Recovery 与持久状态 | 连接重建、状态持久化和长期运行失败能够收敛 |
| Phase8 | Evaluation 与产品化 | 系统可重复评估、定位、交付，并支持新 Adapter 接入 |

---

# 5. Phase3：Agent Identity 与 Stardew Adapter 泛化

## 阶段目标

将当前面向单一 NPC 的 Adapter 验证，升级为具有稳定身份边界、支持多个 NPC 和多个简单能力的 Stardew Environment Adapter。

本阶段主要证明：

> **Runtime 不理解具体 NPC，也能稳定控制同一 Environment 中的多个实体。**

## 主要范围

- 明确 EnvironmentSession 与 AgentSession identity 的区别；
- 建立稳定 entity identity / AgentSession resolution contract，并形成 P0 Agent Identity Contract；
- GameEvent 携带目标 entity 信息，Runtime 通过 identity contract 解析并路由到对应 AgentSession；
- Protocol 层接受 Phase3 升级到 `gameagent.protocol.v1alpha2`，一次性引入 typed `target_entity_id` 与消息级 `world_id` 贯穿，避免 WorldScope 双来源和后续二次协议迁移；
- 同一 AgentSession 同时只允许一个 active Turn，冲突处理策略留待 Phase 技术方案确定；
- 将 NPC 交互从单一目标泛化到多个 NPC；
- 扩展少量必要且稳定的 Observation 当前事实；
- 补强 ProtocolMapper、ObservationBuilder、RuntimeClient 等 Adapter 边界测试；
- 按需增加少量简单、短时、可观察 Capability。

## P0 Mandatory Deliverable：Agent Identity Contract

Phase3 结束前必须形成 Agent Identity Contract。该 contract 冻结逻辑身份组成和不变量，不冻结具体字符串编码、数据库主键或 UUID 方案。

推荐逻辑模型：

```text
AgentSessionIdentity
=
GameScope
+
WorldScope
+
StableEntityIdentity
```

解释：

```text
GameScope
    当前游戏命名空间，例如 game_id。

WorldScope
    当前存档或世界身份。Phase3 技术方案已将 Protocol 术语收敛为 world_id。

StableEntityIdentity
    Adapter 在该世界内提供的稳定、opaque entity_id。
```

必须保持：

```text
session_id
    MUST NOT 参与 AgentSession identity。

display_name
    MUST NOT 参与 AgentSession identity。

本地化名称
    MUST NOT 参与 AgentSession identity。
```

`entity_type` 是否纳入最终 identity 编码不在 Roadmap 层冻结；若 Adapter 无法保证 `entity_id` 在 WorldScope 内跨类型唯一，Phase3 技术方案应明确是否把 `entity_type` 纳入解析规则。

P0 还必须包含 AgentSessionResolver 的最小实现或等价可测试解析逻辑。否则 identity contract 只能停留在文档层，无法证明事件能够稳定路由到同一个 AgentSession。

最低验收不变量：

| 输入变化 | 预期 |
| --- | --- |
| 相同 game、world/save、entity，多次解析 | 同一 identity |
| entity 不同 | identity 不同 |
| world/save 不同 | identity 不同 |
| EnvironmentSession / session_id 不同 | identity 不变 |
| display_name 或语言变化 | identity 不变 |
| 相同 display_name、不同 entity_id | identity 不同 |

## 非目标

```text
长期 Memory
Multi-step ReAct
复杂异步 movement
自动 reconnect（保持 Phase7 的 Environment Recovery 范围）
Event replay
复杂 Permission
大量 Stardew 功能覆盖
```

## 完成条件

- 多个 NPC 可以进入同一条 Runtime AgentTurn 链路；
- Runtime 不需要为具体 NPC 增加分支；
- Agent Identity Contract 已验收，并覆盖最低身份不变量；
- GameEvent 目标实体可以解析并路由到对应 AgentSession；
- 同一 AgentSession 不会同时运行多个 active Turn；
- 稳定 entity identity 足以成为未来 AgentSession 的身份基础；
- 新增简单 Capability 时，Runtime 继续通过动态 Tool Registry 感知；
- Adapter 的关键映射和结果 contract 有自动测试。

## 阶段结束 Review

重点确认身份模型是否足以进入 Memory 阶段，以及 Runtime 是否出现任何 Stardew-specific 泄漏。

---

# 6. Phase4：Context 与短期 Memory

## 阶段目标

让同一个 AgentSession 在多个 AgentTurn 之间保留轻量上下文，并证明不同实体之间的状态不会串线。

本阶段主要回答：

> **Agent 第二次被唤醒时，能否使用第一次 Turn 留下的相关信息？**

## 主要范围

- 建立最小 AgentSession state boundary；
- 实现轻量、可替换的 MemoryStore；
- 在 Turn 进入终态（completed / failed）后，将有限的 recent turns 或 episodic facts 写入 MemoryStore，与 Trace Recorder 解耦；
- 在 Model Request 中组合 Trigger Event、Observation、Recent Context 和 Tools；
- 为 context loaded / context updated 增加必要观测；
- 将现有 fake adapter / fake Environment 收敛为可复用的确定性测试夹具，用于验证多 Entity、多 Turn、Memory 隔离和失败路径。

第一版默认使用 In-Memory Store；如为开发调试使用简单本地文件实现，不承诺跨进程恢复、版本兼容或 Environment Recovery，正式持久 Agent State 与重启恢复属于 Phase7。

## 非目标

```text
向量数据库
复杂长期人格系统
Memory Reflection Agent
Knowledge Graph
复杂摘要与压缩
Multi-step AgentTurn
完整 MiniWorld
Scenario Evaluation Framework
```

## 完成条件

- 同一 NPC 的后续 Turn 可以引用前一次相关信息；
- 不同 NPC 的 Memory 不会串线；
- Memory 绑定 AgentSession，而不是 EnvironmentSession；
- 关闭 Memory 后，现有 One-Turn 链路仍能正常运行；
- Trace 能说明本轮是否加载和更新了 Context；
- 确定性测试夹具可以脚本化多 Entity、多 Turn、Observation、ActionResult 和基础失败路径。

## 阶段结束 Review

重点确认 AgentSession identity 是否稳定、MemoryStore 是否足够小、Context 构造是否已经需要独立模块，以及确定性测试夹具是否足以支撑 Phase5。

---

# 7. Phase5：有界 Multi-step AgentTurn

## 阶段目标

将当前：

```text
1 Turn = 1 Model Call + 1 Tool / Action
```

扩展为：

```text
1 Turn = N AgentSteps
```

同时保持明确的最大步数、总 timeout、失败语义和 Trace 边界。

## 主要范围

- 正式引入 AgentStep 概念；
- Tool / Action Result 可以进入下一次 Model Request；
- 设置 `max_steps` 和 Turn 全局上限；
- 每个 Step 具有顺序、ToolCall 和结果观测；
- Tool 失败可以在有限范围内由模型修正；
- 明确 AgentTurn 的正常结束语义（settle）：模型不再调用 Tool 时，Turn 应在未达到 `max_steps` 时正常收敛，而不是只能被 `max_steps` / timeout 被动终止；具体结束信号形式由 Phase5 技术方案确定；
- 一个 Turn 仍然只有一个最终 terminal result。

本阶段优先只使用短时、可快速返回结果的 Tool，不同时引入长时间异步 Action。

## 非目标

```text
无限 ReAct
复杂 Planner
Sub-agent / Supervisor
长时间 move_to suspend / resume
跨进程 continuation recovery
```

## 完成条件

- 一个 AgentTurn 可以稳定执行至少两个 AgentSteps；
- 每个 Step 都能在同一 `turn_id` 下被追踪；
- AgentTurn 可以在未达到 `max_steps` 时通过正常结束语义收敛为唯一终态，而不是只能依赖 `max_steps` / timeout 被动终止；
- 超过最大步数时 Turn 明确收敛；
- Tool Result 能以 provider-neutral 方式进入下一次模型请求；
- 单步模式仍保持兼容。

## 阶段结束 Review

重点确认 AgentTurn Core 是否仍清晰、Step retry 是否有明确上限，以及异步 Action 是否有自然插入位置。

---

# 8. Phase6：异步 Action Lifecycle 与 AgentTurn Resume

## 阶段目标

证明 GameAgent 可以原生处理长时间运行的游戏动作，而不是把所有 Environment Tool 都当作同步 RPC。

本阶段主要回答：

> **Action 执行期间游戏继续运行，Action 完成后 Runtime 能否恢复原 AgentTurn 并继续推进？**

## 主要范围

- 开发前完成 Async Action Protocol Strategy ADR，优先评估复用现有 `execution_mode / ActionStatusUpdate / ActionResult / CancelActionRequest`；若现有 proto 存在实际缺口，再明确兼容性和版本策略；
- 支持 Action 非终态状态，例如 accepted / running；
- AgentTurn 可以进入 waiting / suspended 状态；
- Action terminal result 到达后可以恢复 Turn；
- 明确 timeout、cancel、interrupt 和迟到结果语义；
- 扩展确定性测试夹具，支持 ActionStatusUpdate（ACCEPTED / RUNNING）、延迟 terminal result、cancel 竞争与 late result 注入；
- 使用一个真实长 Action vertical slice 验证，例如 `move_to`。

## 非目标

```text
复杂行为树
多个并发长 Action
事务回滚
Workflow Engine
Runtime 崩溃后的 continuation 恢复
路径规划进入 Runtime
未通过 ADR 证明必要的 Protocol breaking change
```

## 完成条件

- Adapter 可以返回完整的 Action 非终态与终态生命周期；
- Runtime 不阻塞 Environment 消息接收循环等待长 Action；
- AgentTurn 可以等待 Action，并在 terminal result 后恢复；
- `move_to` 等具体执行仍完全位于 Adapter / Game；
- Trace 可以复盘 suspend、Action lifecycle 和 resume；
- Phase6 实现符合已 Accepted ADR 确定的 Protocol 与 continuation 策略。

## 阶段结束 Review

重点确认 continuation 是否需要持久化、Action 是否需要独立子系统，以及系统是否具备进入 reconnect / recovery 的条件。

---

# 9. Phase7：Environment Recovery 与持久 Agent State

## 阶段目标

让 Runtime 和 Adapter 能够应对连接断开、进程重启和长时间游玩，同时保持 EnvironmentSession 与 AgentSession 的正确边界。

本阶段核心原则：

> **连接可以重建，但 Agent identity、Memory 和失败语义不能混乱。**

## 主要范围

- Adapter reconnect 与 EnvironmentSession 重建；
- Capability refresh；
- pending Observe / Action 的断线收敛；
- 明确 Event idempotency、retry 和 replay policy；
- AgentSession 与必要 Agent State 的持久化；
- 长时间运行日志、Trace 和资源清理。

## 非目标

```text
分布式 Runtime
跨机器高可用
复杂 Event Sourcing
Exactly-once 全链路
多租户平台
大规模并发 Agent 集群
```

## 完成条件

- Runtime 与 Adapter 启动顺序不再构成长期阻塞；
- 连接断开后 Adapter 可以恢复到新的 EnvironmentSession；
- 重连后同一实体仍能解析到正确 AgentSession；
- pending operation 不会永久悬挂；
- 必要 Agent State 可以在明确范围内跨进程重启保留；
- 至少一次较长时间真实游玩测试稳定完成。

## 阶段结束 Review

重点确认 Protocol 是否需要升级、持久化边界是否稳定，以及是否具备进入系统化 Evaluation 的条件。

---

# 10. Phase8：Evaluation、Developer Experience 与产品化

## 阶段目标

将已经具备核心 Harness 能力的 GameAgent，升级为可重复验证、可定位问题、可安装使用、可扩展接入的工程化系统。

本阶段不再以增加 Agent 智能为主，而是回答：

> **如何证明系统长期没有退化，并让新开发者或新 Adapter 可靠接入？**

## 主要范围

- Scenario-based Evaluation；
- MiniWorld 或等价测试 Environment；
- 核心 AgentTurn / Memory / Tool / Action 指标；
- Trace 查询 CLI 或轻量 Viewer；
- 完善 Architecture checks 并固化为 CI merge gates；
- Runtime / Adapter packaging；
- 配置、安装和故障排查文档；
- 新 Adapter 接入规范与 contract tests。

## 非目标

```text
云平台化
复杂多租户控制台
Plugin Marketplace
分布式执行
Multi-Agent 社会模拟平台
```

## 完成条件

- 核心行为可以通过重复 Scenario 自动评估；
- Runtime 回归能够被 CI 发现；
- Trace 可以按 Turn / Entity / Failure 快速查询；
- 新开发者能够按文档启动 Runtime 并安装 Adapter；
- 新 Adapter 可以通过 Protocol contract tests 验证基本兼容性；
- 架构依赖违规能够自动阻止合并。

## 阶段结束 Review

重点判断 GameAgent 是否已经具备稳定 v0.x 产品形态，以及下一轮应该优先扩展 Agent 能力、游戏 Adapter 还是部署体验。

---

# 11. 跨阶段不变量

无论处于哪个 Phase，都必须保持：

```text
Agent owns intent.
Runtime owns cognition.
Protocol owns contracts.
Adapter owns translation.
Game owns execution.
```

以及：

```text
EnvironmentSession != AgentSession
GameEvent != Observation
Capability != Policy != Tool
message_id != action_id != turn_id
Observer != Functional Hook
Action != synchronous function
AgentStep belongs to AgentTurn
```

任何 Phase 如果需要破坏这些不变量，必须先 Review Architecture v0.2，并形成正式 Architecture Decision。

---

# 12. 每阶段固定交付物

从 Phase3 开始，每阶段至少应形成：

1. PhaseN 技术开发与验收方案；
2. 必要的专题设计文档；
3. 自动测试与真实或等价 Environment 验收记录；
4. 阶段小结或学习回顾；
5. Architecture / Protocol / Roadmap Review 结论；
6. Architecture boundary check 与 protocol generated-code 一致性检查（至少：runtime 不依赖 adapters/、adapter 不依赖 runtime/internal/、runtime 不引用具体游戏 API、proto 源与生成代码一致）。

阶段结束状态应明确为：

```text
Accepted
Accepted with Known Limitations
Needs Follow-up
```

不能只以“代码已经写完”作为阶段完成依据。

## 阶段依赖门

为避免后续阶段在关键 contract 未定的情况下开工，以下依赖门必须显式确认：

```text
进入 Phase4 前
    Agent Identity Contract 必须 Accepted。

进入 Phase5 前
    可复用 Deterministic TestEnvironment 必须可用。

进入 Phase6 implementation 前
    Async Action Protocol Strategy ADR 必须 Accepted。
```

---

# 13. 暂不绑定固定 Phase 的候选能力

以下能力保留为未来候选，等核心 Harness 出现真实需求后再进入阶段规划：

```text
复杂 Goal Planner
完整 Scheduled Goal / Scheduled Action
Advanced Permission / Safety Policy
Long-term semantic memory
Vector retrieval
Skills
Multiple concurrent actions
Multi-Agent collaboration
Supervisor / Sub-agent
更多游戏 Adapter
Remote Runtime / Authentication
Cloud deployment
```

---

# 14. 一句话 Roadmap

```text
Phase1
跑通真实游戏 Agent vertical slice

Phase2
让 AgentTurn 可观察、可配置、失败可收敛、Tool 可动态扩展

Phase3
稳定实体身份并泛化 Stardew Adapter

Phase4
让 Agent 在多个 Turn 之间拥有隔离的上下文与短期记忆
并建立最小确定性测试底座

Phase5
让一个 Turn 可以安全执行多个有界 Step

Phase6
让 Turn 能等待和恢复长时间游戏 Action
并先确认异步 Action 协议策略

Phase7
让 Environment 可以重连、恢复，并持久化必要 Agent State

Phase8
让系统可以被重复评估、可靠交付和持续扩展
```

> **Roadmap 的目标不是一次预测所有未来实现，而是保证每一阶段只增加一层可独立验证的复杂度。**
