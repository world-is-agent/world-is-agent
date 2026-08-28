# GameAgent MVP0 Phase5.6 技术开发与验收方案

> **Status:** Implementation Baseline Draft
> **Date:** 2026-08-28
> **Scope:** Stardew Adapter Interaction Surface
> **Architecture Baseline:** GameAgent Runtime Architecture v0.3
> **Roadmap Baseline:** GameAgent Phase3-Phase8 阶段规划 v0.5
> **Protocol Baseline:** gameagent.protocol.v1alpha2 after Phase5
> **Previous Phase:** Phase5.5 Stardew Adapter Context Enrichment Accepted
> **Next Phase:** Phase6 Async Action Lifecycle and AgentTurn Resume
> **Reference:** ValleyTalk, Stardojo, SMAPI 4.5.2

---

# 1. 阶段目标

Phase5 已经完成有界 multi-step AgentTurn、ordered ToolCall batch、ToolResult transcript 和 settle control。

Phase5.5 已经完成 Stardew 当前事实模型，并通过 `Observation.state.stardew` 把时间、天气、人物、关系、场景和日程交给 Runtime。

Phase5.6 要建立 Stardew Adapter 的交互面：

```text
Player input / click
  -> Adapter GameEvent
  -> Runtime AgentTurn
  -> Environment ToolCall
  -> Adapter UI / NPC action
  -> ActionResult
  -> Turn settle
```

本阶段主要证明：

> **对话会话可以跨多个 AgentTurn 进行；Runtime 仍是通用认知中枢，Adapter 负责 Stardew 输入、UI 和 NPC 可见动作。**

---

# 2. 阶段结论

Phase5.6 做这些工作：

```text
1. 定义 Stardew 对话会话为多 Turn session。
2. 新增玩家对话事件 player_said_to_npc。
3. 新增 Adapter 内存 conversation state，并把最近对话行注入 Observation.state.stardew.conversation。
4. 新增 present_dialogue capability，用于展示 NPC 台词、最多 4 个玩家回复选项和自定义输入入口。
5. 新增 face_player capability，用于让 NPC 面向玩家。
6. 更新 speak / emote / present_dialogue 的工具描述，使模型能稳定选择对话展示或简单动作。
7. 新增 Stardew 对话 UI，支持选择回复和自由输入。
8. 更新 Runtime 配置提示词，使模型理解玩家输入事件、回复选项和当前可用能力。
9. 更新 Runtime memory 可见摘要，使 `present_dialogue` 的 NPC 台词可进入 Recent Memory。
10. 明确交互发起到 UI 展示之间的上下文校验边界，作为 Phase6 Turn lifecycle 的接入点。
```

Phase5.6 不做这些工作：

```text
Protocol 字段变更
Runtime async action lifecycle
同一 Turn 内等待玩家输入
ask_player / human-in-loop async tool
movement capability
ActionStatusUpdate / CancelActionRequest 接线
AgentDefinition store
canonical dialogue retrieval
long-term event memory persistence
ValleyTalk prompt builder 迁移
Adapter 内部 LLM 调用
Harmony patch 改写 Stardew 原生 Dialogue 流程
Stardojo player-centric inventory / menu / shop / craft actions
等待 LLM 期间冻结玩家或 NPC
Interaction Context Guard 执行态校验
```

---

# 3. Turn 与 Conversation 边界

## 3.1 Conversation 是跨 Turn 会话

`conversation_id` 是 Adapter 侧对话会话 ID。

`turn_id` 是 Runtime 侧单次 AgentTurn ID。

两者关系：

```text
conversation_id: conv_12
  turn_A: player_interacted_with_npc -> present_dialogue
  turn_B: player_said_to_npc        -> present_dialogue + emote
  turn_C: player_said_to_npc        -> face_player + present_dialogue
```

规则：

