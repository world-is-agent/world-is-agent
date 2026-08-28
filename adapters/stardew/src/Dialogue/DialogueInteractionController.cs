using System;
using System.Collections.Generic;
using System.Linq;
using StardewValley;
using StardewValley.Menus;

namespace GameAgent.Stardew.Dialogue;

public sealed record DialoguePresentation(
    NPC Npc,
    string NpcEntityId,
    string ConversationId,
    PresentDialogueInput Input,
    Action<PlayerDialogueSubmission> OnSubmitted,
    Action OnAbandoned
);

public sealed class DialogueInteractionController
{
    private readonly Queue<PendingDialoguePresentation> pending = new();
    private ActiveDialoguePresentation? active;

    public void QueueOrShow(
        string npcEntityId,
        Func<DialoguePresentation?> createPresentation,
        Action onDisplayed,
        Action<Exception> onFailed
    )
    {
        this.pending.Enqueue(new PendingDialoguePresentation(npcEntityId, createPresentation, onDisplayed, onFailed));
        this.TryShowNext();
    }

    public void Update()
    {
        this.active?.Update();
        if (this.active?.IsFinished == true)
            this.active = null;

        this.TryShowNext();
    }

    public void CloseForNpc(string npcEntityId)
    {
        if (this.active?.NpcEntityId == npcEntityId)
        {
            this.active.CloseWithoutSubmission();
            this.active = null;
        }

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
        while (this.active is null && !IsDialogueUiBusy() && this.pending.Count > 0)
        {
            PendingDialoguePresentation pendingPresentation = this.pending.Dequeue();
            try
            {
                DialoguePresentation? presentation = pendingPresentation.CreatePresentation();
                if (presentation is null)
                    continue;

                this.active = new ActiveDialoguePresentation(
                    presentation,
                    pendingPresentation.OnDisplayed,
                    pendingPresentation.OnFailed
                );
                this.active.Start();
                if (this.active.IsFinished)
                    this.active = null;
            }
            catch (Exception ex)
            {
                pendingPresentation.OnFailed(ex);
            }
        }
    }

    private static bool IsDialogueUiBusy()
    {
        return Game1.activeClickableMenu is not null || Game1.dialogueUp;
    }

    private sealed record PendingDialoguePresentation(
        string NpcEntityId,
        Func<DialoguePresentation?> CreatePresentation,
        Action OnDisplayed,
        Action<Exception> OnFailed
    );

    private sealed class ActiveDialoguePresentation
    {
        private const string ReplyPrompt = "Respond:";
        private readonly DialoguePresentation presentation;
        private readonly Action<Exception> onFailed;
        private readonly IReadOnlyList<DialogueReplyChoice> choices;
        private readonly DialoguePresentationFlow flow;

        public ActiveDialoguePresentation(
            DialoguePresentation presentation,
            Action onDisplayed,
            Action<Exception> onFailed
        )
        {
            this.presentation = presentation;
            this.onFailed = onFailed;
            this.choices = DialogueReplyChoice.BuildVisibleChoices(presentation.Input);
            this.flow = new DialoguePresentationFlow(
                shouldShowReplyMenu: this.choices.Count > 0 || presentation.Input.AllowFreeText,
                onDisplayed: onDisplayed,
                onAbandoned: presentation.OnAbandoned
            );
        }

        public string NpcEntityId => this.presentation.NpcEntityId;
        public bool IsFinished => this.flow.IsFinished;

        public void Start()
        {
            this.flow.Start(() => Game1.DrawDialogue(new StardewValley.Dialogue(this.presentation.Npc, null, this.presentation.Input.Text)));
        }

        public void Update()
        {
            if (this.IsFinished)
                return;

            try
            {
                this.flow.Update(
                    isDialogueUiBusy: IsDialogueUiBusy(),
                    isReplyMenuActive: this.IsReplyMenuActive(),
                    showReplyMenu: this.ShowReplyMenu
                );
            }
            catch (Exception ex)
            {
                this.Fail(ex);
            }
        }

        public void CloseWithoutSubmission()
        {
            this.flow.CloseWithoutSubmission(this.CloseVisibleUi);
        }

        private void ShowReplyMenu()
        {
            Game1.activeClickableMenu = new DialogueInteractionMenu(
                this.presentation.NpcEntityId,
                this.presentation.ConversationId,
                ReplyPrompt,
                this.choices,
                this.presentation.Input.AllowFreeText,
                submission =>
                {
                    this.presentation.OnSubmitted(submission);
                    this.flow.MarkSubmitted();
                },
                () => this.flow.Abandon()
            );
        }

        private bool IsReplyMenuActive()
        {
            return Game1.activeClickableMenu is DialogueInteractionMenu menu
                && menu.ConversationId == this.presentation.ConversationId
                && menu.NpcEntityId == this.presentation.NpcEntityId;
        }

        private void CloseVisibleUi()
        {
            if (Game1.activeClickableMenu is DialogueInteractionMenu menu && menu.NpcEntityId == this.presentation.NpcEntityId)
                menu.CloseWithoutSubmission();
            else if (Game1.currentSpeaker == this.presentation.Npc && (Game1.dialogueUp || Game1.activeClickableMenu is DialogueBox))
                Game1.exitActiveMenu();
        }

        private void Fail(Exception ex)
        {
            this.flow.FinishWithoutAbandon();
            this.onFailed(ex);
        }
    }
}
