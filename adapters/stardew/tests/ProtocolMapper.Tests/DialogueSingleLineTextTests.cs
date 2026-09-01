using GameAgent.Stardew.Dialogue;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class DialogueSingleLineTextTests
{
    [Fact]
    public void ShowsTrailingTextThatFits()
    {
        DialogueSingleLineText trailingText = DialogueSingleLineText.FitTrailingText(
            "ABCDEFGHIJ",
            maxWidth: 5,
            measureText: text => text.Length
        );

        Assert.Equal("FGHIJ", trailingText.VisibleText);
        Assert.Equal(5, trailingText.CaretOffset);
    }

    [Fact]
    public void KeepsTextThatAlreadyFits()
    {
        DialogueSingleLineText exactText = DialogueSingleLineText.FitTrailingText(
            "ABCD",
            maxWidth: 4,
            measureText: text => text.Length
        );

        Assert.Equal("ABCD", exactText.VisibleText);
        Assert.Equal(4, exactText.CaretOffset);
    }
}
