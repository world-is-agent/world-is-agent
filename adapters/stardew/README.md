# GameAgent Stardew Adapter

This is the Stardew Valley SMAPI adapter for GameAgent Runtime MVP0. It connects to the Runtime gRPC stream and provides the three primitives GameAgent needs:

```text
Event: player action-button interaction with a targeted NPC
Observation: Stardew time, weather, NPC, player, relationship, scene, schedule, active conversation
Action: targeted NPC can speak, emote, present dialogue options, or face the player
```

MVP0 supports `speak`, `emote`, `present_dialogue`, and `face_player`, plus NPC interaction and player dialogue input events for villager NPCs.

## Dialogue UX

`present_dialogue` should feel like a normal Stardew conversation, not like a separate centered chat window.

- The NPC line is shown first through Stardew's native dialogue flow.
- Player reply choices appear in a bottom response menu only after the player advances the NPC line.
- The response menu has at most four reply rows. When `allow_free_text=true`, the adapter shows up to three generated `reply_options` plus an inline free-text row that is focused for immediate typing.
- Selecting a generated option immediately sends `player_said_to_npc`.
- The inline free-text row keeps `Close` and `Send`; `Send` submits non-empty text, and `Close` exits without sending a player dialogue event.
- The adapter must not display the NPC line, reply choices, and free-text box together in one centered modal.

## Build

Use a machine with .NET SDK and Stardew Valley installed at the repository root path used by `GameAgent.Stardew.csproj`.

```powershell
dotnet build world-is-agent/adapters/stardew/GameAgent.Stardew.csproj
```

## Manual Smoke Test

1. Start Runtime with `go run ./runtime/cmd/server`.
2. Build the adapter.
3. Copy the build output and `manifest.json` to `Stardew Valley/Mods/GameAgentStardew`.
4. Launch Stardew Valley through `StardewModdingAPI.exe`.
5. Load a save where at least two villager NPCs are reachable.
6. Confirm the SMAPI log shows Runtime connection and `CapabilityList sent: speak, emote, present_dialogue, face_player`.
7. Click two different NPCs with the normal action button or mouse.
8. Confirm the SMAPI log shows GameEvent, EventAck, Observation, ActionRequest, and ActionResult.
9. For `present_dialogue`, confirm the NPC line appears first in Stardew's native dialogue box.
10. Advance the NPC line and confirm the bottom response menu appears afterward, with generated reply choices above the inline input row when enabled.
11. Select a generated reply and confirm the SMAPI log sends `player_said_to_npc` with `input_kind=option`.
12. Type directly in the inline input row, submit with `Send`, and confirm `input_kind=free_text`.
13. Confirm protocol trace logs include stable `world_id` and the clicked NPC's `target_entity_id`.

You can also run this SMAPI console command after loading a save:

```text
gameagent_probe_npc [NPC name]
```
