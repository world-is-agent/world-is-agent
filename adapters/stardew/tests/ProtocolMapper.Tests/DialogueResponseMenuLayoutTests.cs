using GameAgent.Stardew.Dialogue;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class DialogueResponseMenuLayoutTests
{
    [Fact]
    public void CompactResponseMenuFitsViewportAndIncludesFreeTextControls()
    {
        DialogueResponseMenuLayout layout = DialogueResponseMenuLayout.Build(
            viewportWidth: 1226,
            viewportHeight: 489,
            optionCount: 3,
            allowFreeText: true
        );

        Assert.True(layout.MenuBounds.Top >= 12);
        Assert.True(layout.MenuBounds.Bottom <= 489);
        Assert.NotNull(layout.TextInputRow);
        Assert.NotNull(layout.SendButton);
        Assert.Equal(3, layout.OptionRows.Count);
    }

    [Fact]
    public void GeneratedOnlyResponseMenuOmitsFreeTextControls()
    {
        DialogueResponseMenuLayout layout = DialogueResponseMenuLayout.Build(
            viewportWidth: 1226,
            viewportHeight: 489,
            optionCount: 4,
            allowFreeText: false
        );

        Assert.Null(layout.TextInputRow);
        Assert.Null(layout.SendButton);
    }

    [Fact]
    public void SmallViewportLayoutKeepsAllControlsInsideBounds()
    {
        DialogueResponseMenuLayout layout = DialogueResponseMenuLayout.Build(
            viewportWidth: 640,
            viewportHeight: 360,
            optionCount: 3,
            allowFreeText: true
        );

        AssertInsideViewport(layout.MenuBounds, 640, 360);
        AssertInside(layout.TitleArea, layout.MenuBounds);
        AssertInside(layout.CloseButton, layout.MenuBounds);
        foreach (DialogueMenuRectangle optionRow in layout.OptionRows)
            AssertInside(optionRow, layout.MenuBounds);

        DialogueMenuRectangle textInputRow = Assert.IsType<DialogueMenuRectangle>(layout.TextInputRow);
        DialogueMenuRectangle textInputTextArea = Assert.IsType<DialogueMenuRectangle>(layout.TextInputTextArea);
        DialogueMenuRectangle sendButton = Assert.IsType<DialogueMenuRectangle>(layout.SendButton);
        AssertInside(textInputRow, layout.MenuBounds);
        AssertInside(textInputTextArea, textInputRow);
        AssertInside(sendButton, textInputRow);
        Assert.True(textInputTextArea.Width > 0);
    }

    private static void AssertInside(DialogueMenuRectangle inner, DialogueMenuRectangle outer)
    {
        Assert.True(inner.Left >= outer.Left);
        Assert.True(inner.Top >= outer.Top);
        Assert.True(inner.Right <= outer.Right);
        Assert.True(inner.Bottom <= outer.Bottom);
    }

    private static void AssertInsideViewport(DialogueMenuRectangle rectangle, int viewportWidth, int viewportHeight)
    {
        Assert.True(rectangle.Left >= 0);
        Assert.True(rectangle.Top >= 0);
        Assert.True(rectangle.Right <= viewportWidth);
        Assert.True(rectangle.Bottom <= viewportHeight);
    }
}
