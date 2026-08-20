using System;
using System.Threading;
using System.Threading.Tasks;
using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.State;
using Grpc.Core;
using Grpc.Net.Client;
using StardewModdingAPI;
using StardewValley;

namespace GameAgent.Stardew.Runtime;

public sealed class RuntimeClient : IDisposable
{
    private const string ProtocolVersion = "v1alpha2";

    private readonly AdapterConfig config;
    private readonly MainThreadDispatcher dispatcher;
    private readonly ObservationBuilder observationBuilder;
    private readonly SpeakCapability speakCapability;
    private readonly EmoteCapability emoteCapability;
    private readonly ActionCancellationRegistry actionCancellationRegistry = new();
    private readonly IMonitor monitor;
    private readonly SemaphoreSlim sendMu = new(1, 1);

    private GrpcChannel? channel;
    private AsyncDuplexStreamingCall<AdapterMessage, RuntimeMessage>? stream;
    private CancellationTokenSource? cancellation;
    private Task? receiveTask;
    private readonly string sessionId = Guid.NewGuid().ToString("N");
    // currentWorldId is maintained from SMAPI main-thread lifecycle events.
    // Background gRPC threads must not resolve Stardew world state directly.
    private volatile string currentWorldId = string.Empty;
    private long eventSequence;
    private volatile bool isReady;

    public RuntimeClient(
        AdapterConfig config,
        MainThreadDispatcher dispatcher,
        ObservationBuilder observationBuilder,
        SpeakCapability speakCapability,
        EmoteCapability emoteCapability,
        IMonitor monitor
    )
    {
        this.config = config;
        this.dispatcher = dispatcher;
        this.observationBuilder = observationBuilder;
        this.speakCapability = speakCapability;
        this.emoteCapability = emoteCapability;
        this.monitor = monitor;
    }

    public bool IsReady => this.isReady && this.stream is not null;

    public void Start()
    {
        if (this.receiveTask is not null)
            return;

        AppContext.SetSwitch("System.Net.Http.SocketsHttpHandler.Http2UnencryptedSupport", true);

        this.cancellation = new CancellationTokenSource();
        this.channel = GrpcChannel.ForAddress(this.config.RuntimeAddress);

        var client = new GameAgentGateway.GameAgentGatewayClient(this.channel);
        this.stream = client.Connect(cancellationToken: this.cancellation.Token);
        this.receiveTask = Task.Run(() => this.RunAsync(this.cancellation.Token));
    }

    public void SendPlayerInteracted(NPC npc, Farmer player, string trigger)
    {
        this.SendFireAndForget(this.SendPlayerInteractedAsync(npc, player, trigger), "GameEvent");
    }

    private async Task SendPlayerInteractedAsync(NPC npc, Farmer player, string trigger)
    {
        if (!this.IsReady)
        {
            this.monitor.Log("Runtime is not ready; ignoring GameAgent interaction event.", LogLevel.Warn);
            return;
        }

        if (!RuntimeWorldScope.IsAvailable(this.currentWorldId))
        {
            this.monitor.Log("GameAgent world context is unavailable; ignoring interaction event.", LogLevel.Warn);
            return;
        }

        ulong sequence = unchecked((ulong)Interlocked.Increment(ref this.eventSequence));
        GameEvent gameEvent = ProtocolMapper.BuildPlayerInteractedWithNpcEvent(npc, player, trigger, sequence, this.currentWorldId);

        await this.SendAsync(
            new AdapterMessage
            {
                MessageId = ProtocolMapper.NewMessageId("event_msg"),
                Event = gameEvent,
            },
            this.cancellation?.Token ?? CancellationToken.None
        );

        this.monitor.Log($"GameAgent GameEvent sent: {gameEvent.EventId}", LogLevel.Debug);
    }

    public void Dispose()
    {
        this.isReady = false;

        try
        {
            this.cancellation?.Cancel();
            if (this.stream is not null)
                this.stream.RequestStream.CompleteAsync().GetAwaiter().GetResult();
        }
        catch
        {
            // Dispose best-effort only; SMAPI may unload while the stream is already broken.
        }

        this.stream?.Dispose();
        this.channel?.Dispose();
        this.cancellation?.Dispose();
        this.sendMu.Dispose();
    }

    private async Task RunAsync(CancellationToken cancellationToken)
    {
        try
        {
            await this.SendHelloAsync(cancellationToken);

            while (this.stream is not null && await this.stream.ResponseStream.MoveNext(cancellationToken))
            {
                RuntimeMessage message = this.stream.ResponseStream.Current;
                this.TraceRecv(message);
                await this.HandleRuntimeMessageAsync(message, cancellationToken);
            }
        }
        catch (OperationCanceledException)
        {
            this.monitor.Log("GameAgent Runtime stream cancelled.", LogLevel.Debug);
        }
        catch (Exception ex)
        {
            this.monitor.Log($"GameAgent Runtime stream failed: {ex}", LogLevel.Error);
        }
        finally
        {
            this.isReady = false;
        }
    }

