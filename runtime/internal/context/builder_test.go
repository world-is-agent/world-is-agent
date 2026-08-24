package context_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	agentcontext "gameagent/runtime/internal/context"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestBuilderExtractsDefinitionIDFromTargetEntityRef(t *testing.T) {
	builder := agentcontext.NewBuilder()

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey: session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "creature:alpha"},
		Event: &protocolv1alpha2.GameEvent{
			EventId:        "event-1",
			WorldId:        "world-a",
			TargetEntityId: "creature:alpha",
			Entities: []*protocolv1alpha2.EntityRef{
				{EntityId: "player:local", DefinitionId: "player/local"},
				{EntityId: "creature:alpha", DefinitionId: "villager/farmer"},
			},
		},
		Observation: &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "creature:alpha"},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if agentCtx.AgentDescriptor.EntityID != "creature:alpha" {
		t.Fatalf("AgentDescriptor.EntityID = %q, want creature:alpha", agentCtx.AgentDescriptor.EntityID)
	}
	if agentCtx.AgentDescriptor.DefinitionID != "villager/farmer" {
		t.Fatalf("AgentDescriptor.DefinitionID = %q, want villager/farmer", agentCtx.AgentDescriptor.DefinitionID)
	}
}

func TestBuilderDoesNotReadDefinitionIDFromObservationState(t *testing.T) {
	builder := agentcontext.NewBuilder()

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey: session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "creature:alpha"},
		Event: &protocolv1alpha2.GameEvent{
			EventId:        "event-1",
			WorldId:        "world-a",
			TargetEntityId: "creature:alpha",
			Entities:       []*protocolv1alpha2.EntityRef{{EntityId: "creature:alpha"}},
		},
		Observation: &protocolv1alpha2.Observation{
			WorldId:  "world-a",
			EntityId: "creature:alpha",
			State: mustStruct(t, map[string]any{
				"definition_id": "legacy/observation-state",
			}),
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if agentCtx.AgentDescriptor.DefinitionID != "" {
		t.Fatalf("AgentDescriptor.DefinitionID = %q, want empty", agentCtx.AgentDescriptor.DefinitionID)
	}
}

func TestRendererBuildsModelRequestWithMemoryObservationInstructionAndTools(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit: 1024,
	})
	tool := model.ToolDefinition{Name: "speak", Description: "say text", InputSchema: `{"type":"object"}`}

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "You are controlling an NPC in a game.",
		Event: &protocolv1alpha2.GameEvent{
			EventId:        "event-1",
			EventType:      "player_interacted_with_npc",
			WorldId:        "world-a",
			TargetEntityId: "npc:Abigail",
			Entities: []*protocolv1alpha2.EntityRef{
				{EntityId: "npc:Abigail", DefinitionId: "npc:Abigail"},
			},
			GameTime: &protocolv1alpha2.GameTime{
				Year:   ptrInt32(1),
				Season: ptrInt32(1),
				Day:    ptrInt32(2),
				Hour:   ptrInt32(6),
				Minute: ptrInt32(20),
			},
		},
		Observation: &protocolv1alpha2.Observation{
			WorldId:  "world-a",
			EntityId: "npc:Abigail",
		},
		RecentMemories: []memory.Record{{
			MemoryID:      "mem-1",
			SourceTurnID:  "turn-1",
			SourceEventID: "event-1",
			EventType:     "player_interacted_with_npc",
			GameTime:      &memory.GameTimeSnapshot{Year: 1, Season: 1, Day: 2, Hour: 6, Minute: 20},
			Outcome: memory.TurnOutcome{
				ToolName:      "speak",
				ToolArguments: map[string]any{"text": "hello from last turn"},
				ActionStatus:  "ACTION_STATUS_SUCCEEDED",
			},
			CreatedAt: time.Unix(100, 0),
		}},
		Tools: []model.ToolDefinition{tool},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if req.System != "You are controlling an NPC in a game." {
		t.Fatalf("System = %q", req.System)
	}
	if len(req.Tools) != 1 || req.Tools[0] != tool {
		t.Fatalf("Tools = %+v, want %+v", req.Tools, []model.ToolDefinition{tool})
	}
	if len(req.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(req.Messages))
	}
	if len(req.Controls) != 1 || req.Controls[0].Kind != model.ControlSettle {
		t.Fatalf("controls = %+v, want settle control", req.Controls)
	}
	content := req.Messages[0].Content
	for _, want := range []string{
		"[Recent Memory]",
		"today 06:20",
		`said "hello from last turn"`,
		"hello from last turn",
		"[Agent Descriptor]",
		"entity_id: npc:Abigail",
		"definition_id: npc:Abigail",
		"[Current Event]",
		"[Current Observation]",
		"[Instruction]",
		"Current Observation is the current truth.",
		"Recent Memory is historical context.",
		"from today",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered content missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{
		"mem-1",
		"source_turn_id",
		"action_status",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("rendered content should not include storage field %q:\n%s", unwanted, content)
		}
	}
}

