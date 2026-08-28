using System;

namespace GameAgent.Stardew.Events;

public static class PlayerInteractTrigger
{
    public const string ActionButton = "action_button";
    public const string MouseLeft = "mouse_left";
    public const string MouseRight = "mouse_right";
    public const string ConsoleProbe = "console_probe";

    public static string FromButton(string button)
    {
        string normalized = (button ?? string.Empty).Trim().Replace("-", "_", StringComparison.Ordinal).ToLowerInvariant();
        return normalized switch
        {
            "action" or "action_button" => ActionButton,
            "mouseleft" or "mouse_left" or "left_mouse" => MouseLeft,
            "mouseright" or "mouse_right" or "right_mouse" => MouseRight,
            _ => ActionButton,
        };
    }
}
