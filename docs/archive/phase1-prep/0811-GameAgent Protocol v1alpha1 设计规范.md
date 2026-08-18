# GameAgent Protocol v1alpha1 设计规范

> 说明：当前可落地的 Protobuf package 使用 `v1alpha1`。`v1alpha1` 表示 API 结构已经明确，但尚未承诺稳定兼容。

## 1. 定位

GameAgent Protocol 定义：

> **Game Environment Adapter 与 Runtime 之间的标准通信协议。**

它描述的是：

```text
Environment
Entity
Event
Observation
Capability
Action
```

而不是：

```text
LLM
Prompt
Memory
Agent Profile
Planner
Agent Permission
```

因此协议边界为：

```text
               Runtime Internal
────────────────────────────────────────
Agent / Memory / LLM / Tool / Permission
                    │
                    ↓
              Entity Binding

=============== Protocol ===============

 Environment / Entity / Event
 Observation / Capability / Action

========================================

                    ↓
              Game Adapter
                    ↓
             Stardew / SMAPI
```

Protocol 本身不关心 Runtime 后面究竟使用：

```text
LLM Agent
Behavior Tree
Rule Engine
Human Controller
```

这保证 Adapter 与 Agent 实现真正解耦。

------

# 2. 实现技术

v1alpha1 使用：

```text
Protocol Specification
        ↓
GameAgent Protocol

Schema
        ↓
Protocol Buffers

Transport
        ↓
gRPC Bidirectional Streaming
```

Proto package：

```protobuf
syntax = "proto3";

package gameagent.protocol.v1alpha1;

import "google/protobuf/struct.proto";

option csharp_namespace = "GameAgent.Protocol.V1Alpha1";
option go_package = "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1;protocolv1alpha1";
```

生成选项是协议的一部分：Go / C# 生成代码必须使用稳定 namespace，避免 Runtime 和 Adapter 引用不同包名。

MVP 落地目录：

```text
protocol/
└── proto/
    └── gameagent.proto
```

早期先使用单文件，降低生成和引用复杂度。

接口稳定后可拆分为：

```text
protocol/
└── gameagent/
    └── v1alpha1/
        ├── common.proto
        ├── session.proto
        ├── event.proto
        ├── observation.proto
        ├── capability.proto
        ├── action.proto
        └── gateway.proto
```

拆分只改变文件组织，不改变 package 语义。

------

# 3. Environment Session

Protocol 的连接单位是：

> **Environment Session**

而不是 Agent。

一个 Environment Session 表示：

```text
一个当前在线的游戏实例
+
当前游戏存档
+
对应 Game Adapter
```

例如：

```text
Stardew Valley
Farm_001
GameAgent.Stardew Adapter
```

连接模型：

```text
Game
 ↓
Adapter
 ↓
gRPC Bidirectional Stream
 ↓
Runtime
```

一个 Stream：

```text
=
一个 Environment Session
```

同一个 Environment 内可以存在多个 Entity。

例如：

```text
npc:Abigail
npc:Sam
npc:Sebastian
player:local
```

------

# 4. Session 建立

Adapter 必须主动连接 Runtime。

```protobuf
service GameAgentGateway {
  rpc Connect(
    stream AdapterMessage
  ) returns (
    stream RuntimeMessage
  );
}
```

建立连接后，Adapter 第一条有效业务消息必须为：

```text
AdapterHello
```

例如：

```protobuf
message AdapterHello {
  string adapter_id = 1;
  string adapter_version = 2;
  string protocol_version = 3;

  string game_id = 4;
  string game_version = 5;

  string instance_id = 6;
  string save_id = 7;
}
```

例如：

```json
{
  "adapter_id": "gameagent-stardew",
  "adapter_version": "0.1.0",
  "protocol_version": "v1alpha1",
  "game_id": "stardew-valley",
  "game_version": "1.x",
  "instance_id": "instance-f81c",
  "save_id": "Farm_001"
}
```

