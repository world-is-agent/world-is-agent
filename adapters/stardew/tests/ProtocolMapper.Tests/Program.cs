using System;
using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Runtime;
using GameAgent.Stardew.State;
using Google.Protobuf.WellKnownTypes;

static void Assert(bool condition, string message)
{
    if (!condition)
        throw new InvalidOperationException(message);
}

static Struct RequireStruct(Struct parent, string fieldName)
{
    Assert(parent.Fields.TryGetValue(fieldName, out Value? value), $"missing struct field: {fieldName}");
    Assert(value.KindCase == Value.KindOneofCase.StructValue, $"{fieldName} should be a struct");
    return value.StructValue;
}

static ListValue RequireList(Struct parent, string fieldName)
{
    Assert(parent.Fields.TryGetValue(fieldName, out Value? value), $"missing list field: {fieldName}");
    Assert(value.KindCase == Value.KindOneofCase.ListValue, $"{fieldName} should be a list");
    return value.ListValue;
}

static string RequireString(Struct parent, string fieldName)
{
    Assert(parent.Fields.TryGetValue(fieldName, out Value? value), $"missing string field: {fieldName}");
    Assert(value.KindCase == Value.KindOneofCase.StringValue, $"{fieldName} should be a string");
    return value.StringValue;
}

static double RequireNumber(Struct parent, string fieldName)
{
    Assert(parent.Fields.TryGetValue(fieldName, out Value? value), $"missing number field: {fieldName}");
    Assert(value.KindCase == Value.KindOneofCase.NumberValue, $"{fieldName} should be a number");
    return value.NumberValue;
}

GameTime gameTime = new()
{
    Year = 2,
    Season = 3,
    Day = 12,
    Hour = 18,
    Minute = 20,
    Tick = 99,
};

Assert(ProtocolMapper.ToNpcEntityId("Linus") == "npc:Linus", "npc name should map to npc entity id");
Assert(ProtocolMapper.TryParseNpcEntityId("npc:Abigail", out string npcName), "valid npc entity id should parse");
Assert(npcName == "Abigail", "parsed npc name should match");
Assert(!ProtocolMapper.TryParseNpcEntityId("player:local", out _), "non-npc entity id should not parse as npc");
Assert(!ProtocolMapper.TryParseNpcEntityId("npc:", out _), "empty npc name should not parse");

GameEvent gameEvent = ProtocolMapper.BuildPlayerInteractedWithNpcEvent(
    npcEntityId: "npc:Abigail",
    npcDisplayName: "Abigail",
    playerEntityId: "player:local",
    playerDisplayName: "Local Farmer",
    trigger: "player_interact",
    sequence: 42,
    worldId: "Farm_123456",
    gameTime: gameTime
);

Assert(gameEvent.EventType == "player_interacted_with_npc", "event type should be player_interacted_with_npc");
Assert(gameEvent.WorldId == "Farm_123456", "event should carry world_id");
Assert(gameEvent.TargetEntityId == "npc:Abigail", "event should carry target_entity_id");
Assert(gameEvent.Sequence == 42, "event sequence should be preserved");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "npc:Abigail" && entity.EntityType == "npc"), "target npc should be in entities");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "npc:Abigail" && entity.DefinitionId == "npc:Abigail"), "target npc should carry definition_id");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "player:local" && entity.EntityType == "player"), "player should be in entities");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "player:local" && entity.DefinitionId == "player:local"), "player should carry definition_id");
Assert(gameEvent.Payload.Fields["trigger"].StringValue == "player_interact", "payload should keep trigger");
Assert(!gameEvent.Payload.Fields.ContainsKey("target_entity_id"), "payload must not duplicate target_entity_id");

StardewObservation observationModel = StardewObservationFactory.Build(new StardewObservationInput(
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
));

