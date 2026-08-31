using System;
using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.Dialogue;
using GameAgent.Stardew.Events;
using GameAgent.Stardew.Runtime;
using GameAgent.Stardew.State;
using Google.Protobuf.WellKnownTypes;

static void Assert(bool condition, string message)
{
    if (!condition)
        throw new InvalidOperationException(message);
}

static void AssertInside(DialogueMenuRectangle inner, DialogueMenuRectangle outer, string label)
{
    Assert(inner.Left >= outer.Left, $"{label} should stay inside parent left edge");
    Assert(inner.Top >= outer.Top, $"{label} should stay inside parent top edge");
    Assert(inner.Right <= outer.Right, $"{label} should stay inside parent right edge");
    Assert(inner.Bottom <= outer.Bottom, $"{label} should stay inside parent bottom edge");
}

static void AssertInsideViewport(DialogueMenuRectangle rectangle, int viewportWidth, int viewportHeight, string label)
{
    Assert(rectangle.Left >= 0, $"{label} should stay inside viewport left edge");
    Assert(rectangle.Top >= 0, $"{label} should stay inside viewport top edge");
    Assert(rectangle.Right <= viewportWidth, $"{label} should stay inside viewport right edge");
    Assert(rectangle.Bottom <= viewportHeight, $"{label} should stay inside viewport bottom edge");
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

static void ExpectArgumentException(Action action, string expectedMessage)
{
    try
    {
        action();
    }
    catch (ArgumentException ex) when (ex.Message.Contains(expectedMessage, StringComparison.OrdinalIgnoreCase))
    {
        return;
    }

    throw new InvalidOperationException($"expected ArgumentException containing {expectedMessage}");
}

static Value ValueList(params Value[] values)
{
    ListValue list = new();
    list.Values.AddRange(values);
    return new Value { ListValue = list };
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

ConversationStateStore store = new(new FixedConversationIdGenerator("conv_1", "conv_2", "conv_3", "conv_4"));
string conversationId = store.PrepareInteraction("Farm_123456", "npc:Abigail", "player:local", "event_interact_1");
Assert(conversationId == "conv_1", "first NPC interaction should reserve deterministic conversation id");
Assert(store.GetActiveConversation("Farm_123456", "npc:Abigail", "player:local") is null, "conversation should not be active before EventAck.ACCEPTED");
store.CommitPending("event_interact_1");
ConversationSnapshot? activeConversation = store.GetActiveConversation("Farm_123456", "npc:Abigail", "player:local");
Assert(activeConversation?.ConversationId == "conv_1", "accepted interaction should activate conversation");
string reusedConversationId = store.PrepareInteraction("Farm_123456", "npc:Abigail", "player:local", "event_interact_2");
Assert(reusedConversationId == "conv_1", "same-day active NPC interaction should reuse conversation id");
store.DiscardPending("event_interact_2");
store.PreparePlayerLine("Farm_123456", "npc:Abigail", "player:local", "conv_1", "event_player_1", "player:local", "Local Farmer", "Let's go fishing.", 1820);
store.CommitPending("event_player_1");
activeConversation = store.GetActiveConversation("Farm_123456", "npc:Abigail", "player:local");
Assert(activeConversation?.RecentLines.Single().Text == "Let's go fishing.", "accepted player line should enter conversation state");
string activeDialogueId = store.EnsureConversationId("Farm_123456", "npc:Abigail", "player:local");
Assert(activeDialogueId == "conv_1", "dialogue display should reuse active conversation id");
store.CloseIfConversation("Farm_123456", "npc:Abigail", "player:local", "conv_missing");
Assert(store.GetActiveConversation("Farm_123456", "npc:Abigail", "player:local") is not null, "closing a different conversation id should not close active conversation");
store.CloseIfConversation("Farm_123456", "npc:Abigail", "player:local", "conv_1");
Assert(store.GetActiveConversation("Farm_123456", "npc:Abigail", "player:local") is null, "abandoned dialogue should close matching active conversation");
string reservedDialogueId = store.EnsureConversationId("Farm_123456", "npc:Leah", "player:local");
Assert(reservedDialogueId == "conv_2", "new dialogue display should reserve deterministic conversation id");
Assert(store.GetActiveConversation("Farm_123456", "npc:Leah", "player:local") is null, "reserved dialogue id should not activate before menu display");
store.AppendNpcLineToConversation("Farm_123456", "npc:Leah", "player:local", reservedDialogueId, "Leah", "Hi there.", 930);
ConversationSnapshot? displayedConversation = store.GetActiveConversation("Farm_123456", "npc:Leah", "player:local");
if (displayedConversation is null)
    throw new InvalidOperationException("displayed dialogue should activate reserved conversation");
Assert(displayedConversation.ConversationId == "conv_2", "displayed dialogue should activate reserved conversation id");
Assert(displayedConversation.RecentLines.Single().Text == "Hi there.", "displayed dialogue should append NPC line after display");
ExpectArgumentException(
    () => store.PreparePlayerLine("Farm_123456", "npc:Abigail", "player:local", "conv_1", "event_player_2", "player:local", "Local Farmer", new string('x', 241), 1820),
    "240"
);
conversationId = store.PrepareInteraction("Farm_123456", "npc:Abigail", "player:local", "event_interact_3");
Assert(conversationId == "conv_3", "closed conversation should not be reused for a new interaction");
store.DiscardPending("event_interact_3");
store.Clear();
conversationId = store.PrepareInteraction("Farm_123456", "npc:Abigail", "player:local", "event_interact_4");
Assert(conversationId == "conv_4", "cleared store should start a new conversation id");

InteractionContextStore interactionContexts = new();
InteractionContextSnapshot interactionSnapshot = new(
    EventId: "event_guard_1",
    WorldId: "Farm_123456",
    NpcEntityId: "npc:Abigail",
    PlayerEntityId: "player:local",
    ConversationId: "conv_guard",
    NpcLocation: "Town",
    NpcTileX: 10,
    NpcTileY: 12,
    PlayerLocation: "Town",
    PlayerTileX: 11,
    PlayerTileY: 12,
    MaxInteractionDistance: 2
);
interactionContexts.Reserve(interactionSnapshot);
Assert(interactionContexts.TryGet("event_guard_1") is null, "interaction context should not be visible before EventAck.ACCEPTED");
interactionContexts.Commit("event_guard_1");
InteractionContextSnapshot? committedInteraction = interactionContexts.TryGet("event_guard_1");
Assert(committedInteraction?.ConversationId == "conv_guard", "accepted event should expose interaction context snapshot");
if (committedInteraction is null)
    throw new InvalidOperationException("accepted event should expose a non-null interaction context snapshot");
Assert(committedInteraction.NpcLocation == "Town", "interaction context should retain NPC location");
ActionRequest guardedAction = new()
{
    ActionId = "act_guard",
    EntityId = "npc:Abigail",
    WorldId = "Farm_123456",
    SourceEventId = "event_guard_1",
    SourceTurnId = "turn_guard",
};
Assert(interactionContexts.TryResolve(guardedAction, out committedInteraction, out string guardErrorCode, out _), "guarded action should resolve source event context");
Assert(committedInteraction?.NpcEntityId == "npc:Abigail", "guarded action should resolve NPC context");
if (committedInteraction is null)
    throw new InvalidOperationException("guarded action should resolve a non-null NPC context");
InteractionContextCurrentState currentInteraction = new(
    WorldId: "Farm_123456",
    NpcEntityId: "npc:Abigail",
    PlayerEntityId: "player:local",
    ConversationId: "conv_guard",
    NpcLocation: "Town",
    NpcTileX: 10,
    NpcTileY: 12,
    PlayerLocation: "Town",
    PlayerTileX: 11,
    PlayerTileY: 12
);
Assert(interactionContexts.TryValidateCurrentState(committedInteraction, currentInteraction, out guardErrorCode, out _), "unchanged interaction context should pass effect-time guard");
Assert(!interactionContexts.TryValidateCurrentState(committedInteraction, currentInteraction with { ConversationId = "conv_changed" }, out guardErrorCode, out _), "changed conversation should fail effect-time guard");
Assert(guardErrorCode == "interaction_context_changed", "changed conversation should use interaction_context_changed");
Assert(!interactionContexts.TryValidateCurrentState(committedInteraction, currentInteraction with { PlayerLocation = "Farm" }, out guardErrorCode, out _), "changed player location should fail effect-time guard");
Assert(guardErrorCode == "interaction_context_changed", "changed location should use interaction_context_changed");
Assert(!interactionContexts.TryValidateCurrentState(committedInteraction, currentInteraction with { PlayerTileX = 20 }, out guardErrorCode, out _), "distance beyond max interaction distance should fail effect-time guard");
Assert(guardErrorCode == "interaction_context_changed", "changed distance should use interaction_context_changed");
ActionRequest missingSourceAction = guardedAction.Clone();
missingSourceAction.SourceEventId = "";
Assert(!interactionContexts.TryResolve(missingSourceAction, out _, out guardErrorCode, out _), "missing source_event_id should reject interaction-bound action");
Assert(guardErrorCode == "interaction_context_missing", "missing source_event_id should use interaction_context_missing");
ActionRequest wrongEntityAction = guardedAction.Clone();
wrongEntityAction.EntityId = "npc:Leah";
Assert(!interactionContexts.TryResolve(wrongEntityAction, out _, out guardErrorCode, out _), "source context for a different entity should reject interaction-bound action");
Assert(guardErrorCode == "interaction_context_changed", "wrong entity should use interaction_context_changed");
interactionContexts.Release(new TurnCompletion { EventId = "event_guard_1", Status = TurnCompletionStatus.Completed });
Assert(interactionContexts.TryGet("event_guard_1") is null, "TurnCompletion should release interaction context");
interactionContexts.Reserve(interactionSnapshot with { EventId = "event_guard_2" });
interactionContexts.Discard("event_guard_2");
Assert(interactionContexts.TryGet("event_guard_2") is null, "rejected EventAck should discard reserved interaction context");

GameEvent gameEvent = ProtocolMapper.BuildPlayerInteractedWithNpcEvent(
    npcEntityId: "npc:Abigail",
    npcDisplayName: "Abigail",
    playerEntityId: "player:local",
    playerDisplayName: "Local Farmer",
    conversationId: "conv_1",
    trigger: "action_button",
    sequence: 42,
    worldId: "Farm_123456",
    gameTime: gameTime,
    eventId: "event_interact_4"
);

Assert(gameEvent.EventType == "player_interacted_with_npc", "event type should be player_interacted_with_npc");
Assert(gameEvent.WorldId == "Farm_123456", "event should carry world_id");
Assert(gameEvent.TargetEntityId == "npc:Abigail", "event should carry target_entity_id");
Assert(gameEvent.Sequence == 42, "event sequence should be preserved");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "npc:Abigail" && entity.EntityType == "npc"), "target npc should be in entities");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "npc:Abigail" && entity.DefinitionId == "npc:Abigail"), "target npc should carry definition_id");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "player:local" && entity.EntityType == "player"), "player should be in entities");
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "player:local" && entity.DefinitionId == "player:local"), "player should carry definition_id");
Assert(gameEvent.Payload.Fields["conversation_id"].StringValue == "conv_1", "payload should carry conversation_id");
Assert(gameEvent.Payload.Fields["source"].StringValue == "stardew-smapi", "payload should identify adapter source");
Assert(gameEvent.Payload.Fields["trigger"].StringValue == "action_button", "payload should keep trigger");
Assert(!gameEvent.Payload.Fields.ContainsKey("target_entity_id"), "payload must not duplicate target_entity_id");
Assert(gameEvent.ContextFacts.Count == 0, "player_interacted_with_npc should not carry context facts");

