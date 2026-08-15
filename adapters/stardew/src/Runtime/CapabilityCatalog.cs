using GameAgent.Protocol.V1Alpha1;

namespace GameAgent.Stardew.Runtime;

public static class CapabilityCatalog
{
    private const string SpeakInputSchemaJson =
        "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\"}},\"required\":[\"text\"]}";

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
            },
        };
    }
}
