# GameAgent Stardew Adapter

This is the Stardew Valley SMAPI adapter for GameAgent Runtime MVP0. It connects to the Runtime gRPC stream and provides the three primitives GameAgent needs:

```text
Event: player action-button interaction with Linus
Observation: Linus location, game time, player, player location, friendship
Action: Linus displays Runtime's speak action text
```

MVP0 supports one capability, `speak`, and one event, `player_interacted_with_npc`.

## Build

Use a machine with .NET SDK and Stardew Valley installed at the repository root path used by `GameAgent.Stardew.csproj`.

```powershell
dotnet build NPCore/adapters/stardew/GameAgent.Stardew.csproj
```

## Manual Smoke Test

1. Start Runtime with `go run ./runtime/cmd/server`.
2. Build the adapter.
3. Copy the build output and `manifest.json` to `Stardew Valley/Mods/GameAgentStardew`.
4. Launch Stardew Valley through `StardewModdingAPI.exe`.
5. Load a save where Linus is reachable.
6. Confirm the SMAPI log shows Runtime connection and `CapabilityList sent: speak`.
7. Click Linus with the normal action button or mouse.
8. Confirm the SMAPI log shows GameEvent, EventAck, Observation, ActionRequest, and ActionResult.
9. Confirm Linus displays the text returned by Runtime's FakeProvider.

You can also run this SMAPI console command after loading a save:

```text
gameagent_probe_linus
```
