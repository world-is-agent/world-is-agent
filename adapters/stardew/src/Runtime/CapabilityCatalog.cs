using GameAgent.Protocol.V1Alpha2;
using Google.Protobuf.WellKnownTypes;

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
                    Description = "Displays NPC dialogue with optional reply options or free-text input for the player. Use it when the player should be able to reply; it must be the only tool call in its model response. After it succeeds, the current turn ends; wait for player_said_to_npc before continuing that conversation. Omitting reply options and free text means the conversation ends after the NPC line is shown.",
                    InputSchemaJson = PresentDialogueInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                    ConcurrencyMode = CapabilityConcurrencyMode.Sequential,
                    Extensions = PresentDialogueExtensions(),
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

    private static Struct PresentDialogueExtensions()
    {
        Struct toolPolicy = new();
        toolPolicy.Fields.Add("exclusive_per_step", Value.ForBool(true));
        toolPolicy.Fields.Add("settle_after_success", Value.ForBool(true));

        Struct gameagent = new();
        gameagent.Fields.Add("tool_policy", Value.ForStruct(toolPolicy));

        Struct extensions = new();
        extensions.Fields.Add("gameagent", Value.ForStruct(gameagent));
        return extensions;
    }
}
