using System.Collections.Concurrent;

namespace GameAgent.Stardew.Runtime;

public sealed class ActionCancellationRegistry
{
    private readonly ConcurrentDictionary<string, byte> cancelledActions = new(StringComparer.Ordinal);

    public void MarkCancelled(string actionId)
    {
        if (string.IsNullOrWhiteSpace(actionId))
            return;

        this.cancelledActions[actionId] = 0;
    }

    public bool TryConsumeCancelled(string actionId)
    {
        if (string.IsNullOrWhiteSpace(actionId))
            return false;

        return this.cancelledActions.TryRemove(actionId, out _);
    }

    public bool IsCancelled(string actionId)
    {
        if (string.IsNullOrWhiteSpace(actionId))
            return false;

        return this.cancelledActions.ContainsKey(actionId);
    }

    public void Clear(string actionId)
    {
        if (string.IsNullOrWhiteSpace(actionId))
            return;

        this.cancelledActions.TryRemove(actionId, out _);
    }
}