Observation observation = ProtocolMapper.BuildObservation("npc:Abigail", observationModel, "Farm_123456", gameTime);
Assert(observation.EntityId == "npc:Abigail", "observation should carry entity_id");
Assert(observation.WorldId == "Farm_123456", "observation should carry world_id");
Assert(!observation.State.Fields.ContainsKey("definition_id"), "observation state must not publish definition_id");
Assert(!observation.State.Fields.ContainsKey("agent_id"), "observation state must not publish legacy flat agent_id");
Assert(!observation.State.Fields.ContainsKey("friendship"), "observation state must not publish legacy flat friendship");

Struct stardew = RequireStruct(observation.State, "stardew");
Assert(RequireString(stardew, "schema_version") == "0.1", "stardew schema version should be present");

Struct time = RequireStruct(stardew, "time");
Assert(RequireNumber(time, "year") == 2, "time.year should be preserved");
Assert(RequireString(time, "season") == "fall", "time.season should be preserved");
Assert(RequireNumber(time, "day_of_month") == 12, "time.day_of_month should be preserved");
Assert(RequireString(time, "weekday") == "fri", "weekday should use three-letter code");
Assert(RequireNumber(time, "time_of_day") == 1820, "time.time_of_day should be preserved");
Assert(RequireString(time, "time_bucket") == "evening", "time bucket should be derived");

Struct weather = RequireStruct(stardew, "weather");
Assert(weather.Fields["rain"].BoolValue, "weather.rain should be true");
Assert(!weather.Fields["snow"].BoolValue, "weather.snow should be false");
Assert(!weather.Fields["lightning"].BoolValue, "weather.lightning should be false");
Assert(!weather.Fields["green_rain"].BoolValue, "weather.green_rain should be false");

Struct agent = RequireStruct(stardew, "agent");
Assert(RequireString(agent, "entity_id") == "npc:Abigail", "agent entity id should use npc prefix");
Assert(RequireString(agent, "name") == "Abigail", "agent name should use display name");
Assert(RequireString(agent, "location") == "Town", "agent location should be preserved");
Assert(RequireNumber(RequireStruct(agent, "tile"), "x") == 10, "agent tile x should be preserved");

Struct player = RequireStruct(stardew, "player");
Assert(RequireString(player, "entity_id") == "player:local", "player entity id should be stable");
Assert(RequireString(player, "name") == "Local Farmer", "player name should be preserved");
Assert(RequireString(player, "gender") == "female", "player gender should be preserved");

Struct relationship = RequireStruct(stardew, "relationship");
Assert(relationship.Fields["known"].BoolValue, "relationship.known should be true");
Assert(RequireNumber(relationship, "friendship_points") == 850, "friendship points should be present when relationship is known");
Assert(RequireNumber(relationship, "hearts") == 3, "hearts should be derived from friendship points");
Assert(!relationship.Fields["is_spouse"].BoolValue, "is_spouse should be false");
Assert(!relationship.Fields["is_roommate"].BoolValue, "is_roommate should be false");

Struct scene = RequireStruct(stardew, "scene");
Assert(RequireString(scene, "trigger") == "runtime_observe", "scene trigger should be preserved");
Assert(RequireNumber(scene, "nearby_npcs_total") == 7, "nearby total should count same-location non-agent NPCs before top-N");
Assert(RequireNumber(scene, "nearby_npcs_omitted_count") == 2, "omitted count should report top-N truncation");
ListValue nearby = RequireList(scene, "nearby_npcs");
Assert(nearby.Values.Count == 5, "nearby NPCs should be capped at top five");
string[] expectedNearbyIds = new[] { "npc:Alex", "npc:Robin", "npc:Caroline", "npc:Demetrius", "npc:Clint" };
for (int i = 0; i < expectedNearbyIds.Length; i++)
{
    Struct npc = nearby.Values[i].StructValue;
    Assert(RequireString(npc, "entity_id") == expectedNearbyIds[i], $"nearby npc index {i} should be sorted by distance and entity id");
}