func TestRendererIncludesBatchToolCallTranscriptMessages(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit:        1024,
		MaxToolResultOutputBytes:      4096,
		MaxToolResultOutputDepth:      4,
		MaxToolResultOutputFields:     16,
		MaxToolResultOutputArrayItems: 8,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1", TargetEntityId: "npc:Abigail"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{
			{
				Role: model.RoleAssistant,
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "Hello."}},
					{ID: "call_2", Name: "emote", Arguments: map[string]any{"emote": "happy"}},
				},
			},
			{
				Role: model.RoleTool,
				ToolResults: []model.ToolResult{
					{ToolCallID: "call_1", Name: "speak", Status: "succeeded", Code: "action_succeeded", Output: map[string]any{"visible": true}},
					{ToolCallID: "call_2", Name: "emote", Status: "succeeded", Code: "action_succeeded", Output: map[string]any{"visible": true}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if got := len(req.Messages); got != 3 {
		t.Fatalf("len(Messages) = %d, want user context plus two transcript messages", got)
	}
	if req.Messages[1].Role != model.RoleAssistant {
		t.Fatalf("transcript call role = %q, want assistant", req.Messages[1].Role)
	}
	if req.Messages[2].Role != model.RoleTool {
		t.Fatalf("transcript result role = %q, want tool", req.Messages[2].Role)
	}
	if req.Messages[1].ToolCalls[0].ID != "call_1" || req.Messages[1].ToolCalls[1].ID != "call_2" {
		t.Fatalf("tool call order = %+v", req.Messages[1].ToolCalls)
	}
	if req.Messages[2].ToolResults[0].ToolCallID != "call_1" || req.Messages[2].ToolResults[1].ToolCallID != "call_2" {
		t.Fatalf("tool result order = %+v", req.Messages[2].ToolResults)
	}
	assertContainsAll(t, req.Messages[1].Content, "call_1", "speak", "Hello.", "call_2", "emote", "happy")
	assertContainsAll(t, req.Messages[2].Content, "call_1", "succeeded", "action_succeeded", "call_2")
}

func TestRendererSeparatesRecentMemoryFromIntraTurnTranscript(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{MemoryContextSizeLimit: 1024})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		RecentMemories: []memory.Record{{
			Outcome: memory.TurnOutcome{ToolName: "speak", ToolArguments: map[string]any{"text": "previous turn line"}},
		}},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "speak",
				Status:     "succeeded",
				Code:       "action_succeeded",
				Output:     map[string]any{"line": "current turn line"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if !strings.Contains(req.Messages[0].Content, "previous turn line") {
		t.Fatalf("user context missing recent memory:\n%s", req.Messages[0].Content)
	}
	if strings.Contains(req.Messages[0].Content, "current turn line") {
		t.Fatalf("user context leaked transcript:\n%s", req.Messages[0].Content)
	}
	if !strings.Contains(req.Messages[1].Content, "current turn line") {
		t.Fatalf("transcript message missing current turn result:\n%s", req.Messages[1].Content)
	}
}

func TestRendererDoesNotLeakRawToolResultInternals(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{MemoryContextSizeLimit: 1024})
	longDiagnostic := "adapter rejected request\nstack trace line\n{\"raw\":\"json\",\"action_id\":\"runtime-action-123\"}" + strings.Repeat("x", 180)

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "speak",
				Status:     "rejected",
				Code:       "adapter_rejected",
				Message:    longDiagnostic,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[1].Content
	if strings.Contains(content, "stack trace") || strings.Contains(content, "runtime-action-123") || strings.Contains(content, `{"raw"`) {
		t.Fatalf("tool result content leaked raw diagnostic:\n%s", content)
	}
	if len(extractJSONField(t, content, "message")) > 120 {
		t.Fatalf("tool result message was not bounded:\n%s", content)
	}
}

func TestRendererExposesSettleControlInstruction(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{MemoryContextSizeLimit: 1024})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if len(req.Controls) != 1 || req.Controls[0].Kind != model.ControlSettle {
		t.Fatalf("controls = %+v, want settle control", req.Controls)
	}
	if !strings.Contains(req.Messages[0].Content, "settle the current turn") {
		t.Fatalf("instruction missing settle guidance:\n%s", req.Messages[0].Content)
	}
}

