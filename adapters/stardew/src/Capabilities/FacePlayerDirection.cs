using System;

namespace GameAgent.Stardew.Capabilities;

public static class FacePlayerDirection
{
    public const int Up = 0;
    public const int Right = 1;
    public const int Down = 2;
    public const int Left = 3;

    public static string Resolve(int npcTileX, int npcTileY, int playerTileX, int playerTileY)
    {
        return Resolve(npcTileX, npcTileY, playerTileX, playerTileY, Down);
    }

    public static string Resolve(int npcTileX, int npcTileY, int playerTileX, int playerTileY, int currentDirection)
    {
        int dx = playerTileX - npcTileX;
        int dy = playerTileY - npcTileY;

        if (Math.Abs(dx) >= Math.Abs(dy) && dx != 0)
            return dx > 0 ? "right" : "left";

        if (dy != 0)
            return dy > 0 ? "down" : "up";

        return FromStardewDirection(currentDirection);
    }

    public static int ToStardewDirection(string direction, int currentDirection)
    {
        return direction switch
        {
            "up" => Up,
            "right" => Right,
            "down" => Down,
            "left" => Left,
            _ => currentDirection,
        };
    }

    private static string FromStardewDirection(int direction)
    {
        return direction switch
        {
            Up => "up",
            Right => "right",
            Down => "down",
            Left => "left",
            _ => "down",
        };
    }
}