GameEvent playerSaid = ProtocolMapper.BuildPlayerSaidToNpcEvent(
    npcEntityId: "npc:Abigail",
    npcDisplayName: "Abigail",
    playerEntityId: "player:local",
    playerDisplayName: "Local Farmer",
    conversationId: "conv_1",
    inputKind: "option",
    text: "Let's go fishing.",
    selectedOptionIndex: 1,
    trigger: "dialogue_option",
    sequence: 43,
    worldId: "Farm_123456",
    gameTime: gameTime,
    eventId: "event_player_2"
);
Assert(playerSaid.EventType == "player_said_to_npc", "event type should be player_said_to_npc");
Assert(playerSaid.Payload.Fields["conversation_id"].StringValue == "conv_1", "player input should carry conversation_id");
Assert(playerSaid.Payload.Fields["input_kind"].StringValue == "option", "player input should carry input kind");
Assert(playerSaid.Payload.Fields["selected_option_index"].NumberValue == 1, "option input should carry selected option index");
Assert(playerSaid.Payload.Fields["trigger"].StringValue == "dialogue_option", "player input should carry trigger");
Assert(playerSaid.ContextFacts.Count == 1, "player_said_to_npc should carry one context fact");
ContextFact playerSaidFact = playerSaid.ContextFacts[0];
Assert(playerSaidFact.Kind == "utterance", "player_said_to_npc context fact should be an utterance");
Assert(playerSaidFact.ActorEntityId == "player:local", "context fact actor should be the player");
Assert(playerSaidFact.TargetEntityId == "npc:Abigail", "context fact target should be the npc");
Assert(playerSaidFact.ScopeId == "conv_1", "context fact scope should be the conversation id");
Assert(playerSaidFact.Text == "Let's go fishing.", "context fact text should match payload text");
Assert(playerSaidFact.Label == "", "utterance context fact should not require a label");
Assert(playerSaidFact.Attributes.Fields["input_kind"].StringValue == "option", "context fact attributes should carry input kind");
Assert(playerSaidFact.Attributes.Fields["trigger"].StringValue == "dialogue_option", "context fact attributes should carry trigger");
Assert(playerSaidFact.Attributes.Fields["selected_option_index"].NumberValue == 1, "context fact attributes should carry selected option index");
GameEvent freeTextSaid = ProtocolMapper.BuildPlayerSaidToNpcEvent(
    "npc:Abigail",
    "Abigail",
    "player:local",
    "Local Farmer",
    "conv_1",
    "free_text",
    "I need a moment.",
    null,
    "dialogue_free_text",
    44,
    "Farm_123456",
    gameTime
);
Assert(!freeTextSaid.Payload.Fields.ContainsKey("selected_option_index"), "free_text input should not carry selected option index");
Assert(freeTextSaid.ContextFacts.Count == 1, "free_text input should carry one context fact");
Assert(!freeTextSaid.ContextFacts[0].Attributes.Fields.ContainsKey("selected_option_index"), "free_text context fact should not carry selected option index");
ExpectArgumentException(
    () => ProtocolMapper.BuildPlayerSaidToNpcEvent("npc:Abigail", "Abigail", "player:local", "Local Farmer", "conv_1", "option", "Let's go", null, "dialogue_option", 45, "Farm_123456", gameTime),
    "selected_option_index"
);
ExpectArgumentException(
    () => ProtocolMapper.BuildPlayerSaidToNpcEvent("npc:Abigail", "Abigail", "player:local", "Local Farmer", "conv_1", "free_text", new string('x', 241), null, "dialogue_free_text", 44, "Farm_123456", gameTime),
    "240"
);

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

