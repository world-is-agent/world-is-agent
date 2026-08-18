# GameAgent MVP0 Phase2 技术开发与验收方案

> Status: Draft
> Date: 2026-08-15
> Scope: Turn observability + prompt/config + failure stability + second simple tool

------

# 1. 阶段目标

Phase1 已经证明：

```text
真实 Stardew 事件
    ↓
Go Runtime Agent Loop
    ↓
真实 DeepSeek ToolCall
    ↓
真实游戏 speak Action
```

这条 one-turn 链路可以跑通。

Phase2 的目标不是立刻做 Memory / Planner / Multi-Agent，而是把 Phase1 的 demo 链路升级成：

> **可配置、可观测、可扩展、失败可收敛的最小 Agent Turn Runtime。**

换句话说：

```text
Phase1:
    能跑通一轮。

Phase2:
    每一轮都能被追踪、配置、失败处理，并支持不止一个简单 tool。
```

------

# 2. 为什么 Phase2 不直接做 Memory

Memory 很重要，但现在不是最优先。

当前更基础的问题是：

```text
一轮 Agent Turn 没有统一 turn_id / trace_id。

模型请求、ToolCall、ActionResult 没有完整 timeline。

Prompt 输出语言和风格还需要改代码。

LLM HTTP 调用没有独立 timeout。

Adapter / Runtime 的失败路径还没有完整进入 turn result。

ToolRegistry 仍然只跑通过 speak 单工具 happy path，尚未证明新增工具时 Runtime 可以不改注册和转发代码。
```

如果在这些问题没解决前直接加入 Memory，调试成本会迅速上升。

Phase2 先补 Agent Harness 的地基，再进入长期记忆和多轮推理。

------

# 3. Phase2 范围

Phase2 做四类能力：

```text
1. Turn Observability
    让每一轮 Agent Turn 可追踪、可复盘。

2. Prompt / Runtime Config
    让模型语言、风格、timeout 等行为可配置。

3. Failure Stability
    让 LLM / Observe / Action 失败不会卡死 turn。

4. Second Simple Tool
    从 speak 单工具升级为 speak + emote，验证 Adapter schema 驱动的动态工具能力。
```

Phase2 不做：

```text
长期 Memory
向量数据库
复杂 Planner
ReAct 多轮循环
多 NPC 群体协作
复杂异步动作，例如 move_to
断线自动重连
Web Debug UI
完整 session JSONL
通用 EventBus
功能性 Hook 框架
```

------

# 4. Phase2 验收标准

Phase2 完成时，应满足：

```text
1. 每次进入 Agent Turn 的 GameEvent 都会创建一个 turn_id；被事件类型过滤忽略的 GameEvent 不创建 turn_id，可只记录 event_ignored 日志。

2. 正常无 drop / 写入失败 / 崩溃时，Runtime 能输出完整 turn trace。

3. Trace 至少包含：
   turn_started
   observation_requested
   observation_received
   model_request_started
   model_response_received
   tool_call_selected
   action_submit_started
   action_result_received
   turn_completed / turn_failed

4. Prompt 可以通过配置控制输出语言，例如 zh-CN。

5. Prompt 可以通过配置控制 NPC 说话风格。

6. LLM Provider 调用有 timeout，超时不会永久挂住当前 turn。

7. Observe / SubmitAction 失败会进入 turn_failed trace。

8. Runtime 可以从 Adapter CapabilityList 动态注册 speak + emote 两个 tool。

9. Adapter 支持 speak + emote 两个 capability。

10. 真实 Stardew 中可以看到中文 speak 或 emote 效果。
```

其中 `ToolCall -> ActionRequest` 的通用转发在 Phase1 代码中已经基本成立：`BuildActionRequest` 已经使用 `ToolCall.name` 作为 `ActionRequest.capability`，并原样透传 `ToolCall.arguments`。Phase2 P0 不需要重写这部分，只需要确认测试覆盖。

------

# 5. 总体链路

Phase2 不改变 Phase1 的主链路，只在链路上增加 trace、config、failure semantics 和第二个 tool。

