package agent_test

import (
	"context"
	"testing"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/llm/fake"
	"gameagent/runtime/internal/session"
	"gameagent/runtime/internal/tool"
	"gameagent/runtime/internal/trace"

	"google.golang.org/protobuf/types/known/structpb"
)

type recordingTraceRecorder struct {
	events []trace.Event
}

func (r *recordingTraceRecorder) Record(event trace.Event) {
	r.events = append(r.events, event)
}

func (r *recordingTraceRecorder) Close(ctx context.Context) error {
	return nil
}

type fakeEnvironment struct {
	observedWorldID  string
	observedEntityID string
	submittedAction  *protocolv1alpha2.ActionRequest
}

func (f *fakeEnvironment) Observe(ctx context.Context, worldID string, entityID string) (*protocolv1alpha2.Observation, error) {
	f.observedWorldID = worldID
	f.observedEntityID = entityID

	state, err := structpb.NewStruct(map[string]any{
		"weather": "snow",
		"time":    "afternoon",
	})
	if err != nil {
		return nil, err
	}

	return &protocolv1alpha2.Observation{
		EntityId: entityID,
		WorldId:  worldID,
		State:    state,
	}, nil
}

func (f *fakeEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (*protocolv1alpha2.ActionResult, error) {
	f.submittedAction = req

	return &protocolv1alpha2.ActionResult{
		ActionId: req.ActionId,
		Status:   protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED,
	}, nil
}

func TestHandleEventRunsOneTurnNPCInteraction(t *testing.T) {
	registry := tool.NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			Description:     "Make the NPC speak.",
			InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
			ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
		},
	})

	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	loop := agent.NewLoop(fake.NewProvider(), registry, recorder, agent.DefaultConfig())

	event := &protocolv1alpha2.GameEvent{
		EventId:        "event_1",
		EventType:      "player_interacted_with_npc",
		WorldId:        "world:test",
		TargetEntityId: "npc:Robin",
		Entities: []*protocolv1alpha2.EntityRef{
			{
				EntityId:   "player:local",
				EntityType: "player",
			},
			{
				EntityId:   "npc:Linus",
				EntityType: "npc",
			},
			{
				EntityId:   "npc:Robin",
				EntityType: "npc",
			},
		},
	}

	conn := agent.ConnectionContext{
		GameID:    "fake-game",
		SessionID: "session:test",
	}

	key := session.AgentSessionKey{
		GameID:   conn.GameID,
		WorldID:  event.WorldId,
		EntityID: event.TargetEntityId,
	}

	if err := loop.HandleEvent(context.Background(), env, conn, key, event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if env.observedWorldID != "world:test" {
		t.Fatalf("observed world id = %q, want %q", env.observedWorldID, "world:test")
	}

	if env.observedEntityID != "npc:Robin" {
		t.Fatalf("observed entity id = %q, want %q", env.observedEntityID, "npc:Robin")
	}

	if env.submittedAction == nil {
		t.Fatal("expected action to be submitted")
	}

	if env.submittedAction.WorldId != "world:test" {
		t.Fatalf("submitted world id = %q, want %q", env.submittedAction.WorldId, "world:test")
	}

	if env.submittedAction.EntityId != "npc:Robin" {
		t.Fatalf("submitted entity id = %q, want %q", env.submittedAction.EntityId, "npc:Robin")
	}

	if env.submittedAction.Capability != "speak" {
		t.Fatalf("submitted capability = %q, want %q", env.submittedAction.Capability, "speak")
	}

	if env.submittedAction.ActionId == "" {
		t.Fatal("expected submitted action to have an action id")
	}

	textValue := env.submittedAction.Arguments.Fields["text"]
	if textValue == nil || textValue.GetStringValue() == "" {
		t.Fatal("expected submitted speak action to include non-empty text")
	}

	wantTimeline := []trace.EventName{
		trace.EventTurnStarted,
		trace.EventObservationRequested,
		trace.EventObservationReceived,
		trace.EventModelRequestStarted,
		trace.EventModelResponseReceived,
		trace.EventToolCallSelected,
		trace.EventActionSubmitStarted,
		trace.EventActionResultReceived,
		trace.EventTurnCompleted,
	}
	if len(recorder.events) != len(wantTimeline) {
		t.Fatalf("trace event count = %d, want %d: %+v", len(recorder.events), len(wantTimeline), recorder.events)
	}

	for i, want := range wantTimeline {
		got := recorder.events[i]
		if got.Event != want {
			t.Fatalf("trace event[%d] = %q, want %q", i, got.Event, want)
		}
		if got.Seq != uint32(i+1) {
			t.Fatalf("trace event[%d] seq = %d, want %d", i, got.Seq, i+1)
		}
		if got.TraceID != got.TurnID {
			t.Fatalf("trace event[%d] trace_id = %q, want turn_id %q", i, got.TraceID, got.TurnID)
		}
		if got.GameID != conn.GameID || got.SessionID != conn.SessionID || got.WorldID != event.WorldId || got.EventID != event.EventId || got.EventType != event.EventType || got.EntityID != "npc:Robin" {
			t.Fatalf("trace event[%d] context mismatch: %+v", i, got)
		}
	}

	terminal := recorder.events[len(recorder.events)-1]
	if terminal.ActionID == "" {
		t.Fatal("expected terminal event to include action id")
	}
	if terminal.Tool != "speak" {
		t.Fatalf("terminal tool = %q, want %q", terminal.Tool, "speak")
	}
}
