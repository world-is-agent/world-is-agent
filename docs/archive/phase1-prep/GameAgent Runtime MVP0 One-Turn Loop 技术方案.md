# GameAgent Runtime MVP0 One-Turn Loop 技术方案

> Status: Implementation Blueprint
> Scope: 先拉通点击交互到游戏内回复的完整链路
> Related:
> - `0811-GameAgent Protocol v1alpha1 设计规范.md`
> - `0811-GameAgent Runtime 架构设计规范.md`
> - `0811Agent Harness 架构借鉴与改进设计.md`
> - `GameAgent Runtime MVP0 Harness 技术开发方案.md`

------

# 1. 本文档定位

`GameAgent Runtime MVP0 Harness 技术开发方案.md` 仍然有参考价值，不覆盖。

它更像：

```text
未来完整 GameAgent Harness 的方向文档
```

本文档只回答一个更小的问题：

> **当前第一阶段代码到底怎么写，才能把点击交互 → Runtime → Agent Loop → LLM → Adapter → 游戏展示这条链路跑通。**

因此本文档不追求完整 harness。

本文档追求：

```text
文件少
流程清楚
边界正确
能跑起来
后续可扩展
```

------

# 2. MVP0 要跑通的核心流程

第一阶段完整流程分成两段：

```text
A. Environment Bootstrap
   在玩家点击之前完成。

B. One-Turn AgentRun
   玩家点击之后才开始。
```

这样更接近一个真正 Agent Harness 的结构：

```text
Bootstrap = 准备运行环境
Run       = 使用已准备好的环境完成一次智能决策
```

------

## A. Environment Bootstrap

这一段在游戏交互发生前完成：

```text
Go Runtime 启动
        ↓
创建 ModelProvider
        ↓
创建 ToolRegistry
        ↓
创建 AgentLoop
        ↓
创建 Gateway
        ↓
启动 gRPC Server
        ↓
Stardew + Adapter 启动
        ↓
Adapter Connect
        ↓
AdapterHello
        ↓
EnvironmentReady
        ↓
// Protocol session established
        ↓
Capability discovery
        ↓
Tool registration
        ↓
BOOTSTRAPPED
        ↓
// Agent can now run
        ↓
等待 GameEvent
```

这个阶段的目的：

```text
连接已建立
Protocol session 已建立
Adapter 能力已发现
Runtime Tools 已注册
AgentLoop 已可运行
```

MVP0 可以先发送：

```text
CapabilityRequest(entity_id unset)
```

表示查询 Environment-level Capability。

Adapter 返回：

```text
speak
```

表示当前 Stardew 环境支持对可说话 Entity 执行 `speak`。

后续如果不同 NPC 能力不同，再升级为：

```text
CapabilityRequest(entity_id = "npc:Linus")
Capability cache by entity_id
Capability revision refresh
lazy discovery
```

MVP0 暂时不做这些。

------

## B. One-Turn AgentRun

玩家点击之后才进入 AgentRun：

```text
玩家点击 Linus
        ↓
Stardew Adapter 捕获交互
        ↓
Adapter 发送 GameEvent 给 Runtime
        ↓
Runtime Gateway 收到 GameEvent
        ↓
Runtime 启动一次 Agent Loop
        ↓
Agent Loop 向 Adapter 请求 Observation
        ↓
Adapter 返回 Linus 当前上下文
        ↓
Agent Loop 构造 prompt
        ↓
Agent Loop 从 ToolRegistry 取已注册 Tools
        ↓
Agent Loop 调用 ModelProvider
        ↓
LLM 返回 ToolCall: speak(text)
        ↓
Tool Registry 校验 speak 是否存在
        ↓
EnvironmentTool 转成 ActionRequest(speak)
        ↓
Gateway 发送 ActionRequest 给 Adapter
        ↓
Adapter 执行 SpeakCapability
        ↓
游戏中显示 LLM 生成的文本
        ↓
Adapter 返回 ActionResult
        ↓
Agent Loop 结束
```

这段流程是 **one-turn loop**。

也就是：

```text
Observe once
Think once
Act once
Done
```

MVP0 不做多轮：