Struct conversation = RequireStruct(stardew, "conversation");
Assert(RequireString(conversation, "conversation_id") == "conv_1", "conversation state should carry active conversation id");
Assert(conversation.Fields["active"].BoolValue, "conversation should be active");
ListValue recentLines = RequireList(conversation, "recent_lines");
Assert(recentLines.Values.Count == 2, "conversation should expose recent lines");
Struct firstLine = recentLines.Values[0].StructValue;
Assert(RequireString(firstLine, "speaker_entity_id") == "npc:Abigail", "conversation line should carry speaker entity id");
Assert(RequireString(firstLine, "speaker_name") == "Abigail", "conversation line should carry speaker display name");
Assert(RequireString(firstLine, "text") == "Want to explore the mines?", "conversation line should carry text");
Assert(RequireNumber(firstLine, "time_of_day") == 1810, "conversation line should carry append time");

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
Capability presentDialogue = capabilities.Capabilities.Single(capability => capability.Name == "present_dialogue");
Capability facePlayer = capabilities.Capabilities.Single(capability => capability.Name == "face_player");
Capability moveTo = capabilities.Capabilities.Single(capability => capability.Name == "move_to");
Assert(speak.Description.Contains("dialogue text", StringComparison.OrdinalIgnoreCase), "speak description should describe dialogue text effect");
Assert(emote.Description.Contains("emote bubble", StringComparison.OrdinalIgnoreCase), "emote description should describe emote bubble effect");
Assert(presentDialogue.Description.Contains("reply options", StringComparison.OrdinalIgnoreCase), "present_dialogue description should describe reply options");
Assert(presentDialogue.Description.Contains("wait for player_said_to_npc", StringComparison.OrdinalIgnoreCase), "present_dialogue description should tell the agent to wait for player input");
Assert(presentDialogue.Description.Contains("only tool call", StringComparison.OrdinalIgnoreCase), "present_dialogue description should describe exclusive tool-call use");
Assert(presentDialogue.Description.Contains("conversation ends", StringComparison.OrdinalIgnoreCase), "present_dialogue description should describe ending without reply inputs");
Assert(facePlayer.Description.Contains("face the player", StringComparison.OrdinalIgnoreCase), "face_player description should describe facing effect");
Assert(moveTo.Description.Contains("reachable tile", StringComparison.OrdinalIgnoreCase), "move_to description should describe tile movement");
Assert(speak.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "speak capability should be sequential");
Assert(emote.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "emote capability should be sequential");
Assert(presentDialogue.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "present_dialogue capability should be sequential");
Assert(facePlayer.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "face_player capability should be sequential");
Assert(moveTo.ConcurrencyMode == CapabilityConcurrencyMode.Sequential, "move_to capability should be sequential");
Assert(moveTo.ExecutionMode == ExecutionMode.Async, "move_to capability should be async");
Assert(moveTo.InputSchemaJson.Contains("\"location\"", StringComparison.Ordinal), "move_to schema should require location");
Assert(moveTo.InputSchemaJson.Contains("\"tile\"", StringComparison.Ordinal), "move_to schema should require tile");
Assert(moveTo.InputSchemaJson.Contains("\"x\":{\"type\":\"integer\"}", StringComparison.Ordinal), "move_to tile x schema should require integer coordinates");
Assert(moveTo.InputSchemaJson.Contains("\"y\":{\"type\":\"integer\"}", StringComparison.Ordinal), "move_to tile y schema should require integer coordinates");
Struct presentDialogueGameAgentExtensions = RequireStruct(presentDialogue.Extensions, "gameagent");
Struct presentDialogueToolPolicy = RequireStruct(presentDialogueGameAgentExtensions, "tool_policy");
Assert(presentDialogueToolPolicy.Fields["exclusive_per_step"].BoolValue, "present_dialogue should declare exclusive_per_step policy");
Assert(presentDialogueToolPolicy.Fields["settle_after_success"].BoolValue, "present_dialogue should declare settle_after_success policy");