- 一个 `GameEvent` 启动一个新的 AgentTurn；
- 一个 `conversation_id` 可以关联多个 AgentTurn；
- 一个 AgentTurn 不等待玩家选择或输入；
- `present_dialogue` 的 ActionResult 表示 NPC 台词和回复入口已真正显示；
- 玩家选择或输入后，Adapter 发送新的 `player_said_to_npc` 事件；
- 同一 NPC 的事件继续由 Runtime `ExecutionLane` 串行处理。

## 3.2 Phase5.6 对话链路

点击 NPC：

```text
Player clicks NPC
  -> Adapter reserves or resumes conversation_id
  -> GameEvent(player_interacted_with_npc)
  -> EventAck(ACCEPTED)
  -> Adapter commits conversation open state
  -> Runtime Observe
  -> Model calls present_dialogue
  -> Adapter displays NPC line + reply options / free-text entry
  -> ActionResult(SUCCEEDED)
  -> Runtime settle
```

玩家回复：

```text
Player selects option or enters text
  -> Adapter creates pending player-line mutation
  -> GameEvent(player_said_to_npc)
  -> EventAck(ACCEPTED)
  -> Adapter commits player line to conversation state
  -> Runtime Observe, including conversation recent_lines
  -> Model calls present_dialogue / emote / face_player
  -> Adapter displays NPC line and commits NPC line
  -> ActionResult(SUCCEEDED)
  -> Runtime settle
```

## 3.3 Active Conversation

`active conversation` 表示 `ConversationStateStore` 中存在、未 close、未 reset 的当前 NPC 会话。

生命周期规则：

- `player_interacted_with_npc` 在没有 active conversation 时 reserve 新的 `conversation_id`；
- `player_interacted_with_npc` 在存在 active conversation 时复用当前 `conversation_id`；
- reserved conversation 只有在 `EventAck.ACCEPTED` 后进入 active 状态；
- `EventAck.REJECTED` 时丢弃 reserved conversation；
- `EventAck.DUPLICATE` 不创建、不重复写入 conversation state；
- `present_dialogue` 准备显示时预留 `conversation_id`，UI 真正显示后才追加 NPC line；
- 玩家通过 Close / Escape 放弃菜单时按 `conversation_id` 精确关闭 active conversation；
- 玩家提交 option / free text 时不关闭 active conversation；
- Adapter 抢占同一 NPC 的旧菜单时不关闭 active conversation；
- `DayStarted` 到达时执行 `ConversationStateStore.Clear()`，所有 `conversation_id` 失效；
- 新一天对同一 NPC 的首次交互总是创建新的 `conversation_id`；
- world change、returned to title、Runtime reconnect 时执行 `ConversationStateStore.Clear()`；
- MVP0 不恢复跨 stream 的 pending mutation，disconnect 后未确认的 conversation mutation 失效。

---

# 4. Adapter Event Contract

## 4.1 player_said_to_npc

`player_said_to_npc` 是 Environment -> Agent 事件。

它不是 capability，也不由模型调用。

Payload：

```json
{
  "conversation_id": "conv_12",
  "input_kind": "option",
  "text": "I can help you get there.",
  "selected_option_index": 1,
  "trigger": "dialogue_option"
}
```

字段规则：

- `conversation_id` 必须非空；
- `input_kind` 使用 `option / free_text`；
- `text` 必须非空，最大 240 chars，超长输入必须拒绝并返回明确错误；
- `selected_option_index` 只在 `input_kind=option` 时出现，使用 0-based index；
- `trigger` 使用 `dialogue_option / dialogue_free_text`；
- `target_entity_id` 是被对话 NPC 的 `entity_id`；
- `entities` 必须包含目标 NPC 和 `player:local`；
- `EntityRef.definition_id` 继续沿用 `entity_id` alias。

## 4.2 player_interacted_with_npc

现有 `player_interacted_with_npc` 保留，payload 增加 `conversation_id`。

Payload：

