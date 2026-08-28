using System;
using System.Collections.Generic;

namespace GameAgent.Stardew.State;

public sealed record StardewObservation(
    string SchemaVersion,
    StardewTime Time,
    StardewWeather Weather,
    StardewEntity Agent,
    StardewPlayer Player,
    StardewRelationship Relationship,
    StardewScene Scene,
    StardewSchedule? Schedule
)
{
    public const string CurrentSchemaVersion = "0.1";

    public StardewConversation? Conversation { get; init; }
}

public sealed record StardewTime(
    int Year,
    string Season,
    int DayOfMonth,
    string Weekday,
    int TimeOfDay,
    string TimeBucket
);

public sealed record StardewWeather(
    bool Rain,
    bool Snow,
    bool Lightning,
    bool GreenRain
);

public sealed record StardewTile(
    int X,
    int Y
);

public sealed record StardewEntity(
    string EntityId,
    string Name,
    string Location,
    StardewTile Tile
);

public sealed record StardewPlayer(
    string EntityId,
    string Name,
    string Location,
    StardewTile Tile,
    string Gender
);

public sealed record StardewRelationship(
    bool Known,
    int? FriendshipPoints,
    int? Hearts,
    bool IsSpouse,
    bool IsRoommate
);

public sealed record StardewScene(
    string Trigger,
    int NearbyNpcsTotal,
    int NearbyNpcsOmittedCount,
    IReadOnlyList<StardewEntity> NearbyNpcs
);

public sealed record StardewSchedule(
    string? Destination,
    IReadOnlyList<string> FutureLocations
);

public sealed record StardewConversation(
    string ConversationId,
    bool Active,
    int RecentLinesOmittedCount,
    IReadOnlyList<StardewConversationLine> RecentLines
);

public sealed record StardewConversationLine(
    string Role,
    string SpeakerEntityId,
    string SpeakerName,
    string Text,
    int TimeOfDay
);

public sealed record StardewObservationInput(
    int Year,
    string Season,
    int DayOfMonth,
    DayOfWeek? DayOfWeek,
    int TimeOfDay,
    bool Rain,
    bool Snow,
    bool Lightning,
    bool GreenRain,
    string AgentName,
    string AgentDisplayName,
    string AgentLocation,
    int AgentTileX,
    int AgentTileY,
    string PlayerName,
    string PlayerLocation,
    int PlayerTileX,
    int PlayerTileY,
    string PlayerGender,
    bool RelationshipKnown,
    int FriendshipPoints,
    bool IsSpouse,
    bool IsRoommate,
    string Trigger,
    IReadOnlyList<StardewNearbyNpcInput> NearbyNpcs,
    StardewScheduleInput? Schedule
)
{
    public StardewConversationInput? Conversation { get; init; }
}

public sealed record StardewNearbyNpcInput(
    string Name,
    string DisplayName,
    string Location,
    int TileX,
    int TileY
);

public sealed record StardewScheduleInput(
    string? Destination,
    IReadOnlyList<string> FutureLocations
);

public sealed record StardewConversationInput(
    string ConversationId,
    bool Active,
    int RecentLinesOmittedCount,
    IReadOnlyList<StardewConversationLineInput> RecentLines
);

public sealed record StardewConversationLineInput(
    string Role,
    string SpeakerEntityId,
    string SpeakerName,
    string Text,
    int TimeOfDay
);