ActionRequest presentDialogueRequest = new()
{
    ActionId = "act_present",
    EntityId = "npc:Abigail",
    WorldId = "Farm_123456",
    Capability = "present_dialogue",
    Arguments = new Struct
    {
        Fields =
        {
            ["text"] = Value.ForString("Want to explore the mines?"),
            ["reply_options"] = ValueList(Value.ForString("Yes"), Value.ForString("Maybe later")),
            ["allow_free_text"] = Value.ForBool(true),
        },
    },
};
PresentDialogueInput presentInput = ProtocolMapper.RequirePresentDialogueArgument(presentDialogueRequest);
Assert(presentInput.Text == "Want to explore the mines?", "present_dialogue text should parse");
Assert(presentInput.ReplyOptions.SequenceEqual(new[] { "Yes", "Maybe later" }), "present_dialogue reply options should parse in order");
Assert(presentInput.AllowFreeText, "present_dialogue allow_free_text should parse");
IReadOnlyList<DialogueReplyChoice> inlineTextChoices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
    "Question?",
    new[] { "One", "Two", "Three", "Four" },
    true
));
Assert(inlineTextChoices.Count == 3, "inline free text should reserve the fourth row without becoming a clickable choice");
Assert(inlineTextChoices.Select(choice => choice.Text).SequenceEqual(new[] { "One", "Two", "Three" }), "inline free text should only show the first three generated reply options");
Assert(inlineTextChoices[0].SelectedOptionIndex == 0 && inlineTextChoices[2].SelectedOptionIndex == 2, "generated visible choices should keep their source option indexes");
IReadOnlyList<DialogueReplyChoice> shortInlineTextChoices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
    "Question?",
    new[] { "Only option" },
    true
));
Assert(shortInlineTextChoices.Count == 1, "free text should not add a clickable row when fewer than three generated options exist");
Assert(shortInlineTextChoices[0].Text == "Only option", "generated choice should remain visible before the inline free text row");
IReadOnlyList<DialogueReplyChoice> freeTextOnlyChoices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
    "Question?",
    Array.Empty<string>(),
    true
));
Assert(freeTextOnlyChoices.Count == 0, "free-text-only dialogue should open the text row without a Something else choice");
IReadOnlyList<DialogueReplyChoice> generatedOnlyChoices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
    "Question?",
    new[] { "One", "Two", "Three", "Four" },
    false
));
Assert(generatedOnlyChoices.Count == 4, "without free text all four generated options should remain visible");
int npcLineShows = 0;
int displayedCallbacks = 0;
int replyMenuShows = 0;
int abandonCallbacks = 0;
DialoguePresentationFlow replyFlow = new(
    shouldShowReplyMenu: true,
    onDisplayed: () => displayedCallbacks++,
    onAbandoned: () => abandonCallbacks++
);
replyFlow.Start(() => npcLineShows++);
Assert(npcLineShows == 1, "dialogue flow should show the NPC line when started");
Assert(displayedCallbacks == 1, "dialogue flow should mark displayed immediately after showing the NPC line");
replyFlow.Update(isDialogueUiBusy: true, isReplyMenuActive: false, showReplyMenu: () => replyMenuShows++);
Assert(replyMenuShows == 0, "dialogue flow should wait while the native NPC dialogue is still active");
replyFlow.Update(isDialogueUiBusy: false, isReplyMenuActive: false, showReplyMenu: () => replyMenuShows++);
Assert(replyMenuShows == 1, "dialogue flow should show the reply menu after the native NPC dialogue closes");
replyFlow.MarkSubmitted();
replyFlow.Update(isDialogueUiBusy: false, isReplyMenuActive: false, showReplyMenu: () => replyMenuShows++);
Assert(replyFlow.IsFinished && abandonCallbacks == 0, "submitted dialogue flow should finish without abandonment");