string[] nearbyEntityIds = observation.NearbyEntities.Where(entity => entity.EntityType == "npc").Select(entity => entity.EntityId).ToArray();
Assert(nearbyEntityIds.SequenceEqual(expectedNearbyIds), "Observation.nearby_entities should mirror scene.nearby_npcs top-N order");
Assert(observation.NearbyEntities.Any(entity => entity.EntityId == "player:local" && entity.EntityType == "player"), "observation should include local player nearby entity");
Assert(observation.NearbyEntities.All(entity => entity.DefinitionId == entity.EntityId), "EntityRef.definition_id should use entity_id alias in Stardew MVP0");

Struct schedule = RequireStruct(stardew, "schedule");
Assert(RequireString(schedule, "destination") == "Saloon", "schedule destination should be present");
ListValue futureLocations = RequireList(schedule, "future_locations");
Assert(futureLocations.Values.Count == 2, "schedule future locations should be non-empty and distinct");
Assert(futureLocations.Values[0].StringValue == "Saloon", "future location order should be preserved");
Assert(futureLocations.Values[1].StringValue == "Town", "future locations should keep distinct values");

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
Assert(fallbackWeekday.Time.Weekday == observationModel.Time.Weekday, "DayOfWeek and fallback weekday mapping should agree for the same day");

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

Observation unknownRelationshipObservation = ProtocolMapper.BuildObservation("npc:Linus", unknownRelationship, "Farm_123456", gameTime);
Struct unknownRelationshipState = RequireStruct(RequireStruct(unknownRelationshipObservation.State, "stardew"), "relationship");
Assert(!unknownRelationshipState.Fields.ContainsKey("friendship_points"), "unknown relationship should omit friendship points");
Assert(!unknownRelationshipState.Fields.ContainsKey("hearts"), "unknown relationship should omit hearts");
Assert(!RequireStruct(unknownRelationshipObservation.State, "stardew").Fields.ContainsKey("schedule"), "null schedule should be omitted");

CapabilityList capabilities = CapabilityCatalog.BuildEnvironmentCapabilities();
Capability speak = capabilities.Capabilities.Single(capability => capability.Name == "speak");
Capability emote = capabilities.Capabilities.Single(capability => capability.Name == "emote");
Assert(speak.Description.Contains("dialogue text", StringComparison.OrdinalIgnoreCase), "speak description should describe dialogue text effect");
Assert(emote.Description.Contains("emote bubble", StringComparison.OrdinalIgnoreCase), "emote description should describe emote bubble effect");
Assert(speak.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "speak capability should be sequential");
Assert(emote.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "emote capability should be sequential");

ActionRequest actionRequest = new()
{
    ActionId = "act_1",
    EntityId = "npc:Abigail",
    WorldId = "Farm_123456",
};

ActionResult rejected = ProtocolMapper.BuildRejectedActionResult(actionRequest, "world_mismatch", "current world changed");
Assert(rejected.ActionId == "act_1", "rejected action result should keep action id");
Assert(rejected.Status == ActionStatus.Rejected, "world mismatch action result should be rejected");
Assert(rejected.Error.Code == "world_mismatch", "rejected action result should carry error code");

Assert(RuntimeWorldScope.Matches("Farm_123456", "Farm_123456"), "same world_id should match");
Assert(!RuntimeWorldScope.Matches("Farm_123456", "Farm_999999"), "different world_id should not match");
Assert(!RuntimeWorldScope.Matches("", "Farm_123456"), "empty request world_id should not match");
Assert(!RuntimeWorldScope.Matches("Farm_123456", ""), "empty current world_id should not match");
Assert(RuntimeWorldScope.IsAvailable("Farm_123456"), "non-empty current world_id should be available");
Assert(!RuntimeWorldScope.IsAvailable(""), "empty current world_id should not be available");
Assert(!RuntimeWorldScope.IsAvailable("   "), "blank current world_id should not be available");

Console.WriteLine("ProtocolMapper tests passed.");
