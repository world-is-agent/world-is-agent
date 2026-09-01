using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Runtime;
using GameAgent.Stardew.State;
using Google.Protobuf.WellKnownTypes;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class ProtocolMapperObservationTests
{
    [Fact]
    public void BuildsStructuredStardewObservation()
    {
        StardewObservation observationModel = CreateObservationModel();

        Observation observation = ProtocolMapper.BuildObservation("npc:Abigail", observationModel, "Farm_123456", TestSupport.FixedGameTime());

        Assert.Equal("npc:Abigail", observation.EntityId);
        Assert.Equal("Farm_123456", observation.WorldId);
        Assert.False(observation.State.Fields.ContainsKey("definition_id"));
        Assert.False(observation.State.Fields.ContainsKey("agent_id"));
        Assert.False(observation.State.Fields.ContainsKey("friendship"));

        Struct stardew = TestSupport.RequireStruct(observation.State, "stardew");
        Assert.Equal("0.1", TestSupport.RequireString(stardew, "schema_version"));

        Struct time = TestSupport.RequireStruct(stardew, "time");
        Assert.Equal(2, TestSupport.RequireNumber(time, "year"));
        Assert.Equal("fall", TestSupport.RequireString(time, "season"));
        Assert.Equal(12, TestSupport.RequireNumber(time, "day_of_month"));
        Assert.Equal("fri", TestSupport.RequireString(time, "weekday"));
        Assert.Equal(1820, TestSupport.RequireNumber(time, "time_of_day"));
        Assert.Equal("evening", TestSupport.RequireString(time, "time_bucket"));

        Struct weather = TestSupport.RequireStruct(stardew, "weather");
        Assert.True(weather.Fields["rain"].BoolValue);
        Assert.False(weather.Fields["snow"].BoolValue);
        Assert.False(weather.Fields["lightning"].BoolValue);
        Assert.False(weather.Fields["green_rain"].BoolValue);

        Struct agent = TestSupport.RequireStruct(stardew, "agent");
        Assert.Equal("npc:Abigail", TestSupport.RequireString(agent, "entity_id"));
        Assert.Equal("Abigail", TestSupport.RequireString(agent, "name"));
        Assert.Equal("Town", TestSupport.RequireString(agent, "location"));
        Assert.Equal(10, TestSupport.RequireNumber(TestSupport.RequireStruct(agent, "tile"), "x"));

        Struct player = TestSupport.RequireStruct(stardew, "player");
        Assert.Equal("player:local", TestSupport.RequireString(player, "entity_id"));
        Assert.Equal("Local Farmer", TestSupport.RequireString(player, "name"));
        Assert.Equal("female", TestSupport.RequireString(player, "gender"));

        Struct relationship = TestSupport.RequireStruct(stardew, "relationship");
        Assert.True(relationship.Fields["known"].BoolValue);
        Assert.Equal(850, TestSupport.RequireNumber(relationship, "friendship_points"));
        Assert.Equal(3, TestSupport.RequireNumber(relationship, "hearts"));
        Assert.False(relationship.Fields["is_spouse"].BoolValue);
        Assert.False(relationship.Fields["is_roommate"].BoolValue);

        Struct scene = TestSupport.RequireStruct(stardew, "scene");
        Assert.Equal("runtime_observe", TestSupport.RequireString(scene, "trigger"));
        Assert.Equal(7, TestSupport.RequireNumber(scene, "nearby_npcs_total"));
        Assert.Equal(2, TestSupport.RequireNumber(scene, "nearby_npcs_omitted_count"));
        ListValue nearby = TestSupport.RequireList(scene, "nearby_npcs");
        Assert.Equal(5, nearby.Values.Count);
        string[] expectedNearbyIds = new[] { "npc:Alex", "npc:Robin", "npc:Caroline", "npc:Demetrius", "npc:Clint" };
        for (int i = 0; i < expectedNearbyIds.Length; i++)
        {
            Struct npc = nearby.Values[i].StructValue;
            Assert.Equal(expectedNearbyIds[i], TestSupport.RequireString(npc, "entity_id"));
        }

        string[] nearbyEntityIds = observation.NearbyEntities.Where(entity => entity.EntityType == "npc").Select(entity => entity.EntityId).ToArray();
        Assert.Equal(expectedNearbyIds, nearbyEntityIds);
        Assert.Contains(observation.NearbyEntities, entity => entity.EntityId == "player:local" && entity.EntityType == "player");
        Assert.All(observation.NearbyEntities, entity => Assert.Equal(entity.EntityId, entity.DefinitionId));

        Struct schedule = TestSupport.RequireStruct(stardew, "schedule");
        Assert.Equal("Saloon", TestSupport.RequireString(schedule, "destination"));
        ListValue futureLocations = TestSupport.RequireList(schedule, "future_locations");
        Assert.Equal(2, futureLocations.Values.Count);
        Assert.Equal("Saloon", futureLocations.Values[0].StringValue);
        Assert.Equal("Town", futureLocations.Values[1].StringValue);

        Struct conversation = TestSupport.RequireStruct(stardew, "conversation");
        Assert.Equal("conv_1", TestSupport.RequireString(conversation, "conversation_id"));
        Assert.True(conversation.Fields["active"].BoolValue);
        ListValue recentLines = TestSupport.RequireList(conversation, "recent_lines");
        Assert.Equal(2, recentLines.Values.Count);
        Struct firstLine = recentLines.Values[0].StructValue;
        Assert.Equal("npc:Abigail", TestSupport.RequireString(firstLine, "speaker_entity_id"));
        Assert.Equal("Abigail", TestSupport.RequireString(firstLine, "speaker_name"));
        Assert.Equal("Want to explore the mines?", TestSupport.RequireString(firstLine, "text"));
        Assert.Equal(1810, TestSupport.RequireNumber(firstLine, "time_of_day"));
    }

    [Fact]
    public void FallbackWeekdayMatchesNativeDayOfWeekMapping()
    {
        StardewObservation observationModel = CreateObservationModel();
        StardewObservation fallbackWeekday = StardewObservationFactory.Build(new StardewObservationInput(
            Year: 2,
            Season: "fall",
            DayOfMonth: 12,
            DayOfWeek: null,
            TimeOfDay: 1820,
            Rain: false,
            Snow: false,
            Lightning: false,
            GreenRain: false,
            AgentName: "Abigail",
            AgentDisplayName: "Abigail",
            AgentLocation: "Town",
            AgentTileX: 10,
            AgentTileY: 10,
            PlayerName: "Local Farmer",
            PlayerLocation: "Town",
            PlayerTileX: 12,
            PlayerTileY: 13,
            PlayerGender: "female",
            RelationshipKnown: true,
            FriendshipPoints: 850,
            IsSpouse: false,
            IsRoommate: false,
            Trigger: "runtime_observe",
            NearbyNpcs: Array.Empty<StardewNearbyNpcInput>(),
            Schedule: null
        ));

        Assert.Equal(observationModel.Time.Weekday, fallbackWeekday.Time.Weekday);
    }

    [Fact]
    public void OmitsUnknownRelationshipDetailsAndNullSchedule()
    {
        StardewObservation unknownRelationship = StardewObservationFactory.Build(new StardewObservationInput(
            Year: 1,
            Season: "spring",
            DayOfMonth: 1,
            DayOfWeek: DayOfWeek.Monday,
            TimeOfDay: 800,
            Rain: false,
            Snow: false,
            Lightning: false,
            GreenRain: false,
            AgentName: "Linus",
            AgentDisplayName: "Linus",
            AgentLocation: "Mountain",
            AgentTileX: 35,
            AgentTileY: 12,
            PlayerName: "Local Farmer",
            PlayerLocation: "Mountain",
            PlayerTileX: 34,
            PlayerTileY: 12,
            PlayerGender: "male",
            RelationshipKnown: false,
            FriendshipPoints: 0,
            IsSpouse: false,
            IsRoommate: false,
            Trigger: "runtime_observe",
            NearbyNpcs: Array.Empty<StardewNearbyNpcInput>(),
            Schedule: null
        ));

        Observation observation = ProtocolMapper.BuildObservation("npc:Linus", unknownRelationship, "Farm_123456", TestSupport.FixedGameTime());
        Struct stardew = TestSupport.RequireStruct(observation.State, "stardew");
        Struct relationship = TestSupport.RequireStruct(stardew, "relationship");

        Assert.False(relationship.Fields.ContainsKey("friendship_points"));
        Assert.False(relationship.Fields.ContainsKey("hearts"));
        Assert.False(stardew.Fields.ContainsKey("schedule"));
    }

    private static StardewObservation CreateObservationModel() => StardewObservationFactory.Build(new StardewObservationInput(
        Year: 2,
        Season: "fall",
        DayOfMonth: 12,
        DayOfWeek: DayOfWeek.Friday,
        TimeOfDay: 1820,
        Rain: true,
        Snow: false,
        Lightning: false,
        GreenRain: false,
        AgentName: "Abigail",
        AgentDisplayName: "Abigail",
        AgentLocation: "Town",
        AgentTileX: 10,
        AgentTileY: 10,
        PlayerName: "Local Farmer",
        PlayerLocation: "Town",
        PlayerTileX: 12,
        PlayerTileY: 13,
        PlayerGender: "female",
        RelationshipKnown: true,
        FriendshipPoints: 850,
        IsSpouse: false,
        IsRoommate: false,
        Trigger: "runtime_observe",
        NearbyNpcs: new[]
        {
            new StardewNearbyNpcInput("Zed", "Zed", "Town", 18, 10),
            new StardewNearbyNpcInput("Robin", "Robin", "Town", 11, 10),
            new StardewNearbyNpcInput("Demetrius", "Demetrius", "Town", 13, 10),
            new StardewNearbyNpcInput("Caroline", "Caroline", "Town", 10, 13),
            new StardewNearbyNpcInput("Sam", "Sam", "Forest", 10, 11),
            new StardewNearbyNpcInput("Alex", "Alex", "Town", 10, 11),
            new StardewNearbyNpcInput("Abigail", "Abigail", "Town", 10, 10),
            new StardewNearbyNpcInput("Emily", "Emily", "Town", 10, 14),
            new StardewNearbyNpcInput("Clint", "Clint", "Town", 14, 10),
        },
        Schedule: new StardewScheduleInput("Saloon", new[] { "Saloon", "Town", "", "Saloon" })
    )
    {
        Conversation = new StardewConversationInput(
            ConversationId: "conv_1",
            Active: true,
            RecentLinesOmittedCount: 0,
            RecentLines: new[]
            {
                new StardewConversationLineInput("npc", "npc:Abigail", "Abigail", "Want to explore the mines?", 1810),
                new StardewConversationLineInput("player", "player:local", "Local Farmer", "Maybe after fishing.", 1820),
            }
        ),
    });
}