    private async Task SendHelloAsync(CancellationToken cancellationToken)
    {
        var hello = new AdapterHello
        {
            AdapterId = this.config.AdapterId,
            AdapterVersion = this.config.AdapterVersion,
            ProtocolVersion = ProtocolVersion,
            GameId = this.config.GameId,
            GameVersion = "unknown",
            SessionId = this.sessionId,
        };

        await this.SendAsync(
            new AdapterMessage
            {
                MessageId = ProtocolMapper.NewMessageId("hello"),
                Hello = hello,
            },
            cancellationToken
        );

        this.monitor.Log($"GameAgent AdapterHello sent to {this.config.RuntimeAddress}.", LogLevel.Info);
    }

    private async Task HandleRuntimeMessageAsync(RuntimeMessage message, CancellationToken cancellationToken)
    {
        switch (message.PayloadCase)
        {
            case RuntimeMessage.PayloadOneofCase.EnvironmentReady:
                this.monitor.Log("GameAgent Runtime EnvironmentReady received.", LogLevel.Info);
                break;

            case RuntimeMessage.PayloadOneofCase.CapabilityRequest:
                await this.SendCapabilitiesAsync(message.MessageId, cancellationToken);
                break;

            case RuntimeMessage.PayloadOneofCase.EventAck:
                this.monitor.Log($"GameAgent EventAck received: {message.EventAck?.EventId}", LogLevel.Debug);
                break;

            case RuntimeMessage.PayloadOneofCase.Observe:
                if (message.Observe is not null)
                    this.dispatcher.Enqueue(() => this.HandleObserveOnMainThread(message.MessageId, message.Observe));
                break;

            case RuntimeMessage.PayloadOneofCase.Action:
                if (message.Action is not null)
                    this.dispatcher.Enqueue(() => this.HandleActionOnMainThread(message.Action));
                break;

            case RuntimeMessage.PayloadOneofCase.CancelAction:
                if (message.CancelAction is not null)
                    this.HandleCancelAction(message.CancelAction);
                break;

            case RuntimeMessage.PayloadOneofCase.Error:
                this.monitor.Log($"GameAgent Runtime error: {message.Error?.Code} {message.Error?.Message}", LogLevel.Warn);
                break;

            default:
                this.monitor.Log($"Ignoring unsupported RuntimeMessage payload: {message.PayloadCase}", LogLevel.Debug);
                break;
        }
    }

    private async Task SendCapabilitiesAsync(string correlationId, CancellationToken cancellationToken)
    {
        CapabilityList capabilities = CapabilityCatalog.BuildEnvironmentCapabilities();

        await this.SendAsync(
            new AdapterMessage
            {
                MessageId = ProtocolMapper.NewMessageId("capabilities_msg"),
                CorrelationId = correlationId,
                Capabilities = capabilities,
            },
            cancellationToken
        );

        this.isReady = true;
        this.monitor.Log($"GameAgent CapabilityList sent: {string.Join(", ", capabilities.Capabilities.Select(capability => capability.Name))}.", LogLevel.Info);
    }

    private void HandleCancelAction(CancelActionRequest request)
    {
        this.actionCancellationRegistry.MarkCancelled(request.ActionId);
        this.monitor.Log($"GameAgent CancelAction recorded: {request.ActionId} reason={request.Reason}", LogLevel.Debug);
    }

    private void HandleObserveOnMainThread(string correlationId, ObserveRequest request)
    {
        try
        {
            if (!RuntimeWorldScope.Matches(request.WorldId, this.currentWorldId))
            {
                string message = RuntimeWorldScope.MismatchMessage(request.WorldId, this.currentWorldId);
                this.monitor.Log($"GameAgent ObserveRequest rejected: {message}", LogLevel.Warn);
                this.SendFireAndForget(
                    this.SendAsync(ProtocolMapper.BuildErrorMessage(correlationId, "world_mismatch", message), this.cancellation?.Token ?? CancellationToken.None),
                    "Observe world mismatch"
                );
                return;
            }

            NPC npc = this.RequireNpc(request.EntityId);
            ProbeObservation probe = this.observationBuilder.Build(npc, Game1.player, "runtime_observe");
            Observation observation = ProtocolMapper.BuildObservation(request.EntityId, probe, this.currentWorldId);

            this.SendFireAndForget(
                this.SendAsync(
                    new AdapterMessage
                    {
                        MessageId = ProtocolMapper.NewMessageId("observation_msg"),
                        CorrelationId = correlationId,
                        Observation = observation,
                    },
                    this.cancellation?.Token ?? CancellationToken.None
                ),
                "Observation"
            );

            this.monitor.Log($"GameAgent Observation sent for {request.EntityId}.", LogLevel.Debug);
        }
        catch (Exception ex)
        {
            this.monitor.Log($"GameAgent ObserveRequest failed: {ex.Message}", LogLevel.Error);
            this.SendFireAndForget(
                this.SendAsync(ProtocolMapper.BuildErrorMessage(correlationId, "observe_failed", ex), this.cancellation?.Token ?? CancellationToken.None),
                "Observe error"
            );
        }
    }

