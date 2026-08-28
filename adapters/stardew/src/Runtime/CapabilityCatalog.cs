using GameAgent.Protocol.V1Alpha2;

namespace GameAgent.Stardew.Runtime;

public static class CapabilityCatalog
{
    private const string SpeakInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\",\"maxLength\":240}},\"required\":[\"text\"],\"additionalProperties\":false}";

    private const string EmoteInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"emote\":{\"type\":\"string\",\"enum\":[\"happy\",\"sad\",\"surprised\",\"neutral\"]}},\"required\":[\"emote\"],\"additionalProperties\":false}";

    private const string PresentDialogueInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\",\"maxLength\":240},\"reply_options\":{\"type\":\"array\",\"maxItems\":4,\"items\":{\"type\":\"string\",\"maxLength\":80}},\"allow_free_text\":{\"type\":\"boolean\"}},\"required\":[\"text\"],\"additionalProperties\":false}";

    private const string FacePlayerInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{},\"additionalProperties\":false}";

    public static CapabilityList BuildEnvironmentCapabilities()
    {
        return new CapabilityList
        {
            Revision = 1,
            Capabilities =
            {
                new Capability
                {
                    Name = "speak",
                    Version = "0.1.0",
                    Description = "Displays one dialogue text line from the NPC to the player.",
                    InputSchemaJson = SpeakInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                    ConcurrencyMode = CapabilityConcurrencyMode.Sequential,
                },
                new Capability
                {
                    Name = "emote",
                    Version = "0.1.0",
                    Description = "Displays one emote bubble above the NPC.",
                    InputSchemaJson = EmoteInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                    ConcurrencyMode = CapabilityConcurrencyMode.Sequential,
                },
                new Capability
                {
                    Name = "present_dialogue",
                    Version = "0.1.0",
                    Description = "Displays NPC dialogue with optional reply options or free-text input for the player.",
                    InputSchemaJson = PresentDialogueInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                    ConcurrencyMode = CapabilityConcurrencyMode.Sequential,
                },
                new Capability
                {
                    Name = "face_player",
                    Version = "0.1.0",
                    Description = "Turns the NPC to face the player when both are in the same location.",
                    InputSchemaJson = FacePlayerInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                    ConcurrencyMode = CapabilityConcurrencyMode.Sequential,
                },
            },
        };
    }
}