func TestToolResultNormalizationIsDeterministic(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit:        1024,
		MaxToolResultOutputBytes:      4096,
		MaxToolResultOutputDepth:      4,
		MaxToolResultOutputFields:     16,
		MaxToolResultOutputArrayItems: 8,
	})

	input := agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "inspect",
				Status:     "succeeded",
				Code:       "action_succeeded",
				Output:     map[string]any{"b": float64(2), "a": float64(1)},
			}},
		}},
	}
	firstCtx, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	secondCtx, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	first, err := renderer.Render(firstCtx)
	if err != nil {
		t.Fatalf("first Render returned error: %v", err)
	}
	second, err := renderer.Render(secondCtx)
	if err != nil {
		t.Fatalf("second Render returned error: %v", err)
	}
	if first.Messages[1].Content != second.Messages[1].Content {
		t.Fatalf("tool result rendering is not deterministic:\nfirst=%s\nsecond=%s", first.Messages[1].Content, second.Messages[1].Content)
	}
	if strings.Index(first.Messages[1].Content, `"a"`) > strings.Index(first.Messages[1].Content, `"b"`) {
		t.Fatalf("tool result output keys are not stable:\n%s", first.Messages[1].Content)
	}
}

func TestToolResultNormalizationIsProviderNeutral(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{MemoryContextSizeLimit: 1024})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "speak",
				Status:     "succeeded",
				Code:       "action_succeeded",
				Output:     map[string]any{"visible": true},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[1].Content
	for _, unwanted := range []string{"structpb", "protocolv1alpha2", "ActionResult", "protobuf"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("provider-neutral transcript leaked %q:\n%s", unwanted, content)
		}
	}
}