------

# 5. EnvironmentReady

Runtime 接受 Adapter 后返回：

```text
EnvironmentReady
```

表示：

> Environment Session 已成功建立，可以开始业务通信。

建议：

```protobuf
message EnvironmentReady {
  string environment_id = 1;
  int64 server_time_unix_ms = 2;
}
```

链路：

```text
Adapter
   │
   │ AdapterHello
   ↓
Runtime
   │
   │ 创建 Environment Session
   ↓
Adapter
   │
   │ EnvironmentReady
   ↓
Ready
```

如果 Hello 校验失败，例如：

```text
协议版本不兼容
认证失败
Game 不支持
Adapter 不兼容
```

Runtime 返回：

```text
Error
```

然后关闭 Stream。

不使用：

```text
accepted=false
```

这种双状态表达。

成功就是：

```text
EnvironmentReady
```

失败就是：

```text
Error + close stream
```

------

# 6. environment_id

`environment_id` 由 Runtime 分配。

例如：

```text
env_01JQ8Y...
```

Environment 建立以后：

> 后续每条 Stream Message 不需要重复携带 `environment_id`。

因为：

```text
Stream
=
Environment Session
```

Runtime 可以直接根据当前 Stream 获取 Environment。

这样避免：

```text
Stream 属于 env_A
但消息字段写 env_B
```

这类不一致。

------

# 7. Entity

Entity 表示：

> 游戏世界中的稳定对象身份。

定义：

```protobuf
message EntityRef {
  string entity_id = 1;
  string entity_type = 2;
  string display_name = 3;
}
```

例如：

```json
{
  "entity_id": "npc:Abigail",
  "entity_type": "npc",
  "display_name": "阿比盖尔"
}
```

------

# 8. entity_id 规范

`entity_id` 必须满足：

```text
Stable
Opaque
Adapter-defined
Non-localized
```

即：

> 同一个游戏存档中的同一个 Entity，应保持稳定 ID。

不能依赖本地化显示名称。

例如中文游戏仍然应该：

```text
entity_id:
npc:Abigail

display_name:
阿比盖尔
```

而不是：

```text
entity_id:
npc:阿比盖尔
```

Runtime 必须把：

```text
entity_id
```

视为 opaque string。

不能依赖：

```text
npc:
player:
monster:
```

这样的字符串前缀实现核心业务逻辑。

这些只是 Adapter 可以采用的命名习惯。

------

# 9. GameTime

统一定义游戏内时间：

```protobuf
message GameTime {
  optional int32 year = 1;
  optional int32 season = 2;
  optional int32 day = 3;

  optional int32 hour = 4;
  optional int32 minute = 5;

  optional int64 tick = 6;
}
```

不同游戏无法提供的字段允许留空。使用 `optional` 是为了区分：

```text
字段未知 / 游戏不支持
```

和：

```text
字段值确实为 0
```

Protocol 不要求所有游戏拥有 Stardew 式时间概念。

------

# 10. GameEvent

GameEvent 表示：

> 游戏世界中已经发生的事实。

例如：

```text
player_interacted_with_npc

player_gave_item

day_started

player_entered_location

npc_reached_location
```

定义：

```protobuf
message GameEvent {
  string event_id = 1;

  string event_type = 2;

  repeated EntityRef entities = 3;

  GameTime game_time = 4;

  google.protobuf.Struct payload = 5;

  uint64 sequence = 6;
}
```

动态事件数据统一使用：

```protobuf
google.protobuf.Struct payload
```

例如：

```json
{
  "event_id": "evt_1001",
  "event_type": "player_gave_item",

  "entities": [
    {
      "entity_id": "player:local",
      "entity_type": "player"
    },
    {
      "entity_id": "npc:Abigail",
      "entity_type": "npc"
    }
  ],

  "payload": {
    "item_id": "amethyst",
    "amount": 1
  },

  "sequence": 1204
}
```

------

