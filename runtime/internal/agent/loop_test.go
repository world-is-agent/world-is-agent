package agent_test

import (
	"context"
	"testing"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/llm/fake"
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
	observedEntityID string
	submittedAction  *protocolv1alpha1.ActionRequest
}

func (f *fakeEnvironment) Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error) {
	f.observedEntityID = entityID

	state, err := structpb.NewStruct(map[string]any{
		"weather": "snow",
		"time":    "afternoon",
	})
	if err != nil {
		return nil, err
	}

	return &protocolv1alpha1.Observation{
		EntityId: entityID,
		State:    state,
	}, nil
}

func (f *fakeEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error) {
	f.submittedAction = req

	return &protocolv1alpha1.ActionResult{
		ActionId: req.ActionId,
		Status:   protocolv1alpha1.ActionStatus_ACTION_STATUS_SUCCEEDED,
	}, nil
}

func TestHandleEventRunsOneTurnNPCInteraction(t *testing.T) {
	registry := tool.NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha1.Capability{
		{
			Name:            "speak",
			Description:     "Make the NPC speak.",
			InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
			ExecutionMode:   protocolv1alpha1.ExecutionMode_EXECUTION_MODE_SYNC,
		},
	})

	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	loop := agent.NewLoop(fake.NewProvider(), registry, recorder)

	event := &protocolv1alpha1.GameEvent{
		EventId:   "event_1",
		EventType: "player_interacted_with_npc",
		Entities: []*protocolv1alpha1.EntityRef{
			{
				EntityId:   "player:local",
				EntityType: "player",
			},
			{
				EntityId:   "npc:Linus",
				EntityType: "npc",
			},
		},
	}

	conn := agent.ConnectionContext{
		GameID: "fake-game",
		EnvID:  "env:test",
	}

	if err := loop.HandleEvent(context.Background(), env, conn, event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if env.observedEntityID != "npc:Linus" {
		t.Fatalf("observed entity id = %q, want %q", env.observedEntityID, "npc:Linus")
	}

	if env.submittedAction == nil {
		t.Fatal("expected action to be submitted")
	}

	if env.submittedAction.EntityId != "npc:Linus" {
		t.Fatalf("submitted entity id = %q, want %q", env.submittedAction.EntityId, "npc:Linus")
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
		if got.GameID != conn.GameID || got.EnvID != conn.EnvID || got.EventID != event.EventId || got.EventType != event.EventType || got.EntityID != "npc:Linus" {
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
