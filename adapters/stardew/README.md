# GameAgent Stardew Adapter

This is the Stardew Valley SMAPI adapter for GameAgent Runtime MVP0. It connects to the Runtime gRPC stream and provides the three primitives GameAgent needs:

```text
Event: player action-button interaction with a targeted NPC
Observation: NPC location, game time, player, player location, friendship
Action: targeted NPC displays Runtime's speak action text or emote
```

MVP0 Phase3 supports `speak` and `emote`, plus the `player_interacted_with_npc` event for multiple villager NPCs.

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
5. Load a save where at least two villager NPCs are reachable.
6. Confirm the SMAPI log shows Runtime connection and `CapabilityList sent: speak, emote`.
7. Click two different NPCs with the normal action button or mouse.
8. Confirm the SMAPI log shows GameEvent, EventAck, Observation, ActionRequest, and ActionResult.
9. Confirm the clicked NPC displays the text or emote returned by Runtime.
10. Confirm protocol trace logs include stable `world_id` and the clicked NPC's `target_entity_id`.

You can also run this SMAPI console command after loading a save:

```text
gameagent_probe_npc [NPC name]
```