# 11. GameEvent 幂等

所有 GameEvent 必须提供：

```text
event_id
```

Protocol 默认：

> Event Delivery 使用 At-Least-Once 语义。

即 Adapter 可以在无法确认 Runtime 是否成功处理时重新发送。

Runtime 应根据：

```text
environment session
+
event_id
```

进行幂等处理。

同一个 Event 不应该：

```text
重复写 Memory

重复触发 Agent

重复产生 Action
```

------

# 12. sequence

同一个 Environment 内：

```text
GameEvent.sequence
```

应该单调递增。

例如：

```text
1201
1202
1203
1204
```

用途：

```text
事件排序
检测缺失
断线调试
Trace
Replay 能力预留
```

Transport 本身虽然保证 Stream 内消息顺序，但 Protocol 不完全依赖 Transport 顺序表达业务语义。

------

# 13. EventAck

Runtime 可以确认 Event：

```protobuf
enum EventAckStatus {
  EVENT_ACK_STATUS_UNSPECIFIED = 0;
  EVENT_ACK_STATUS_ACCEPTED = 1;
  EVENT_ACK_STATUS_DUPLICATE = 2;
  EVENT_ACK_STATUS_REJECTED = 3;
}
```

```protobuf
message EventAck {
  string event_id = 1;

  EventAckStatus status = 2;

  Error error = 3;
}
```

主要用于 Adapter：

```text
本地 pending event queue
```

清理。

语义：

```text
ACCEPTED
Runtime 已经可靠接纳该事件，并记录 event_id 用于后续幂等处理。
Adapter 收到该 ACK 后，可以安全删除本地 pending event。

DUPLICATE
Runtime 已处理过相同 event_id，Adapter 可以清理本地 pending event。

REJECTED
Runtime 拒绝该事件，error 说明原因。
```

Runtime 不得在仅网络收到 Event、但事件尚未进入可靠处理边界时发送 `EVENT_ACK_STATUS_ACCEPTED`。

Protocol 不规定 Runtime 使用 PostgreSQL、SQLite、本地 WAL 或其他具体存储实现；Protocol 只定义 ACK 语义和投递保证。

v1alpha1 不要求实现复杂 Event Replay。

------

# 14. Observation

Observation 表示：

> 某 Entity 当前可以观察到的游戏世界状态。

请求：

```protobuf
message ObserveRequest {
  string entity_id = 1;
}
```

响应：

```protobuf
message Observation {
  string entity_id = 1;

  uint64 revision = 2;

  GameTime game_time = 3;

  google.protobuf.Struct state = 4;

  repeated EntityRef nearby_entities = 5;

  google.protobuf.Struct extensions = 6;
}
```

例如：

```json
{
  "entity_id": "npc:Abigail",

  "revision": 250,

  "state": {
    "location": "Town",
    "weather": "rain",
    "season": "fall",
    "current_action": null
  },

  "nearby_entities": [
    {
      "entity_id": "player:local",
      "entity_type": "player",
      "display_name": "Player"
    }
  ]
}
```

------

# 15. GameEvent 与 Observation

必须明确：

```text
GameEvent
=
发生了什么变化
```

例如：

```text
玩家刚刚进入 Town。
```

而：

```text
Observation
=
当前是什么状态
```

例如：

```text
Abigail 在 Town；
玩家也在 Town；
当前下雨。
```

推荐 Agent 触发模式：

```text
GameEvent
    ↓
Runtime decides whether relevant
    ↓
ObserveRequest
    ↓
Observation
    ↓
Agent Step
```

------

# 16. Capability

Capability 表示：

> 当前 Adapter 能够为游戏 Entity 提供的游戏操作能力。

例如：

```text
speak
move_to
give_item
schedule_activity
```

定义：