int plainDialogueShows = 0;
DialoguePresentationFlow plainFlow = new(
    shouldShowReplyMenu: false,
    onDisplayed: () => { },
    onAbandoned: () => throw new InvalidOperationException("plain dialogue should not abandon")
);
plainFlow.Start(() => plainDialogueShows++);
plainFlow.Update(isDialogueUiBusy: true, isReplyMenuActive: false, showReplyMenu: () => throw new InvalidOperationException("plain dialogue should not show reply menu"));
plainFlow.Update(isDialogueUiBusy: false, isReplyMenuActive: false, showReplyMenu: () => throw new InvalidOperationException("plain dialogue should not show reply menu"));
Assert(plainDialogueShows == 1 && plainFlow.IsFinished, "dialogue flow without replies should finish after the native NPC dialogue closes");

int closedVisibleUi = 0;
int preemptedAbandonCallbacks = 0;
DialoguePresentationFlow preemptedFlow = new(
    shouldShowReplyMenu: true,
    onDisplayed: () => { },
    onAbandoned: () => preemptedAbandonCallbacks++
);
preemptedFlow.Start(() => { });
preemptedFlow.CloseWithoutSubmission(() => closedVisibleUi++);
Assert(preemptedFlow.IsFinished && closedVisibleUi == 1 && preemptedAbandonCallbacks == 0, "preempted dialogue flow should close visible UI without abandoning conversation");

