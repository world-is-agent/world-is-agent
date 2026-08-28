using System;
using System.Collections.Generic;
using System.Linq;

namespace GameAgent.Stardew.State;

public sealed class StardewObservationFactory
{
    public const int NearbyNpcLimit = 5;
    private const string Unknown = "unknown";
    private const string NpcEntityPrefix = "npc:";
    private const string PlayerEntityId = "player:local";

    private StardewObservationFactory()
    {
    }

    public static StardewObservation Build(StardewObservationInput input)
    {
        StardewEntity agent = new(
            EntityId: ToNpcEntityId(input.AgentName),
            Name: DisplayName(input.AgentDisplayName, input.AgentName),
            Location: NonEmpty(input.AgentLocation, Unknown),
            Tile: new StardewTile(input.AgentTileX, input.AgentTileY)
        );

        IReadOnlyList<StardewEntity> nearbyNpcs = NormalizeNearbyNpcs(input, agent);
        int nearbyTotal = CountNearbyNpcs(input, agent);

        return new StardewObservation(
            SchemaVersion: StardewObservation.CurrentSchemaVersion,
            Time: new StardewTime(
                Year: input.Year,
                Season: NonEmpty(input.Season, Unknown),
                DayOfMonth: input.DayOfMonth,
                Weekday: WeekdayCode(input.DayOfWeek, input.DayOfMonth),
                TimeOfDay: input.TimeOfDay,
                TimeBucket: TimeBucket(input.TimeOfDay)
            ),
            Weather: new StardewWeather(
                Rain: input.Rain,
                Snow: input.Snow,
                Lightning: input.Lightning,
                GreenRain: input.GreenRain
            ),
            Agent: agent,
            Player: new StardewPlayer(
                EntityId: PlayerEntityId,
                Name: NonEmpty(input.PlayerName, Unknown),
                Location: NonEmpty(input.PlayerLocation, Unknown),
                Tile: new StardewTile(input.PlayerTileX, input.PlayerTileY),
                Gender: NonEmpty(input.PlayerGender, Unknown)
            ),
            Relationship: new StardewRelationship(
                Known: input.RelationshipKnown,
                FriendshipPoints: input.RelationshipKnown ? input.FriendshipPoints : null,
                Hearts: input.RelationshipKnown ? input.FriendshipPoints / 250 : null,
                IsSpouse: input.IsSpouse,
                IsRoommate: input.IsRoommate
            ),
            Scene: new StardewScene(
                Trigger: NonEmpty(input.Trigger, Unknown),
                NearbyNpcsTotal: nearbyTotal,
                NearbyNpcsOmittedCount: Math.Max(0, nearbyTotal - nearbyNpcs.Count),
                NearbyNpcs: nearbyNpcs
            ),
            Schedule: NormalizeSchedule(input.Schedule)
        )
        {
            Conversation = NormalizeConversation(input.Conversation),
        };
    }

    private static int CountNearbyNpcs(StardewObservationInput input, StardewEntity agent)
    {
        return (input.NearbyNpcs ?? Array.Empty<StardewNearbyNpcInput>())
            .Count(candidate => IsNearbyCandidate(candidate, input.AgentName, agent.Location));
    }

    private static IReadOnlyList<StardewEntity> NormalizeNearbyNpcs(StardewObservationInput input, StardewEntity agent)
    {
        return (input.NearbyNpcs ?? Array.Empty<StardewNearbyNpcInput>())
            .Where(candidate => IsNearbyCandidate(candidate, input.AgentName, agent.Location))
            .Select(candidate => new StardewEntity(
                EntityId: ToNpcEntityId(candidate.Name),
                Name: DisplayName(candidate.DisplayName, candidate.Name),
                Location: NonEmpty(candidate.Location, Unknown),
                Tile: new StardewTile(candidate.TileX, candidate.TileY)
            ))
            .OrderBy(candidate => ManhattanDistance(agent.Tile, candidate.Tile))
            .ThenBy(candidate => candidate.EntityId, StringComparer.Ordinal)
            .Take(NearbyNpcLimit)
            .ToArray();
    }

