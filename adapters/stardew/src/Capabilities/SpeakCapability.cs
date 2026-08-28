using System;
using StardewValley;

namespace GameAgent.Stardew.Capabilities;

/// <summary>Prototype speak capability for displaying a Stardew NPC dialogue.</summary>
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
