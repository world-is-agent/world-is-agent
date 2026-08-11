# Player Interact Speak Vertical Slice Design

Date: 2026-08-11
Status: Draft for user review
Scope: MVP 0 first vertical slice

## Goal

Build the first real end-to-end GameAgent Runtime chain:

```text
Player clicks Abigail
  -> Stardew Adapter captures player_interact
  -> Runtime receives GameEvent
  -> Runtime requests Observation
  -> Runtime calls real LLM Provider
  -> LLM returns ToolCall(speak)
  -> Runtime validates capability and permission
  -> Runtime submits speak Action
  -> Stardew Adapter displays dialogue in game
  -> Runtime records ActionResult and Trace
```

This slice proves the architecture works across Runtime, Protocol, Adapter, SMAPI, Stardew Game API, and a real LLM API.

## Non-Goals

This slice does not implement memory, vector retrieval, goal scheduling, move_to, received_gift, multi-agent behavior, dashboard UI, or advanced evaluation.

`FakeLLMProvider` may exist for tests, but the product path uses a real LLM API from the start.

## Architecture

The implementation keeps the core boundary from `GameAgent Runtime 架构设计规范.md`:

```text
Runtime owns cognition.
Protocol owns contracts.
Adapter owns translation.
```

Runtime must not reference Stardew, SMAPI, Game1, Farmer, Abigail, or other game-specific types. The Stardew Adapter owns all Stardew-specific mapping.

## Components

### Stardew Adapter

Responsibilities:

- Load as a SMAPI mod.
- Detect player interaction with Abigail.
- Convert the interaction into a protocol-level `GameEvent` with type `player_interact`.
- Build an Observation for the relevant agent.
- Declare `speak` as an available capability.
- Execute `speak` by calling Stardew Game API, such as `Game1.DrawDialogue(new Dialogue(npc, null, text))` or an equivalent safe dialogue path.
- Return `ActionResult` to Runtime.

The Adapter must not call the LLM and must not decide what Abigail should say.

### Protocol

MVP protocol objects:

- `GameEvent`
- `Observation`
- `Capability`
- `ActionRequest`
- `ActionResult`

The protocol should keep fields generic. Stardew-only details belong in an `extensions` map or Adapter-local code.

### Runtime

Responsibilities:

- Receive `GameEvent`.
- Apply a simple TriggerPolicy: `player_interact` triggers an Agent Step.
- Request Observation from Environment.
- Build a minimal prompt.
- Call a real `LLMProvider`.
- Parse and validate a `ToolCall`.
- Check basic permission and capability availability.
- Submit a `speak` action through Environment.
- Record trace data.

### LLM Provider

Runtime uses a provider interface rather than hardcoding any vendor in the Agent loop.

The first implementation should support a real API provider configured through environment variables. API keys must not be committed.

Suggested environment variables:

```text
OPENAI_API_KEY
GAMEAGENT_LLM_MODEL
```

Tests should use a mock provider to avoid network calls.

## Observation MVP

The first Observation should include:

- agent id
- agent display name
- game time
- weather
- agent location
- agent tile position
- player location
- player tile position
- friendship value
- nearby entities
- triggering event summary
- available capabilities

No long-term memory is included in this slice.

## Prompt Contract

The prompt should instruct the model to return one tool call only. For MVP 0, the only allowed environment tool is `speak`.

Expected model output shape:

```json
{
  "tool": "speak",
  "arguments": {
    "text": "Hey, I didn't expect to see you here."
  }
}
```

Runtime validates:

- tool name is `speak`
- text exists
- text is not empty
- text length is within a conservative limit
- agent has `speak` capability

## Error Handling

If Adapter cannot identify Abigail, it returns a failed event or failed observation result.

If Observation fails, Runtime records trace and stops the Agent Step.

If LLM call fails, Runtime records trace and does not submit a speak action.

If LLM returns invalid tool output, Runtime records trace and does not submit a speak action.

If Stardew dialogue execution fails, Adapter returns `ActionResult` with status `FAILED` and an error message.

## Trace

MVP trace records:

- event id and type
- agent id
- observation snapshot
- LLM request metadata
- raw LLM response or parsed tool call
- submitted action
- action result
- errors, if any

Trace can start as structured logs or JSONL. A database is not required for MVP 0.

## Testing

Minimum tests:

- Runtime handles `player_interact` by requesting observation.
- Runtime sends prompt to `LLMProvider`.
- Runtime accepts valid `speak` tool call.
- Runtime rejects invalid tool names.
- Runtime rejects unavailable capability.
- Adapter observation builder maps Stardew NPC/player state into generic Observation.
- Adapter speak executor handles success and failure paths.

Manual smoke test:

1. Start Runtime.
2. Start Stardew through SMAPI with the Adapter loaded.
3. Load a save where Abigail is reachable.
4. Player clicks Abigail.
5. Abigail displays LLM-generated dialogue.
6. Runtime trace shows event, observation, LLM tool call, speak action, and result.

## Acceptance Criteria

The slice is complete when a real player click on Abigail causes an LLM-generated `speak` action to appear as in-game dialogue, and the Runtime records the full trace without Stardew-specific symbols leaking into Runtime code.