    private void HandleActionOnMainThread(ActionRequest request)
    {
        ActionResult result;

        if (!RuntimeWorldScope.Matches(request.WorldId, this.currentWorldId))
        {
            string message = RuntimeWorldScope.MismatchMessage(request.WorldId, this.currentWorldId);
            result = ProtocolMapper.BuildRejectedActionResult(request, "world_mismatch", message);
            this.monitor.Log($"GameAgent ActionRequest rejected: {message}", LogLevel.Warn);
        }
        else if (this.actionCancellationRegistry.TryConsumeCancelled(request.ActionId))
        {
            result = ProtocolMapper.BuildCancelledActionResult(request, "action cancelled before execution");
            this.monitor.Log($"GameAgent ActionRequest skipped because it was cancelled: {request.ActionId}", LogLevel.Debug);
        }
        else
        {
            try
            {
                result = request.Capability switch
                {
                    "speak" => this.HandleSpeakAction(request),
                    "emote" => this.HandleEmoteAction(request),
                    _ => throw new InvalidOperationException($"unsupported capability: {request.Capability}"),
                };
            }
            catch (Exception ex)
            {
                this.monitor.Log($"GameAgent ActionRequest failed: {ex.Message}", LogLevel.Error);
                result = ProtocolMapper.BuildFailedActionResult(request, "action_failed", ex);
            }
        }

        this.SendFireAndForget(
            this.SendAsync(
                new AdapterMessage
                {
                    MessageId = ProtocolMapper.NewMessageId("action_result_msg"),
                    ActionResult = result,
                },
                this.cancellation?.Token ?? CancellationToken.None
            ),
            "ActionResult"
        );

        this.monitor.Log($"GameAgent ActionResult sent: {result.ActionId} {result.Status}.", LogLevel.Debug);
    }

    private NPC RequireNpc(string entityId)
    {
        if (!Context.IsWorldReady)
            throw new InvalidOperationException("world is not ready");

        if (!ProtocolMapper.TryParseNpcEntityId(entityId, out string npcName))
            throw new InvalidOperationException($"invalid npc entity_id: {entityId}");

        NPC? npc = Game1.getCharacterFromName(npcName, mustBeVillager: true);
        if (npc is null)
            throw new InvalidOperationException($"npc not found: {npcName}");

        return npc;
    }

    public void RefreshWorldContext()
    {
        this.currentWorldId = this.ResolveCurrentWorldId();
        this.monitor.Log($"GameAgent world context refreshed: world_id={this.currentWorldId}", LogLevel.Debug);
    }

    public void ClearWorldContext()
    {
        this.currentWorldId = string.Empty;
    }

    private string ResolveCurrentWorldId()
    {
        if (!Context.IsWorldReady)
            return string.Empty;

        if (!string.IsNullOrWhiteSpace(Constants.SaveFolderName))
            return Constants.SaveFolderName;

        return string.Empty;
    }

    private async Task SendAsync(AdapterMessage message, CancellationToken cancellationToken)
    {
        if (this.stream is null)
            throw new InvalidOperationException("runtime stream is not connected");

        await this.sendMu.WaitAsync(cancellationToken);
        try
        {
            this.TraceSend(message);
            await this.stream.RequestStream.WriteAsync(message);
        }
        finally
        {
            this.sendMu.Release();
        }
    }

