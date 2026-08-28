using System;

namespace GameAgent.Stardew.Dialogue;

public sealed class DialoguePresentationFlow
{
    private readonly bool shouldShowReplyMenu;
    private readonly Action onDisplayed;
    private readonly Action onAbandoned;
    private PresentationStage stage = PresentationStage.NotStarted;
    private bool observedNpcDialogue;
    private bool displayed;
    private bool suppressAbandon;

    public DialoguePresentationFlow(
        bool shouldShowReplyMenu,
        Action onDisplayed,
        Action onAbandoned
    )
    {
        this.shouldShowReplyMenu = shouldShowReplyMenu;
        this.onDisplayed = onDisplayed;
        this.onAbandoned = onAbandoned;
    }

    public bool IsFinished { get; private set; }

    public void Start(Action showNpcLine)
    {
        if (this.stage != PresentationStage.NotStarted || this.IsFinished)
            return;

        showNpcLine();
        this.MarkDisplayed();
        this.stage = PresentationStage.ShowingNpcLine;
    }

    public void Update(bool isDialogueUiBusy, bool isReplyMenuActive, Action showReplyMenu)
    {
        if (this.IsFinished)
            return;

        switch (this.stage)
        {
            case PresentationStage.ShowingNpcLine:
                this.UpdateShowingNpcLine(isDialogueUiBusy, showReplyMenu);
                break;
            case PresentationStage.ShowingReplyMenu:
                this.UpdateShowingReplyMenu(isReplyMenuActive);
                break;
        }
    }

    public void MarkSubmitted()
    {
        this.IsFinished = true;
    }

    public void FinishWithoutAbandon()
    {
        this.IsFinished = true;
    }

    public void CloseWithoutSubmission(Action closeVisibleUi)
    {
        if (this.IsFinished)
            return;

        this.suppressAbandon = true;
        closeVisibleUi();
        this.IsFinished = true;
    }

    public void Abandon()
    {
        if (this.IsFinished)
            return;

        this.IsFinished = true;
        if (!this.suppressAbandon)
            this.onAbandoned();
    }

    private void UpdateShowingNpcLine(bool isDialogueUiBusy, Action showReplyMenu)
    {
        if (isDialogueUiBusy)
        {
            this.observedNpcDialogue = true;
            return;
        }

        if (!this.observedNpcDialogue)
            return;

        if (!this.shouldShowReplyMenu)
        {
            this.IsFinished = true;
            return;
        }

        showReplyMenu();
        this.stage = PresentationStage.ShowingReplyMenu;
    }

    private void UpdateShowingReplyMenu(bool isReplyMenuActive)
    {
        if (isReplyMenuActive)
            return;

        this.Abandon();
    }

    private void MarkDisplayed()
    {
        if (this.displayed)
            return;

        this.displayed = true;
        this.onDisplayed();
    }

    private enum PresentationStage
    {
        NotStarted,
        ShowingNpcLine,
        ShowingReplyMenu,
    }
}
