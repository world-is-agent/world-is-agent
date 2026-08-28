using System;
using System.Collections.Generic;
using System.Linq;
using GameAgent.Stardew.Dialogue;
using StardewValley;

namespace GameAgent.Stardew.State;

public sealed class ObservationBuilder
{
    private const string NpcEntityPrefix = "npc:";
    private const string PlayerEntityId = "player:local";

    private readonly ConversationStateStore conversationStore;

    public ObservationBuilder(ConversationStateStore conversationStore)
    {
        this.conversationStore = conversationStore;
    }

    public StardewObservation Build(NPC agent, Farmer player, string trigger, string worldId)
    {
        string agentLocation = agent.currentLocation?.Name ?? "unknown";
        string playerLocation = player.currentLocation?.Name ?? "unknown";
        bool relationshipKnown = player.friendshipData.ContainsKey(agent.Name);
        string npcEntityId = ToNpcEntityId(agent.Name);
        ConversationSnapshot? conversation = this.conversationStore.GetActiveConversation(worldId, npcEntityId, PlayerEntityId);

        int friendshipPoints = relationshipKnown ? player.getFriendshipLevelForNPC(agent.Name) : 0;
        bool isSpouse = false;
        bool isRoommate = false;
        if (relationshipKnown && player.friendshipData.TryGetValue(agent.Name, out Friendship? friendship))
        {
            isSpouse = friendship.IsMarried();
            isRoommate = friendship.IsRoommate();
        }

        return StardewObservationFactory.Build(new StardewObservationInput(
            Year: Game1.year,
            Season: Game1.currentSeason,
            DayOfMonth: Game1.dayOfMonth,
            DayOfWeek: Game1.Date.DayOfWeek,
            TimeOfDay: Game1.timeOfDay,
            Rain: Game1.IsRainingHere(),
            Snow: Game1.IsSnowingHere(),
            Lightning: Game1.IsLightningHere(),
            GreenRain: Game1.IsGreenRainingHere(),
            AgentName: agent.Name,
            AgentDisplayName: agent.displayName,
            AgentLocation: agentLocation,
            AgentTileX: agent.TilePoint.X,
            AgentTileY: agent.TilePoint.Y,
            PlayerName: player.Name,
            PlayerLocation: playerLocation,
            PlayerTileX: player.TilePoint.X,
            PlayerTileY: player.TilePoint.Y,
            PlayerGender: player.IsMale ? "male" : "female",
            RelationshipKnown: relationshipKnown,
            FriendshipPoints: friendshipPoints,
            IsSpouse: isSpouse,
            IsRoommate: isRoommate,
            Trigger: trigger,
            NearbyNpcs: BuildNearbyNpcInputs(agent),
            Schedule: BuildScheduleInput(agent)
        )
        {
            Conversation = conversation?.ToObservationInput(),
        });
    }

    private static string ToNpcEntityId(string npcName)
    {
        return $"{NpcEntityPrefix}{npcName}";
    }

    private static IReadOnlyList<StardewNearbyNpcInput> BuildNearbyNpcInputs(NPC agent)
    {
        if (agent.currentLocation?.characters is null)
            return Array.Empty<StardewNearbyNpcInput>();

        var nearby = new List<StardewNearbyNpcInput>();
        foreach (NPC candidate in agent.currentLocation.characters)
        {
            if (candidate.currentLocation is null)
                continue;

            if (!ReferenceEquals(candidate.currentLocation, agent.currentLocation))
                continue;

            if (!candidate.IsVillager)
                continue;

            nearby.Add(new StardewNearbyNpcInput(
                Name: candidate.Name,
                DisplayName: candidate.displayName,
                Location: candidate.currentLocation.Name,
                TileX: candidate.TilePoint.X,
                TileY: candidate.TilePoint.Y
            ));
        }

        return nearby;
    }

    private static StardewScheduleInput? BuildScheduleInput(NPC agent)
    {
        try
        {
            string? destination = null;
            if (agent.DirectionsToNewLocation is not null)
            {
                string targetLocation = agent.DirectionsToNewLocation.targetLocationName;
                if (!string.IsNullOrWhiteSpace(targetLocation) && targetLocation != agent.currentLocation?.Name)
                    destination = targetLocation;
            }

            var futureLocations = new List<string>();
            if (agent.Schedule is not null)
            {
                futureLocations.AddRange(
                    agent.Schedule
                        .Where(entry => entry.Key > Game1.timeOfDay)
                        .OrderBy(entry => entry.Key)
                        .Select(entry => entry.Value?.targetLocationName)
                        .Where(location => !string.IsNullOrWhiteSpace(location))
                        .Select(location => location!)
                );
            }

            if (string.IsNullOrWhiteSpace(destination) && futureLocations.Count == 0)
                return null;

            return new StardewScheduleInput(destination, futureLocations);
        }
        catch
        {
            return null;
        }
    }
}
