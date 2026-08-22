package memory_test

import (
	"errors"
	"testing"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestProjectorBuildsRecordFromSuccessfulTurn(t *testing.T) {
	now := time.Unix(200, 0)
	projector := memory.NewProjector(func() time.Time { return now })
	key := session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"}
	args := mustStruct(t, map[string]any{"text": "hello"})

	record, err := projector.Project(memory.ProjectInput{
		SessionKey: key,
		TurnID:     "turn-1",
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-1",
			EventType: "player_interacted_with_npc",
			Sequence:  42,
			GameTime: &protocolv1alpha2.GameTime{
				Year:   ptrInt32(1),
				Season: ptrInt32(2),
				Day:    ptrInt32(3),
				Hour:   ptrInt32(6),
				Minute: ptrInt32(20),
				Tick:   ptrInt64(9001),
			},
		},
		ToolCall: model.ToolCall{
			Name:      "speak",
			Arguments: args,
		},
		ActionResult: &protocolv1alpha2.ActionResult{
			ActionId: "act-1",
			Status:   protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED,
		},
	})
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}

	if record.MemoryID == "" {
		t.Fatal("MemoryID is empty")
	}
	if record.SessionKey != key {
		t.Fatalf("SessionKey = %+v, want %+v", record.SessionKey, key)
	}
	if record.SourceTurnID != "turn-1" {
		t.Fatalf("SourceTurnID = %q, want turn-1", record.SourceTurnID)
	}
	if record.SourceEventID != "event-1" {
		t.Fatalf("SourceEventID = %q, want event-1", record.SourceEventID)
	}
	if record.SourceEventSequence != 42 {
		t.Fatalf("SourceEventSequence = %d, want 42", record.SourceEventSequence)
	}
	if record.EventType != "player_interacted_with_npc" {
		t.Fatalf("EventType = %q", record.EventType)
	}
	if record.GameTime == nil {
		t.Fatal("GameTime is nil")
	}
	if *record.GameTime != (memory.GameTimeSnapshot{Year: 1, Season: 2, Day: 3, Hour: 6, Minute: 20, Tick: 9001}) {
		t.Fatalf("GameTime = %+v", *record.GameTime)
	}
	if record.Outcome.ToolName != "speak" {
		t.Fatalf("ToolName = %q, want speak", record.Outcome.ToolName)
	}
	if got := record.Outcome.ToolArguments["text"]; got != "hello" {
		t.Fatalf("ToolArguments[text] = %v, want hello", got)
	}
	if record.Outcome.ActionStatus != "ACTION_STATUS_SUCCEEDED" {
		t.Fatalf("ActionStatus = %q, want ACTION_STATUS_SUCCEEDED", record.Outcome.ActionStatus)
	}
	if !record.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", record.CreatedAt, now)
	}
}

func TestProjectorRejectsMissingTurnID(t *testing.T) {
	projector := memory.NewProjector(func() time.Time { return time.Unix(200, 0) })

	_, err := projector.Project(memory.ProjectInput{
		SessionKey: session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-1",
			EventType: "player_interacted_with_npc",
		},
		ToolCall: model.ToolCall{Name: "speak"},
		ActionResult: &protocolv1alpha2.ActionResult{
			Status: protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED,
		},
	})
	if !errors.Is(err, memory.ErrProject) {
		t.Fatalf("Project error = %v, want ErrProject", err)
	}
}

func TestProjectorRejectsNilActionResult(t *testing.T) {
	projector := memory.NewProjector(func() time.Time { return time.Unix(200, 0) })

	_, err := projector.Project(memory.ProjectInput{
		SessionKey: session.AgentSessionKey{GameID: "stardew-valley", WorldID: "world-a", EntityID: "npc:Abigail"},
		TurnID:     "turn-1",
		Event: &protocolv1alpha2.GameEvent{
			EventId:   "event-1",
			EventType: "player_interacted_with_npc",
		},
		ToolCall: model.ToolCall{Name: "speak"},
	})
	if !errors.Is(err, memory.ErrProject) {
		t.Fatalf("Project error = %v, want ErrProject", err)
	}
}

func mustStruct(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()

	value, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("NewStruct returned error: %v", err)
	}
	return value
}

func ptrInt32(value int32) *int32 {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}