```protobuf
enum ExecutionMode {
  EXECUTION_MODE_UNSPECIFIED = 0;
  EXECUTION_MODE_SYNC = 1;
  EXECUTION_MODE_ASYNC = 2;
}

message Capability {
  string name = 1;

  string version = 2;

  string description = 3;

  string input_schema_json = 4;

  ExecutionMode execution_mode = 5;

  google.protobuf.Struct extensions = 6;
}
```

其中：

```text
input_schema_json
```

必须是合法 JSON Schema 字符串。

------

# 17. 为什么 JSON Schema 使用 string

Capability 最终需要被 Runtime 转换成：

```text
LLM Tool Schema
```

而 JSON Schema 本身就是一个完整 JSON 文档。

例如：

```json
{
  "type": "object",

  "properties": {
    "location": {
      "type": "string"
    }
  },

  "required": [
    "location"
  ]
}
```

因此直接：

```protobuf
string input_schema_json
```

避免在 Protobuf 中重新实现一套 JSON Schema 类型系统。

------

# 18. CapabilityRequest

定义：

```protobuf
message CapabilityRequest {
  optional string entity_id = 1;
}
```

如果：

```text
entity_id 已设置
```

表示：

> 查询当前 Entity 可执行能力。

例如：

```text
npc:Abigail
```

返回：

```text
move_to
speak
give_item
```

如果：

```text
entity_id 未设置
```

可表示：

> 查询 Environment-level Capability。

是否实现 Environment-level 查询可由 Adapter 决定。

------

# 19. CapabilityList

```protobuf
message CapabilityList {
  optional string entity_id = 1;

  repeated Capability capabilities = 2;

  uint64 revision = 3;
}
```

`revision` 用于表示 Capability 集是否发生变化。

------

# 20. Capability ≠ Permission ≠ Tool

三层必须严格区分。

## Capability

```text
游戏技术上能不能做
```

由 Adapter 决定。

------

## Permission

```text
Runtime 是否允许当前 Agent 做
```

由 Runtime 决定。

------

## Tool

```text
最终暴露给 LLM 的能力
```

由 Runtime 动态生成。

流程：

```text
Adapter Capabilities
        ↓
Runtime Capability Registry
        ↓
Permission Filter
        ↓
Tool Registry
        ↓
LLM Tool Schema
```

Adapter 永远不直接定义 LLM Tool。

------

# 21. Capability → Tool

Adapter：

```text
move_to
```

Capability：

```json
{
  "name": "move_to",
  "description": "Move entity to target location.",
  "input_schema_json": "{...}",
  "execution_mode": "ASYNC"
}
```

Runtime：

```text
parse JSON Schema
↓
Permission Filter
↓
generate provider-specific tool schema
```

例如转换成 OpenAI/Anthropic 对应 Tool 定义。

------

# 22. ToolCall 不属于 Game Protocol

LLM 返回的：

```text
ToolCall
```

属于：

```text
Runtime Internal
```

例如：

```json
{
  "name": "move_to",

  "arguments": {
    "location": "Beach"
  }
}
```

Game Protocol 不直接传输 LLM 原始 ToolCall。

Runtime 必须执行：

```text
ToolCall
↓
Schema Validation
↓
Permission Validation
↓
Capability Validation
↓
ActionRequest
```

------

# 23. ActionRequest

ActionRequest 表示：

> Runtime 请求 Adapter 对某个 Entity 执行 Capability。

注意：

> v1alpha1 不要求向 Adapter 暴露 `agent_id`。

定义：

```protobuf
message ActionRequest {
  string action_id = 1;

  string entity_id = 2;

  string capability = 3;

  google.protobuf.Struct arguments = 4;

  google.protobuf.Struct extensions = 5;
}
```

例如：

```json
{
  "action_id": "act_1001",

  "entity_id": "npc:Abigail",

  "capability": "move_to",

  "arguments": {
    "location": "Beach"
  }
}
```

------

# 24. 为什么 Protocol 不需要 agent_id

Runtime 内部可能存在：

```text
agent_abigail_001
        ↓
Entity Binding
        ↓
npc:Abigail
```

