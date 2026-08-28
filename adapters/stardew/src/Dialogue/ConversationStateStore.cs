using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading;
using GameAgent.Stardew.State;

namespace GameAgent.Stardew.Dialogue;

public interface IConversationIdGenerator
{
    string NextConversationId();
}

public sealed class ConversationIdGenerator : IConversationIdGenerator
{
    private long sequence;

    public string NextConversationId()
    {
        long id = Interlocked.Increment(ref this.sequence);
        return $"conv_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{id}";
    }
}

public sealed record ConversationLine(
    string Role,
    string SpeakerEntityId,
    string SpeakerName,
    string Text,
    int TimeOfDay
);

public sealed record ConversationSnapshot(
    string ConversationId,
    bool Active,
    int RecentLinesOmittedCount,
    IReadOnlyList<ConversationLine> RecentLines
)
{
    public StardewConversationInput ToObservationInput()
    {
        return new StardewConversationInput(
            ConversationId: this.ConversationId,
            Active: this.Active,
            RecentLinesOmittedCount: this.RecentLinesOmittedCount,
            RecentLines: this.RecentLines.Select(line => new StardewConversationLineInput(
                Role: line.Role,
                SpeakerEntityId: line.SpeakerEntityId,
                SpeakerName: line.SpeakerName,
                Text: line.Text,
                TimeOfDay: line.TimeOfDay
            )).ToArray()
        );
    }
}

public sealed class ConversationStateStore
{
    public const int MaxLineTextChars = 240;
    public const int MaxRecentLines = 12;

    private readonly object gate = new();
    private readonly IConversationIdGenerator idGenerator;
    private readonly Dictionary<ConversationKey, ConversationState> conversations = new();

    public ConversationStateStore(IConversationIdGenerator idGenerator)
    {
        this.idGenerator = idGenerator;
    }

