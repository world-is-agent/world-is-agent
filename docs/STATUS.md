# Status

World Is Agent is in experimental MVP0 development.

This document is the public source of truth for current repository capabilities, validation scope, and known limits.

Last updated: 2026-09-01.

## Validation Scope

The current validated path is:

```text
Stardew Valley + SMAPI
  -> WIA Stardew Adapter
  -> WIA Protocol v1alpha2
  -> Go Runtime
  -> LLM Provider
```

Stardew Valley is the first real adapter and validation environment.

## Implemented

| Area | Current support |
| --- | --- |
| Runtime transport | gRPC bidirectional streaming between adapter and runtime. |
| Environment bootstrap | Adapter hello, environment ready, capability publication. |
| Protocol | `gameagent.protocol.v1alpha2` with world scope, target entity routing, action lifecycle, and turn completion messages. |
| Identity | `AgentSessionKey = game_id + world_id + entity_id`. |
| Scheduling | Per-EnvironmentSession same-agent FIFO lane scheduling. |
| AgentTurn | Bounded multi-step turns with model/tool feedback. |
| Tools | Runtime tools plus dynamic environment capabilities. |
| Tool policy | Capability metadata for exclusive-per-step and settle-after-success behavior. |
| Actions | Sync action result handling and async action status/result handling. |
| Turn completion | Runtime can send best-effort `TurnCompletion` to the adapter. |
| Memory | Process-local short-term memory scoped by AgentSession. |
| Context | Context projection from observation, recent memory, tool results, and runtime policy. |
| Providers | Provider-neutral model interface with Fake, DeepSeek, and OpenAI implementations. |
| Trace | JSONL turn trace written under `runtime/.local/traces.jsonl`. |

## Experimental

| Area | Current support |
| --- | --- |
| Stardew adapter | Real SMAPI adapter used as the first validation adapter. |
| Stardew observation | Adapter-owned Stardew fact projection for time, weather, scene, relationship, schedule, conversation, and nearby NPC context. |
| Stardew interaction events | NPC interaction and player dialogue input events. |
| Stardew dialogue | Native NPC dialogue line followed by generated reply choices and optional free-text input. |
| Stardew actions | `speak`, `emote`, `present_dialogue`, `face_player`, and same-location `move_to`. |
| Model providers | DeepSeek and OpenAI providers work through local config and external API keys. |
| Architecture checks | Local scripts exist for protocol and architecture checks. CI enforcement is still evolving. |

## Not Yet Supported

| Area | Current limit |
| --- | --- |
| Durable agent state | Memory is in-process and lost on runtime restart. |
| Long-term semantic memory | No vector store, embedding index, or durable episodic memory backend. |
| Automatic reconnect | Adapter reconnect and environment recovery are future work. |
| Durable async continuation | Async action waiting is process-local. |
| Cross-stream ordering | Same-agent FIFO is validated inside one live EnvironmentSession. |
| Multiple heterogeneous worlds | The runtime architecture allows adapters, but only Stardew has real validation. |
| Heartbeat/liveness productization | Heartbeat exists in protocol but is not yet a completed recovery mechanism. |
| Scenario evaluation | No public scenario evaluation suite yet. |
| Packaged release | Local developer workflow is the primary installation path. |

## Maintenance Rule

When a PR changes a user-visible capability, validation scope, or known limit, update this file in the same PR.