int closedMenuAbandonCallbacks = 0;
DialoguePresentationFlow closedMenuFlow = new(
    shouldShowReplyMenu: true,
    onDisplayed: () => { },
    onAbandoned: () => closedMenuAbandonCallbacks++
);
closedMenuFlow.Start(() => { });
closedMenuFlow.Update(isDialogueUiBusy: true, isReplyMenuActive: false, showReplyMenu: () => { });
closedMenuFlow.Update(isDialogueUiBusy: false, isReplyMenuActive: false, showReplyMenu: () => { });
closedMenuFlow.Update(isDialogueUiBusy: false, isReplyMenuActive: false, showReplyMenu: () => throw new InvalidOperationException("reply menu should only show once"));
Assert(closedMenuFlow.IsFinished && closedMenuAbandonCallbacks == 1, "closed reply menu should abandon the active conversation once");
DialogueResponseMenuLayout compactReplyLayout = DialogueResponseMenuLayout.Build(
    viewportWidth: 1226,
    viewportHeight: 489,
    optionCount: 3,
    allowFreeText: true
);
Assert(compactReplyLayout.MenuBounds.Top >= 12, "compact response menu should stay inside the top of the viewport");
Assert(compactReplyLayout.MenuBounds.Bottom <= 489, "compact response menu should stay inside the bottom of the viewport");
Assert(compactReplyLayout.TextInputRow is not null, "free-text response layout should include an inline input row");
Assert(compactReplyLayout.SendButton is not null, "free-text response layout should include Send");
Assert(compactReplyLayout.OptionRows.Count == 3, "free-text response layout should still show three generated options");
DialogueResponseMenuLayout generatedOnlyLayout = DialogueResponseMenuLayout.Build(
    viewportWidth: 1226,
    viewportHeight: 489,
    optionCount: 4,
    allowFreeText: false
);
Assert(generatedOnlyLayout.TextInputRow is null, "generated-only response layout should not include an input row");
Assert(generatedOnlyLayout.SendButton is null, "generated-only response layout should not include Send");
DialogueResponseMenuLayout smallViewportLayout = DialogueResponseMenuLayout.Build(
    viewportWidth: 640,
    viewportHeight: 360,
    optionCount: 3,
    allowFreeText: true
);
AssertInsideViewport(smallViewportLayout.MenuBounds, 640, 360, "small viewport response menu");
AssertInside(smallViewportLayout.TitleArea, smallViewportLayout.MenuBounds, "small viewport title area");
AssertInside(smallViewportLayout.CloseButton, smallViewportLayout.MenuBounds, "small viewport close button");
foreach (DialogueMenuRectangle optionRow in smallViewportLayout.OptionRows)
    AssertInside(optionRow, smallViewportLayout.MenuBounds, "small viewport option row");