    private void TraceSend(AdapterMessage message)
    {
        if (!this.config.EnableProtocolTrace)
            return;

        string detail = message.PayloadCase switch
        {
            AdapterMessage.PayloadOneofCase.Hello =>
                $"AdapterHello message_id={message.MessageId} adapter_id={message.Hello?.AdapterId} game_id={message.Hello?.GameId} session_id={message.Hello?.SessionId}",
            AdapterMessage.PayloadOneofCase.Capabilities =>
                $"CapabilityList message_id={message.MessageId} correlation_id={message.CorrelationId} capabilities=[{string.Join(",", message.Capabilities.Capabilities.Select(capability => capability.Name))}]",
            AdapterMessage.PayloadOneofCase.Event =>
                $"GameEvent message_id={message.MessageId} event_id={message.Event?.EventId} event_type={message.Event?.EventType} world_id={message.Event?.WorldId} target_entity_id={message.Event?.TargetEntityId} entities=[{FormatEntities(message.Event)}]",
            AdapterMessage.PayloadOneofCase.Observation =>
                $"Observation message_id={message.MessageId} correlation_id={message.CorrelationId} entity_id={message.Observation?.EntityId} world_id={message.Observation?.WorldId}",
            AdapterMessage.PayloadOneofCase.ActionResult =>
                $"ActionResult message_id={message.MessageId} action_id={message.ActionResult?.ActionId} status={message.ActionResult?.Status}",
            AdapterMessage.PayloadOneofCase.Error =>
                $"Error message_id={message.MessageId} correlation_id={message.CorrelationId} code={message.Error?.Code} message={message.Error?.Message}",
            _ =>
                $"{message.PayloadCase} message_id={message.MessageId} correlation_id={message.CorrelationId}",
        };

        this.monitor.Log($"[GameAgent][send] {detail}", LogLevel.Info);
    }

    private void TraceRecv(RuntimeMessage message)
    {
        if (!this.config.EnableProtocolTrace)
            return;

        string detail = message.PayloadCase switch
        {
            RuntimeMessage.PayloadOneofCase.EnvironmentReady =>
                $"EnvironmentReady message_id={message.MessageId} session_id={message.EnvironmentReady?.SessionId}",
            RuntimeMessage.PayloadOneofCase.CapabilityRequest =>
                $"CapabilityRequest message_id={message.MessageId}",
            RuntimeMessage.PayloadOneofCase.EventAck =>
                $"EventAck message_id={message.MessageId} correlation_id={message.CorrelationId} event_id={message.EventAck?.EventId} status={message.EventAck?.Status}",
            RuntimeMessage.PayloadOneofCase.Observe =>
                $"ObserveRequest message_id={message.MessageId} entity_id={message.Observe?.EntityId} world_id={message.Observe?.WorldId}",
            RuntimeMessage.PayloadOneofCase.Action =>
                $"ActionRequest message_id={message.MessageId} action_id={message.Action?.ActionId} entity_id={message.Action?.EntityId} world_id={message.Action?.WorldId} capability={message.Action?.Capability} {FormatActionArguments(message.Action)}",
            RuntimeMessage.PayloadOneofCase.CancelAction =>
                $"CancelActionRequest message_id={message.MessageId} action_id={message.CancelAction?.ActionId} reason={message.CancelAction?.Reason}",
            RuntimeMessage.PayloadOneofCase.Error =>
                $"Error message_id={message.MessageId} correlation_id={message.CorrelationId} code={message.Error?.Code} message={message.Error?.Message}",
            _ =>
                $"{message.PayloadCase} message_id={message.MessageId} correlation_id={message.CorrelationId}",
        };

        this.monitor.Log($"[GameAgent][recv] {detail}", LogLevel.Info);
    }

    private ActionResult HandleSpeakAction(ActionRequest request)
    {
        NPC npc = this.RequireNpc(request.EntityId);
        string text = ProtocolMapper.RequireTextArgument(request);

        this.speakCapability.Speak(npc, text);
        return ProtocolMapper.BuildSucceededActionResult(request, text);
    }

    private ActionResult HandleEmoteAction(ActionRequest request)
    {
        NPC npc = this.RequireNpc(request.EntityId);
        string emote = ProtocolMapper.RequireEmoteArgument(request);

        string appliedEmote = this.emoteCapability.Emote(npc, emote);
        return ProtocolMapper.BuildSucceededActionResult(request, "emote", appliedEmote);
    }

    private static string FormatEntities(GameEvent? gameEvent)
    {
        if (gameEvent is null)
            return string.Empty;

        return string.Join(",", gameEvent.Entities.Select(entity => $"{entity.EntityType}:{entity.EntityId}"));
    }

    private static string FormatActionArguments(ActionRequest? request)
    {
        return request?.Capability switch
        {
            "speak" => $"text=\"{FormatStringArgument(request, "text")}\"",
            "emote" => $"emote=\"{FormatStringArgument(request, "emote")}\"",
            _ => string.Empty,
        };
    }

    private static string FormatStringArgument(ActionRequest? request, string name)
    {
        if (request?.Arguments is null || !request.Arguments.Fields.TryGetValue(name, out var value))
            return string.Empty;

        string text = value.StringValue ?? string.Empty;
        return text.Length <= 80 ? text : $"{text[..80]}...";
    }

    private void SendFireAndForget(Task task, string operation)
    {
        _ = task.ContinueWith(
            failed => this.monitor.Log($"GameAgent {operation} send failed: {failed.Exception}", LogLevel.Error),
            TaskContinuationOptions.OnlyOnFaulted
        );
    }
}
