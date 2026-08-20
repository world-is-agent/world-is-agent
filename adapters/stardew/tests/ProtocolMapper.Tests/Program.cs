using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Runtime;
using GameAgent.Stardew.State;

static void Assert(bool condition, string message)
{
    if (!condition)
        throw new InvalidOperationException(message);
}

GameTime gameTime = new()
{
    Year = 2,
    Season = 3,
    Day = 4,
    Hour = 12,
    Minute = 30,
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
Assert(gameEvent.Entities.Any(entity => entity.EntityId == "player:local" && entity.EntityType == "player"), "player should be in entities");
Assert(gameEvent.Payload.Fields["trigger"].StringValue == "player_interact", "payload should keep trigger");
Assert(!gameEvent.Payload.Fields.ContainsKey("target_entity_id"), "payload must not duplicate target_entity_id");

ProbeObservation probe = new(
    AgentId: "Abigail",
    AgentName: "Abigail",
    AgentLocation: "Town",
    AgentTileX: 10,
    AgentTileY: 11,
    GameTime: 1230,
    PlayerName: "Local Farmer",
    PlayerLocation: "Town",
    PlayerTileX: 12,
    PlayerTileY: 13,
    Friendship: 250,
    Trigger: "runtime_observe"
);

Observation observation = ProtocolMapper.BuildObservation("npc:Abigail", probe, "Farm_123456", gameTime);
Assert(observation.EntityId == "npc:Abigail", "observation should carry entity_id");
Assert(observation.WorldId == "Farm_123456", "observation should carry world_id");
Assert(observation.NearbyEntities.Any(entity => entity.EntityId == "player:local" && entity.EntityType == "player"), "observation should include local player nearby entity");

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