```text
AdapterMessage.Event
    ↓
EventAck
    ↓
AgentLoop.HandleEvent
    ↓
event type / target entity filter
    ↓
event accepted into Agent Turn
    ↓
create turn_id / trace_id
    ↓
trace(turn_started)
    ↓
trace(observation_requested)
    ↓
Environment.Observe
    ↓
trace(observation_received)
    ↓
BuildModelRequest(config-driven prompt)
    ↓
trace(model_request_started)
    ↓
Provider.Generate(with timeout)
    ↓
trace(model_response_received)
    ↓
Resolve ToolCall(speak / emote)
    ↓
trace(tool_call_selected)
    ↓
trace(action_submit_started)
    ↓
Environment.SubmitAction
    ↓
ActionResult
    ↓
trace(action_result_received)
    ↓
trace(turn_completed / turn_failed)
```

------

# 6. Turn Trace 设计

Phase2 增加 Runtime 内部 trace，不进入 GameAgent Protocol v1alpha1。

更详细的 ID 语义、标准事件名、recorder 分层和验收标准见：

```text
docs/phase2/GameAgent MVP0 Phase2 Trace 链路观测设计.md
```

原因：

```text
Protocol 描述 Runtime 与 Adapter 的通信 contract。

Trace 描述 Runtime 内部 Agent Turn 的执行过程。

二者职责不同，不应混在一起。
```

建议新增包：

```text
runtime/internal/trace
```

核心结构：

```go
type Recorder interface {
    Record(event Event)
    Close(ctx context.Context) error
}

type EventName string

type Fields map[string]any

type EventData struct {
    ActionID string
    Tool     string
    Fields   Fields
}

type TurnTracer interface {
    Emit(name EventName, data EventData)
    Complete(data EventData)
    Fail(stage string, reason string, err error, data EventData)
}

type Event struct {
    SchemaVersion int       `json:"schema_version"`
    TraceID       string    `json:"trace_id"`
    TurnID        string    `json:"turn_id"`
    Seq           uint32    `json:"seq"`
    Event         EventName `json:"event"`
    Time          time.Time `json:"time"`
    ElapsedMS     int64     `json:"elapsed_ms"`

    GameID    string `json:"game_id,omitempty"`
    SaveID    string `json:"save_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    EventID   string `json:"event_id,omitempty"`
    EventType string `json:"event_type,omitempty"`
    EntityID  string `json:"entity_id,omitempty"`
    ActionID  string `json:"action_id,omitempty"`
    Tool      string `json:"tool,omitempty"`

    Stage        string         `json:"stage,omitempty"`
    Reason       string         `json:"reason,omitempty"`
    ErrorMessage string         `json:"error,omitempty"`
    Fields       Fields `json:"fields,omitempty"`
}
```

Phase2 默认实现 JSONL recorder，trace event 直接写入文件，不输出到 Runtime 终端：

```text
runtime/.local/traces.jsonl
```

每条 trace 是一行 JSON：

```json
{
  "schema_version": 1,
  "trace_id": "turn_...",
  "turn_id": "turn_...",
  "seq": 6,
  "event": "tool_call_selected",
  "time": "2026-08-15T17:00:00Z",
  "elapsed_ms": 42,
  "entity_id": "npc:Linus",
  "tool": "speak",
  "fields": {
    "tool_schema_count": 2
  }
}
```

Recorder 必须采用轻量非阻塞设计：

```text
AgentLoop 只做 O(1) enqueue。

JSON encode / file write 在单独 writer goroutine 中执行。

队列满时允许丢弃 trace event，不能阻塞游戏 LLM 主链路。