    private static bool IsNearbyCandidate(StardewNearbyNpcInput candidate, string agentName, string agentLocation)
    {
        if (string.IsNullOrWhiteSpace(candidate.Name))
            return false;

        if (string.Equals(candidate.Name, agentName, StringComparison.Ordinal))
            return false;

        return string.Equals(NonEmpty(candidate.Location, Unknown), agentLocation, StringComparison.Ordinal);
    }

    private static int ManhattanDistance(StardewTile left, StardewTile right)
    {
        return Math.Abs(left.X - right.X) + Math.Abs(left.Y - right.Y);
    }

    private static StardewSchedule? NormalizeSchedule(StardewScheduleInput? schedule)
    {
        if (schedule is null)
            return null;

        string? destination = NormalizeOptional(schedule.Destination);
        string[] futureLocations = (schedule.FutureLocations ?? Array.Empty<string>())
            .Select(NormalizeOptional)
            .Where(location => location is not null)
            .Select(location => location!)
            .Distinct(StringComparer.Ordinal)
            .ToArray();

        if (destination is null && futureLocations.Length == 0)
            return null;

        return new StardewSchedule(destination, futureLocations);
    }

    private static StardewConversation? NormalizeConversation(StardewConversationInput? conversation)
    {
        if (conversation is null || string.IsNullOrWhiteSpace(conversation.ConversationId) || !conversation.Active)
            return null;

        StardewConversationLine[] lines = (conversation.RecentLines ?? Array.Empty<StardewConversationLineInput>())
            .Select(NormalizeConversationLine)
            .Where(line => line is not null)
            .Select(line => line!)
            .ToArray();

        return new StardewConversation(
            ConversationId: conversation.ConversationId.Trim(),
            Active: true,
            RecentLinesOmittedCount: Math.Max(0, conversation.RecentLinesOmittedCount),
            RecentLines: lines
        );
    }

    private static StardewConversationLine? NormalizeConversationLine(StardewConversationLineInput line)
    {
        string? text = NormalizeOptional(line.Text);
        if (text is null)
            return null;

        return new StardewConversationLine(
            Role: NonEmpty(line.Role, Unknown),
            SpeakerEntityId: NonEmpty(line.SpeakerEntityId, Unknown),
            SpeakerName: NonEmpty(line.SpeakerName, Unknown),
            Text: text,
            TimeOfDay: line.TimeOfDay
        );
    }

    private static string WeekdayCode(DayOfWeek? dayOfWeek, int dayOfMonth)
    {
        if (dayOfWeek.HasValue)
        {
            return dayOfWeek.Value switch
            {
                DayOfWeek.Sunday => "sun",
                DayOfWeek.Monday => "mon",
                DayOfWeek.Tuesday => "tue",
                DayOfWeek.Wednesday => "wed",
                DayOfWeek.Thursday => "thu",
                DayOfWeek.Friday => "fri",
                DayOfWeek.Saturday => "sat",
                _ => "unknown",
            };
        }

        int index = ((dayOfMonth % 7) + 7) % 7;
        return index switch
        {
            0 => "sun",
            1 => "mon",
            2 => "tue",
            3 => "wed",
            4 => "thu",
            5 => "fri",
            6 => "sat",
            _ => "unknown",
        };
    }

    private static string TimeBucket(int timeOfDay)
    {
        return timeOfDay switch
        {
            <= 800 => "early_morning",
            <= 1130 => "late_morning",
            <= 1400 => "midday",
            <= 1700 => "afternoon",
            <= 2200 => "evening",
            _ => "late_night",
        };
    }

    private static string ToNpcEntityId(string name)
    {
        return $"{NpcEntityPrefix}{NonEmpty(name, Unknown)}";
    }

    private static string DisplayName(string displayName, string stableName)
    {
        return NonEmpty(displayName, NonEmpty(stableName, Unknown));
    }

    private static string NonEmpty(string value, string fallback)
    {
        return string.IsNullOrWhiteSpace(value) ? fallback : value.Trim();
    }

    private static string? NormalizeOptional(string? value)
    {
        return string.IsNullOrWhiteSpace(value) ? null : value.Trim();
    }
}