```text
Observe
Think
Act
Observe result
Think again
Act again
```

多轮 ReAct loop 放到后续阶段。

------

# 3. 正确边界

MVP0 必须保持三层边界：

```text
Adapter owns game translation.
Runtime owns agent loop.
Protocol owns message contracts.
```

换成中文：

```text
Adapter 负责接入 Stardew。
Runtime 负责 Agent 思考流程。
Protocol 负责 Adapter 和 Runtime 怎么通信。
```

禁止：

```text
Adapter 直接调用 LLM
Adapter 决定 NPC 说什么
Agent Loop 引用 Stardew / SMAPI / Game1 / Farmer / NPC 类型
Runtime 写死 Stardew 游戏 API
```

允许：

```text
Adapter 把 Stardew NPC 转成 entity_id = "npc:Linus"
Adapter 把 Stardew 状态转成 Observation
Adapter 把 speak ActionRequest 转成游戏内 Dialogue
Runtime 根据 Observation 和 Capability 决定调用 speak
```

------

# 4. 最小代码结构

当前先采用这个目录：

```text
runtime/
├── cmd/
│   └── server/
│       └── main.go
│
└── internal/
    ├── gateway/
    │   └── gateway.go
    │
    ├── agent/
    │   └── loop.go
    │
    ├── tool/
    │   ├── registry.go
    │   └── environment_tool.go
    │
    └── model/
        └── provider.go
```

先不要拆：

```text
session/
trace/
permission/
memory/
scheduler/
contextengine/
```

这些概念可以先在上面几个文件里以非常薄的形式存在。

等代码真的变大，再拆。

------

# 5. 文件职责总览

```text
cmd/server/main.go
  启动进程。

gateway/gateway.go
  负责 Adapter ↔ Runtime 的 gRPC stream，以及 Environment Bootstrap。

agent/loop.go
  负责一次 one-turn AgentRun。

tool/registry.go
  保存 Bootstrap 阶段发现的 Capability，并向 AgentRun 提供可用 Tool。

tool/environment_tool.go
  把 tool call 转成 ActionRequest。

model/provider.go
  定义 LLM Provider 接口和 MVP0 Provider 实现。
```

它们在流程中的位置：

```text
Adapter
  ↓
gateway.go
  ↓
tool/registry.go
  ↓
BOOTSTRAPPED
```

运行阶段：

```text
Adapter
  ↓
gateway.go
  ↓
agent/loop.go
  ↓
model/provider.go
  ↓
tool/registry.go
  ↓
tool/environment_tool.go
  ↓
gateway.go
  ↓
Adapter
```

------

# 6. `cmd/server/main.go`

职责：

```text
创建 Runtime 依赖
启动 gRPC server
注册 GameAgentGateway
监听本地端口
```

MVP0 中它应该做：

```go
func main() {
    modelProvider := model.NewProviderFromEnv()
    toolRegistry := tool.NewRegistry()
    agentLoop := agent.NewLoop(modelProvider, toolRegistry)
    gatewayServer := gateway.NewServer(agentLoop, toolRegistry)

    grpcServer := grpc.NewServer()
    protocolv1alpha1.RegisterGameAgentGatewayServer(grpcServer, gatewayServer)

    listen on 127.0.0.1:50051
}
```

它不应该做：

```text
解析 GameEvent
构造 prompt
执行 tool
写 Stardew 相关逻辑
```

`main.go` 只是组装器。

------

# 7. `gateway/gateway.go`

职责：

> **Runtime 和 Adapter 之间的门。**

它实现：

```go
func (s *Server) Connect(stream protocolv1alpha1.GameAgentGateway_ConnectServer) error
```

它负责处理 Protocol 消息：

```text
AdapterHello
GameEvent
CapabilityList
Observation
ActionResult
Error
Heartbeat
```

它在 Bootstrap 阶段负责：

```text
AdapterHello 校验
EnvironmentReady 返回
CapabilityRequest 发送
CapabilityList 接收
ToolRegistry 注册
BOOTSTRAPPED 标记
```

它在 AgentRun 阶段向 Agent Loop 提供两个环境能力：

