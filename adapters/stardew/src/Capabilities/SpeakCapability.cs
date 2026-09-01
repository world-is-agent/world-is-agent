using System;
using StardewValley;

namespace GameAgent.Stardew.Capabilities;

/// <summary>
/// Adapter-local one-line NPC dialogue helper retained for probes or special scenes.
/// It is not registered in the production Runtime capability list.
/// </summary>
public sealed class SpeakCapability
{
    public const string ProbeText = "Hello from GameAgent";

    public void Speak(NPC speaker, string text)
    {
        if (string.IsNullOrWhiteSpace(text))
            throw new ArgumentException("Dialogue text must not be empty.", nameof(text));

        Game1.DrawDialogue(new StardewValley.Dialogue(speaker, null, text));
    }
}
