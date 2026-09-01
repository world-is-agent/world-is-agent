# Roadmap

World Is Agent is an experimental MVP0 project. This public roadmap uses Now / Next / Later so the current direction stays readable while detailed phase plans continue to live under `docs/`.

## Now

Stabilize the current Runtime + Stardew vertical slice.

Focus:

- Keep README, architecture, status, and testing docs aligned with implementation.
- Tighten Stardew dialogue and async action behavior around real game UX.
- Make local setup and adapter installation more reproducible.
- Expand protocol and adapter checks that can run outside the game.
- Clarify provider compatibility and configuration expectations.

Exit signals:

- A new contributor can understand the project from public docs.
- The Go Runtime and C# adapter checks are documented and repeatable.
- Stardew smoke testing has a clear path and known limits.
- Current capabilities and limits are captured in `docs/STATUS.md`.

## Next

Add environment recovery and durable agent state.

Focus:

- Adapter reconnect and EnvironmentSession recovery.
- Heartbeat and liveness semantics.
- Capability registry scoping across reconnects.
- Disconnect, late result, and idempotency behavior.
- Persistent memory backend for AgentSession state.
- Durable continuation strategy for async actions.

Exit signals:

- Runtime restart and adapter reconnect behavior are specified and tested.
- Agent memory can survive process restart for a selected backend.
- Async action outcomes have clear recovery semantics.

## Later

Grow WIA from one validated adapter into a reusable multi-world agent harness.

Focus:

- Scenario evaluation and regression suites.
- Fault injection for runtime, adapter, and provider boundaries.
- Adapter conformance tests.
- A second real adapter outside Stardew Valley.
- More domain-neutral context and visible memory projection.
- Versioned releases, migration policy, and trace retention/export.

Exit signals:

- New adapters can be built against documented conformance checks.
- Runtime behavior is measurable through scenario evaluation.
- Public releases have stable install, upgrade, and compatibility notes.

## Detailed Plans

Detailed phase plans, ADRs, and acceptance records are kept under [docs/](docs/README.md).
