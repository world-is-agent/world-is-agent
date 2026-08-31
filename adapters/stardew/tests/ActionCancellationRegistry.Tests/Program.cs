using GameAgent.Stardew.Runtime;

static void Assert(bool condition, string message)
{
    if (!condition)
        throw new InvalidOperationException(message);
}

var registry = new ActionCancellationRegistry();

Assert(!registry.TryConsumeCancelled("act_1"), "unknown action should not be cancelled");

registry.MarkCancelled("act_1");
Assert(registry.IsCancelled("act_1"), "marked action should be visible as cancelled before consume");
Assert(registry.TryConsumeCancelled("act_1"), "cancelled action should be consumed once");
Assert(!registry.TryConsumeCancelled("act_1"), "cancel marker should be removed after consume");
Assert(!registry.IsCancelled("act_1"), "consumed action should no longer be visible as cancelled");

registry.MarkCancelled("");
Assert(!registry.TryConsumeCancelled(""), "empty action id should be ignored");
Assert(!registry.IsCancelled(""), "empty action id should not be visible as cancelled");

registry.MarkCancelled("act_clear");
Assert(registry.IsCancelled("act_clear"), "marked action should be visible before clear");
registry.Clear("act_clear");
Assert(!registry.IsCancelled("act_clear"), "clear should remove cancellation marker");

Parallel.For(0, 100, i => registry.MarkCancelled($"act_parallel_{i}"));
Parallel.For(0, 100, i => Assert(registry.IsCancelled($"act_parallel_{i}"), $"parallel action {i} should be visible as cancelled"));
Parallel.For(0, 100, i => Assert(registry.TryConsumeCancelled($"act_parallel_{i}"), $"parallel action {i} should be cancelled"));
Parallel.For(0, 100, i => Assert(!registry.TryConsumeCancelled($"act_parallel_{i}"), $"parallel action {i} should be consumed only once"));

Console.WriteLine("ActionCancellationRegistry tests passed.");
