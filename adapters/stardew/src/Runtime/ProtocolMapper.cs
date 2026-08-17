using System;
using System.Threading;
using GameAgent.Protocol.V1Alpha1;
using GameAgent.Stardew.State;
using Google.Protobuf.WellKnownTypes;
using StardewValley;

namespace GameAgent.Stardew.Runtime;

public static class ProtocolMapper
{
    private const string NpcEntityPrefix = "npc:";
    private const string PlayerEntityId = "player:local";
    private static long messageSequence;

    public static string ToNpcEntityId(NPC npc)
    {
        return $"{NpcEntityPrefix}{npc.Name}";
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

    public static GameEvent BuildPlayerInteractedWithNpcEvent(NPC npc, Farmer player, string trigger, ulong sequence)
    {
        return new GameEvent
        {
            EventId = NewMessageId("event"),
            EventType = "player_interacted_with_npc",
            GameTime = BuildGameTime(),
            Payload = new Struct
            {
                Fields =
                {
                    ["trigger"] = Value.ForString(trigger),
                    ["source"] = Value.ForString("stardew-smapi"),
                },
            },
            Sequence = sequence,
            Entities =
            {
                BuildPlayerEntity(player),
                BuildNpcEntity(npc),
            },
        };
    }

    public static Observation BuildObservation(string entityId, ProbeObservation probe)
    {
        return new Observation
        {
            EntityId = entityId,
            Revision = 1,
            GameTime = BuildGameTime(),
            State = new Struct
            {
                Fields =
                {
                    ["agent_id"] = Value.ForString(probe.AgentId),
                    ["agent_name"] = Value.ForString(probe.AgentName),
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
                new EntityRef
                {
                    EntityId = PlayerEntityId,
                    EntityType = "player",
                    DisplayName = probe.PlayerName,
                },
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

    public static AdapterMessage BuildErrorMessage(string correlationId, string code, Exception ex)
    {
        return new AdapterMessage
        {
            MessageId = NewMessageId("error"),
            CorrelationId = correlationId,
            Error = new Error
            {
                Code = code,
                Message = ex.Message,
            },
        };
    }

    public static string NewMessageId(string prefix)
    {
        long sequence = Interlocked.Increment(ref messageSequence);
        return $"{prefix}_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{sequence}";
    }

    private static EntityRef BuildPlayerEntity(Farmer player)
    {
        return new EntityRef
        {
            EntityId = PlayerEntityId,
            EntityType = "player",
            DisplayName = player.Name,
        };
    }

    private static EntityRef BuildNpcEntity(NPC npc)
    {
        return new EntityRef
        {
            EntityId = ToNpcEntityId(npc),
            EntityType = "npc",
            DisplayName = npc.displayName,
        };
    }

    private static GameTime BuildGameTime()
    {
        int rawTime = Game1.timeOfDay;

        return new GameTime
        {
            Year = Game1.year,
            Season = SeasonToNumber(Game1.currentSeason),
            Day = Game1.dayOfMonth,
            Hour = rawTime / 100,
            Minute = rawTime % 100,
            Tick = Game1.ticks,
        };
    }

    private static int SeasonToNumber(string season)
    {
        return season switch
        {
            "spring" => 1,
            "summer" => 2,
            "fall" => 3,
            "winter" => 4,
            _ => 0,
        };
    }
}