但 Adapter 真正需要知道的只是：

```text
哪个游戏 Entity
执行哪个 Capability
```

而不是：

```text
哪个 LLM Agent 发出的命令
```

因此：

```text
Agent
```

属于 Runtime 概念。

```text
Entity
```

属于 Game Protocol 概念。

这样未来 Runtime 即使不用 LLM，而改为：

```text
Behavior Tree
Rule Engine
Human Controller
```

Adapter 仍然完全无需修改。

------

# 25. ActionStatus

```protobuf
enum ActionStatus {
  ACTION_STATUS_UNSPECIFIED = 0;

  ACTION_STATUS_PENDING = 1;

  ACTION_STATUS_ACCEPTED = 2;

  ACTION_STATUS_RUNNING = 3;

  ACTION_STATUS_SUCCEEDED = 4;

  ACTION_STATUS_FAILED = 5;

  ACTION_STATUS_INTERRUPTED = 6;

  ACTION_STATUS_CANCELLED = 7;

  ACTION_STATUS_REJECTED = 8;
}
```

------

# 26. PENDING

`PENDING` 主要属于 Runtime 内部状态：

```text
Action 已创建
但 Adapter 尚未确认
```

Adapter 通常不需要发送：

```text
PENDING
```

------

# 27. ActionStatusUpdate

用于：

> Action 非终态状态变化。

定义：

```protobuf
message ActionStatusUpdate {
  string action_id = 1;

  ActionStatus status = 2;

  google.protobuf.Struct metadata = 3;
}
```

v1alpha1 Adapter 主要通过该消息发送：

```text
ACCEPTED

RUNNING
```

例如：

```text
ActionRequest
      ↓
ActionStatusUpdate(ACCEPTED)
      ↓
ActionStatusUpdate(RUNNING)
```

因此 v1alpha1 不定义独立 `ActionAccepted` 消息。

------

# 28. ActionResult

`ActionResult` 只表示：

> Action 已进入终态。

终态包括：

```text
SUCCEEDED

FAILED

INTERRUPTED

CANCELLED

REJECTED
```

定义：

```protobuf
message ActionResult {
  string action_id = 1;

  ActionStatus status = 2;

  google.protobuf.Struct output = 3;

  Error error = 4;
}
```

例如成功：

```json
{
  "action_id": "act_1001",

  "status": "SUCCEEDED",

  "output": {
    "location": "Beach"
  }
}
```

失败：

```json
{
  "action_id": "act_1001",

  "status": "FAILED",

  "error": {
    "code": "PATH_NOT_FOUND",
    "message": "Entity cannot reach target location."
  }
}
```

------

# 29. Action 状态规范

推荐完整生命周期：

```text
Runtime:

PENDING
   ↓

Adapter:

ACCEPTED
   ↓
RUNNING
   ↓

SUCCEEDED
```

失败则：

```text
ACCEPTED
   ↓
RUNNING
   ↓
FAILED
```

拒绝：

```text
PENDING
   ↓
REJECTED
```

------

# 30. ActionStatusUpdate 与 ActionResult 边界

正式约定：

```text
ActionStatusUpdate
=
非终态通知
```

主要：

```text
ACCEPTED
RUNNING
```

而：

```text
ActionResult
=
终态结果
```

包括：

```text
SUCCEEDED
FAILED
INTERRUPTED
CANCELLED
REJECTED
```

不发送：

```text
ActionStatusUpdate(SUCCEEDED)
+
ActionResult(SUCCEEDED)
```

这种重复消息。

------

# 31. CancelActionRequest

Runtime 可以发送：

```protobuf
message CancelActionRequest {
  string action_id = 1;

  string reason = 2;
}
```

Adapter 是否支持取消具体 Action，由 Capability / 实现决定。

取消成功时，Adapter 必须让原 Action 进入终态：

```text
ActionResult(CANCELLED)
```

取消失败时，Adapter 返回：

