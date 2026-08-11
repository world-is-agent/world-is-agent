namespace GameAgent.Stardew.State;

public sealed record ProbeObservation(
    string AgentId,
    string AgentName,
    string AgentLocation,
    int AgentTileX,
    int AgentTileY,
    int GameTime,
    string PlayerName,
    string PlayerLocation,
    int PlayerTileX,
    int PlayerTileY,
    int Friendship,
    string Trigger
)
{
    public string ToLogLine()
    {
        return
            $"GameAgent Probe Observation: agent={this.AgentId}; " +
            $"location={this.AgentLocation}; " +
            $"time={this.GameTime}; " +
            $"player={this.PlayerName}; " +
            $"player_location={this.PlayerLocation}; " +
            $"friendship={this.Friendship}; " +
            $"trigger={this.Trigger}";
    }
}
