# Stardew Adapter Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a minimal SMAPI mod that proves Stardew can expose Event, Observation, and Action primitives for Linus.

**Architecture:** This is Adapter-only work. The SMAPI `ModEntry` wires the game into small Adapter core classes for event detection, observation building, and speak execution. Runtime, protobuf, gRPC, LLM, Memory, Tool Registry, and Permission stay out of this spike.

**Tech Stack:** C#, SMAPI, Stardew Valley public API, local static verification script.

## Global Constraints

- Runtime code must not reference Stardew, SMAPI, Game1, Farmer, Abigail, PelicanTown, StardewValley, Minecraft, Unity, or Godot.
- The Adapter may reference SMAPI and Stardew Valley APIs.
- No protobuf or gRPC is introduced in this probe.
- The probe target is hardcoded as Stardew internal NPC name `Linus`.
- The probe dialogue text is hardcoded as `Hello from GameAgent`.

---

### Task 1: Adapter Probe Mod

**Files:**
- Create: `NPCore/adapters/stardew/GameAgent.Stardew.csproj`
- Create: `NPCore/adapters/stardew/manifest.json`
- Create: `NPCore/adapters/stardew/src/ModEntry.cs`
- Create: `NPCore/adapters/stardew/src/Events/PlayerInteractProbe.cs`
- Create: `NPCore/adapters/stardew/src/State/ProbeObservation.cs`
- Create: `NPCore/adapters/stardew/src/State/ObservationBuilder.cs`
- Create: `NPCore/adapters/stardew/src/Capabilities/SpeakCapability.cs`
- Create: `NPCore/adapters/stardew/tests/check-probe-static.ps1`

**Interfaces:**
- Produces: `PlayerInteractProbe.HandleButtonPressed(ButtonPressedEventArgs e): bool`
- Produces: `ObservationBuilder.Build(NPC agent, Farmer player, string trigger): ProbeObservation`
- Produces: `SpeakCapability.Speak(NPC speaker, string text): void`

- [x] **Step 1: Write static verification**

Run: `powershell -ExecutionPolicy Bypass -File NPCore/adapters/stardew/tests/check-probe-static.ps1`

Expected before implementation: FAIL because required files are missing.

- [x] **Step 2: Implement minimal SMAPI mod**

The mod registers `Input.ButtonPressed`, checks action-button or mouse interaction with Linus, logs observation fields, and shows hardcoded dialogue.

- [x] **Step 3: Run verification**

Run:

```powershell
powershell -ExecutionPolicy Bypass -File NPCore/adapters/stardew/tests/check-probe-static.ps1
powershell -ExecutionPolicy Bypass -File NPCore/scripts/check-architecture.ps1
```

Expected: both pass.

- [ ] **Step 4: Manual smoke test**

Copy/build the mod into `Stardew Valley/Mods/GameAgentStardew`, launch through SMAPI, load a save with Linus reachable, click Linus, and confirm the dialogue shows `Hello from GameAgent`.

## Self-Review

Spec coverage: the plan covers Event capture, Observation read, and speak Action execution. Runtime/protocol/LLM are intentionally excluded.

Placeholder scan: no placeholder work remains.

Type consistency: `PlayerInteractProbe`, `ObservationBuilder`, `ProbeObservation`, and `SpeakCapability` are the only Adapter core types used by `ModEntry`.
