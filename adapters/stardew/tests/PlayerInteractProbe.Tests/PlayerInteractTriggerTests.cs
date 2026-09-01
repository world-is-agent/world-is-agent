using GameAgent.Stardew.Events;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class PlayerInteractTriggerTests
{
    [Theory]
    [InlineData("action", "action_button")]
    [InlineData("mouse_left", "mouse_left")]
    [InlineData("mouse_right", "mouse_right")]
    public void MapsButtonToProtocolTrigger(string button, string expectedTrigger)
    {
        Assert.Equal(expectedTrigger, PlayerInteractTrigger.FromButton(button));
    }
}
