using GameAgent.Protocol.V1Alpha2;

namespace GameAgent.Stardew.Runtime;

public sealed record InteractionContextSnapshot(
    string EventId,
    string WorldId,
    string NpcEntityId,
    string PlayerEntityId,
    string ConversationId,
    string NpcLocation,
    int NpcTileX,
    int NpcTileY,
    string PlayerLocation,
    int PlayerTileX,
    int PlayerTileY,
    int MaxInteractionDistance
);

public sealed record InteractionContextCurrentState(
    string WorldId,
    string NpcEntityId,
    string PlayerEntityId,
    string ConversationId,
    string NpcLocation,
    int NpcTileX,
    int NpcTileY,
    string PlayerLocation,
    int PlayerTileX,
    int PlayerTileY
);

public sealed class InteractionContextStore
{
    private readonly object gate = new();
    private readonly Dictionary<string, PendingInteractionContext> contexts = new(StringComparer.Ordinal);

    public void Reserve(InteractionContextSnapshot snapshot)
    {
        ArgumentNullException.ThrowIfNull(snapshot);
        string eventId = RequireNonEmpty(snapshot.EventId, nameof(snapshot.EventId));

        lock (this.gate)
        {
            if (this.contexts.TryGetValue(eventId, out PendingInteractionContext? existing) && existing.Committed)
                return;

            this.contexts[eventId] = new PendingInteractionContext(snapshot, Committed: false);
        }
    }

    public void Commit(string eventId)
    {
        string normalizedEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            if (!this.contexts.TryGetValue(normalizedEventId, out PendingInteractionContext? pending))
                return;

            this.contexts[normalizedEventId] = pending with { Committed = true };
        }
    }

    public void DiscardPending(string eventId)
    {
        string normalizedEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            if (!this.contexts.TryGetValue(normalizedEventId, out PendingInteractionContext? pending) || pending.Committed)
                return;

            this.contexts.Remove(normalizedEventId);
        }
    }

    public void Release(TurnCompletion? completion)
    {
        if (completion?.EventId is null || string.IsNullOrWhiteSpace(completion.EventId))
            return;

        lock (this.gate)
        {
            this.contexts.Remove(completion.EventId);
        }
    }

    public InteractionContextSnapshot? TryGet(string eventId)
    {
        string normalizedEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            if (!this.contexts.TryGetValue(normalizedEventId, out PendingInteractionContext? pending) || !pending.Committed)
                return null;

            return pending.Snapshot;
        }
    }

    public bool TryResolve(ActionRequest request, out InteractionContextSnapshot? snapshot, out string errorCode, out string message)
    {
        snapshot = null;
        errorCode = string.Empty;
        message = string.Empty;

        if (string.IsNullOrWhiteSpace(request.SourceEventId))
        {
            errorCode = "interaction_context_missing";
            message = "ActionRequest.source_event_id is required for interaction-bound actions";
            return false;
        }

        snapshot = this.TryGet(request.SourceEventId);
        if (snapshot is null)
        {
            errorCode = "interaction_context_missing";
            message = $"interaction context not found for source_event_id {request.SourceEventId}";
            return false;
        }

        if (!string.Equals(snapshot.WorldId, request.WorldId, StringComparison.Ordinal) ||
            !string.Equals(snapshot.NpcEntityId, request.EntityId, StringComparison.Ordinal))
        {
            errorCode = "interaction_context_changed";
            message = "ActionRequest source context does not match requested world or entity";
            return false;
        }

        return true;
    }

    public bool TryValidateCurrentState(InteractionContextSnapshot snapshot, InteractionContextCurrentState current, out string errorCode, out string message)
    {
        ArgumentNullException.ThrowIfNull(snapshot);
        ArgumentNullException.ThrowIfNull(current);
        errorCode = string.Empty;
        message = string.Empty;

        if (!string.Equals(snapshot.WorldId, current.WorldId, StringComparison.Ordinal) ||
            !string.Equals(snapshot.NpcEntityId, current.NpcEntityId, StringComparison.Ordinal) ||
            !string.Equals(snapshot.PlayerEntityId, current.PlayerEntityId, StringComparison.Ordinal) ||
            !string.Equals(snapshot.ConversationId, current.ConversationId, StringComparison.Ordinal) ||
            !string.Equals(snapshot.NpcLocation, current.NpcLocation, StringComparison.Ordinal) ||
            !string.Equals(snapshot.PlayerLocation, current.PlayerLocation, StringComparison.Ordinal) ||
            ManhattanDistance(current.NpcTileX, current.NpcTileY, current.PlayerTileX, current.PlayerTileY) > snapshot.MaxInteractionDistance)
        {
            errorCode = "interaction_context_changed";
            message = "interaction context changed before action execution";
            return false;
        }

        return true;
    }

    public void Clear()
    {
        lock (this.gate)
        {
            this.contexts.Clear();
        }
    }

    private static string RequireNonEmpty(string value, string name)
    {
        if (string.IsNullOrWhiteSpace(value))
            throw new ArgumentException($"{name} must not be empty");

        return value.Trim();
    }

    private static int ManhattanDistance(int leftX, int leftY, int rightX, int rightY)
    {
        return Math.Abs(leftX - rightX) + Math.Abs(leftY - rightY);
    }

    private sealed record PendingInteractionContext(InteractionContextSnapshot Snapshot, bool Committed);
}