if (smallViewportLayout.TextInputRow is not DialogueMenuRectangle smallTextInputRow)
    throw new InvalidOperationException("small viewport free-text response layout should include an input row");
if (smallViewportLayout.TextInputTextArea is not DialogueMenuRectangle smallTextInputTextArea)
    throw new InvalidOperationException("small viewport free-text response layout should include an input text area");
if (smallViewportLayout.SendButton is not DialogueMenuRectangle smallSendButton)
    throw new InvalidOperationException("small viewport free-text response layout should include Send");
AssertInside(smallTextInputRow, smallViewportLayout.MenuBounds, "small viewport text input row");
AssertInside(smallTextInputTextArea, smallTextInputRow, "small viewport text input text area");
AssertInside(smallSendButton, smallTextInputRow, "small viewport send button");
Assert(smallTextInputTextArea.Width > 0, "small viewport text input text area should keep positive width");

DialogueSingleLineText trailingText = DialogueSingleLineText.FitTrailingText(
    "ABCDEFGHIJ",
    maxWidth: 5,
    measureText: text => text.Length
);
Assert(trailingText.VisibleText == "FGHIJ", "single-line input should show the trailing text that fits");
Assert(trailingText.CaretOffset == 5, "single-line input caret should align with the visible trailing text");
DialogueSingleLineText exactText = DialogueSingleLineText.FitTrailingText(
    "ABCD",
    maxWidth: 4,
    measureText: text => text.Length
);
Assert(exactText.VisibleText == "ABCD" && exactText.CaretOffset == 4, "single-line input should keep text that already fits");

ActionResult presentResult = ProtocolMapper.BuildPresentDialogueSucceededActionResult(presentDialogueRequest, "conv_1", presentInput);
Assert(presentResult.Status == ActionStatus.Succeeded, "present_dialogue result should succeed after menu display");
Assert(presentResult.Output.Fields["conversation_id"].StringValue == "conv_1", "present_dialogue result should carry conversation id");
Assert(presentResult.Output.Fields["reply_options_count"].NumberValue == 2, "present_dialogue result should carry option count");
Assert(presentResult.Output.Fields["allow_free_text"].BoolValue, "present_dialogue result should carry allow_free_text");
Assert(!presentResult.Output.Fields.ContainsKey("free_text_enabled"), "present_dialogue result should use allow_free_text as the canonical field");
ExpectArgumentException(
    () =>
    {
        ActionRequest invalid = presentDialogueRequest.Clone();
        invalid.Arguments.Fields["text"] = Value.ForString(new string('x', 241));
        ProtocolMapper.RequirePresentDialogueArgument(invalid);
    },
    "240"
);