Record 热路径只允许非阻塞 enqueue 和 atomic dropped count，不打印每次 drop。
```

`game_id / session_id` 来自 `AdapterHello`，由 `gateway.Connect` 在握手阶段保存为 connection context；`save_id` 来自触发本轮 turn 的 `GameEvent`：

```text
game_id    = AdapterHello.GameId
session_id = AdapterHello.SessionId
save_id    = GameEvent.SaveId
```

之后 gateway 调用 AgentLoop 时应传入该 connection context，或通过 EnvironmentSession 注入。AgentLoop 创建 TurnTracer 时绑定 `game_id / save_id / session_id / event_id / event_type / entity_id` 等固定上下文，`Emit / Complete / Fail` 只传阶段动态字段。

协议兼容性说明：

```text
AdapterHello.instance_id -> session_id 是 v1alpha breaking rename。
Runtime 与 Adapter 必须同步升级。
后续开放第三方 Adapter 时，应使用新 tag + deprecated 旧字段，而不是复用 tag。
```

Trace 的实现契约：

```text
TurnTracer 发射语义和 JSONL 落盘语义分离。

TurnTracer 内部 seq 连续递增，并保证一轮 turn 最多发射一个终态。

TurnTracer terminal event 必须是最后一个发射事件；Complete / Fail 成功后，后续 Emit / Complete / Fail 全部 no-op。

JSONL 是 best effort，已落盘事件按 seq 递增，但允许因为 drop / 写入失败 / 崩溃出现缺口或缺少终态。

正常无 drop 时，JSONL 应包含完整 timeline 和唯一终态。

traces.jsonl 是派生观测数据，不是 Runtime / Agent / 游戏状态的 source of truth。

Fields 在 Emit / Record 后所有权转移给 trace 系统，调用方不得继续修改或复用同一个 map。

NewJSONLRecorder 失败时降级为 NoopRecorder 并写一次普通日志，不因为 trace 文件不可写导致 Runtime 启动失败。

Close 负责 drain / flush，必须幂等；Close 后 Record 直接丢弃事件。
```

Trace Recorder 属于 best-effort Observer，不是功能性 Hook：

```text
Observer 不参与 AgentLoop 状态一致性。
Observer 不允许产生背压。
Observer 失败不改变 AgentLoop 主结果。

未来如果需要权限、策略拦截、tool preflight 这类会改变 Agent 行为的扩展点，应单独设计 Hook，不复用 Trace Recorder 接口。
```

------

# 7. Turn Lifecycle

Phase2 建议显式引入 Turn 概念。

`message_id`、`correlation_id`、`action_id` 都不能表示一次完整 Agent Turn。

新增：

```text
turn_id
    表示一次 GameEvent 触发的一轮 Agent 执行。

trace_id
    表示一组可观测事件的关联 ID。Phase2 固定 trace_id == turn_id，后续进入跨 turn trace 时再拆分。
```

GameAgent Turn 的边界：

```text
GameAgent Turn 是一个有效 GameEvent 触发的一次完整 AgentLoop。

未来 ReAct 的多次 model -> tool/action 循环属于同一个 GameAgent Turn 下的多个 step。

届时通过 step_index / tool_call_id / attempt 扩展，不重新定义 turn_id。
```

Turn 状态：

```text
started
observing
thinking
acting
completed
failed
```

Phase2 不需要实现复杂状态机，可以先用 trace event 表达状态变化。

TurnTracer 发射层每个 turn 必须保证终态唯一，并且终态必须是最后一个发射事件：

```text
turn_completed XOR turn_failed
terminal event is final event
```

`ActionResult.status == SUCCEEDED` 时记录 `turn_completed`。

`ActionResult.status != SUCCEEDED` 时记录 `turn_failed`，不再记录 `turn_completed`：

```text
stage=action
reason=action_result_failed
fields.action_status=FAILED / REJECTED / CANCELED
```

------

# 8. Prompt / Config 设计

Phase1 的 Prompt 还比较硬编码。

Phase2 目标是让常见行为通过配置调整：

```text
输出语言
NPC 说话风格
回复长度
是否强制使用 tool
LLM timeout
```

建议新增配置文件：

```text
runtime/config/agent.json
```

示例：

```json
{
  "language": "zh-CN",
  "npc_style": "自然、简短、符合 Stardew Valley NPC 的语气",
  "max_reply_chars": 80,
  "tool_policy": "best_effort_hint",
  "turn_timeout_ms": 15000,
  "llm_timeout_ms": 8000,
  "observe_timeout_ms": 3000,
  "action_timeout_ms": 3000
}
```

`tool_policy` 在 Phase2 只作为 prompt hint。OpenAI Provider 后续可以把它映射成 `tool_choice=required`，但 DeepSeek `deepseek-v4-flash` 当前不支持强制 `tool_choice=required`，因此不能承诺 provider 层强制执行。

`turn_timeout_ms` 是整轮 turn 的全局硬上限，应大于常见阶段耗时组合。阶段 timeout 控制局部等待，turn timeout 控制整轮兜底。

本地私密覆盖文件：

```text
runtime/config/agent.local.json
```

`*.local.json` 已加入 `.gitignore`。

Prompt 构造仍放在：

```text
runtime/internal/agent/prompt.go
```

但 `BuildSystemPrompt` 应读取配置对象，而不是硬编码语言和风格。

------

# 9. Timeout 策略

Phase1 已知缺口：

```text
LLM Provider 暂无独立 HTTP timeout。
```

Phase2 需要为 turn 的关键阶段增加 timeout。

建议分层：

```text
turn_timeout_ms
    一整轮 Agent Turn 的最大耗时。

