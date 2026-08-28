using System;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.Dialogue;
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
    private ConversationStateStore? conversationStore;
    private DialogueInteractionController? dialogueController;
    private ObservationBuilder? observationBuilder;
    private SpeakCapability? speakCapability;
    private EmoteCapability? emoteCapability;
    private PresentDialogueCapability? presentDialogueCapability;
    private FacePlayerCapability? facePlayerCapability;
    private PlayerInteractProbe? playerInteractProbe;
    private RuntimeClient? runtimeClient;

    public override void Entry(IModHelper helper)
    {
        this.config = helper.ReadConfig<AdapterConfig>();
        this.dispatcher = new MainThreadDispatcher(this.Monitor);
        this.conversationStore = new ConversationStateStore(new ConversationIdGenerator());
        this.dialogueController = new DialogueInteractionController();
        this.observationBuilder = new ObservationBuilder(this.conversationStore);
        this.speakCapability = new SpeakCapability();
        this.emoteCapability = new EmoteCapability();
        this.presentDialogueCapability = new PresentDialogueCapability(this.conversationStore, this.dialogueController);
        this.facePlayerCapability = new FacePlayerCapability();
        this.runtimeClient = new RuntimeClient(
            this.config,
            this.dispatcher,
            this.observationBuilder,
            this.conversationStore,
            this.speakCapability,
            this.emoteCapability,
            this.presentDialogueCapability,
            this.facePlayerCapability,
            this.Monitor
        );
        this.playerInteractProbe = new PlayerInteractProbe(
            this.config.AgentTargets,
            this.runtimeClient,
            this.Monitor,
            helper.Input
        );

        helper.Events.GameLoop.GameLaunched += this.OnGameLaunched;
        helper.Events.GameLoop.SaveLoaded += this.OnSaveLoaded;
        helper.Events.GameLoop.DayStarted += this.OnDayStarted;
        helper.Events.GameLoop.ReturnedToTitle += this.OnReturnedToTitle;
        helper.Events.GameLoop.UpdateTicked += this.OnUpdateTicked;
        helper.Events.Input.ButtonPressed += this.OnButtonPressed;
        helper.ConsoleCommands.Add(
            "gameagent_probe_npc",
            "Run the GameAgent NPC probe without clicking. Usage: gameagent_probe_npc [NPC name]",
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

    private void OnSaveLoaded(object? sender, SaveLoadedEventArgs e)
    {
        this.runtimeClient?.ClearConversations();
        this.runtimeClient?.RefreshWorldContext();
    }

    private void OnDayStarted(object? sender, DayStartedEventArgs e)
    {
        this.runtimeClient?.ClearConversations();
        this.runtimeClient?.RefreshWorldContext();
    }

    private void OnReturnedToTitle(object? sender, ReturnedToTitleEventArgs e)
    {
        this.runtimeClient?.ClearWorldContext();
    }

    private void OnUpdateTicked(object? sender, UpdateTickedEventArgs e)
    {
        this.dispatcher?.Drain();
        this.dialogueController?.Update();
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

        string targetAgentName = args.Length > 0
            ? string.Join(" ", args).Trim()
            : this.config?.AgentTargets?.FirstOrDefault(name => !string.IsNullOrWhiteSpace(name)) ?? "Linus";
        NPC? target = Game1.getCharacterFromName(targetAgentName, mustBeVillager: true);
        if (target is null)
        {
            this.Monitor.Log($"Could not find {targetAgentName} in this save.", LogLevel.Warn);
            return;
        }

        if (this.runtimeClient is null || !this.runtimeClient.IsReady)
        {
            this.Monitor.Log("GameAgent Runtime is not ready; command ignored.", LogLevel.Warn);
            return;
        }

        this.runtimeClient.SendPlayerInteracted(target, Game1.player, PlayerInteractTrigger.ConsoleProbe);
    }
}