Assert(FacePlayerDirection.Resolve(npcTileX: 10, npcTileY: 10, playerTileX: 12, playerTileY: 10) == "right", "face_player should prefer horizontal delta when larger");
Assert(FacePlayerDirection.Resolve(npcTileX: 10, npcTileY: 10, playerTileX: 10, playerTileY: 12) == "down", "face_player should map positive y to down");
Assert(FacePlayerDirection.Resolve(npcTileX: 10, npcTileY: 10, playerTileX: 10, playerTileY: 10, currentDirection: FacePlayerDirection.Left) == "left", "face_player should preserve current facing direction when already on the same tile");
ActionResult facePlayerResult = ProtocolMapper.BuildFacePlayerSucceededActionResult(presentDialogueRequest, "down");
Assert(facePlayerResult.Output.Fields["facing"].StringValue == "down", "face_player result should carry facing");
Assert(!facePlayerResult.Output.Fields.ContainsKey("facing_direction"), "face_player result should not expose facing_direction");

ActionRequest moveToRequest = new()
{
    ActionId = "act_move",
    EntityId = "npc:Abigail",
    WorldId = "Farm_123456",
    Capability = "move_to",
    SourceEventId = "event_guard_1",
    SourceTurnId = "turn_guard",
    Arguments = new Struct
    {
        Fields =
        {
            ["location"] = Value.ForString("Town"),
            ["tile"] = new Value
            {
                StructValue = new Struct
                {
                    Fields =
                    {
                        ["x"] = Value.ForNumber(12),
                        ["y"] = Value.ForNumber(20),
                    },
                },
            },
        },
    },
};
MoveToInput moveInput = ProtocolMapper.RequireMoveToArgument(moveToRequest);
Assert(moveInput.Location == "Town", "move_to location should parse");
Assert(moveInput.TileX == 12 && moveInput.TileY == 20, "move_to tile should parse");
ActionResult moveResult = ProtocolMapper.BuildMoveToSucceededActionResult(moveToRequest, new MoveToProgress("Town", 12, 20));
Assert(moveResult.Status == ActionStatus.Succeeded, "move_to result should succeed after reaching target");
Assert(moveResult.Output.Fields["location"].StringValue == "Town", "move_to result should carry current location");
Assert(RequireNumber(moveResult.Output.Fields["tile"].StructValue, "x") == 12, "move_to result should carry current tile x");
Assert(RequireNumber(ProtocolMapper.BuildMoveToStatusMetadata(new MoveToProgress("Town", 12, 20)).Fields["tile"].StructValue, "y") == 20, "move_to status metadata should carry current tile y");
ExpectArgumentException(
    () =>
    {
        ActionRequest invalid = moveToRequest.Clone();
        invalid.Arguments.Fields["tile"].StructValue.Fields["x"] = Value.ForNumber(12.5);
        ProtocolMapper.RequireMoveToArgument(invalid);
    },
    "integer"
);
ExpectArgumentException(
    () =>
    {
        ActionRequest invalid = moveToRequest.Clone();
        invalid.Arguments.Fields.Remove("location");
        ProtocolMapper.RequireMoveToArgument(invalid);
    },
    "location"
);
Assert(PlayerInteractTrigger.FromButton("action") == "action_button", "action button should map to action_button trigger");
Assert(PlayerInteractTrigger.FromButton("mouse_left") == "mouse_left", "left mouse should map to mouse_left trigger");
Assert(PlayerInteractTrigger.FromButton("mouse_right") == "mouse_right", "right mouse should map to mouse_right trigger");

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

public sealed class FixedConversationIdGenerator : IConversationIdGenerator
{
    private readonly Queue<string> ids;

    public FixedConversationIdGenerator(params string[] ids)
    {
        this.ids = new Queue<string>(ids);
    }

    public string NextConversationId()
    {
        if (this.ids.Count == 0)
            throw new InvalidOperationException("no fixed conversation ids remain");

        return this.ids.Dequeue();
    }
}
