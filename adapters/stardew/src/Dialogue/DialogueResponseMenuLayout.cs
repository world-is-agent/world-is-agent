using System;
using System.Collections.Generic;

namespace GameAgent.Stardew.Dialogue;

public sealed record DialogueResponseMenuLayout(
    DialogueMenuRectangle MenuBounds,
    DialogueMenuRectangle TitleArea,
    IReadOnlyList<DialogueMenuRectangle> OptionRows,
    DialogueMenuRectangle? TextInputRow,
    DialogueMenuRectangle? TextInputTextArea,
    DialogueMenuRectangle CloseButton,
    DialogueMenuRectangle? SendButton
)
{
    private const int MenuWidth = 1180;
    private const int MinimumMenuWidth = 320;
    private const int OuterMargin = 20;
    private const int TitleHeight = 52;
    private const int TitleGap = 8;
    private const int OptionRowHeight = 66;
    private const int MinimumOptionRowHeight = 36;
    private const int TextInputHeight = 76;
    private const int MinimumTextInputHeight = 52;
    private const int RowGap = 8;
    private const int MinimumRowGap = 4;
    private const int ButtonWidth = 132;
    private const int MinimumButtonWidth = 96;
    private const int ButtonHeight = 48;
    private const int BottomOffset = 24;
    private const int MinimumTopMargin = 12;
    private const int MinimumHorizontalMargin = 48;
    private const int MinimumViewportEdgeMargin = 8;

    public static DialogueResponseMenuLayout Build(int viewportWidth, int viewportHeight, int optionCount, bool allowFreeText)
    {
        if (viewportWidth <= 0)
            throw new ArgumentOutOfRangeException(nameof(viewportWidth), "Viewport width must be positive.");
        if (viewportHeight <= 0)
            throw new ArgumentOutOfRangeException(nameof(viewportHeight), "Viewport height must be positive.");
        if (optionCount < 0)
            throw new ArgumentOutOfRangeException(nameof(optionCount), "Option count must not be negative.");

        int horizontalMargin = ResolveHorizontalMargin(viewportWidth);
        int maxWidth = Math.Max(1, viewportWidth - horizontalMargin * 2);
        int width = Math.Min(MenuWidth, maxWidth);
        if (maxWidth >= MinimumMenuWidth)
            width = Math.Max(MinimumMenuWidth, width);

        int rowCount = optionCount + (allowFreeText ? 1 : 0);
        int maxHeight = Math.Max(TitleHeight + OuterMargin * 2, viewportHeight - MinimumTopMargin * 2);
        int availableReplyHeight = Math.Max(0, maxHeight - OuterMargin * 2 - TitleHeight - (rowCount > 0 ? TitleGap : 0));
        DialogueMenuLayoutMetrics metrics = ResolveMetrics(optionCount, allowFreeText, availableReplyHeight);
        int replyAreaHeight = CalculateReplyAreaHeight(optionCount, allowFreeText, metrics);
        int height = OuterMargin * 2 + TitleHeight + (rowCount > 0 ? TitleGap + replyAreaHeight : 0);
        height = Math.Min(height, maxHeight);

        int x = (viewportWidth - width) / 2;
        int y = Math.Max(MinimumTopMargin, viewportHeight - height - BottomOffset);

        int contentX = x + OuterMargin;
        int contentWidth = width - OuterMargin * 2;
        int buttonWidth = ResolveButtonWidth(contentWidth);
        int buttonHeight = Math.Min(ButtonHeight, Math.Max(32, TitleHeight - 4));
        DialogueMenuRectangle closeButton = new(
            contentX + contentWidth - buttonWidth,
            y + OuterMargin + (TitleHeight - buttonHeight) / 2,
            buttonWidth,
            buttonHeight
        );
        DialogueMenuRectangle titleArea = new(contentX, y + OuterMargin, Math.Max(1, contentWidth - buttonWidth - RowGap), TitleHeight);

        int currentY = y + OuterMargin + TitleHeight + TitleGap;
        var optionRows = new List<DialogueMenuRectangle>();
        for (int i = 0; i < optionCount; i++)
        {
            optionRows.Add(new DialogueMenuRectangle(contentX, currentY, contentWidth, metrics.OptionRowHeight));
            currentY += metrics.OptionRowHeight + metrics.RowGap;
        }

        DialogueMenuRectangle? textInputRow = null;
        DialogueMenuRectangle? textInputTextArea = null;
        DialogueMenuRectangle? sendButton = null;
        if (allowFreeText)
        {
            int textInset = Math.Min(18, Math.Max(8, contentWidth / 24));
            int sendButtonHeight = Math.Min(ButtonHeight, Math.Max(30, metrics.TextInputHeight - 12));
            textInputRow = new DialogueMenuRectangle(contentX, currentY, contentWidth, metrics.TextInputHeight);
            sendButton = new DialogueMenuRectangle(
                contentX + contentWidth - buttonWidth - 12,
                currentY + (metrics.TextInputHeight - sendButtonHeight) / 2,
                buttonWidth,
                sendButtonHeight
            );
            textInputTextArea = new DialogueMenuRectangle(
                contentX + textInset,
                currentY + 8,
                Math.Max(1, sendButton.Value.Left - metrics.RowGap - (contentX + textInset)),
                Math.Max(1, metrics.TextInputHeight - 16)
            );
        }

        return new DialogueResponseMenuLayout(
            new DialogueMenuRectangle(x, y, width, height),
            titleArea,
            optionRows,
            textInputRow,
            textInputTextArea,
            closeButton,
            sendButton
        );
    }

    private static int CalculateReplyAreaHeight(int optionCount, bool allowFreeText, DialogueMenuLayoutMetrics metrics)
    {
        int rowCount = optionCount + (allowFreeText ? 1 : 0);
        if (rowCount == 0)
            return 0;

        return optionCount * metrics.OptionRowHeight
            + (allowFreeText ? metrics.TextInputHeight : 0)
            + (rowCount - 1) * metrics.RowGap;
    }

    private static int ResolveHorizontalMargin(int viewportWidth)
    {
        if (viewportWidth >= MinimumMenuWidth + MinimumHorizontalMargin * 2)
            return MinimumHorizontalMargin;

        return Math.Max(MinimumViewportEdgeMargin, Math.Min(24, viewportWidth / 20));
    }

    private static int ResolveButtonWidth(int contentWidth)
    {
        int width = Math.Min(ButtonWidth, Math.Max(MinimumButtonWidth, (contentWidth - RowGap) / 3));
        return Math.Min(width, Math.Max(48, contentWidth / 2));
    }

    private static DialogueMenuLayoutMetrics ResolveMetrics(int optionCount, bool allowFreeText, int availableReplyHeight)
    {
        int rowCount = optionCount + (allowFreeText ? 1 : 0);
        DialogueMenuLayoutMetrics defaultMetrics = new(OptionRowHeight, TextInputHeight, RowGap);
        if (rowCount == 0 || CalculateReplyAreaHeight(optionCount, allowFreeText, defaultMetrics) <= availableReplyHeight)
            return defaultMetrics;

        int rowGap = MinimumRowGap;
        int gapsHeight = (rowCount - 1) * rowGap;
        int textInputHeight = allowFreeText ? Math.Min(TextInputHeight, Math.Max(MinimumTextInputHeight, availableReplyHeight - gapsHeight)) : 0;
        int optionSpace = availableReplyHeight - gapsHeight - textInputHeight;
        int optionRowHeight = optionCount == 0
            ? OptionRowHeight
            : Math.Min(OptionRowHeight, Math.Max(MinimumOptionRowHeight, optionSpace / optionCount));

        while (optionCount > 0 && optionCount * optionRowHeight + textInputHeight + gapsHeight > availableReplyHeight && optionRowHeight > 1)
            optionRowHeight--;

        while (allowFreeText && optionCount * optionRowHeight + textInputHeight + gapsHeight > availableReplyHeight && textInputHeight > 1)
            textInputHeight--;

        return new DialogueMenuLayoutMetrics(optionRowHeight, textInputHeight, rowGap);
    }
}

internal readonly record struct DialogueMenuLayoutMetrics(int OptionRowHeight, int TextInputHeight, int RowGap);

public readonly record struct DialogueMenuRectangle(int X, int Y, int Width, int Height)
{
    public int Left => this.X;
    public int Top => this.Y;
    public int Right => this.X + this.Width;
    public int Bottom => this.Y + this.Height;

    public bool Contains(int x, int y)
    {
        return x >= this.Left && x < this.Right && y >= this.Top && y < this.Bottom;
    }
}
