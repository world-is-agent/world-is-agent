using System;

namespace GameAgent.Stardew.Runtime;

public static class RuntimeWorldScope
{
    public static bool IsAvailable(string worldId)
    {
        return !string.IsNullOrWhiteSpace(worldId);
    }

    public static bool Matches(string requestWorldId, string currentWorldId)
    {
        return
            IsAvailable(requestWorldId) &&
            IsAvailable(currentWorldId) &&
            string.Equals(requestWorldId, currentWorldId, StringComparison.Ordinal);
    }

    public static string MismatchMessage(string requestWorldId, string currentWorldId)
    {
        return $"world mismatch: request world_id={requestWorldId}; current world_id={currentWorldId}";
    }
}