```go
Observe(ctx, entityID) (*Observation, error)
SubmitAction(ctx, action) (*ActionResult, error)
```

Bootstrap 核心流程：

```text
Adapter Connect
  ↓
receive AdapterHello
  ↓
send EnvironmentReady
  ↓
send CapabilityRequest(entity_id unset)
  ↓
receive CapabilityList
  ↓
toolRegistry.RegisterEnvironmentCapabilities(...)
  ↓
BOOTSTRAPPED
```

这里的 `EnvironmentReady` 是 Protocol message，只表示：

```text
gRPC stream / Environment Session 已建立。
```

`BOOTSTRAPPED` 是 Runtime 内部状态，只表示：

```text
Capability 已发现。
ToolRegistry 已注册。
AgentRun 可以开始。
```

不需要新增 Protocol message。

收到事件后的核心流程：

```text
receive GameEvent
  ↓
send EventAck
  ↓
start AgentRun goroutine
  ↓
go agentLoop.HandleEvent(ctx, env, event)
```

Capability 不在每次 AgentRun 内重新查询。

MVP0 的规则是：

```text
Environment Bootstrap 时发现一次 Capability。
ToolRegistry 持有已注册 Tool。
AgentRun 只读取 ToolRegistry。
```

它仍然要支持 Agent Loop 反向请求 Adapter：

```text
agentLoop calls gateway.Observe
  ↓
gateway sends RuntimeMessage(observe)
  ↓
Adapter returns AdapterMessage(observation)
  ↓
gateway returns Observation to agentLoop
```

```text
agentLoop calls gateway.SubmitAction
  ↓
gateway sends RuntimeMessage(action)
  ↓
Adapter executes speak
  ↓
Adapter returns AdapterMessage(action_result)
  ↓
gateway returns ActionResult to agentLoop
```

gRPC stream 必须拆成两个循环：

```text
recvLoop
  只负责 stream.Recv()
  只做消息分发
  不执行阻塞 AgentRun

sendLoop
  只负责 stream.Send()
  从 outgoing channel 读取 RuntimeMessage
```

原因：

```text
AgentRun 会等待 Observation / ActionResult。
Observation / ActionResult 又必须由 recvLoop 收到。
如果 recvLoop 同步执行 AgentRun，就会造成死锁。
```

推荐内部模型：

```text
recvLoop
  ↓
AdapterMessage
  ↓
dispatch
  ├── GameEvent -> go agentLoop.HandleEvent(ctx, env, event)
  ├── Observation -> pendingRequests[correlation_id].resolve(...)
  ├── CapabilityList -> bootstrapCapabilityWaiter.resolve(...)
  └── ActionResult -> pendingActions[action_id].resolve(...)

sendLoop
  ↑
outgoing chan *RuntimeMessage
```

`gateway.Server` 是服务对象，不持有单个 stream 的 pending 状态。

每次 `Connect()` 都必须创建连接级 `environment` 实例：

```go
type environment struct {
    stream protocolv1alpha1.GameAgentGateway_ConnectServer

    outgoing chan *protocolv1alpha1.RuntimeMessage

    pendingRequests map[string]chan *protocolv1alpha1.AdapterMessage
    pendingActions  map[string]chan *protocolv1alpha1.ActionResult
}

func (s *Server) Connect(stream protocolv1alpha1.GameAgentGateway_ConnectServer) error {
    env := newEnvironment(stream)

    // recvLoop(env)
    // sendLoop(env)
}
```

`recvLoop`、`sendLoop`、`pendingRequests`、`pendingActions` 都属于这一次 `Connect()` 的 `environment`。

收到事件时：

```go
go s.agentLoop.HandleEvent(ctx, env, event)
```

这样未来支持多个 Adapter 连接时，不会让 Adapter A 的 Observation 唤醒 Adapter B 的 waiter。

pending request 规则：