observe_timeout_ms
    Environment.Observe 的最大等待时间。

llm_timeout_ms
    Provider.Generate 的最大等待时间。

action_timeout_ms
    Environment.SubmitAction 的最大等待时间。
```

实现原则：

```text
AgentLoop 创建 turn-level context。

Observe / Generate / SubmitAction 使用阶段性 context.WithTimeout。

阶段 context 必须从 turn-level context 派生，不能各自从 background context 创建；这样 turn_timeout 才能中断卡在任一阶段里的调用。

超时后写 turn_failed trace。

超时错误返回给 gateway 日志，不让 goroutine 永久挂住。
```

Phase2 不做自动重试。

## 9.1 Action Timeout 与 CancelActionRequest

Timeout 分为两层语义：

```text
bounded waiting
    Runtime 不再无限等待某个阶段返回。

best-effort cancellation
    对已经提交给 Adapter 的有副作用动作，Runtime 请求 Adapter 尽量取消。
```

Phase2 已经通过 turn / observe / llm / action 四层 timeout 解决 bounded waiting。

但 action 和 observe / llm 不同：

```text
Observe
    只是读取环境，没有游戏副作用；超时后 Runtime 放弃等待即可。

LLM Generate
    只发生在 Runtime 和模型服务之间，Adapter 还没收到游戏动作；超时后 Runtime 放弃等待即可。

Action
    ActionRequest 已经发送给 Adapter，可能改变游戏状态；超时后需要额外发送 CancelActionRequest。
```

因此 Phase2 后续需要补齐 Action timeout 的取消传播：

```text
Runtime 侧：
1. SubmitAction(ctx, req) 发送 ActionRequest。
2. 等待 ActionResult。
3. 如果收到 ActionResult，则正常返回。
4. 如果 ctx.Done()，则发送 CancelActionRequest(action_id=req.action_id, reason="action_timeout")。
5. SubmitAction 返回 ctx.Err()，AgentLoop 写 turn_failed(action_timeout)。
```

如果 turn_timeout 发生在 action 阶段，本质上也是 action 等待被父 context 取消，也应触发同一条 CancelActionRequest。

Adapter 侧负责 best-effort 安全取消：

```text
1. 收到 CancelActionRequest 后记录 cancelled action_id。
2. 执行 ActionRequest 前检查 action_id 是否已取消。
3. 如果已取消，不执行游戏动作，并返回或记录 ACTION_STATUS_CANCELLED。
4. 如果动作已经执行或不可撤销，不做回滚，只记录日志。
```

CancelActionRequest 不是事务回滚机制，而是：

```text
如果这个 action 还没真正执行，请不要再执行。
```

游戏体验降级仍由 Adapter 负责：

```text
Runtime timeout
    表示 Agent 不能继续等待。

Runtime cancel
    表示已经提交的 action 不应晚到乱执行。

Adapter fallback
    表示 AI 来不及时，游戏如何自然继续。