```text
Error
correlation_id = CancelActionRequest.message_id
```

原 Action 状态保持不变，后续仍可能继续：

```text
RUNNING
↓
SUCCEEDED / FAILED / INTERRUPTED
```

正式约定：

```text
Cancel rejected
≠
Action rejected
```

因此取消请求被拒绝时，不得发送：

```text
ActionResult(REJECTED)
```

因为 `REJECTED` 属于原 Action 生命周期，只表示原始 `ActionRequest` 在执行前被 Adapter 拒绝。

v1alpha1 不新增：

```text
CancelActionResult
```

取消请求本身的失败，用带 `correlation_id` 的 `Error` 已足够表达。

------

# 32. Error

定义统一错误结构。`Error` 可以承载 Action Error 或 Protocol Error，具体类别由 `code` 约定表达。

```protobuf
message Error {
  string code = 1;
  string message = 2;

  google.protobuf.Struct details = 3;
}
```

例如：

```json
{
  "code": "PATH_NOT_FOUND",
  "message": "Entity cannot reach requested location.",

  "details": {
    "entity_id": "npc:Abigail",
    "location": "UnknownMap"
  }
}
```

------

# 33. Error 分类

建议区分：

## Validation Error

Runtime 内部：

```text
Tool arguments 不符合 JSON Schema
```

通常不会进入 Adapter。

------

## Action Error

例如：

```text
PATH_NOT_FOUND
ENTITY_BUSY
INVALID_TARGET
```

通过：

```text
ActionResult
```

表达。

------

## Protocol Error

例如：

```text
UNSUPPORTED_PROTOCOL_VERSION

INVALID_MESSAGE

SESSION_NOT_READY
```

通过：

```text
Error
```

表达。

------

## Transport Error

例如：

```text
gRPC Stream disconnected
```

属于 Transport 层，不使用业务 Message 模拟。

------

# 34. Heartbeat

定义：

```protobuf
message Heartbeat {
  int64 timestamp_unix_ms = 1;

  uint64 last_event_sequence = 2;
}
```

Adapter 定期发送。

Runtime 用于判断 Environment：

```text
ONLINE

OFFLINE
```

Heartbeat 属于：

```text
Environment Session 生命周期
```

而不是 Agent 行为。

------

# 35. AdapterMessage

Adapter → Runtime：

```protobuf
message AdapterMessage {
  string message_id = 1;

  string correlation_id = 2;

  oneof payload {
    AdapterHello hello = 10;

    GameEvent event = 11;

    Observation observation = 12;

    CapabilityList capabilities = 13;

    ActionStatusUpdate action_status = 14;

    ActionResult action_result = 15;

    Heartbeat heartbeat = 16;

    Error error = 17;
  }
}
```

------

# 36. RuntimeMessage

Runtime → Adapter：

```protobuf
message RuntimeMessage {
  string message_id = 1;

  string correlation_id = 2;

  oneof payload {
    EnvironmentReady environment_ready = 10;

    ObserveRequest observe = 11;

    CapabilityRequest capability_request = 12;

    ActionRequest action = 13;

    CancelActionRequest cancel_action = 14;

    EventAck event_ack = 15;

    Error error = 16;
  }
}
```

------

# 37. message_id

每条消息拥有唯一：

```text
message_id
```

用于：

```text
请求关联

日志

Trace

Debug

故障排查
```

它和：

```text
event_id
action_id
```

不是同一个概念。

例如：

```text
message_id
=
一次网络消息身份

event_id
=
一个游戏事件身份

action_id
=
一次 Action 生命周期身份
```

------

# 38. correlation_id

请求/响应型消息通过：

```text
correlation_id
```

建立关联。

例如：

```text
Runtime:

ObserveRequest
message_id = msg_100
```

Adapter：

```text
Observation
message_id = msg_101
correlation_id = msg_100
```

Runtime 由此知道：

> Observation 对应哪个 ObserveRequest。