```text
Observe(...)
  ↓
create message_id
  ↓
pendingRequests[message_id] = waiter
  ↓
send RuntimeMessage(observe, message_id)
  ↓
recvLoop receives AdapterMessage(observation, correlation_id = message_id)
  ↓
wake waiter

SubmitAction(...)
  ↓
create action_id
  ↓
pendingActions[action_id] = waiter
  ↓
send RuntimeMessage(action)
  ↓
recvLoop receives AdapterMessage(action_result, action_id = action_id)
  ↓
wake waiter
```

MVP0 可以先只支持一个 Adapter 连接。

但代码不要写死 Stardew 类型。

------

# 8. `agent/loop.go`

职责：

> **跑一次 Agent 思考流程。**

MVP0 的 Agent Loop 是 one-turn：

```text
GameEvent
  ↓
Observe
  ↓
Build Prompt
  ↓
Read available tools from ToolRegistry
  ↓
ModelProvider.Generate
  ↓
Receive structured ToolCall
  ↓
ToolRegistry.Validate
  ↓
EnvironmentTool.Execute
  ↓
ActionResult
```

推荐接口：

```go
type Environment interface {
    Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error)
    SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error)
}

type Loop struct {
    model model.Provider
    tools *tool.Registry
}

func (l *Loop) HandleEvent(
    ctx context.Context,
    env Environment,
    event *protocolv1alpha1.GameEvent,
) error
```

`Loop` 不长期持有 `Environment`。

`Gateway` 实现 `agent.Environment`，并在每次 AgentRun 时传入：

```text
Gateway -> agentLoop.HandleEvent(ctx, gatewayEnvironment, event)
```

这样依赖方向保持为：

```text
gateway depends on agent.Loop
agent.Loop depends only on agent.Environment interface
agent.Loop does not own gateway
```

`HandleEvent` 里只处理：

```text
event_type == "player_interacted_with_npc"
```

第一版 entity 选择规则：

```text
从 event.entities 中找到 entity_type == "npc"
拿到 entity_id
```

AgentLoop 不再请求 CapabilityList。

它只读取 Bootstrap 阶段注册好的工具：

```go
tools := l.tools.Available(entityID)
```

MVP0 也可以更简单：

```go
tools := l.tools.Available()
```

后续如果不同 NPC 能力不同，再升级为按 `entity_id` 过滤。

prompt 可以先非常短：

```text
You are controlling an NPC in a game.
The player just interacted with you.
Observation: ...
Available tools: ...
```

AgentLoop 构造模型输入：

```text
Prompt
+
available tools
```

然后：

```text
Provider.Generate(request)
  ↓
Provider 使用对应厂商的 tool calling / fake output
  ↓
Provider 返回统一 model.ToolCall
```

`agent/loop.go` 不知道 Linus 是谁，也不知道 Stardew 怎么显示对话。

它也不解析 provider-specific JSON。

`FakeProvider`、`OpenAIProvider` 或后续其他模型 Provider 必须先把模型输出转换成统一 `ToolCall`。

它只知道：

```text
entity_id
observation
available tools
tool call
action result
```

------

# 9. `model/provider.go`

职责：

> **隔离 LLM 调用。**

定义接口：

```go
type Provider interface {
    Generate(ctx context.Context, req Request) (Response, error)
}

type Request struct {
    Prompt string
    Tools  []ToolDefinition
}

type ToolDefinition struct {
    Name        string
    Description string
    InputSchema string
}

type ToolCall struct {
    Name      string
    Arguments *structpb.Struct
}

type Response struct {
    ToolCall ToolCall
}
```

`Provider` 不返回纯文本给 AgentLoop 解析。

不同 Provider 负责把自己的模型输出转换成统一 `ToolCall`：

```text
FakeProvider
  直接返回 ToolCall{Name: "speak", Arguments: ...}

OpenAIProvider
  把 OpenAI response / tool calling / JSON output 转换成统一 ToolCall
```

AgentLoop 只消费：

```text
model.Response.ToolCall
```

AgentLoop 不关心：

```text
OpenAI response shape
Claude tool_use shape
Local model raw JSON shape
FakeProvider hardcoded JSON shape
```

Provider 输入链路：

```text
Capability
  ↓
ToolRegistry
  ↓
ToolDefinition
  ↓
ModelProvider
  ↓
ToolCall
```

MVP0 建议同时提供两个实现：

