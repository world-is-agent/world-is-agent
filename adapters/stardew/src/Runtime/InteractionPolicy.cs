namespace GameAgent.Stardew.Runtime;

public static class InteractionPolicy
{
    public const int MaxInteractionDistance = 2;

    public static bool IsWithinMaxInteractionDistance(int npcTileX, int npcTileY, int playerTileX, int playerTileY)
    {
        return ManhattanDistance(npcTileX, npcTileY, playerTileX, playerTileY) <= MaxInteractionDistance;
    }

    public static int ManhattanDistance(int leftX, int leftY, int rightX, int rightY)
    {
        return Math.Abs(leftX - rightX) + Math.Abs(leftY - rightY);
    }
}
