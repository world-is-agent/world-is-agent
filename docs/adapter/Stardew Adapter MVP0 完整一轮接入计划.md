# Stardew Adapter MVP0 完整一轮接入计划

## Summary

Adapter 作为 SMAPI Mod 内的 gRPC client，直接连接 Runtime 的 `GameAgentGateway.Connect` 双向流，不复用 StarDojo 的 TCP/Python 桥。借鉴 StarDojo 的核心边界：网络线程只收发协议，Stardew `Game1`、NPC、Dialogue 等游戏 API 统一切到 SMAPI 主线程执行。

目标验收链路：

```text
点击 Linus
-> Adapter 发送 GameEvent
-> Runtime 返回 EventAck
-> Runtime 返回 ObserveRequest
-> Adapter 返回 Observation
-> Runtime 返回 ActionRequest(speak)
-> Adapter 调用 SpeakCapability
-> Adapter 返回 ActionResult
```

## Key Changes

- 在 `adapters/stardew` 接入 C# gRPC/protobuf：
  - 增加 `Grpc.Net.Client`、`Google.Protobuf`、`Grpc.Tools`。
  - `<Protobuf Include="..\..\protocol\proto\gameagent.proto" GrpcServices="Client" />`。
  - 默认连接 `http://127.0.0.1:50051`，启动前设置：
    `AppContext.SetSwitch("System.Net.Http.SocketsHttpHandler.Http2UnencryptedSupport", true);`
  - 不提交生成代码，由 build 自动生成 C# client。

- 新增 Runtime 接入层：
  - `RuntimeClient`：建立 gRPC stream、发送 `AdapterHello`、接收 `RuntimeMessage`、串行发送 `AdapterMessage`。
  - `CapabilityCatalog`：返回 `speak` capability；Runtime MVP0 当前只读取 `capability.name`，schema / description / execution_mode 先保持协议完整，不作为注册校验依据。
  - `MainThreadDispatcher`：后台 receive loop 只入队任务并继续接收；`UpdateTicked` 在主线程排干队列。
  - `ProtocolMapper`：集中处理 Stardew 和协议之间的映射，尤其是 `NPC <-> entity_id`。

- 改造现有 Probe：
  - `PlayerInteractProbe` 点击 Linus 后发送 `player_interacted_with_npc`。
  - NPC 的 `EntityRef` 必须使用 `entity_type = "npc"`，`entity_id = "npc:Linus"`。
  - `ObservationBuilder` 和 `SpeakCapability` 都通过同一套 `ProtocolMapper` 解析 `npc:Linus`，避免两处规则不一致。

## Runtime Message Handling

- `EnvironmentReady`：记录日志。
- `CapabilityRequest`：回复 `CapabilityList(speak)`；建议填 `correlation_id = capability_request.message_id`，但 Runtime 当前不强校验。
- `EventAck`：记录 debug 日志并安全忽略，不能让 receive loop 退出。
- `ObserveRequest`：派发主线程任务读取 NPC 状态，完成后发送 `Observation`，`correlation_id = observe.message_id`。
- `ActionRequest`：派发主线程任务执行 `speak`，完成后发送 `ActionResult`；`ActionResult.action_id` 必须等于收到的 `ActionRequest.action_id`，Runtime 依赖它唤醒 `SubmitAction`。
- `Error`：记录 warning/error 日志，MVP0 不做恢复。
- default / unknown oneof：记录 debug 日志并忽略，禁止抛异常杀死接收循环。

`ActionResult` 失败时必须填 `error` 字段，例如空文本、找不到 NPC、不支持的 capability、主线程执行异常，都返回 `status = FAILED` 并携带错误 code/message。

`ObserveRequest` 失败（world 未就绪、`entity_id` 非法、NPC 找不到）时，Adapter 发送 `AdapterMessage{Error}`，`correlation_id = observe.message_id`。Runtime recvLoop 已能消费 inbound `Error`：按 `correlation_id` 调 `failObservation` 唤醒对应的 `Observe` 等待者并返回错误，避免该 turn 阻塞到连接超时。注意这是 observe 侧的失败通道；action 失败仍走 `ActionResult{status=FAILED}`，由 `action_id` 匹配，两条通道互不干扰。

`ActionResult.status` 和 `error` 在 Runtime MVP0 中暂不触发重试或模型反馈，主要用于日志和手动链路排查。

## Test Plan

- 构建验证：
  - `dotnet build adapters/stardew/GameAgent.Stardew.csproj`
  - 首次 restore 可能需要联网获取 NuGet 包。

- Runtime 回归：
  - `go test ./runtime/cmd/server ./runtime/internal/gateway ./runtime/internal/agent ./runtime/internal/tool ./runtime/internal/model -count=1`

- 手动完整链路：
  - 启动 Runtime：`go run ./runtime/cmd/server`
  - 启动 Stardew + SMAPI Adapter。
  - 加载包含 Linus 的存档。
  - 点击 Linus。
  - 期望日志出现：connected、CapabilityList sent、GameEvent sent、EventAck received、Observation sent、ActionRequest received、ActionResult sent。
  - 期望游戏内 Linus 弹出 Runtime FakeProvider 生成的文本。

## Assumptions

- MVP0 连接时机：`GameLaunched` 后启动 RuntimeClient；如果尚未加载存档，点击事件自然不会产生。
- MVP0 不做断线重连；连接失败只记录日志，可后续加手动重连命令。
- MVP0 只支持 `npc:Linus` 和 `speak`。
- 不改 `gameagent.proto`，沿用当前 Runtime 协议。
- 不引入 StarDojo 的 TCP server / mmap / Python env。
- Adapter 未连接或能力未完成注册时，点击 Linus 只记录 warning 并忽略。
- 多次点击 Linus 会由 Runtime `eventCh` 串行消费，MVP0 不处理跨 turn 并发执行。
