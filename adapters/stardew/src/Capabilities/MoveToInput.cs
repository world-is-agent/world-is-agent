namespace GameAgent.Stardew.Capabilities;

public sealed record MoveToInput(string Location, int TileX, int TileY);

public sealed record MoveToProgress(string Location, int TileX, int TileY);

public sealed record MoveToStart(MoveToProgress Progress, bool AlreadyAtTarget);
