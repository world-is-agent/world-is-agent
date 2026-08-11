using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.State;
using Microsoft.Xna.Framework;
using StardewModdingAPI;
using StardewModdingAPI.Events;
using StardewValley;

namespace GameAgent.Stardew.Events;

/// <summary>Detects the first spike event: player action-button interaction with the target NPC.</summary>
public sealed class PlayerInteractProbe
{
    private readonly string targetAgentName;
    private readonly ObservationBuilder observationBuilder;
    private readonly SpeakCapability speakCapability;
    private readonly IMonitor monitor;
    private readonly IInputHelper input;

    public PlayerInteractProbe(
        string targetAgentName,
        ObservationBuilder observationBuilder,
        SpeakCapability speakCapability,
        IMonitor monitor,
        IInputHelper input
    )
    {
        this.targetAgentName = targetAgentName;
        this.observationBuilder = observationBuilder;
        this.speakCapability = speakCapability;
        this.monitor = monitor;
        this.input = input;
    }

    public bool HandleButtonPressed(ButtonPressedEventArgs e)
    {
        if (!Context.IsWorldReady || Game1.activeClickableMenu is not null || Game1.dialogueUp)
            return false;

        if (!this.IsCandidateInteractionButton(e.Button))
            return false;

        NPC? target = this.FindClickedTarget(e.Cursor);
        if (target is null)
            return false;

        ProbeObservation observation = this.observationBuilder.Build(target, Game1.player, "player_interact");
        this.monitor.Log(observation.ToLogLine(), LogLevel.Info);
        this.input.Suppress(e.Button);
        this.speakCapability.Speak(target, SpeakCapability.ProbeText);

        return true;
    }

    private bool IsCandidateInteractionButton(SButton button)
    {
        return button.IsActionButton() || button is SButton.MouseLeft or SButton.MouseRight;
    }

    private NPC? FindClickedTarget(ICursorPosition cursor)
    {
        NPC? target = Game1.getCharacterFromName(this.targetAgentName, mustBeVillager: true);
        if (target?.currentLocation is null)
            return null;

        if (!ReferenceEquals(target.currentLocation, Game1.currentLocation))
            return null;

        Vector2 absolutePixels = cursor.AbsolutePixels;
        if (target.GetBoundingBox().Contains((int)absolutePixels.X, (int)absolutePixels.Y))
            return target;

        Vector2 grabTile = cursor.GrabTile;
        if (Vector2.Distance(target.Tile, grabTile) <= 1.25f)
            return target;

        return null;
    }
}