```json
{
  "conversation_id": "conv_12",
  "trigger": "action_button",
  "source": "stardew-smapi"
}
```

字段规则：

- 如果目标 NPC 没有 active conversation，Adapter 创建新的 `conversation_id`；
- 如果目标 NPC 有 active conversation，Adapter 复用当前 `conversation_id`；
- `source` 固定表示事件来源系统，本阶段使用 `stardew-smapi`；
- `trigger` 表示 Adapter 捕获的交互类型，可用值为 `action_button / mouse_left / mouse_right / console_probe`。

## 4.3 EventAck 与 conversation mutation

Adapter 对 conversation state 的写入必须与 Runtime `EventAck` 对齐。

规则：

- 发送 `player_interacted_with_npc` 前，Adapter 只 reserve `conversation_id`；
- 发送 `player_said_to_npc` 前，Adapter 只创建 pending player-line mutation；
- 每个 conversation 最多保留一个 pending mutation；
- pending mutation 保存 `event_id`，收到 ACK 时必须按 `event_id` 匹配；
- 收到匹配的 `EventAck.ACCEPTED` 后 commit pending mutation；
- 收到匹配的 `EventAck.REJECTED` 后丢弃 pending mutation；
- 收到匹配的 `EventAck.DUPLICATE` 后不重复 commit；
- gRPC server-to-adapter 消息顺序要求 Adapter 在处理同一 stream 后续 `ObservationRequest` 前先处理已收到的 `EventAck`；
- pending mutation 不写入 `Observation.state.stardew.conversation`。

## 4.4 Interaction Context Guard 边界

`Interaction Context Guard` 是 Phase6 前需要接入的 Adapter 执行前校验边界。

目标：

```text
防止玩家点击 NPC 后，在 LLM 响应前玩家或 NPC 已经离开，随后 dialogue UI 又在错误位置弹出。
```

设计规则：

- Phase5.6 不实现执行态校验，只明确边界；
- Phase6 在 Adapter 发送 `player_interacted_with_npc` 和 `player_said_to_npc` 时记录当前 interaction context；
- interaction context 至少包含 `world_id`、`conversation_id`、`npc_entity_id`、`player_entity_id`、location、NPC tile、player tile 和最大交互距离；
- `present_dialogue` 显示 UI 前校验当前世界、location 和距离仍满足该 interaction context；
- 校验通过后显示 UI，并在 UI 显示成功后追加 NPC conversation line；
- 校验失败时返回 `ActionResult(REJECTED)`，错误码为 `interaction_context_changed`，并关闭匹配的 active conversation；
- 该 guard 不冻结玩家或 NPC，不等待同一 Turn 内玩家输入，不引入 Runtime Stardew-specific parser；
- Phase6 在该 guard 基础上增加 `turn_id / action_id` 绑定、等待锁和 Turn terminal 释放信号。

---

# 5. Conversation State

## 5.1 Adapter 内存状态

新增 Adapter 侧状态组件：

```text
adapters/stardew/src/Dialogue/ConversationStateStore.cs
adapters/stardew/src/Dialogue/ConversationState.cs
adapters/stardew/src/Dialogue/ConversationLine.cs
adapters/stardew/src/Dialogue/ConversationIdGenerator.cs
```

状态 key：

```text
world_id + npc_entity_id + player_entity_id
```

状态内容：

```json
{
  "conversation_id": "conv_12",
  "npc_entity_id": "npc:Linus",
  "player_entity_id": "player:local",
  "recent_lines": [
    {
      "role": "npc",
      "speaker_entity_id": "npc:Linus",
      "speaker_name": "Linus",
      "text": "The mountain is quiet tonight.",
      "time_of_day": 1820
    },
    {
      "role": "player",
      "speaker_entity_id": "player:local",
      "speaker_name": "ZLC",
      "text": "Do you want company?",
      "time_of_day": 1820
    }
  ]
}
```