func TestToolResultIncludesBoundedStructuredOutput(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit:        1024,
		MaxToolResultOutputBytes:      4096,
		MaxToolResultOutputDepth:      4,
		MaxToolResultOutputFields:     16,
		MaxToolResultOutputArrayItems: 8,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "inspect",
				Status:     "succeeded",
				Code:       "action_succeeded",
				Output: map[string]any{
					"visible": true,
					"nested":  map[string]any{"mood": "happy"},
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	assertContainsAll(t, req.Messages[1].Content, `"visible": true`, `"mood": "happy"`)
}

func TestToolResultOutputProjectionAppliesBounds(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit:        1024,
		MaxToolResultOutputBytes:      300,
		MaxToolResultOutputDepth:      2,
		MaxToolResultOutputFields:     2,
		MaxToolResultOutputArrayItems: 2,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-1"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		Transcript: []model.Message{{
			Role: model.RoleTool,
			ToolResults: []model.ToolResult{{
				ToolCallID: "call_1",
				Name:       "inspect",
				Status:     "succeeded",
				Code:       "action_succeeded",
				Output: map[string]any{
					"a": []any{"one", "two", "three"},
					"b": map[string]any{"nested": map[string]any{"leaf": "too deep"}},
					"c": "extra field",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[1].Content
	assertContainsAll(t, content, `"a"`, `"one"`, `"two"`, "_truncated")
	for _, unwanted := range []string{"three", "extra field", "too deep"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("bounded projection leaked %q:\n%s", unwanted, content)
		}
	}
	if outputSize := len(mustMarshalJSONBytes(t, extractJSONFieldMap(t, content, "output"))); outputSize > 300 {
		t.Fatalf("tool result output = %d bytes, want <= 300:\n%s", outputSize, content)
	}
}

func TestRendererMemoryBudgetKeepsLatestRecordWhenSingleRecordExceedsLimit(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit: 10,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event:         &protocolv1alpha2.GameEvent{EventId: "event-2", EventType: "player_interacted_with_npc"},
		Observation:   &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		RecentMemories: []memory.Record{
			{MemoryID: "old", Outcome: memory.TurnOutcome{ToolName: "speak", ToolArguments: map[string]any{"text": "old"}}},
			{MemoryID: "latest", Outcome: memory.TurnOutcome{ToolName: "speak", ToolArguments: map[string]any{"text": strings.Repeat("x", 100)}}},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[0].Content
	if !strings.Contains(content, strings.Repeat("x", 100)) {
		t.Fatalf("rendered content should keep latest memory even when over limit:\n%s", content)
	}
	if strings.Contains(content, "old") {
		t.Fatalf("rendered content should drop older memory first:\n%s", content)
	}
}

func TestRendererSummarizesNonSpeakToolMemory(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit: 1024,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-2",
			EventType: "player_interacted_with_npc",
			GameTime: &protocolv1alpha2.GameTime{
				Year:   ptrInt32(1),
				Season: ptrInt32(1),
				Day:    ptrInt32(2),
				Hour:   ptrInt32(6),
				Minute: ptrInt32(30),
			},
		},
		Observation: &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		RecentMemories: []memory.Record{{
			Outcome: memory.TurnOutcome{
				ToolName:      "emote",
				ToolArguments: map[string]any{"emote": "happy"},
			},
			GameTime: &memory.GameTimeSnapshot{Year: 1, Season: 1, Day: 2, Hour: 6, Minute: 20},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[0].Content
	for _, want := range []string{
		"today 06:20",
		`used emote "happy"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered content missing %q:\n%s", want, content)
		}
	}
}

func TestRendererSummarizesMultiOutcomeMemory(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit: 1024,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "fake-game", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-2",
			EventType: "player_interacted_with_npc",
		},
		Observation: &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		RecentMemories: []memory.Record{{
			Outcomes: []memory.TurnOutcome{
				{ToolName: "speak", ToolArguments: map[string]any{"text": "hello"}},
				{ToolName: "emote", ToolArguments: map[string]any{"emote": "happy"}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[0].Content
	assertContainsAll(t, content, `said "hello"`, `used emote "happy"`)
}

func TestRendererMarksPreviousDayMemory(t *testing.T) {
	builder := agentcontext.NewBuilder()
	renderer := agentcontext.NewRenderer(agentcontext.RendererConfig{
		MemoryContextSizeLimit: 1024,
	})

	agentCtx, err := builder.Build(agentcontext.BuildInput{
		SessionKey:    session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		RuntimePolicy: "policy",
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-2",
			EventType: "player_interacted_with_npc",
			GameTime: &protocolv1alpha2.GameTime{
				Year:   ptrInt32(1),
				Season: ptrInt32(1),
				Day:    ptrInt32(3),
				Hour:   ptrInt32(7),
				Minute: ptrInt32(10),
			},
		},
		Observation: &protocolv1alpha2.Observation{WorldId: "world-a", EntityId: "npc:Abigail"},
		RecentMemories: []memory.Record{{
			Outcome: memory.TurnOutcome{
				ToolName:      "speak",
				ToolArguments: map[string]any{"text": "see you tomorrow"},
			},
			GameTime: &memory.GameTimeSnapshot{Year: 1, Season: 1, Day: 2, Hour: 18, Minute: 20},
		}},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	req, err := renderer.Render(agentCtx)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	content := req.Messages[0].Content
	for _, want := range []string{
		"previous day Y1 S1 D2 18:20",
		`said "see you tomorrow"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered content missing %q:\n%s", want, content)
		}
	}
}

func TestBuilderRejectsMissingCurrentObservation(t *testing.T) {
	builder := agentcontext.NewBuilder()

	_, err := builder.Build(agentcontext.BuildInput{
		SessionKey: session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		Event:      &protocolv1alpha2.GameEvent{EventId: "event-1"},
	})
	if err == nil {
		t.Fatal("Build returned nil error, want structural failure")
	}
}

func ptrInt32(value int32) *int32 {
	return &value
}

func mustStruct(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()

	value, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("NewStruct returned error: %v", err)
	}
	return value
}

func assertContainsAll(t *testing.T, content string, values ...string) {
	t.Helper()

	for _, want := range values {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func extractJSONField(t *testing.T, content string, field string) string {
	t.Helper()

	var values []map[string]any
	if err := json.Unmarshal([]byte(content), &values); err != nil {
		t.Fatalf("content is not JSON array: %v\n%s", err, content)
	}
	if len(values) == 0 {
		t.Fatalf("content has no values: %s", content)
	}
	got, _ := values[0][field].(string)
	return got
}

func extractJSONFieldMap(t *testing.T, content string, field string) map[string]any {
	t.Helper()

	var values []map[string]any
	if err := json.Unmarshal([]byte(content), &values); err != nil {
		t.Fatalf("content is not JSON array: %v\n%s", err, content)
	}
	if len(values) == 0 {
		t.Fatalf("content has no values: %s", content)
	}
	got, ok := values[0][field].(map[string]any)
	if !ok {
		t.Fatalf("content field %q = %#v, want object", field, values[0][field])
	}
	return got
}

func mustMarshalJSONBytes(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	return data
}