同理：

```text
CapabilityRequest
→
CapabilityList
```

也使用 correlation。

------

# 39. Action 不完全依赖 correlation_id

Action 本身拥有稳定：

```text
action_id
```

因此：

```text
ActionStatusUpdate
ActionResult
```

主要根据：

```text
action_id
```

关联整个 Action 生命周期。

可以同时带：

```text
correlation_id = ActionRequest.message_id
```

用于 Trace。

但业务主关联键为：

```text
action_id
```

------

# 40. 动态业务数据规范

v1alpha1 正式规定：

## 使用 `google.protobuf.Struct`

用于：

```text
GameEvent.payload

Observation.state

Observation.extensions

ActionRequest.arguments

ActionRequest.extensions

ActionStatusUpdate.metadata

ActionResult.output

Error.details
```

原因：

```text
跨游戏字段动态

Go / C# 都有 Protobuf 支持

无需二次 JSON Parse

保持结构化数据语义
```

数值规范：

```text
google.protobuf.Struct 遵循 JSON Value 模型，number 具有浮点数语义。

精度敏感的 int64、稳定 ID、雪花 ID、时间戳等，不得放入 Struct 的 number 字段。

这类值应使用正式 protobuf 字段，或在动态 payload 中编码为 string。
```

不推荐：

```json
{
  "player_id": 9223372036854775807
}
```

推荐：

```json
{
  "player_id": "9223372036854775807"
}
```

普通小整数可以继续使用 number：

```json
{
  "amount": 3,
  "friendship": 250
}
```

------

## 使用 JSON string

仅主要用于：

```text
Capability.input_schema_json
```

因为：

> JSON Schema 本身就是 JSON 格式规范。

Protocol 不重新定义 JSON Schema protobuf 类型。

------

# 41. Extensions

部分消息预留：

```text
extensions
```

使用：

```protobuf
google.protobuf.Struct
```

允许 Adapter 提供非核心、游戏特定元数据。

但原则是：

> Runtime 核心逻辑不能依赖 Stardew-specific extension 才能正常运行。

例如允许：

```json
{
  "extensions": {
    "stardew_schedule_key": "...",
    "smapi_location_type": "..."
  }
}
```

但通用 Runtime 不应写：

```text
if extensions.smapi_location_type ...
```

这种核心逻辑。

------

# 42. Protocol Version

当前：

```text
v1alpha1
```

表示：

> Protocol 已具备明确 API 结构，但尚未稳定。

未来：

```text
v1alpha1
↓
v1alpha2
↓
v1beta1
↓
v1
```

Breaking Change 可以通过新 package 引入。

例如：

```protobuf
gameagent.protocol.v1alpha2
```

稳定之后：

```protobuf
gameagent.protocol.v1
```

------

# 43. Disconnect

v1alpha1 不实现复杂 Session Resume。

Stream 断开：

```text
Environment
↓
OFFLINE
```

Runtime 中所有等待请求：

```text
UNAVAILABLE
```

运行中的 Action：

```text
UNKNOWN
```

重新连接：

```text
AdapterHello
↓
EnvironmentReady
↓
Capability Refresh
↓
Observation Refresh
```

后续版本再考虑：

```text
ResumeToken

Event Replay

Running Action Recovery
```

------

# 44. 本地与远程部署

Protocol 本身完全一致。

## 本地

```text
Stardew
↓
Adapter
↓
gRPC localhost
↓
Runtime
```

例如：

```text
127.0.0.1:50051
```

------

## 远程

```text
Stardew
↓
Adapter
↓
gRPC + TLS
↓
Remote Runtime
```

不同之处仅为：

```text
Endpoint
Authentication
TLS
Storage
```

不修改 Game Protocol。

------

# 45. Runtime Implementation Guidance

以下内容不属于 Game Protocol 本身，只作为 Runtime 实现建议。

## Agent Engine

服务器可以只有一套：