规则：

- MVP0 只使用内存状态；
- 状态在 mod reload、Runtime reconnect、world change、returned to title 或 day started 时重置；
- `DayStarted` handler 必须调用 `ConversationStateStore.Clear()`；
- `ConversationStateStore` 必须是 thread-safe，所有 public mutation/read API 通过同一把锁保护；
- `ConversationStateStore` 通过构造注入 conversation id generator，生产实现生成唯一 ID，测试实现返回确定性 ID；
- 每个 conversation 最多保留 12 行；
- 单行 text 最大 240 chars；
- 单行 text 超出 240 chars 时拒绝写入，并返回明确错误；
- line 的 `time_of_day` 取追加该 line 时的 `Game1.timeOfDay`；
- 超出行数上限时保留最近行，并记录 `recent_lines_omitted_count`；
- Adapter 只记录已被 Runtime 接纳的玩家输入和 `present_dialogue` 展示的 NPC 台词；
- `speak` 不写入 conversation state；
- Adapter 不把 conversation state 持久化到 Stardew save data。

## 5.2 Observation 注入

扩展 `Observation.state.stardew`：

```json
{
  "stardew": {
    "conversation": {
      "conversation_id": "conv_12",
      "active": true,
      "recent_lines_omitted_count": 0,
      "recent_lines": [
        {
          "role": "npc",
          "speaker_entity_id": "npc:Linus",
          "speaker_name": "Linus",
          "text": "The mountain is quiet tonight.",
          "time_of_day": 1820
        }
      ]
    }
  }
}
```

字段规则：

- `conversation` 只在目标 NPC 有 active conversation 时出现；
- `role` 使用 `npc / player`；
- `speaker_entity_id` 必须是当前 Observation 可解析的实体 ID；
- `speaker_name` 使用追加 line 时的游戏展示名；
- `recent_lines` 按发生顺序输出；
- `recent_lines` 不进入 `Observation.nearby_entities`；
- Runtime 不读取 `state.stardew.conversation.*` 的具体语义，只通过通用 renderer 输出。

---

# 6. Capability Contract

## 6.1 present_dialogue

`present_dialogue` 是 Agent -> Environment 的同步 capability。

Capability：

```text
name: present_dialogue
version: 0.1.0
execution_mode: SYNC
concurrency_mode: Sequential
```

Description：

```text
Displays one NPC dialogue line and optional player reply choices. The action completes when the dialogue UI is shown; player replies arrive later as player_said_to_npc events.
```

Input schema：

```json
{
  "type": "object",
  "properties": {
    "text": {
      "type": "string",
      "minLength": 1,
      "maxLength": 240
    },
    "reply_options": {
      "type": "array",
      "items": {
        "type": "string",
        "minLength": 1,
        "maxLength": 80
      },
      "maxItems": 4
    },
    "allow_free_text": {
      "type": "boolean"
    }
  },
  "required": ["text"],
  "additionalProperties": false
}
```

Execution rules：

- `text` 显示为 NPC 台词；
- `reply_options` 最多 4 个；
- `allow_free_text` 缺省为 `false`；
- Adapter handler 必须校验 `text` 非空且不超过 240 chars；
- Adapter handler 必须校验 `reply_options` 为字符串数组，最多 4 个，每个 option 非空且不超过 80 chars；
- 超出长度或数量上限时返回 `ActionResult(REJECTED)`，错误信息必须包含具体 limit；
- Action handler 使用 `world_id + ActionRequest.entity_id + player:local` 查找 active conversation；
- 如果不存在 active conversation，Action handler 创建新的 active conversation；
- 当 `reply_options` 为空且 `allow_free_text=false` 时，只展示 NPC 台词；
- `DialogueInteractionController` 持有 pending action；
- Adapter 真正显示 UI 后返回 terminal `ActionResult(SUCCEEDED)`；
- Adapter 在 UI 显示成功后把 `text` 追加为 NPC conversation line；
- 当 `reply_options` 为空且 `allow_free_text=false` 时，UI 展示完成后关闭 active conversation；
- 玩家后续选择或输入由 UI 发送 `player_said_to_npc` 事件。

