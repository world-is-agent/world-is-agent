using System.Collections.Generic;

namespace GameAgent.Stardew.Dialogue;

public sealed record PresentDialogueInput(
    string Text,
    IReadOnlyList<string> ReplyOptions,
    bool AllowFreeText
);
