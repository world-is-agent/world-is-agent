using System;
using System.Collections.Generic;
using System.Linq;

namespace GameAgent.Stardew.Dialogue;

public sealed record DialogueReplyChoice(
    string ResponseKey,
    string Text,
    int SelectedOptionIndex
)
{
    private const int MaxVisibleChoices = 4;
    private const string OptionResponseKeyPrefix = "gameagent_option_";

    public static IReadOnlyList<DialogueReplyChoice> BuildVisibleChoices(PresentDialogueInput input)
    {
        ArgumentNullException.ThrowIfNull(input);

        int generatedLimit = input.AllowFreeText ? MaxVisibleChoices - 1 : MaxVisibleChoices;
        List<DialogueReplyChoice> choices = input.ReplyOptions
            .Take(generatedLimit)
            .Select((text, index) => new DialogueReplyChoice(BuildOptionResponseKey(index), text, index))
            .ToList();

        return choices;
    }

    public static string BuildOptionResponseKey(int selectedOptionIndex)
    {
        if (selectedOptionIndex < 0)
            throw new ArgumentOutOfRangeException(nameof(selectedOptionIndex), "Selected option index must not be negative.");

        return $"{OptionResponseKeyPrefix}{selectedOptionIndex}";
    }
}