ActionResult output：

```json
{
  "conversation_id": "conv_12",
  "displayed_text": "The mountain is quiet tonight.",
  "reply_options_count": 2,
  "allow_free_text": true
}
```

## 6.2 face_player

`face_player` 是 Agent -> Environment 的同步 capability。

Capability：

```text
name: face_player
version: 0.1.0
execution_mode: SYNC
concurrency_mode: Sequential
```

Description：

```text
Turns the NPC to face the local player when both are in the same location.
```

Input schema：

```json
{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}
```

Execution rules：

- NPC 与玩家不在同一 location 时返回 `ActionResult(REJECTED)`；
- 同 location 时按 tile delta 设置 NPC facing direction；
- tile delta 相同或方向无法确定时保持当前方向并返回 `SUCCEEDED`；
- 不移动 NPC。

ActionResult output：

```json
{
  "facing": "down"
}
```

## 6.3 speak 与 emote

`speak` 和 `emote` 保留。

规则：

- `speak` 用于普通单句 NPC 台词，不展示玩家回复选项；
- `present_dialogue` 用于可交互对话；
- `emote` 继续用于 NPC 头顶表情；
- 三个 capability 均为 `Sequential`；
- `speak` 不写入 conversation state。

---

# 7. Dialogue UI

新增 UI 组件：

```text
adapters/stardew/src/Dialogue/DialogueInteractionMenu.cs
adapters/stardew/src/Dialogue/DialogueInteractionController.cs
```

UI 行为：

- 使用 SMAPI/Stardew 主线程显示；
- 如果 `Game1.activeClickableMenu` 非空，延迟到下一帧再显示；
- Adapter 准备发送同一 NPC 新 GameEvent 前，先关闭该 NPC 未决 dialogue UI；
- 显示 NPC 台词；
- 显示最多 4 个回复选项；
- 当 `allow_free_text=true` 时显示自定义输入入口；
- 玩家选择选项时关闭菜单并发送 `player_said_to_npc`；
- 玩家提交自定义输入时关闭菜单并发送 `player_said_to_npc`；
- 玩家取消或关闭菜单时不发送 `player_said_to_npc`；
- 玩家取消或关闭菜单时关闭 active conversation；
- Adapter 抢占关闭旧菜单时不关闭 active conversation；
- 文本输入为空时不发送事件。

实现边界：

- 参考 ValleyTalk 的 `IClickableMenu`、`Game1.activeClickableMenu` 和 `Game1.keyboardDispatcher.Subscriber` 用法；
- 不 patch `Dialogue.chooseResponse`；
- 不覆盖 Stardew 原生 `Dialogue` 内部状态；
- 不在 UI 组件内调用 Runtime 或 LLM，由 controller 接收 UI 结果后调用 `RuntimeClient`。

---

# 8. Runtime Scope

Runtime 本阶段只做通用接线。

修改范围：

```text
runtime/config/agent.json
runtime/internal/context/renderer.go
runtime/internal/context/builder_test.go
```

允许改动：

- 更新 `tool_instruction`，说明玩家文本通过 `player_said_to_npc` 事件到达；
- 更新 `tool_instruction`，说明 `present_dialogue` 可展示回复选项，玩家回复会在后续事件中到达；
- 更新 `tool_instruction`，说明有回复选项或允许玩家输入时使用 `present_dialogue`，普通单句使用 `speak`；
- 为 `visibleActionSummary` 增加 `present_dialogue` 与 `face_player` 摘要；
- 增加一个通用 context renderer 测试，验证 `GameEvent.payload.text` 和 nested `Observation.state.stardew.conversation.recent_lines` 可以进入模型上下文；
- 增加 Recent Memory 回归测试，验证 `present_dialogue` 的 `text` 可以进入可见摘要。

