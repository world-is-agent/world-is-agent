using GameAgent.Stardew.Events;

static void Assert(bool condition, string message)
{
    if (!condition)
        throw new InvalidOperationException(message);
}

var linus = new InteractCandidate("Linus", Left: 0, Top: 0, Right: 64, Bottom: 64, TileX: 10, TileY: 10);
var abigail = new InteractCandidate("Abigail", Left: 100, Top: 100, Right: 164, Bottom: 164, TileX: 20, TileY: 20);

InteractCandidate? selected = PlayerInteractTargetSelector.Select(
    new[] { linus, abigail },
    allowedNames: Array.Empty<string>(),
    cursorPixelX: 120,
    cursorPixelY: 120,
    grabTileX: 20,
    grabTileY: 20
);
Assert(selected?.Name == "Abigail", "empty allow-list should accept any villager candidate");

selected = PlayerInteractTargetSelector.Select(
    new[] { linus, abigail },
    allowedNames: new[] { "Linus" },
    cursorPixelX: 120,
    cursorPixelY: 120,
    grabTileX: 20,
    grabTileY: 20
);
Assert(selected is null, "allow-list should reject candidates outside the list");

var adjacent = new InteractCandidate("Abigail", Left: 300, Top: 300, Right: 364, Bottom: 364, TileX: 5.1f, TileY: 5.1f);
var bounding = new InteractCandidate("Linus", Left: 10, Top: 10, Right: 74, Bottom: 74, TileX: 50, TileY: 50);
selected = PlayerInteractTargetSelector.Select(
    new[] { adjacent, bounding },
    allowedNames: Array.Empty<string>(),
    cursorPixelX: 12,
    cursorPixelY: 12,
    grabTileX: 5,
    grabTileY: 5
);
Assert(selected?.Name == "Linus", "bounding-box hit should beat adjacent-tile hit");

var robin = new InteractCandidate("Robin", Left: 300, Top: 300, Right: 364, Bottom: 364, TileX: 6.0f, TileY: 5.0f);
var alex = new InteractCandidate("Alex", Left: 400, Top: 400, Right: 464, Bottom: 464, TileX: 4.0f, TileY: 5.0f);
selected = PlayerInteractTargetSelector.Select(
    new[] { robin, alex },
    allowedNames: Array.Empty<string>(),
    cursorPixelX: 0,
    cursorPixelY: 0,
    grabTileX: 5,
    grabTileY: 5
);
Assert(selected?.Name == "Alex", "equal adjacent distance should tie-break by ordinal npc name");

selected = PlayerInteractTargetSelector.Select(
    new[] { adjacent },
    allowedNames: Array.Empty<string>(),
    cursorPixelX: 0,
    cursorPixelY: 0,
    grabTileX: 5,
    grabTileY: 5,
    allowAdjacentTile: false
);
Assert(selected is null, "left-click mode should not select adjacent tile hits");

selected = PlayerInteractTargetSelector.Select(
    new[] { bounding },
    allowedNames: Array.Empty<string>(),
    cursorPixelX: 12,
    cursorPixelY: 12,
    grabTileX: 5,
    grabTileY: 5,
    allowAdjacentTile: false
);
Assert(selected?.Name == "Linus", "left-click mode should still select direct bounding-box hits");

Console.WriteLine("PlayerInteractProbe selector tests passed.");