```text
Agent Engine
```

同时服务多个 Agent Context。

------

## Agent Context

不同 NPC / 用户拥有独立：

```text
Profile

Memory

Conversation History

Goal

Entity Binding
```

------

## Storage

v1alpha1 可以只支持：

```text
单机 PostgreSQL
```

或开发环境：

```text
SQLite
```

无需设计：

```text
Distributed DB
Redis Cluster
Runtime Cluster
```

------

## Tenant Isolation

Remote Runtime 负责：

```text
Authentication
↓
Tenant
↓
Environment Session
```

Tenant 不要求出现在每一条 Game Protocol Message 中。

------

# 46. v1alpha1 不包含的内容

明确不进入 Protocol：

```text
Agent Memory

Prompt

Conversation History

Persona

Planner

Agent Goal

LLM Provider

Tool Selection

Permission Policy

Evaluation

Tenant Storage

Database Schema
```

这些都属于：

```text
Runtime Architecture
```

------

# 47. 核心消息分类

## Domain

```text
EntityRef
GameTime

GameEvent
Observation

Capability

ActionRequest
ActionStatus
ActionResult
```

------

## Request / Response

```text
ObserveRequest

CapabilityRequest
CapabilityList

ActionStatusUpdate

CancelActionRequest

EventAck
```

------

## Session

```text
AdapterHello

EnvironmentReady

Heartbeat
```

------

## Envelope

```text
AdapterMessage

RuntimeMessage
```

------

## Common

```text
Error
```

------

# 48. 最短完整调用链

玩家点击 Abigail：

```text
Stardew
↓
Adapter
↓
GameEvent
```

Runtime：

```text
GameEvent
↓
Agent Context Resolver
↓
ObserveRequest
```

Adapter：

```text
Observation
```

Runtime 内部：

```text
Observation
+
Profile
+
Memory
+
Available Tools
↓
LLM
```

LLM：

```text
speak(...)
```

Runtime：

```text
ToolCall
↓
Schema Validation
↓
Permission
↓
ActionRequest
```

Protocol：

```json
{
  "action_id": "act_001",
  "entity_id": "npc:Abigail",
  "capability": "speak",

  "arguments": {
    "target_entity_id": "player:local",
    "text": "..."
  }
}
```

Adapter：

```text
ActionStatusUpdate
ACCEPTED
```

然后调用 Stardew：

```text
Dialogue API
```

完成后：

```text
ActionResult
SUCCEEDED
```

最终形成：

```text
GameEvent
↓
Observation
↓
Runtime Decision
↓
ToolCall
↓
ActionRequest
↓
Game Action
↓
ActionResult
```

------

# 49. v1alpha1 最终设计原则

### 一

```text
Protocol 描述 Environment，
而不是描述 Agent 实现。
```

### 二

```text
一个 Stream
=
一个 Environment Session。
```

### 三

```text
Entity 是 Game 概念。

Agent 是 Runtime 概念。
```

### 四

```text
Capability
=
游戏能做什么。

Permission
=
Runtime 允许什么。

Tool
=
LLM 最终看到什么。
```

### 五

```text
动态业务字段
=
google.protobuf.Struct
```

### 六

```text
JSON Schema
=
string input_schema_json
```

### 七

```text
ActionStatusUpdate
=
非终态。

ActionResult
=
终态。
```

### 八

```text
Adapter 主动连接 Runtime。
```

### 九

```text
本地和远程使用相同 Protocol。
```

### 十

```text
Runtime / Storage / Tenant 实现
不污染 Protocol 领域模型。
```

------

# 50. 一句话定义

> **GameAgent Protocol v1alpha1 是 Game Environment Adapter 与 Runtime 之间的通用环境通信协议，通过 Entity、Event、Observation、Capability 与异步 Action 生命周期描述智能控制系统与游戏世界之间的双向交互，并使用 gRPC Bidirectional Streaming 实现本地与远程环境的统一接入。**
