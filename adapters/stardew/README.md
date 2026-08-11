# GameAgent Stardew Adapter Probe

This is the first Adapter Capability Spike. It is a SMAPI mod that proves Stardew Valley can provide the three primitives GameAgent needs:

```text
Event: player action-button interaction with Linus
Observation: Linus location, game time, player, player location, friendship
Action: Linus displays "Hello from GameAgent"
```

This probe intentionally does not use protobuf, gRPC, Runtime, LLM, Memory, Tool Registry, or Permission.

## Build

Use a machine with .NET SDK and Stardew Valley installed at the repository root path used by `GameAgent.Stardew.csproj`.

```powershell
dotnet build NPCore/adapters/stardew/GameAgent.Stardew.csproj
```

## Manual Smoke Test

1. Build the project.
2. Copy the build output and `manifest.json` to `Stardew Valley/Mods/GameAgentStardew`.
3. Launch Stardew Valley through `StardewModdingAPI.exe`.
4. Load a save where Linus is reachable.
5. Click Linus with the normal action button or mouse.
6. Confirm the SMAPI log contains `GameAgent Probe Observation`.
7. Confirm Abigail displays `Hello from GameAgent`.

You can also run this SMAPI console command after loading a save:

```text
gameagent_probe_linus
```
