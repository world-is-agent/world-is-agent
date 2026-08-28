using System;

namespace GameAgent.Stardew.Dialogue;

public readonly record struct DialogueSingleLineText(string VisibleText, float CaretOffset)
{
    public static DialogueSingleLineText FitTrailingText(string text, float maxWidth, Func<string, float> measureText)
    {
        if (measureText is null)
            throw new ArgumentNullException(nameof(measureText));

        if (string.IsNullOrEmpty(text) || maxWidth <= 0)
            return new DialogueSingleLineText(string.Empty, 0);

        string sanitized = text.Replace('\r', ' ').Replace('\n', ' ');
        if (measureText(sanitized) <= maxWidth)
            return new DialogueSingleLineText(sanitized, Math.Max(0, measureText(sanitized)));

        int start = 0;
        while (start < sanitized.Length && measureText(sanitized[start..]) > maxWidth)
            start++;

        string visibleText = sanitized[start..];
        return new DialogueSingleLineText(visibleText, Math.Max(0, measureText(visibleText)));
    }
}