```

MVP0 不实现复杂动作事务、重试、补偿或执行中强制中断；只实现 action 执行前的 best-effort cancel gate。

------

# 10. Failure Semantics

Phase1 的 happy path 已经跑通。

Phase2 要明确失败如何收敛。

失败类型：

```text
no_target_entity
observe_timeout
observe_failed
llm_timeout
llm_failed
invalid_tool_call
unknown_tool
action_timeout
action_failed
stream_closed
```

要求：

```text
每一种失败都要写 trace。

AgentLoop.HandleEvent 的错误不能静默丢弃。

turn_failed 必须记录 stage / reason。error 只在存在底层技术错误时记录，业务失败不伪造 error。

ActionResult.status != SUCCEEDED 时记录 action_result_failed，并把 action_status / action_reason 等业务失败细节放入 fields。

沿用 Phase1 已有的 inbound Error -> fail pending 机制，Observe 失败不应再阻塞到连接断开；Phase2 要把这类失败写入 turn trace。

LLM timeout 不应导致 Runtime 无法处理下一次 GameEvent。
```

Phase2 不要求把失败反馈回游戏内 UI，但应在 Runtime / SMAPI 日志中可见。

------

# 11. 第二个 Tool：emote

Phase2 需要从单工具 `speak` 进入多工具。

建议新增：

```text
emote
```

而不是 `move_to`。

原因：

```text
emote 简单、同步、容易验证。

move_to 涉及寻路、异步动作、中断、状态机，不适合 Phase2。
```

`emote` 输入 schema：

```json
{
  "type": "object",
  "properties": {
    "emote": {
      "type": "string",
      "enum": ["happy", "sad", "surprised", "neutral"]
    }
  },
  "required": ["emote"],
  "additionalProperties": false
}
```

这个 schema 由 Adapter 上报，不由 Runtime 内置。这里写出来只是为了说明 Stardew Adapter 可以声明什么能力。

能力链路：

```text
Adapter CapabilityList:
    speak
    emote

Runtime ToolRegistry:
    从 Capability.input_schema_json 动态生成 ToolDefinition(speak)
    从 Capability.input_schema_json 动态生成 ToolDefinition(emote)

LLM ToolCall:
    speak(text)
    or
    emote(emote)

Runtime ActionRequest:
    capability = LLM 选择的 tool name
    arguments = LLM 返回的 tool arguments

Adapter:
    SpeakCapability
    EmoteCapability
```

验收：

```text
模型可以选择 speak 或 emote。

Runtime 不写死 speak / emote 注册逻辑。

Runtime 能把 Adapter 上报的 schema 原样注册成模型 tool。

Adapter 能执行两个 capability。

Trace 能记录选中了哪个 tool。
```

------

# 12. Capability Schema 演进

Phase1 当前事实：

```text
Adapter 上报 CapabilityList。

Runtime 主要读取 capability.name。

speak 的 ToolDefinition schema 仍由 Runtime registry 写死。
```

Phase2 要把 Capability schema 权威交还给 Adapter。

核心结论：

```text
Runtime 无条件信任 Adapter 上报的 Capability schema，并原样注册成 tool。

schema 的正确性由 Adapter 负责，不由 Runtime 判断。
```

原因：

```text
Adapter 是能力的唯一事实来源。

哪些工具存在、每个工具吃什么参数、在游戏里怎么执行，只有 Adapter 知道。

Runtime 对 Stardew 一无所知，也必须一无所知。

谁执行，谁定义 schema。
```

Phase2 修改目标：

```text
1. gateway 把完整 Capability 透传给 ToolRegistry。

2. ToolRegistry 使用 Capability.name / description / input_schema_json 注册 ToolDefinition。

3. 删除 speak 硬编码白名单。

4. 删除 Runtime 写死的 speak schema。

5. ToolCall 转 ActionRequest 时，capability 使用 ToolCall.name，arguments 原样透传。
```

Runtime 只做解析健壮性，不做 schema 语义判断：

```text
做：
    input_schema_json 必须能 json.Unmarshal。
    name 必须非空。
    schema 字符串不能超过合理大小。
    解析失败时跳过该 capability，并写普通 warning log。

