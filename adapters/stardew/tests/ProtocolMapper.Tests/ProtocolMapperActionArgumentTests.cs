using GameAgent.Protocol.V1Alpha2;
using GameAgent.Stardew.Capabilities;
using GameAgent.Stardew.Dialogue;
using GameAgent.Stardew.Runtime;
using Google.Protobuf.WellKnownTypes;
using Xunit;

namespace GameAgent.Stardew.Tests;

public sealed class ProtocolMapperActionArgumentTests
{
    [Fact]
    public void ParsesPresentDialogueArguments()
    {
        ActionRequest request = TestSupport.CreatePresentDialogueRequest();

        PresentDialogueInput input = ProtocolMapper.RequirePresentDialogueArgument(request);

        Assert.Equal("Want to explore the mines?", input.Text);
        Assert.Equal(new[] { "Yes", "Maybe later" }, input.ReplyOptions);
        Assert.True(input.AllowFreeText);
    }

    [Fact]
    public void PresentDialogueDefaultsAllowFreeTextToTrue()
    {
        ActionRequest request = TestSupport.CreatePresentDialogueRequest();
        request.Arguments.Fields.Remove("allow_free_text");

        PresentDialogueInput input = ProtocolMapper.RequirePresentDialogueArgument(request);

        Assert.True(input.AllowFreeText);
    }

    [Fact]
    public void PresentDialogueHonorsExplicitAllowFreeTextFalse()
    {
        ActionRequest request = TestSupport.CreatePresentDialogueRequest();
        request.Arguments.Fields["allow_free_text"] = Value.ForBool(false);

        PresentDialogueInput input = ProtocolMapper.RequirePresentDialogueArgument(request);

        Assert.False(input.AllowFreeText);
    }

    [Fact]
    public void PresentDialogueRejectsMoreThanThreeReplyOptions()
    {
        ActionRequest request = TestSupport.CreatePresentDialogueRequest();
        request.Arguments.Fields["reply_options"] = TestSupport.ValueList(
            Value.ForString("One"),
            Value.ForString("Two"),
            Value.ForString("Three"),
            Value.ForString("Four")
        );

        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.RequirePresentDialogueArgument(request),
            "3 options or fewer"
        );
    }

    [Fact]
    public void PresentDialogueRejectsOverlongText()
    {
        ActionRequest request = TestSupport.CreatePresentDialogueRequest();
        request.Arguments.Fields["text"] = Value.ForString(new string('x', 241));

        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.RequirePresentDialogueArgument(request),
            "240"
        );
    }

    [Fact]
    public void ParsesMoveToArguments()
    {
        ActionRequest request = TestSupport.CreateMoveToRequest();

        MoveToInput input = ProtocolMapper.RequireMoveToArgument(request);

        Assert.Equal("Town", input.Location);
        Assert.Equal(12, input.TileX);
        Assert.Equal(20, input.TileY);
    }

    [Fact]
    public void MoveToRejectsNonIntegerCoordinates()
    {
        ActionRequest request = TestSupport.CreateMoveToRequest();
        request.Arguments.Fields["tile"].StructValue.Fields["x"] = Value.ForNumber(12.5);

        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.RequireMoveToArgument(request),
            "integer"
        );
    }

    [Fact]
    public void MoveToRejectsMissingLocation()
    {
        ActionRequest request = TestSupport.CreateMoveToRequest();
        request.Arguments.Fields.Remove("location");

        TestSupport.ExpectArgumentException(
            () => ProtocolMapper.RequireMoveToArgument(request),
            "location"
        );
    }
}
