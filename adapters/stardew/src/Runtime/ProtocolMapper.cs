using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.State;
using StardewValley;

namespace GameAgent.Stardew.Runtime;

public static partial class ProtocolMapper
{
    public static string ToNpcEntityId(NPC npc)
    {
        return ToNpcEntityId(npc.Name);
    }

    public static GameEvent BuildPlayerInteractedWithNpcEvent(NPC npc, Farmer player, string trigger, ulong sequence, string worldId)
    {
        return BuildPlayerInteractedWithNpcEvent(
            npcEntityId: ToNpcEntityId(npc),
            npcDisplayName: npc.displayName,
            playerEntityId: PlayerEntityId,
            playerDisplayName: player.Name,
            trigger: trigger,
            sequence: sequence,
            worldId: worldId,
            gameTime: BuildGameTime()
        );
    }

    public static Observation BuildObservation(string entityId, ProbeObservation probe, string worldId)
    {
        return BuildObservation(entityId, probe, worldId, BuildGameTime());
    }

    private static GameTime BuildGameTime()
    {
        int rawTime = Game1.timeOfDay;

        return new GameTime
        {
            Year = Game1.year,
            Season = SeasonToNumber(Game1.currentSeason),
            Day = Game1.dayOfMonth,
            Hour = rawTime / 100,
            Minute = rawTime % 100,
            Tick = Game1.ticks,
        };
    }

    private static int SeasonToNumber(string season)
    {
        return season switch
        {
            "spring" => 1,
            "summer" => 2,
            "fall" => 3,
            "winter" => 4,
            _ => 0,
        };
    }
}