禁止改动：

- 不新增 Stardew-specific Runtime parser；
- 不改 `agent.Loop`；
- 不改 `gateway`；
- 不改 `model.Provider`；
- 不改 protocol；
- 不实现 async resume。

---

# 9. 开发里程碑

## M1 Conversation State Model

目标：

```text
建立 Adapter 内存 conversation state，并能被 ObservationBuilder 注入 StardewObservation。
```

修改范围：

```text
adapters/stardew/src/Dialogue/ConversationLine.cs
adapters/stardew/src/Dialogue/ConversationState.cs
adapters/stardew/src/Dialogue/ConversationStateStore.cs
adapters/stardew/src/Dialogue/ConversationIdGenerator.cs
adapters/stardew/src/State/StardewObservation.cs
adapters/stardew/src/State/StardewObservationFactory.cs
adapters/stardew/src/State/ObservationBuilder.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/ModEntry.cs
adapters/stardew/tests/ProtocolMapper.Tests
adapters/stardew/tests/check-context-static.ps1
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
powershell -ExecutionPolicy Bypass -File adapters/stardew/tests/check-context-static.ps1
```

通过标准：

- `ConversationStateStore` 可创建、复用、重置 conversation；
- `ConversationStateStore` public read/mutation API 是 thread-safe；
- conversation id generator 通过构造注入，测试使用确定性实现；
- `DayStarted` reset 规则进入 static check；
- 新一天首次交互同一 NPC 创建新的 `conversation_id`；
- recent lines 按发生顺序输出；
- recent lines 超出 12 行时只保留最近 12 行；
- line 的 `time_of_day` 取追加时的游戏时间；
- 单行 text 超出 240 chars 时被拒绝，错误信息说明 240 chars limit；
- `Observation.state.stardew.conversation` 在有 active conversation 时输出；
- 无 active conversation 时不输出 `conversation`。

## M2 Player Dialogue Event

目标：

```text
Adapter 能把玩家选择或输入转换为 player_said_to_npc GameEvent。
```

修改范围：

```text
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Events/PlayerInteractProbe.cs
adapters/stardew/tests/ProtocolMapper.Tests
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
```

通过标准：

- `BuildPlayerSaidToNpcEvent(...)` 输出 `event_type=player_said_to_npc`；
- payload 包含 `conversation_id`、`input_kind`、`text` 和 `trigger`；
- option 输入包含 `selected_option_index`；
- free text 输入不包含 `selected_option_index`；
- 事件 `entities` 包含目标 NPC 和 `player:local`；
- 事件 `target_entity_id` 指向目标 NPC；
- 空 text 被拒绝；
- 超长 text 被拒绝，错误信息说明 240 chars limit；
- `player_interacted_with_npc.source` 保持 `stardew-smapi`；
- `player_interacted_with_npc.trigger` 区分 `action_button / mouse_left / mouse_right / console_probe`；
- `PlayerInteractProbe` 负责把输入按钮映射为 `action_button / mouse_left / mouse_right`；
- console probe 负责产生 `console_probe`；
- player line 只在 `EventAck.ACCEPTED` 后写入 conversation state；
- `EventAck.REJECTED` 不写入 conversation state；
- `EventAck.DUPLICATE` 不重复写入 conversation state。

## M3 present_dialogue Capability

目标：

```text
模型可以请求 Adapter 展示 NPC 台词和玩家回复入口。
```

修改范围：

