namespace GameAgent.Stardew.Runtime;

public sealed class AdapterConfig
{
    public string RuntimeAddress { get; set; } = "http://127.0.0.1:50051";

    public string AdapterId { get; set; } = "stardew-smapi";

    public string AdapterVersion { get; set; } = "0.1.0";

    public string ProtocolVersion { get; set; } = "v1alpha1";

    public string GameId { get; set; } = "stardew-valley";

    public string TargetAgentName { get; set; } = "Linus";

    public bool EnableProtocolTrace { get; set; } = true;
}
