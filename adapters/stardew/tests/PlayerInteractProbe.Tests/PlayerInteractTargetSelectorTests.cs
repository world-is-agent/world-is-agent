using GameAgent.Stardew.Events;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class PlayerInteractTargetSelectorTests
{
    [Fact]
    public void EmptyAllowListAcceptsAnyVillagerCandidate()
    {
        InteractCandidate linus = new("Linus", Left: 0, Top: 0, Right: 64, Bottom: 64, TileX: 10, TileY: 10);
        InteractCandidate abigail = new("Abigail", Left: 100, Top: 100, Right: 164, Bottom: 164, TileX: 20, TileY: 20);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { linus, abigail },
            allowedNames: Array.Empty<string>(),
            cursorPixelX: 120,
            cursorPixelY: 120,
            grabTileX: 20,
            grabTileY: 20
        );

        Assert.Equal("Abigail", selected?.Name);
    }

    [Fact]
    public void AllowListRejectsCandidatesOutsideList()
    {
        InteractCandidate linus = new("Linus", Left: 0, Top: 0, Right: 64, Bottom: 64, TileX: 10, TileY: 10);
        InteractCandidate abigail = new("Abigail", Left: 100, Top: 100, Right: 164, Bottom: 164, TileX: 20, TileY: 20);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { linus, abigail },
            allowedNames: new[] { "Linus" },
            cursorPixelX: 120,
            cursorPixelY: 120,
            grabTileX: 20,
            grabTileY: 20
        );

        Assert.Null(selected);
    }

    [Fact]
    public void BoundingBoxHitBeatsAdjacentTileHit()
    {
        InteractCandidate adjacent = new("Abigail", Left: 300, Top: 300, Right: 364, Bottom: 364, TileX: 5.1f, TileY: 5.1f);
        InteractCandidate bounding = new("Linus", Left: 10, Top: 10, Right: 74, Bottom: 74, TileX: 50, TileY: 50);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { adjacent, bounding },
            allowedNames: Array.Empty<string>(),
            cursorPixelX: 12,
            cursorPixelY: 12,
            grabTileX: 5,
            grabTileY: 5
        );

        Assert.Equal("Linus", selected?.Name);
    }

    [Fact]
    public void EqualAdjacentDistanceTieBreaksByNpcName()
    {
        InteractCandidate robin = new("Robin", Left: 300, Top: 300, Right: 364, Bottom: 364, TileX: 6.0f, TileY: 5.0f);
        InteractCandidate alex = new("Alex", Left: 400, Top: 400, Right: 464, Bottom: 464, TileX: 4.0f, TileY: 5.0f);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { robin, alex },
            allowedNames: Array.Empty<string>(),
            cursorPixelX: 0,
            cursorPixelY: 0,
            grabTileX: 5,
            grabTileY: 5
        );

        Assert.Equal("Alex", selected?.Name);
    }

    [Fact]
    public void LeftClickModeDoesNotSelectAdjacentTileHits()
    {
        InteractCandidate adjacent = new("Abigail", Left: 300, Top: 300, Right: 364, Bottom: 364, TileX: 5.1f, TileY: 5.1f);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { adjacent },
            allowedNames: Array.Empty<string>(),
            cursorPixelX: 0,
            cursorPixelY: 0,
            grabTileX: 5,
            grabTileY: 5,
            allowAdjacentTile: false
        );

        Assert.Null(selected);
    }

    [Fact]
    public void LeftClickModeStillSelectsDirectBoundingBoxHits()
    {
        InteractCandidate bounding = new("Linus", Left: 10, Top: 10, Right: 74, Bottom: 74, TileX: 50, TileY: 50);

        InteractCandidate? selected = PlayerInteractTargetSelector.Select(
            new[] { bounding },
            allowedNames: Array.Empty<string>(),
            cursorPixelX: 12,
            cursorPixelY: 12,
            grabTileX: 5,
            grabTileY: 5,
            allowAdjacentTile: false
        );

        Assert.Equal("Linus", selected?.Name);
    }
}
