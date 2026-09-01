using GameAgent.Stardew.Dialogue;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class DialogueReplyChoiceTests
{
    [Fact]
    public void ShowsAtMostThreeGeneratedReplyOptionsWithFreeText()
    {
        IReadOnlyList<DialogueReplyChoice> choices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
            "Question?",
            new[] { "One", "Two", "Three", "Four" },
            true
        ));

        Assert.Equal(3, choices.Count);
        Assert.Equal(new[] { "One", "Two", "Three" }, choices.Select(choice => choice.Text));
        Assert.Equal(0, choices[0].SelectedOptionIndex);
        Assert.Equal(2, choices[2].SelectedOptionIndex);
    }

    [Fact]
    public void FreeTextDoesNotAddClickableRow()
    {
        IReadOnlyList<DialogueReplyChoice> choices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
            "Question?",
            new[] { "Only option" },
            true
        ));

        Assert.Single(choices);
        Assert.Equal("Only option", choices[0].Text);
    }

    [Fact]
    public void FreeTextOnlyDialogueHasNoGeneratedChoices()
    {
        IReadOnlyList<DialogueReplyChoice> choices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
            "Question?",
            Array.Empty<string>(),
            true
        ));

        Assert.Empty(choices);
    }

    [Fact]
    public void GeneratedOnlyChoicesAreStillCappedAtThree()
    {
        IReadOnlyList<DialogueReplyChoice> choices = DialogueReplyChoice.BuildVisibleChoices(new PresentDialogueInput(
            "Question?",
            new[] { "One", "Two", "Three", "Four" },
            false
        ));

        Assert.Equal(3, choices.Count);
    }
}