```text
FakeProvider
  用于本地链路和测试，不需要 API key。

OpenAIProvider
  用于真实 LLM 测试，通过环境变量配置。
```

如果想进一步压缩范围，第一版可以只实现 `FakeProvider`。

但是代码结构要允许后续替换成 `OpenAIProvider`。

环境变量建议：

```text
GAMEAGENT_MODEL_PROVIDER=fake|openai
OPENAI_API_KEY=...
GAMEAGENT_LLM_MODEL=...
```

`NewProviderFromEnv()` 规则：

```text
GAMEAGENT_MODEL_PROVIDER 未设置
  → 使用 FakeProvider

GAMEAGENT_MODEL_PROVIDER=fake
  → 使用 FakeProvider

GAMEAGENT_MODEL_PROVIDER=openai
  → 使用 OpenAIProvider，要求 OPENAI_API_KEY 存在
```

MVP0 的 `FakeProvider` 返回：

```go
model.ToolCall{
    Name: "speak",
    Arguments: structpb.StructFromMap(map[string]any{
        "text": "Hello from GameAgent Runtime",
    }),
}
```

上面是概念示例；实际 Go 实现中应处理 `structpb.NewStruct(...)` 返回的 `(*structpb.Struct, error)`。

------

# 10. `tool/registry.go`

职责：

> **管理 Runtime 当前可用的工具。**

注意：

```text
Adapter 提供的是 Capability。
Runtime 暴露给 Agent Loop 的才是 Tool。
```

MVP0 只支持：

```text
speak
```

Registry 做三件事：

```text
RegisterEnvironmentCapabilities(capabilities)
HasTool("speak")
ValidateToolCall(entityID, toolCall)
```

校验规则：

```text
tool == "speak"
arguments.text exists
arguments.text != ""
len(arguments.text) <= 300
ToolRegistry has speak
```

MVP0 先把 `speak` 作为 Environment-level Tool。

后续如果不同 NPC capability 不一样，再升级为：

```text
RegisterCapabilities(entityID, capabilities)
HasCapability(entityID, "speak")
```

后续可扩展：

```text
move_to
give_item
inspect
```

但 MVP0 不需要。

------

# 11. `tool/environment_tool.go`

职责：

> **把 Agent 的 tool call 转成 Adapter 能执行的 ActionRequest。**

输入：

```json
{
  "tool": "speak",
  "arguments": {
    "text": "Hi, good to see you."
  }
}
```

输出：

```protobuf
ActionRequest {
  action_id: "act_..."
  entity_id: "npc:Linus"
  capability: "speak"
  arguments: {
    "text": "Hi, good to see you."
  }
}
```

然后调用：

```go
env.SubmitAction(ctx, actionRequest)
```

它不直接接触 gRPC stream。

它不直接接触 Stardew API。

它只负责：

```text
ToolCall -> ActionRequest
```

------

# 12. 完整代码调用链

## A. Environment Bootstrap 调用链

```text
cmd/server/main.go
  ↓
create model.Provider
  ↓
create tool.Registry
  ↓
create agent.Loop
  ↓
create gateway.Server
  ↓
start gRPC server
  ↓
Stardew RuntimeClient.Connect
  ↓
gateway.Server.Connect receives AdapterHello
  ↓
gateway sends EnvironmentReady
  ↓
gateway sends CapabilityRequest(entity_id unset)
  ↓
Adapter returns CapabilityList(speak)
  ↓
tool.Registry.RegisterEnvironmentCapabilities(capabilities)
  ↓
BOOTSTRAPPED
  ↓
wait for GameEvent
```

## B. One-Turn AgentRun 调用链

