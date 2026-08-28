using System;
using System.Collections.Generic;
using System.Linq;
using StardewValley;

namespace GameAgent.Stardew.Dialogue;

public sealed class DialogueInteractionController
{
    private readonly Queue<PendingDialoguePresentation> pending = new();

    public void QueueOrShow(string npcEntityId, Func<DialogueInteractionMenu?> createMenu, Action onDisplayed, Action<Exception> onFailed)
    {
        this.pending.Enqueue(new PendingDialoguePresentation(npcEntityId, createMenu, onDisplayed, onFailed));
        this.TryShowNext();
    }

    public void Update()
    {
        this.TryShowNext();
    }

    public void CloseForNpc(string npcEntityId)
    {
        if (Game1.activeClickableMenu is DialogueInteractionMenu menu && menu.NpcEntityId == npcEntityId)
            menu.CloseWithoutSubmission();

        if (this.pending.Count == 0)
            return;

        PendingDialoguePresentation[] remaining = this.pending
            .Where(presentation => presentation.NpcEntityId != npcEntityId)
            .ToArray();
        this.pending.Clear();
        foreach (PendingDialoguePresentation presentation in remaining)
            this.pending.Enqueue(presentation);
    }

    private void TryShowNext()
    {
        if (Game1.activeClickableMenu is not null || Game1.dialogueUp || this.pending.Count == 0)
            return;

        PendingDialoguePresentation pendingPresentation = this.pending.Dequeue();
        try
        {
            DialogueInteractionMenu? menu = pendingPresentation.CreateMenu();
            if (menu is null)
                return;

            Game1.activeClickableMenu = menu;
            pendingPresentation.OnDisplayed();
        }
        catch (Exception ex)
        {
            pendingPresentation.OnFailed(ex);
        }
    }

    private sealed record PendingDialoguePresentation(string NpcEntityId, Func<DialogueInteractionMenu?> CreateMenu, Action OnDisplayed, Action<Exception> OnFailed);
}