```text
adapters/stardew/src/Runtime/CapabilityCatalog.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/Capabilities/PresentDialogueCapability.cs
adapters/stardew/src/Dialogue/ConversationStateStore.cs
adapters/stardew/tests/ProtocolMapper.Tests
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

- `CapabilityCatalog` 注册 `present_dialogue`；
- `present_dialogue` 为 `ExecutionMode.Sync`；
- `present_dialogue` 为 `CapabilityConcurrencyMode.Sequential`；
- input schema 限制 `text`、`reply_options`、`allow_free_text` 和 `additionalProperties=false`；
- Adapter handler 校验 `text`、`reply_options`、`allow_free_text` 的类型、长度和数量；
- `RuntimeClient` 能处理 `present_dialogue` ActionRequest；
- `reply_options` 最多 4 个；
- 超长 `text` 或 option 返回 `ActionResult(REJECTED)`，错误信息说明对应 limit；
- `ActionResult.output` 包含 `conversation_id`、`displayed_text`、`reply_options_count` 和 `allow_free_text`；
- `present_dialogue` 成功显示 UI 后追加 NPC conversation line；
- 无 active conversation 时 `present_dialogue` 创建新的 active conversation；
- 无 `reply_options` 且 `allow_free_text=false` 时，展示完成后关闭 active conversation。

## M4 Dialogue UI

目标：

```text
Adapter 展示可交互 Stardew 对话 UI，并把玩家选择或输入送回 Runtime。
```

修改范围：

```text
adapters/stardew/src/Dialogue/DialogueInteractionMenu.cs
adapters/stardew/src/Dialogue/DialogueInteractionController.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/src/ModEntry.cs
adapters/stardew/tests/ProtocolMapper.Tests
adapters/stardew/tests/check-context-static.ps1
```

验收命令：

```powershell
powershell -ExecutionPolicy Bypass -File adapters/stardew/tests/check-context-static.ps1
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

- UI 在主线程显示；
- active menu 存在时延迟显示；
- 菜单真正显示后才发送 `ActionResult(SUCCEEDED)`；
- Adapter 准备发送同一 NPC 新 GameEvent 前关闭该 NPC 未决 dialogue UI；
- 最多展示 4 个回复选项；
- `allow_free_text=true` 时可以输入自定义文本；
- 玩家选择 option 后发送 `player_said_to_npc`；
- 玩家提交 free text 后发送 `player_said_to_npc`；
- 玩家关闭菜单不发送事件，并关闭 active conversation；
- Adapter 抢占同一 NPC 旧菜单不关闭 active conversation；
- UI 组件不直接调用 LLM；
- 本阶段不新增 Harmony patch；
- 手工加载 mod 到 Stardew 实测：点 NPC、选择 option、输入 free text、关闭菜单，确认事件发送与 conversation 状态符合预期。

## M5 face_player Capability

目标：

```text
模型可以让 NPC 面向玩家，为对话与后续移动能力建立低风险动作基础。
```

修改范围：

```text
adapters/stardew/src/Capabilities/FacePlayerCapability.cs
adapters/stardew/src/Runtime/CapabilityCatalog.cs
adapters/stardew/src/Runtime/ProtocolMapper.Core.cs
adapters/stardew/src/Runtime/RuntimeClient.cs
adapters/stardew/tests/ProtocolMapper.Tests
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet build adapters/stardew/GameAgent.Stardew.csproj
```

通过标准：

- `CapabilityCatalog` 注册 `face_player`；
- `face_player` 为 `ExecutionMode.Sync`；
- `face_player` 为 `CapabilityConcurrencyMode.Sequential`；
- NPC 与玩家同 location 时返回 `SUCCEEDED`；
- NPC 与玩家不同 location 时返回 `REJECTED`；
- tile delta 到 facing direction 的纯函数有测试覆盖；
- 成功 output 包含 `facing`。

## M6 Runtime Prompt And Context Regression

目标：

```text
Runtime 通用 context renderer 可以承载玩家对话事件与 conversation nested state。
```

修改范围：

```text
runtime/config/agent.json
runtime/internal/context/renderer.go
runtime/internal/context/builder_test.go
```

验收命令：

```powershell
go test ./runtime/internal/context
go test ./runtime/...
```

