using System;
using StardewValley;

namespace GameAgent.Stardew.Capabilities;

public sealed class FacePlayerCapability
{
    public string FacePlayer(NPC npc, Farmer player)
    {
        if (npc.currentLocation is null || player.currentLocation is null || !ReferenceEquals(npc.currentLocation, player.currentLocation))
            throw new ArgumentException("NPC and player are not in the same location");

        string direction = FacePlayerDirection.Resolve(npc.TilePoint.X, npc.TilePoint.Y, player.TilePoint.X, player.TilePoint.Y, npc.FacingDirection);
        npc.faceDirection(FacePlayerDirection.ToStardewDirection(direction, npc.FacingDirection));
        return direction;
    }
}
