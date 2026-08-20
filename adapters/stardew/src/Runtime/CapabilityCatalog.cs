using GameAgent.Protocol.V1Alpha2;

namespace GameAgent.Stardew.Runtime;

public static class CapabilityCatalog
{
    private const string SpeakInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\"}},\"required\":[\"text\"]}";

    private const string EmoteInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"emote\":{\"type\":\"string\",\"enum\":[\"happy\",\"sad\",\"surprised\",\"neutral\"]}},\"required\":[\"emote\"],\"additionalProperties\":false}";

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
                    Description = "Make the NPC speak.",
                    InputSchemaJson = SpeakInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                },
                new Capability
                {
                    Name = "emote",
                    Version = "0.1.0",
                    Description = "Make the NPC play an emote bubble.",
                    InputSchemaJson = EmoteInputSchemaJson,
                    ExecutionMode = ExecutionMode.Sync,
                },
            },
        };
    }
}
