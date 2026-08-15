using System;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.Events;
using GameAgent.Stardew.Runtime;
using GameAgent.Stardew.State;
using StardewModdingAPI;
using StardewModdingAPI.Events;
using StardewValley;

namespace GameAgent.Stardew;

/// <summary>SMAPI entry point for the Adapter Capability Spike.</summary>
public sealed class ModEntry : Mod
{
    private AdapterConfig? config;
    private MainThreadDispatcher? dispatcher;
    private ObservationBuilder? observationBuilder;
    private SpeakCapability? speakCapability;
    private PlayerInteractProbe? playerInteractProbe;
    private RuntimeClient? runtimeClient;

    public override void Entry(IModHelper helper)
    {
        this.config = helper.ReadConfig<AdapterConfig>();
        this.dispatcher = new MainThreadDispatcher(this.Monitor);
        this.observationBuilder = new ObservationBuilder();
        this.speakCapability = new SpeakCapability();
        this.runtimeClient = new RuntimeClient(
            this.config,
            this.dispatcher,
            this.observationBuilder,
            this.speakCapability,
            this.Monitor
        );
        this.playerInteractProbe = new PlayerInteractProbe(
            this.config.TargetAgentName,
            this.runtimeClient,
            this.Monitor,
            helper.Input
        );

        helper.Events.GameLoop.GameLaunched += this.OnGameLaunched;
        helper.Events.GameLoop.UpdateTicked += this.OnUpdateTicked;
        helper.Events.Input.ButtonPressed += this.OnButtonPressed;
        helper.ConsoleCommands.Add(
            "gameagent_probe_linus",
            "Run the GameAgent Linus probe without clicking.",
            this.RunProbeCommand
        );

        this.Monitor.Log("GameAgent Stardew Adapter Probe loaded.", LogLevel.Info);
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
            this.runtimeClient?.Dispose();
    }

    private void OnGameLaunched(object? sender, GameLaunchedEventArgs e)
    {
        try
        {
            this.runtimeClient?.Start();
        }
        catch (Exception ex)
        {
            this.Monitor.Log($"Failed to start GameAgent Runtime client: {ex}", LogLevel.Error);
        }
    }

    private void OnUpdateTicked(object? sender, UpdateTickedEventArgs e)
    {
        this.dispatcher?.Drain();
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

        string targetAgentName = this.config?.TargetAgentName ?? "Linus";
        NPC? linus = Game1.getCharacterFromName(targetAgentName, mustBeVillager: true);
        if (linus is null)
        {
            this.Monitor.Log($"Could not find {targetAgentName} in this save.", LogLevel.Warn);
            return;
        }

        if (this.runtimeClient is null || !this.runtimeClient.IsReady)
        {
            this.Monitor.Log("GameAgent Runtime is not ready; command ignored.", LogLevel.Warn);
            return;
        }

        this.runtimeClient.SendPlayerInteracted(linus, Game1.player, "console_probe");
    }
}
