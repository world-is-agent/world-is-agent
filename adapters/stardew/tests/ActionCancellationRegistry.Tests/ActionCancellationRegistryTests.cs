using GameAgent.Stardew.Runtime;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class ActionCancellationRegistryTests
{
    [Fact]
    public void UnknownActionIsNotCancelled()
    {
        ActionCancellationRegistry registry = new();

        Assert.False(registry.TryConsumeCancelled("act_1"));
        Assert.False(registry.IsCancelled("act_1"));
    }

    [Fact]
    public void CancelMarkerIsConsumedOnce()
    {
        ActionCancellationRegistry registry = new();

        registry.MarkCancelled("act_1");

        Assert.True(registry.IsCancelled("act_1"));
        Assert.True(registry.TryConsumeCancelled("act_1"));
        Assert.False(registry.TryConsumeCancelled("act_1"));
        Assert.False(registry.IsCancelled("act_1"));
    }

    [Fact]
    public void EmptyActionIdIsIgnored()
    {
        ActionCancellationRegistry registry = new();

        registry.MarkCancelled("");

        Assert.False(registry.TryConsumeCancelled(""));
        Assert.False(registry.IsCancelled(""));
    }

    [Fact]
    public void ClearRemovesCancellationMarker()
    {
        ActionCancellationRegistry registry = new();

        registry.MarkCancelled("act_clear");
        Assert.True(registry.IsCancelled("act_clear"));

        registry.Clear("act_clear");

        Assert.False(registry.IsCancelled("act_clear"));
    }

    [Fact]
    public void ParallelCancellationMarkersAreThreadSafe()
    {
        ActionCancellationRegistry registry = new();

        Parallel.For(0, 100, i => registry.MarkCancelled($"act_parallel_{i}"));
        Parallel.For(0, 100, i => Assert.True(registry.IsCancelled($"act_parallel_{i}")));
        Parallel.For(0, 100, i => Assert.True(registry.TryConsumeCancelled($"act_parallel_{i}")));
        Parallel.For(0, 100, i => Assert.False(registry.TryConsumeCancelled($"act_parallel_{i}")));
    }
}
