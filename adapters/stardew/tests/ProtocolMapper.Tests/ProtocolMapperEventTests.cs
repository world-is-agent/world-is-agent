using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Events;
using GameAgent.Stardew.Runtime;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class ProtocolMapperEventTests
{
    [Fact]
    public void MapsNpcEntityIds()
    {
        Assert.Equal("npc:Linus", ProtocolMapper.ToNpcEntityId("Linus"));
        Assert.True(ProtocolMapper.TryParseNpcEntityId("npc:Abigail", out string npcName));
        Assert.Equal("Abigail", npcName);
        Assert.False(ProtocolMapper.TryParseNpcEntityId("player:local", out _));
        Assert.False(ProtocolMapper.TryParseNpcEntityId("npc:", out _));
    }

    [Fact]
    public void BuildsPlayerInteractedWithNpcEvent()
    {
        GameEvent gameEvent = ProtocolMapper.BuildPlayerInteractedWithNpcEvent(
            npcEntityId: "npc:Abigail",
            npcDisplayName: "Abigail",
            playerEntityId: "player:local",
            playerDisplayName: "Local Farmer",
            conversationId: "conv_1",
            trigger: "action_button",
            sequence: 42,
            worldId: "Farm_123456",
            gameTime: TestSupport.FixedGameTime(),
            eventId: "event_interact_4"
        );

        Assert.Equal("player_interacted_with_npc", gameEvent.EventType);
        Assert.Equal("Farm_123456", gameEvent.WorldId);
        Assert.Equal("npc:Abigail", gameEvent.TargetEntityId);
        Assert.True(gameEvent.Sequence == 42);
        Assert.Contains(gameEvent.Entities, entity => entity.EntityId == "npc:Abigail" && entity.EntityType == "npc");
        Assert.Contains(gameEvent.Entities, entity => entity.EntityId == "npc:Abigail" && entity.DefinitionId == "npc:Abigail");
        Assert.Contains(gameEvent.Entities, entity => entity.EntityId == "player:local" && entity.EntityType == "player");
        Assert.Contains(gameEvent.Entities, entity => entity.EntityId == "player:local" && entity.DefinitionId == "player:local");
        Assert.Equal("conv_1", gameEvent.Payload.Fields["conversation_id"].StringValue);
        Assert.Equal("stardew-smapi", gameEvent.Payload.Fields["source"].StringValue);
        Assert.Equal("action_button", gameEvent.Payload.Fields["trigger"].StringValue);
        Assert.False(gameEvent.Payload.Fields.ContainsKey("target_entity_id"));
        Assert.Empty(gameEvent.ContextFacts);
    }

    [Fact]
    public void BuildsPlayerSaidToNpcEventWithContextFact()
    {
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
            gameTime: TestSupport.FixedGameTime(),
            eventId: "event_player_2"
        );

        Assert.Equal("player_said_to_npc", playerSaid.EventType);
        Assert.Equal("conv_1", playerSaid.Payload.Fields["conversation_id"].StringValue);
        Assert.Equal("option", playerSaid.Payload.Fields["input_kind"].StringValue);
        Assert.Equal(1, playerSaid.Payload.Fields["selected_option_index"].NumberValue);
        Assert.Equal("dialogue_option", playerSaid.Payload.Fields["trigger"].StringValue);

        ContextFact playerSaidFact = Assert.Single(playerSaid.ContextFacts);
        Assert.Equal("utterance", playerSaidFact.Kind);
        Assert.Equal("player:local", playerSaidFact.ActorEntityId);
        Assert.Equal("npc:Abigail", playerSaidFact.TargetEntityId);
        Assert.Equal("conv_1", playerSaidFact.ScopeId);
        Assert.Equal("Let's go fishing.", playerSaidFact.Text);
        Assert.Equal("", playerSaidFact.Label);
        Assert.Equal("option", playerSaidFact.Attributes.Fields["input_kind"].StringValue);
        Assert.Equal("dialogue_option", playerSaidFact.Attributes.Fields["trigger"].StringValue);
        Assert.Equal(1, playerSaidFact.Attributes.Fields["selected_option_index"].NumberValue);
    }

    [Fact]
    public void BuildsFreeTextPlayerSaidEventWithoutSelectedOptionIndex()
    {
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
            TestSupport.FixedGameTime()
        );

        Assert.False(freeTextSaid.Payload.Fields.ContainsKey("selected_option_index"));
        ContextFact contextFact = Assert.Single(freeTextSaid.ContextFacts);
        Assert.False(contextFact.Attributes.Fields.ContainsKey("selected_option_index"));
    }

    [Fact]
    public void RejectsInvalidPlayerSaidEvents()
    {
        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.BuildPlayerSaidToNpcEvent("npc:Abigail", "Abigail", "player:local", "Local Farmer", "conv_1", "option", "Let's go", null, "dialogue_option", 45, "Farm_123456", TestSupport.FixedGameTime()),
            "selected_option_index"
        );
        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.BuildPlayerSaidToNpcEvent("npc:Abigail", "Abigail", "player:local", "Local Farmer", "conv_1", "free_text", new string('x', 241), null, "dialogue_free_text", 44, "Farm_123456", TestSupport.FixedGameTime()),
            "240"
        );
    }

    [Theory]
    [InlineData("action", "action_button")]
    [InlineData("mouse_left", "mouse_left")]
    [InlineData("mouse_right", "mouse_right")]
    public void MapsButtonToProtocolTrigger(string button, string expectedTrigger)
    {
        Assert.Equal(expectedTrigger, PlayerInteractTrigger.FromButton(button));
    }

    [Fact]
    public void RuntimeWorldScopeMatchesOnlyAvailableSameWorldIds()
    {
        Assert.True(RuntimeWorldScope.Matches("Farm_123456", "Farm_123456"));
        Assert.False(RuntimeWorldScope.Matches("Farm_123456", "Farm_999999"));
        Assert.False(RuntimeWorldScope.Matches("", "Farm_123456"));
        Assert.False(RuntimeWorldScope.Matches("Farm_123456", ""));
        Assert.True(RuntimeWorldScope.IsAvailable("Farm_123456"));
        Assert.False(RuntimeWorldScope.IsAvailable(""));
        Assert.False(RuntimeWorldScope.IsAvailable("   "));
    }
}
