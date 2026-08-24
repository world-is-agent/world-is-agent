using System;
using System.Threading;
using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.State;
using Google.Protobuf.WellKnownTypes;

namespace GameAgent.Stardew.Runtime;

public static partial class ProtocolMapper
{
    private const string NpcEntityPrefix = "npc:";
    public const string PlayerEntityId = "player:local";
    private static long messageSequence;

    public static string ToNpcEntityId(string npcName)
    {
        return $"{NpcEntityPrefix}{npcName}";
    }

    public static bool TryParseNpcEntityId(string entityId, out string npcName)
    {
        if (entityId.StartsWith(NpcEntityPrefix, StringComparison.Ordinal) && entityId.Length > NpcEntityPrefix.Length)
        {
            npcName = entityId[NpcEntityPrefix.Length..];
            return true;
        }

        npcName = string.Empty;
        return false;
    }

    public static GameEvent BuildPlayerInteractedWithNpcEvent(
        string npcEntityId,
        string npcDisplayName,
        string playerEntityId,
        string playerDisplayName,
        string trigger,
        ulong sequence,
        string worldId,
        GameTime gameTime
    )
    {
        return new GameEvent
        {
            EventId = NewMessageId("event"),
            EventType = "player_interacted_with_npc",
            GameTime = gameTime,
            Payload = new Struct
            {
                Fields =
                {
                    ["trigger"] = Value.ForString(trigger),
                    ["source"] = Value.ForString("stardew-smapi"),
                },
            },
            Sequence = sequence,
            WorldId = worldId,
            TargetEntityId = npcEntityId,
            Entities =
            {
                BuildEntity(playerEntityId, "player", playerDisplayName),
                BuildEntity(npcEntityId, "npc", npcDisplayName),
            },
        };
    }

    public static Observation BuildObservation(string entityId, ProbeObservation probe, string worldId, GameTime gameTime)
    {
        return new Observation
        {
            EntityId = entityId,
            Revision = 1,
            GameTime = gameTime,
            WorldId = worldId,
            State = new Struct
            {
                Fields =
                {
                    ["agent_id"] = Value.ForString(probe.AgentId),
                    ["agent_name"] = Value.ForString(probe.AgentName),
                    ["definition_id"] = Value.ForString(entityId),
                    ["agent_location"] = Value.ForString(probe.AgentLocation),
                    ["agent_tile_x"] = Value.ForNumber(probe.AgentTileX),
                    ["agent_tile_y"] = Value.ForNumber(probe.AgentTileY),
                    ["game_time"] = Value.ForNumber(probe.GameTime),
                    ["player_name"] = Value.ForString(probe.PlayerName),
                    ["player_location"] = Value.ForString(probe.PlayerLocation),
                    ["player_tile_x"] = Value.ForNumber(probe.PlayerTileX),
                    ["player_tile_y"] = Value.ForNumber(probe.PlayerTileY),
                    ["friendship"] = Value.ForNumber(probe.Friendship),
                    ["trigger"] = Value.ForString(probe.Trigger),
                },
            },
            NearbyEntities =
            {
                BuildEntity(PlayerEntityId, "player", probe.PlayerName),
            },
        };
    }

    public static string RequireTextArgument(ActionRequest request)
    {
        if (request.Arguments is null || !request.Arguments.Fields.TryGetValue("text", out Value? value))
            throw new ArgumentException("missing required speak argument: text");

        string text = value.StringValue;
        if (string.IsNullOrWhiteSpace(text))
            throw new ArgumentException("speak argument text must not be empty");

        return text;
    }

    public static string RequireEmoteArgument(ActionRequest request)
    {
        if (request.Arguments is null || !request.Arguments.Fields.TryGetValue("emote", out Value? value))
            throw new ArgumentException("missing required emote argument: emote");

        string emote = value.StringValue;
        if (string.IsNullOrWhiteSpace(emote))
            throw new ArgumentException("emote argument must not be empty");

        return emote;
    }

    public static ActionResult BuildSucceededActionResult(ActionRequest request, string displayedText)
    {
        return BuildSucceededActionResult(request, "displayed_text", displayedText);
    }

    public static ActionResult BuildSucceededActionResult(ActionRequest request, string outputFieldName, string outputValue)
    {
        return new ActionResult
        {
            ActionId = request.ActionId,
            Status = ActionStatus.Succeeded,
            Output = new Struct
            {
                Fields =
                {
                    [outputFieldName] = Value.ForString(outputValue),
                },
            },
        };
    }

    public static ActionResult BuildFailedActionResult(ActionRequest request, string code, Exception ex)
    {
        return new ActionResult
        {
            ActionId = request.ActionId,
            Status = ActionStatus.Failed,
            Error = new Error
            {
                Code = code,
                Message = ex.Message,
            },
        };
    }

    public static ActionResult BuildRejectedActionResult(ActionRequest request, string code, string message)
    {
        return new ActionResult
        {
            ActionId = request.ActionId,
            Status = ActionStatus.Rejected,
            Error = new Error
            {
                Code = code,
                Message = message,
            },
        };
    }

    public static ActionResult BuildCancelledActionResult(ActionRequest request, string reason)
    {
        return new ActionResult
        {
            ActionId = request.ActionId,
            Status = ActionStatus.Cancelled,
            Error = new Error
            {
                Code = "action_cancelled",
                Message = reason,
            },
        };
    }

    public static AdapterMessage BuildErrorMessage(string correlationId, string code, Exception ex)
    {
        return BuildErrorMessage(correlationId, code, ex.Message);
    }

    public static AdapterMessage BuildErrorMessage(string correlationId, string code, string message)
    {
        return new AdapterMessage
        {
            MessageId = NewMessageId("error"),
            CorrelationId = correlationId,
            Error = new Error
            {
                Code = code,
                Message = message,
            },
        };
    }

    public static string NewMessageId(string prefix)
    {
        long sequence = Interlocked.Increment(ref messageSequence);
        return $"{prefix}_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{sequence}";
    }

    private static EntityRef BuildEntity(string entityId, string entityType, string displayName)
    {
        return new EntityRef
        {
            EntityId = entityId,
            EntityType = entityType,
            DisplayName = displayName,
        };
    }
}