```text
Stardew PlayerInteractProbe
  ↓
RuntimeClient.Send(GameEvent)
  ↓
gateway.Server.Connect recvLoop receives AdapterMessage(event)
  ↓
gateway sends EventAck(ACCEPTED)
  ↓
go agent.Loop.HandleEvent(ctx, env, event)
  ↓
gateway.Observe(entityID)
  ↓
Adapter returns Observation
  ↓
tool.Registry.Available()
  ↓
agent.Loop builds prompt
  ↓
model.Provider.Generate(request)
  ↓
model.Provider returns structured ToolCall
  ↓
tool.Registry.ValidateToolCall(entityID, toolCall)
  ↓
tool.EnvironmentTool.BuildActionRequest(entityID, toolCall)
  ↓
gateway.SubmitAction(actionRequest)
  ↓
Adapter SpeakCapability displays dialogue
  ↓
Adapter returns ActionResult(SUCCEEDED)
  ↓
agent.Loop returns nil
```

------

# 13. Adapter 侧需要配合什么

Adapter 需要从 probe 升级成 gRPC client。

新增：

```text
adapters/stardew/src/RuntimeClient/
    RuntimeClient.cs
```

它负责：

```text
连接 Runtime
发送 AdapterHello
发送 GameEvent
接收 RuntimeMessage
响应 CapabilityRequest
响应 ObserveRequest
执行 ActionRequest
返回 ActionResult
```

Adapter 里已有能力继续复用：

```text
Events/PlayerInteractProbe.cs
State/ObservationBuilder.cs
Capabilities/SpeakCapability.cs
```

变化是：

```text
原来点击后直接 Speak("Hello from GameAgent")

现在点击后：
  发送 GameEvent
  等 Runtime 返回 ActionRequest(speak)
  再 Speak(text)
```

------

# 14. MVP0 最小验收标准

完成时必须能做到：

```text
1. 启动 Go Runtime server。

2. 启动 Stardew + SMAPI Adapter。

3. Adapter 主动连接 Runtime。

4. Runtime 返回 EnvironmentReady。

5. Runtime 请求 CapabilityList。

6. Adapter 返回 speak capability。

7. Runtime 把 speak 注册进 ToolRegistry。

8. Runtime 内部状态进入 BOOTSTRAPPED。

9. 玩家点击 Linus。

10. Adapter 发送 GameEvent。

11. Runtime 请求 Observation。

12. Adapter 返回 Linus Observation。

13. Runtime 调用 model.Provider。

14. Provider 返回 speak tool call。

15. Runtime 发送 ActionRequest(speak)。

16. Adapter 在游戏里显示文本。

17. Adapter 返回 ActionResult(SUCCEEDED)。
```

第一版可以先用 `FakeProvider`。

当 FakeProvider 链路跑通后，再接真实 LLM。

------

# 15. 为什么这个结构适合简历项目

这个版本虽然小，但结构是对的：

```text
gRPC Environment Gateway
Protocol-driven Adapter
One-turn Agent Loop
Capability-to-Tool Mapping
Model Provider Abstraction
Game-side Action Execution
```

简历上可以表达成：

> Built a game-native agent runtime that connects a Stardew Valley SMAPI adapter to a Go agent server through a protobuf/gRPC environment gateway, maps live game capabilities into tools, runs a one-turn LLM agent loop, and returns tool actions back into the game world.

这比一开始堆完整 Hermes/Pi-style harness 更好。

因为它先证明了：

```text
真实游戏
真实协议
真实 Runtime
真实 Agent Loop
真实游戏内效果
```

后续再逐步升级：

```text
FakeProvider -> OpenAIProvider
one-turn loop -> multi-turn loop
speak -> move_to / give_item
stdout trace -> JSONL trace
in-memory registry -> persistent session
```

------

# 16. 下一步实现顺序

推荐实现顺序：

```text
1. 写 Go Runtime server skeleton
2. 实现 gateway.Connect + AdapterHello
3. 实现 Bootstrap 阶段 CapabilityRequest / CapabilityList
4. 实现 ToolRegistry 注册 speak
5. 实现 Adapter RuntimeClient 连接 Runtime
6. 实现 GameEvent 上报
7. 实现 ObserveRequest / Observation
8. 实现 FakeProvider + one-turn Agent Loop
9. 实现 ToolRegistry 校验 speak ToolCall
10. 实现 ActionRequest / ActionResult
11. 手工测试点击 Linus 显示 Runtime 文本
12. 再接真实 LLM Provider
```

每一步都应该能单独验证。

不要等所有代码写完才进游戏测试。
