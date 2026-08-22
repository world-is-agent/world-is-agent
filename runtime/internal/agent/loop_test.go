package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/llm/fake"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
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

type recordingProvider struct {
	requests []model.Request
}

func (p *recordingProvider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)

	args, err := structpb.NewStruct(map[string]any{
		"text": "remember this line",
	})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{
		ToolCall: model.ToolCall{
			Name:      "speak",
			Arguments: args,
		},
	}, nil
}

type failRecentStore struct {
	appended []memory.Record
}

func (s *failRecentStore) Append(ctx context.Context, record memory.Record) error {
	s.appended = append(s.appended, record)
	return nil
}

func (s *failRecentStore) Recent(ctx context.Context, key session.AgentSessionKey, limit int) ([]memory.Record, error) {
	return nil, errors.New("memory read failed")
}

type failAppendStore struct{}

func (s failAppendStore) Append(ctx context.Context, record memory.Record) error {
	return errors.New("memory append failed")
}

func (s failAppendStore) Recent(ctx context.Context, key session.AgentSessionKey, limit int) ([]memory.Record, error) {
	return nil, nil
}

type failProjector struct{}

func (failProjector) Project(input memory.ProjectInput) (memory.Record, error) {
	return memory.Record{}, memory.ErrProject
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

func TestHandleEventLoadsRecentMemoryOnLaterTurn(t *testing.T) {
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
	provider := &recordingProvider{}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	first := &protocolv1alpha2.GameEvent{
		EventId:        "event_1",
		EventType:      "player_interacted_with_npc",
		WorldId:        key.WorldID,
		TargetEntityId: key.EntityID,
	}
	second := &protocolv1alpha2.GameEvent{
		EventId:        "event_2",
		EventType:      "player_interacted_with_npc",
		WorldId:        key.WorldID,
		TargetEntityId: key.EntityID,
	}

	if err := loop.HandleEvent(context.Background(), env, conn, key, first); err != nil {
		t.Fatalf("first HandleEvent returned error: %v", err)
	}
	if err := loop.HandleEvent(context.Background(), env, conn, key, second); err != nil {
		t.Fatalf("second HandleEvent returned error: %v", err)
	}

	if len(provider.requests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(provider.requests))
	}
	secondContent := provider.requests[1].Messages[0].Content
	for _, want := range []string{
		"[Recent Memory]",
		"previous interaction",
		`said "remember this line"`,
		"remember this line",
	} {
		if !strings.Contains(secondContent, want) {
			t.Fatalf("second request missing %q:\n%s", want, secondContent)
		}
	}
	for _, unwanted := range []string{
		"event_1",
		"ACTION_STATUS_SUCCEEDED",
		"source_turn_id",
	} {
		if strings.Contains(secondContent, unwanted) {
			t.Fatalf("second request should not expose storage field %q:\n%s", unwanted, secondContent)
		}
	}

	assertTraceContains(t, recorder.events, trace.EventContextLoaded)
	assertTraceContains(t, recorder.events, trace.EventContextUpdated)
}

func TestHandleEventDefaultStoreRetainsAtLeastRecentMemoryLimit(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	config := agent.DefaultConfig()
	config.RecentMemoryLimit = 25
	config.MemoryContextSizeLimit = 65536
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	for i := 1; i <= 26; i++ {
		if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent(fmt.Sprintf("event_%02d", i), key)); err != nil {
			t.Fatalf("HandleEvent(%d) returned error: %v", i, err)
		}
	}

	if len(provider.requests) != 26 {
		t.Fatalf("provider request count = %d, want 26", len(provider.requests))
	}
	lastContent := provider.requests[25].Messages[0].Content
	if got := strings.Count(lastContent, "remember this line"); got != 25 {
		t.Fatalf("default memory store should retain recent_memory_limit records; rendered memory count = %d, want 25:\n%s", got, lastContent)
	}
}

func TestHandleEventSkipsMemoryWhenDisabled(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	config := agent.DefaultConfig()
	config.MemoryEnabled = boolPtr(false)
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("first HandleEvent returned error: %v", err)
	}
	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_2", key)); err != nil {
		t.Fatalf("second HandleEvent returned error: %v", err)
	}

	if len(provider.requests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(provider.requests))
	}
	secondContent := provider.requests[1].Messages[0].Content
	if strings.Contains(secondContent, "remember this line") {
		t.Fatalf("second request contains memory while memory disabled:\n%s", secondContent)
	}
	assertTraceContains(t, recorder.events, trace.EventContextLoaded)
	assertTraceNotContains(t, recorder.events, trace.EventContextUpdated)
}

func TestWithMemoryStoreNilDoesNotDisableDefaultMemoryStore(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig(), agent.WithMemoryStore(nil))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("first HandleEvent returned error: %v", err)
	}
	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_2", key)); err != nil {
		t.Fatalf("second HandleEvent returned error: %v", err)
	}

	if len(provider.requests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(provider.requests))
	}
	secondContent := provider.requests[1].Messages[0].Content
	if !strings.Contains(secondContent, `said "remember this line"`) {
		t.Fatalf("nil memory store option should keep default store; second request:\n%s", secondContent)
	}
}

func TestHandleEventFailOpenWhenMemoryLoadFails(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	store := &failRecentStore{}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig(), agent.WithMemoryStore(store))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if len(provider.requests) != 1 {
		t.Fatalf("provider request count = %d, want 1", len(provider.requests))
	}
	if len(store.appended) != 1 {
		t.Fatalf("append count = %d, want 1", len(store.appended))
	}
	assertTraceContains(t, recorder.events, trace.EventContextLoadFailed)
	assertTraceContains(t, recorder.events, trace.EventContextUpdated)
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventCompletesWhenMemoryAppendFails(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig(), agent.WithMemoryStore(failAppendStore{}))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	assertTraceContains(t, recorder.events, trace.EventContextUpdateFailed)
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventCompletesWhenMemoryProjectionFails(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig(), agent.WithMemoryProjector(failProjector{}))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	assertTraceContains(t, recorder.events, trace.EventContextUpdateFailed)
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func assertTraceContains(t *testing.T, events []trace.Event, want trace.EventName) {
	t.Helper()

	for _, event := range events {
		if event.Event == want {
			return
		}
	}
	t.Fatalf("trace missing event %q; got %+v", want, events)
}

func assertTraceNotContains(t *testing.T, events []trace.Event, unwanted trace.EventName) {
	t.Helper()

	for _, event := range events {
		if event.Event == unwanted {
			t.Fatalf("trace unexpectedly contains event %q; got %+v", unwanted, events)
		}
	}
}

func newSpeakRegistry() *tool.Registry {
	registry := tool.NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			Description:     "Make the NPC speak.",
			InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
			ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
		},
	})
	return registry
}

func gameEvent(eventID string, key session.AgentSessionKey) *protocolv1alpha2.GameEvent {
	return &protocolv1alpha2.GameEvent{
		EventId:        eventID,
		EventType:      "player_interacted_with_npc",
		WorldId:        key.WorldID,
		TargetEntityId: key.EntityID,
	}
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
		trace.EventContextLoaded,
		trace.EventModelRequestStarted,
		trace.EventModelResponseReceived,
		trace.EventToolCallSelected,
		trace.EventActionSubmitStarted,
		trace.EventActionResultReceived,
		trace.EventContextUpdated,
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