不做：
    不校验 schema 语义。
    不改写 schema。
    不注入 additionalProperties。
    不设字段白名单。
    不设 enum 值域。
    不判断 emote 到底有哪些合法值。
```

这不是 Runtime 在审 Adapter，而是 Runtime 解析字符串时必要的错误处理。

Provider 层为了适配具体模型 API 可以做自己的格式转换。例如 OpenAI strict 模式可能需要在发送给 OpenAI 的 schema copy 上补 `additionalProperties:false`，这是 Provider 对 OpenAI API 的适配，不是 Runtime 对 Adapter schema 的不信任。

`ValidateToolCall` 的职责也要随之收窄：

```text
做：
    tool name 已注册。
    arguments 非 nil。
    arguments 能被 protocol Struct 承载。

不做：
    不按 schema 校验 required 字段。
    不校验 enum。
    不校验文本长度。
    不判断具体字段语义。
```

参数语义错误由 Adapter 在执行 capability 时判断，并通过 `ActionResult(FAILED)` 返回。

最终链路：

```text
Adapter 定义 capability + schema + 执行方式
    ↓
Runtime 信任透传并注册 tool
    ↓
LLM 看到 Adapter 定义的 tool
    ↓
Runtime 把 ToolCall 转成 ActionRequest
    ↓
Adapter 执行；失败由 Adapter 返回 ActionResult(FAILED)
```

------

# 13. Runtime 修改范围

建议新增：

```text
runtime/internal/trace
runtime/internal/agent/config.go
runtime/config/agent.json
```

建议修改：

```text
runtime/internal/agent/loop.go
    引入 turn_id、trace、timeout、failure semantics。
    接收 gateway 传入的 connection context。
    删除现有 fmt.Printf 占位日志，由 turn trace 取代。

runtime/internal/agent/prompt.go
    使用 AgentConfig 构造 System Prompt。

runtime/internal/tool/registry.go
    接收完整 Capability，并从 input_schema_json 动态注册 ToolDefinition。
    capability schema 解析失败时使用普通 warning log，不写入 turn trace。

runtime/internal/tool/environment_tool.go
    已基本满足通用转发；Phase2 只补测试，不作为主要改造点。

runtime/internal/gateway/gateway.go
    CapabilityList 不再裁剪成 []string，而是把完整 Capability 透传给 registry，并确保 turn 失败有日志。
    从 AdapterHello 保存 game_id / session_id，并传给 AgentLoop / TurnTracer；save_id 来自 GameEvent。

runtime/internal/llm/factory.go
    读取 timeout 相关配置，或让 AgentLoop 包装 timeout context。
```

------

# 14. Adapter 修改范围

建议新增：

```text
adapters/stardew/src/Capabilities/EmoteCapability.cs
```

建议修改：

```text
CapabilityCatalog
    上报 speak + emote。

RuntimeClient
    处理 ActionRequest(capability=emote)。

ProtocolMapper
    保持 entity_id 映射集中管理。

SMAPI 日志
    打印 tool/capability/action_id/status。
