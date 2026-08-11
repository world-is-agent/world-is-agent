using StardewValley;

namespace GameAgent.Stardew.State;

/// <summary>Reads Stardew state and normalizes it into the probe observation.</summary>
public sealed class ObservationBuilder
{
    public ProbeObservation Build(NPC agent, Farmer player, string trigger)
    {
        string agentLocation = agent.currentLocation?.Name ?? "unknown";
        string playerLocation = player.currentLocation?.Name ?? "unknown";

        return new ProbeObservation(
            AgentId: agent.Name,
            AgentName: agent.Name,
            AgentLocation: agentLocation,
            AgentTileX: agent.TilePoint.X,
            AgentTileY: agent.TilePoint.Y,
            GameTime: Game1.timeOfDay,
            PlayerName: player.Name,
            PlayerLocation: playerLocation,
            PlayerTileX: player.TilePoint.X,
            PlayerTileY: player.TilePoint.Y,
            Friendship: player.getFriendshipLevelForNPC(agent.Name),
            Trigger: trigger
        );
    }
}