通过标准：

- prompt 说明玩家回复通过后续 `player_said_to_npc` 事件到达；
- prompt 说明 `present_dialogue` 可生成最多 4 个玩家回复选项；
- prompt 说明有回复选项或允许玩家输入时使用 `present_dialogue`，普通单句使用 `speak`；
- prompt 说明省略回复选项且不允许自由输入表示当前对话结束；
- context renderer 测试覆盖 `GameEvent.payload.text`；
- context renderer 测试覆盖 nested `Observation.state.stardew.conversation.recent_lines`；
- Recent Memory 摘要显示 `present_dialogue.text`；
- Recent Memory 摘要包含 `face_player` 行为；
- Runtime 不新增 Stardew-specific parser。

## M7 Full Regression And Commit

目标：

```text
完成 Phase5.6 全量验收，并提交一个清晰开发块。
```

验收命令：

```powershell
dotnet run --project adapters/stardew/tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj
dotnet run --project adapters/stardew/tests/ActionCancellationRegistry.Tests/ActionCancellationRegistry.Tests.csproj
dotnet run --project adapters/stardew/tests/PlayerInteractProbe.Tests/PlayerInteractProbe.Tests.csproj
powershell -ExecutionPolicy Bypass -File adapters/stardew/tests/check-context-static.ps1
dotnet build adapters/stardew/GameAgent.Stardew.csproj
go test ./runtime/internal/context
go test ./runtime/...
go test ./protocol/gen/go/...
git diff --check
```

通过标准：

- 全部命令通过；
- Stardew 手工 smoke 通过：点 NPC、选择 option、输入 free text、关闭菜单；
- Phase5.6 commit 只包含 Stardew interaction surface 和通用 Runtime prompt/context regression；
- 不包含 Phase6 async lifecycle；
- 不包含 movement capability；
- 不包含 protocol codegen。

提交信息：

```text
feat: add stardew dialogue interaction surface
```

---

# 10. 实现顺序

```text
M1 Conversation State Model
M2 Player Dialogue Event
M3 present_dialogue Capability
M4 Dialogue UI
M5 face_player Capability
M6 Runtime Prompt And Context Regression
M7 Full Regression And Commit
```

约束：

- M1 完成前不开发 UI；
- M2 完成前不让 UI 发送 Runtime event；
- M3 完成前不把 `present_dialogue` 暴露给模型；
- M4 完成前不修改 Phase6 async 文档；
- Phase5.6 完成前不进入 Phase6 movement / async runtime 开发。

---

# 11. 与 Phase6 的衔接

Phase5.6 完成后，Phase6 可以使用真实对话事件作为 movement 触发来源：

```text
player_said_to_npc: "Can you come here?"
  -> Runtime AgentTurn
  -> ModelDecision(movement capability)
  -> Async Action lifecycle
  -> terminal ActionResult
  -> re-observe
  -> present_dialogue
  -> settle
```

具体 movement capability 名称与输入 schema 由 Phase6 文档定义。

Phase5.6 不新增 movement capability，也不修改 Phase6 async lifecycle。

---

# 12. 验收记录

Phase5.6 验收完成后记录：

```text
Adapter:
  - player_interacted_with_npc carries conversation_id
  - player_said_to_npc event accepted
  - Observation.state.stardew.conversation rendered
  - present_dialogue displays NPC text and reply affordances
  - present_dialogue writes NPC dialogue lines into active conversation
  - speak remains plain one-line dialogue and does not write conversation state
  - face_player works as sync action

Runtime:
  - generic renderer carries event payload and nested conversation state
  - Recent Memory summary carries present_dialogue text
  - no Stardew-specific parser
  - no async lifecycle changes

Deferred:
  - ask_player same-turn human-in-loop
  - Phase6 movement capability
  - AgentDefinition store
  - long-term conversation persistence
  - canonical dialogue retrieval
```
