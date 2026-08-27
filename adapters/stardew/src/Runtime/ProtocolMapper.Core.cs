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

    public static Observation BuildObservation(string entityId, StardewObservation stardewObservation, string worldId, GameTime gameTime)
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
                    ["stardew"] = ForStruct(BuildStardewState(stardewObservation)),
                },
            },
            NearbyEntities =
            {
                BuildEntity(PlayerEntityId, "player", stardewObservation.Player.Name),
                stardewObservation.Scene.NearbyNpcs.Select(npc => BuildEntity(npc.EntityId, "npc", npc.Name)),
            },
        };
    }

    private static Struct BuildStardewState(StardewObservation observation)
    {
        Struct stardew = new()
        {
            Fields =
            {
                ["schema_version"] = Value.ForString(observation.SchemaVersion),
                ["time"] = ForStruct(BuildTime(observation.Time)),
                ["weather"] = ForStruct(BuildWeather(observation.Weather)),
                ["agent"] = ForStruct(BuildStateEntity(observation.Agent)),
                ["player"] = ForStruct(BuildPlayer(observation.Player)),
                ["relationship"] = ForStruct(BuildRelationship(observation.Relationship)),
                ["scene"] = ForStruct(BuildScene(observation.Scene)),
            },
        };

        if (observation.Schedule is not null)
            stardew.Fields["schedule"] = ForStruct(BuildSchedule(observation.Schedule));

        return stardew;
    }

    private static Struct BuildTime(StardewTime time)
    {
        return new Struct
        {
            Fields =
            {
                ["year"] = Value.ForNumber(time.Year),
                ["season"] = Value.ForString(time.Season),
                ["day_of_month"] = Value.ForNumber(time.DayOfMonth),
                ["weekday"] = Value.ForString(time.Weekday),
                ["time_of_day"] = Value.ForNumber(time.TimeOfDay),
                ["time_bucket"] = Value.ForString(time.TimeBucket),
            },
        };
    }

    private static Struct BuildWeather(StardewWeather weather)
    {
        return new Struct
        {
            Fields =
            {
                ["rain"] = Value.ForBool(weather.Rain),
                ["snow"] = Value.ForBool(weather.Snow),
                ["lightning"] = Value.ForBool(weather.Lightning),
                ["green_rain"] = Value.ForBool(weather.GreenRain),
            },
        };
    }

    private static Struct BuildStateEntity(StardewEntity entity)
    {
        return new Struct
        {
            Fields =
            {
                ["entity_id"] = Value.ForString(entity.EntityId),
                ["name"] = Value.ForString(entity.Name),
                ["location"] = Value.ForString(entity.Location),
                ["tile"] = ForStruct(BuildTile(entity.Tile)),
            },
        };
    }

    private static Struct BuildPlayer(StardewPlayer player)
    {
        Struct playerStruct = BuildStateEntity(new StardewEntity(player.EntityId, player.Name, player.Location, player.Tile));
        playerStruct.Fields["gender"] = Value.ForString(player.Gender);
        return playerStruct;
    }

    private static Struct BuildTile(StardewTile tile)
    {
        return new Struct
        {
            Fields =
            {
                ["x"] = Value.ForNumber(tile.X),
                ["y"] = Value.ForNumber(tile.Y),
            },
        };
    }

    private static Struct BuildRelationship(StardewRelationship relationship)
    {
        Struct relationshipStruct = new()
        {
            Fields =
            {
                ["known"] = Value.ForBool(relationship.Known),
                ["is_spouse"] = Value.ForBool(relationship.IsSpouse),
                ["is_roommate"] = Value.ForBool(relationship.IsRoommate),
            },
        };

        if (relationship.Known)
        {
            if (relationship.FriendshipPoints.HasValue)
                relationshipStruct.Fields["friendship_points"] = Value.ForNumber(relationship.FriendshipPoints.Value);

            if (relationship.Hearts.HasValue)
                relationshipStruct.Fields["hearts"] = Value.ForNumber(relationship.Hearts.Value);
        }

        return relationshipStruct;
    }

    private static Struct BuildScene(StardewScene scene)
    {
        return new Struct
        {
            Fields =
            {
                ["trigger"] = Value.ForString(scene.Trigger),
                ["nearby_npcs_total"] = Value.ForNumber(scene.NearbyNpcsTotal),
                ["nearby_npcs_omitted_count"] = Value.ForNumber(scene.NearbyNpcsOmittedCount),
                ["nearby_npcs"] = ForList(scene.NearbyNpcs.Select(npc => ForStruct(BuildStateEntity(npc)))),
            },
        };
    }

    private static Struct BuildSchedule(StardewSchedule schedule)
    {
        Struct scheduleStruct = new()
        {
            Fields =
            {
                ["future_locations"] = ForList(schedule.FutureLocations.Select(Value.ForString)),
            },
        };

        if (!string.IsNullOrWhiteSpace(schedule.Destination))
            scheduleStruct.Fields["destination"] = Value.ForString(schedule.Destination);

        return scheduleStruct;
    }

    private static Value ForStruct(Struct structure)
    {
        return new Value { StructValue = structure };
    }

    private static Value ForList(IEnumerable<Value> values)
    {
        ListValue list = new();
        list.Values.AddRange(values);
        return new Value { ListValue = list };
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
            // Stardew MVP0 uses stable game entity IDs as definition aliases.
            DefinitionId = entityId,
        };
    }
}
