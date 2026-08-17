using System;
using System.Collections.Generic;
using StardewValley;

namespace GameAgent.Stardew.Capabilities;

/// <summary>Prototype emote capability for displaying a Stardew NPC emote bubble.</summary>
public sealed class EmoteCapability
{
    private static readonly IReadOnlyDictionary<string, int> EmoteIds = new Dictionary<string, int>(StringComparer.OrdinalIgnoreCase)
    {
        ["happy"] = 32,
        ["sad"] = 28,
        ["surprised"] = 16,
        ["neutral"] = 40,
    };

    public string Emote(NPC speaker, string emote)
    {
        if (string.IsNullOrWhiteSpace(emote))
            throw new ArgumentException("Emote name must not be empty.", nameof(emote));

        string normalizedEmote = emote.Trim().ToLowerInvariant();
        if (!EmoteIds.TryGetValue(normalizedEmote, out int emoteId))
            throw new ArgumentException($"Unsupported emote: {emote}", nameof(emote));

        speaker.doEmote(emoteId);
        return normalizedEmote;
    }
}
