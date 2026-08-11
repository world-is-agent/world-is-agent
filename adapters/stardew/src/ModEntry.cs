using System;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.Events;
using GameAgent.Stardew.State;
using StardewModdingAPI;
using StardewModdingAPI.Events;
using StardewValley;

namespace GameAgent.Stardew;

/// <summary>SMAPI entry point for the Adapter Capability Spike.</summary>
public sealed class ModEntry : Mod
{
    private const string TargetAgentName = "Linus";

    private ObservationBuilder? observationBuilder;
    private SpeakCapability? speakCapability;
    private PlayerInteractProbe? playerInteractProbe;

    public override void Entry(IModHelper helper)
    {
        this.observationBuilder = new ObservationBuilder();
        this.speakCapability = new SpeakCapability();
        this.playerInteractProbe = new PlayerInteractProbe(
            TargetAgentName,
            this.observationBuilder,
            this.speakCapability,
            this.Monitor,
            helper.Input
        );

        helper.Events.Input.ButtonPressed += this.OnButtonPressed;
        helper.ConsoleCommands.Add(
            "gameagent_probe_linus",
            "Run the GameAgent Linus probe without clicking.",
            this.RunProbeCommand
        );

        this.Monitor.Log("GameAgent Stardew Adapter Probe loaded.", LogLevel.Info);
    }

    private void OnButtonPressed(object? sender, ButtonPressedEventArgs e)
    {
        try
        {
            this.playerInteractProbe?.HandleButtonPressed(e);
        }
        catch (Exception ex)
        {
            this.Monitor.Log($"GameAgent probe failed: {ex}", LogLevel.Error);
        }
    }

    private void RunProbeCommand(string command, string[] args)
    {
        if (!Context.IsWorldReady)
        {
            this.Monitor.Log("Load a save before running the GameAgent probe.", LogLevel.Warn);
            return;
        }

        NPC? linus = Game1.getCharacterFromName(TargetAgentName, mustBeVillager: true);
        if (linus is null)
        {
            this.Monitor.Log("Could not find Linus in this save.", LogLevel.Warn);
            return;
        }

        ProbeObservation observation = this.observationBuilder!.Build(linus, Game1.player, "console_probe");
        this.Monitor.Log(observation.ToLogLine(), LogLevel.Info);
        this.speakCapability!.Speak(linus, SpeakCapability.ProbeText);
    }
}
