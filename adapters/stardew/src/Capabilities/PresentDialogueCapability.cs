using System;
using GameAgent.Stardew.Dialogue;
using StardewValley;

namespace GameAgent.Stardew.Capabilities;

public sealed class PresentDialogueCapability
{
    private const string NpcEntityPrefix = "npc:";
    private const string PlayerEntityId = "player:local";

    private readonly ConversationStateStore conversationStore;
    private readonly DialogueInteractionController dialogueController;

    public PresentDialogueCapability(ConversationStateStore conversationStore, DialogueInteractionController dialogueController)
    {
        this.conversationStore = conversationStore;
        this.dialogueController = dialogueController;
    }

    public void CloseForNpc(string npcEntityId)
    {
        this.dialogueController.CloseForNpc(npcEntityId);
    }

    public void Present(
        NPC npc,
        Farmer player,
        string worldId,
        PresentDialogueInput input,
        Func<bool> isCancelled,
        Action onCancelled,
        Action<string> onDisplayed,
        Action<Exception> onFailed,
        Action<PlayerDialogueSubmission> onSubmitted
    )
    {
        string npcEntityId = ToNpcEntityId(npc.Name);
        string? conversationId = null;
        this.dialogueController.CloseForNpc(npcEntityId);
        this.dialogueController.QueueOrShow(
            npcEntityId,
            createMenu: () =>
            {
                if (isCancelled())
                {
                    onCancelled();
                    return null;
                }

                conversationId = this.conversationStore.AppendNpcLine(worldId, npcEntityId, PlayerEntityId, npc.displayName, input.Text, Game1.timeOfDay);
                DialogueInteractionMenu menu = new(npcEntityId, conversationId, input, onSubmitted);
                if (input.ReplyOptions.Count == 0 && !input.AllowFreeText)
                    this.conversationStore.Close(worldId, npcEntityId, PlayerEntityId);
                return menu;
            },
            onDisplayed: () => onDisplayed(conversationId ?? string.Empty),
            onFailed: onFailed
        );
    }

    private static string ToNpcEntityId(string npcName)
    {
        return $"{NpcEntityPrefix}{npcName}";
    }
}
