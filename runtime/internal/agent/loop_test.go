package agent_test

import (
	"context"
	"testing"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"

	"google.golang.org/protobuf/types/known/structpb"
)

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
	registry.RegisterEnvironmentCapabilities([]string{"speak"})

	env := &fakeEnvironment{}
	loop := agent.NewLoop(model.FakeProvider{}, registry)

	event := &protocolv1alpha1.GameEvent{
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

	if err := loop.HandleEvent(context.Background(), env, event); err != nil {
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
}
