package context_test

import (
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