    public string PrepareInteraction(string worldId, string npcEntityId, string playerEntityId, string eventId)
    {
        ConversationKey key = BuildKey(worldId, npcEntityId, playerEntityId);
        string pendingEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            ConversationState state = this.GetOrCreateState(key);
            state.Pending = PendingMutation.Interaction(pendingEventId);
            return state.ConversationId;
        }
    }

    public void PreparePlayerLine(
        string worldId,
        string npcEntityId,
        string playerEntityId,
        string conversationId,
        string eventId,
        string speakerEntityId,
        string speakerName,
        string text,
        int timeOfDay
    )
    {
        ConversationKey key = BuildKey(worldId, npcEntityId, playerEntityId);
        string normalizedConversationId = RequireNonEmpty(conversationId, nameof(conversationId));
        string pendingEventId = RequireNonEmpty(eventId, nameof(eventId));
        ConversationLine line = BuildLine("player", speakerEntityId, speakerName, text, timeOfDay);

        lock (this.gate)
        {
            if (!this.conversations.TryGetValue(key, out ConversationState? state) || !state.Active || state.ConversationId != normalizedConversationId)
                throw new ArgumentException("active conversation not found");

            state.Pending = PendingMutation.PlayerLine(pendingEventId, line);
        }
    }

    public string AppendNpcLine(string worldId, string npcEntityId, string playerEntityId, string speakerName, string text, int timeOfDay)
    {
        ConversationKey key = BuildKey(worldId, npcEntityId, playerEntityId);
        ConversationLine line = BuildLine("npc", npcEntityId, speakerName, text, timeOfDay);

        lock (this.gate)
        {
            ConversationState state = this.GetOrCreateState(key);
            state.Active = true;
            AppendLine(state, line);
            return state.ConversationId;
        }
    }

    public void Close(string worldId, string npcEntityId, string playerEntityId)
    {
        ConversationKey key = BuildKey(worldId, npcEntityId, playerEntityId);
        lock (this.gate)
        {
            if (this.conversations.TryGetValue(key, out ConversationState? state))
            {
                state.Active = false;
                state.Pending = null;
            }
        }
    }

    public void CommitPending(string eventId)
    {
        string pendingEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            foreach (ConversationState state in this.conversations.Values)
            {
                if (state.Pending?.EventId != pendingEventId)
                    continue;

                PendingMutation pending = state.Pending;
                state.Pending = null;
                if (pending.Line is not null)
                    AppendLine(state, pending.Line);
                state.Active = true;
                return;
            }
        }
    }

    public void DiscardPending(string eventId)
    {
        string pendingEventId = RequireNonEmpty(eventId, nameof(eventId));

        lock (this.gate)
        {
            foreach (var pair in this.conversations.ToArray())
            {
                ConversationState state = pair.Value;
                if (state.Pending?.EventId != pendingEventId)
                    continue;

                state.Pending = null;
                if (!state.Active && state.RecentLines.Count == 0)
                    this.conversations.Remove(pair.Key);
                return;
            }
        }
    }

    public ConversationSnapshot? GetActiveConversation(string worldId, string npcEntityId, string playerEntityId)
    {
        ConversationKey key = BuildKey(worldId, npcEntityId, playerEntityId);
        lock (this.gate)
        {
            if (!this.conversations.TryGetValue(key, out ConversationState? state) || !state.Active)
                return null;

            return Snapshot(state);
        }
    }

    public void Clear()
    {
        lock (this.gate)
        {
            this.conversations.Clear();
        }
    }

    private ConversationState GetOrCreateState(ConversationKey key)
    {
        if (this.conversations.TryGetValue(key, out ConversationState? state) && (state.Active || state.Pending is not null))
            return state;

        state = new ConversationState(RequireNonEmpty(this.idGenerator.NextConversationId(), "conversation_id"));
        this.conversations[key] = state;
        return state;
    }

    private static void AppendLine(ConversationState state, ConversationLine line)
    {
        state.RecentLines.Add(line);
        while (state.RecentLines.Count > MaxRecentLines)
        {
            state.RecentLines.RemoveAt(0);
            state.RecentLinesOmittedCount++;
        }
    }

    private static ConversationSnapshot Snapshot(ConversationState state)
    {
        return new ConversationSnapshot(
            ConversationId: state.ConversationId,
            Active: state.Active,
            RecentLinesOmittedCount: state.RecentLinesOmittedCount,
            RecentLines: state.RecentLines.ToArray()
        );
    }

    private static ConversationLine BuildLine(string role, string speakerEntityId, string speakerName, string text, int timeOfDay)
    {
        string normalizedText = RequireNonEmpty(text, "text");
        if (normalizedText.Length > MaxLineTextChars)
            throw new ArgumentException($"conversation text must be {MaxLineTextChars} chars or fewer");

        return new ConversationLine(
            Role: RequireNonEmpty(role, nameof(role)),
            SpeakerEntityId: RequireNonEmpty(speakerEntityId, nameof(speakerEntityId)),
            SpeakerName: RequireNonEmpty(speakerName, nameof(speakerName)),
            Text: normalizedText,
            TimeOfDay: timeOfDay
        );
    }

    private static ConversationKey BuildKey(string worldId, string npcEntityId, string playerEntityId)
    {
        return new ConversationKey(
            WorldId: RequireNonEmpty(worldId, nameof(worldId)),
            NpcEntityId: RequireNonEmpty(npcEntityId, nameof(npcEntityId)),
            PlayerEntityId: RequireNonEmpty(playerEntityId, nameof(playerEntityId))
        );
    }

    private static string RequireNonEmpty(string value, string name)
    {
        if (string.IsNullOrWhiteSpace(value))
            throw new ArgumentException($"{name} must not be empty");

        return value.Trim();
    }

    private sealed class ConversationState
    {
        public ConversationState(string conversationId)
        {
            this.ConversationId = conversationId;
        }

        public string ConversationId { get; }
        public bool Active { get; set; }
        public int RecentLinesOmittedCount { get; set; }
        public List<ConversationLine> RecentLines { get; } = new();
        public PendingMutation? Pending { get; set; }
    }

    private sealed record PendingMutation(string EventId, ConversationLine? Line)
    {
        public static PendingMutation Interaction(string eventId)
        {
            return new PendingMutation(eventId, null);
        }

        public static PendingMutation PlayerLine(string eventId, ConversationLine line)
        {
            return new PendingMutation(eventId, line);
        }
    }

    private sealed record ConversationKey(string WorldId, string NpcEntityId, string PlayerEntityId);
}