```

emote 的具体游戏表现可以先选择简单实现：

```text
使用 Stardew NPC.doEmote(int) 显示原生表情气泡。
```

如果不同表情名称到 int code 的映射不稳定，可以先在 Adapter 内部维护一个小映射表。没有准确对应关系的值，例如 `neutral`，不要硬映射成错误表情；Adapter 应返回 `ActionResult(FAILED)` 并写明 error。

------

# 15. 测试计划

Runtime 单元测试：

```text
TraceRecorder writes JSONL event with schema_version and snake_case fields
TraceRecorder non-blocking enqueue does not block AgentLoop when queue is full
TraceRecorder records dropped count without logging in hot Record path
JSONL persisted seq is increasing and may have gaps when drops occur
JSONLRecorder Close drains, flushes, and is idempotent
TurnTracer emission seq is continuous
TurnTracer emits at most one terminal event
TurnTracer terminal event is final event
TurnTracer ignores Emit after Complete
TurnTracer ignores Emit after Fail
TurnTracer ignores Fail after Complete
TurnTracer ignores Complete after Fail
TurnTracer Fail accepts nil error
AgentLoop creates turn_id
AgentLoop records turn_completed
AgentLoop records turn_failed on provider error
AgentLoop records exactly one terminal event per turn
AgentLoop records turn_failed when ActionResult status is not SUCCEEDED
AgentLoop does not fabricate error for business ActionResult failure
AgentLoop times out slow provider
Prompt config controls language
ToolRegistry registers tools from Capability.input_schema_json
EnvironmentTool forwards generic ToolCall as ActionRequest without tool-specific branches
ValidateToolCall only checks registered tool name and non-nil arguments
Malformed capability schema is skipped and logged
```

Runtime 集成测试：

```text
fake adapter connects
capability list returns speak + emote
game event triggers turn
observation resolves
fake provider returns emote
runtime sends ActionRequest(capability=emote)
adapter returns ActionResult
trace contains full timeline in normal no-drop path
```

Adapter 手工测试：

```text
SMAPI 启动并连接 Runtime
CapabilityList sent: speak, emote
点击 Linus
真实 Provider 调用不报 schema error / 400 error，emote tool schema 能被 provider 正常接受
Runtime 返回中文 speak 或 emote
游戏内能看到效果
SMAPI 日志能看到完整 send/recv
Runtime trace 能按 turn_id 查到整轮执行
```

------

# 16. 实施顺序

Phase2 分层交付：

```text
P0 必须完成：
    动态 Capability -> ToolDefinition
    通用 ToolCall envelope 校验
    确认 BuildActionRequest 通用转发测试覆盖
    speak + emote 验证动态工具链路
    turn_id + 非阻塞、best-effort JSONL trace timeline
    TurnTracer 终态唯一且 terminal final
    llm_timeout

P1 建议完成：
    prompt language config
    observe_timeout
    action_timeout
    turn_failed trace 完整化

P2 可延后：
    更丰富的 agent config
    文件轮转 / 多进程 trace 文件策略
    Web trace viewer
    更复杂 capability
```

推荐开发顺序：

```text
1. gateway 将完整 Capability 透传给 ToolRegistry。

2. ToolRegistry 改为根据 Capability.input_schema_json 动态注册 ToolDefinition。

3. ValidateToolCall 收窄为 tool 已注册 + arguments 非 nil 的通用 envelope 校验。

4. Adapter CapabilityCatalog 增加 emote。

5. Adapter 实现 EmoteCapability。

6. 补动态 capability / emote / BuildActionRequest 通用转发的 Runtime 单测和 fake adapter integration test。

7. 增加 trace.Event / TurnTracer / 非阻塞 JSONL recorder。

8. AgentLoop 创建 turn_id，并写最小 started/completed/failed trace，保证终态唯一且 terminal final。

9. 给 Provider.Generate 增加 llm_timeout_ms 包装。

10. 定义 Phase2 AgentConfig。

11. Prompt 使用 AgentConfig，先实现中文输出。

12. 给 Observe / SubmitAction 增加阶段 timeout。

13. action_timeout 时 Runtime 发送 CancelActionRequest，Adapter 执行前做 best-effort cancel gate。

14. 真实 Stardew smoke test。
```

每一步都应该能独立验证。

不要等所有功能写完才进游戏测试。

------

# 17. Phase2 完成后的状态

Phase2 完成后，GameAgent 应该从：

```text
能跑通 one-turn demo
```

升级为：

```text
能稳定运行、能追踪每一轮、能配置模型行为、能支持多个简单工具的最小 Agent Turn Runtime。
```

这时再进入 Phase3，会更自然。

Phase3 可以考虑：

```text
短期 Memory
多轮 ReAct
turn resume
更多 NPC 事件
更复杂 capability
Web trace viewer
```

------

# 18. 一句话总结

> Phase2 的核心不是“补几个 bug”，而是把 Phase1 的真实 one-turn 链路沉淀成一个可观察、可配置、可扩展的最小 Agent Harness 基座。
