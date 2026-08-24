package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
	submittedActions []*protocolv1alpha2.ActionRequest
	statusByTool     map[string]protocolv1alpha2.ActionStatus
}

type recordingProvider struct {
	requests []model.Request
	response model.Response
}

func (p *recordingProvider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)

	if p.response.Decision.ToolCalls != nil || p.response.Decision.Control.Kind != "" {
		return p.response, nil
	}

	return model.Response{
		Decision: model.ModelDecision{
			ToolCalls: []model.ToolCall{
				{
					ID:        "call_1",
					Name:      "speak",
					Arguments: map[string]any{"text": "remember this line"},
				},
			},
			Control: model.ControlDirective{Kind: model.ControlSettle},
		},
	}, nil
}

type scriptedProvider struct {
	requests  []model.Request
	responses []model.Response
	delay     time.Duration
}

func (p *scriptedProvider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)

	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		}
	}

	if len(p.responses) == 0 {
		return model.Response{
			Decision: model.ModelDecision{
				Control: model.ControlDirective{Kind: model.ControlSettle},
			},
		}, nil
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
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

func assertTraceContainsInOrder(t *testing.T, events []trace.Event, want []trace.EventName) {
	t.Helper()

	next := 0
	for _, event := range events {
		if next < len(want) && event.Event == want[next] {
			next++
		}
	}
	if next != len(want) {
		t.Fatalf("trace did not contain ordered events %v; got %+v", want, events)
	}
}

func traceEventsByName(events []trace.Event, name trace.EventName) []trace.Event {
	out := make([]trace.Event, 0)
	for _, event := range events {
		if event.Event == name {
			out = append(out, event)
		}
	}
	return out
}

func traceEventCount(events []trace.Event, name trace.EventName) int {
	count := 0
	for _, event := range events {
		if event.Event == name {
			count++
		}
	}
	return count
}

func indexOfTrace(events []trace.Event, name trace.EventName) int {
	for i, event := range events {
		if event.Event == name {
			return i
		}
	}
	return -1
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

func newSpeakEmoteRegistry() *tool.Registry {
	registry := tool.NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			Description:     "Make the NPC speak.",
			InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
			ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
		},
		{
			Name:            "emote",
			Description:     "Make the NPC emote.",
			InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string"}},"required":["emote"]}`,
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
		Entities: []*protocolv1alpha2.EntityRef{
			{EntityId: key.EntityID, DefinitionId: key.EntityID},
		},
	}
}

func (f *fakeEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (*protocolv1alpha2.ActionResult, error) {
	f.submittedAction = req
	f.submittedActions = append(f.submittedActions, req)

	status := protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED
	if configured := f.statusByTool[req.Capability]; configured != protocolv1alpha2.ActionStatus_ACTION_STATUS_UNSPECIFIED {
		status = configured
	}

	return &protocolv1alpha2.ActionResult{
		ActionId: req.ActionId,
		Status:   status,
		Error: &protocolv1alpha2.Error{
			Code:    "adapter_" + strings.ToLower(strings.TrimPrefix(status.String(), "ACTION_STATUS_")),
			Message: "adapter returned " + status.String(),
		},
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
				EntityId:     "player:local",
				EntityType:   "player",
				DefinitionId: "player:local",
			},
			{
				EntityId:     "npc:Linus",
				EntityType:   "npc",
				DefinitionId: "npc:Linus",
			},
			{
				EntityId:     "npc:Robin",
				EntityType:   "npc",
				DefinitionId: "npc:Robin",
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
	assertTraceContainsInOrder(t, recorder.events, wantTimeline)

	for i, got := range recorder.events {
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

func TestHandleEventExecutesSingleToolCallFromModelDecision(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{
		response: model.Response{
			Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{{
					ID:        "call_1",
					Name:      "speak",
					Arguments: map[string]any{"text": "from decision"},
				}},
				Control: model.ControlDirective{Kind: model.ControlSettle},
			},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if env.submittedAction == nil {
		t.Fatal("expected action to be submitted")
	}
	if text := env.submittedAction.Arguments.Fields["text"].GetStringValue(); text != "from decision" {
		t.Fatalf("submitted text = %q, want from decision", text)
	}
	assertTraceContains(t, recorder.events, trace.EventToolCallSelected)
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventCompletesOnSettleOnlyDecision(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &recordingProvider{
		response: model.Response{
			Decision: model.ModelDecision{
				Control: model.ControlDirective{Kind: model.ControlSettle, Reason: "idle"},
			},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if env.submittedAction != nil {
		t.Fatalf("settle-only decision submitted action: %+v", env.submittedAction)
	}
	assertTraceContains(t, recorder.events, trace.EventModelResponseReceived)
	assertTraceNotContains(t, recorder.events, trace.EventActionSubmitStarted)
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventRunsBatchToolCallsThenSettle(t *testing.T) {
	registry := newSpeakEmoteRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{{
			Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}},
					{ID: "call_2", Name: "emote", Arguments: map[string]any{"emote": "happy"}},
				},
				Control: model.ControlDirective{Kind: model.ControlSettle},
			},
		}},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if got := len(env.submittedActions); got != 2 {
		t.Fatalf("submitted action count = %d, want 2", got)
	}
	if env.submittedActions[0].Capability != "speak" || env.submittedActions[1].Capability != "emote" {
		t.Fatalf("submitted capabilities = %s, %s; want speak, emote", env.submittedActions[0].Capability, env.submittedActions[1].Capability)
	}
	if got := len(provider.requests); got != 1 {
		t.Fatalf("provider request count = %d, want 1 because settle completed same step", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventRunsMultipleStepsUntilSettle(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{
				Decision: model.ModelDecision{
					ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "first step"}}},
					Control:   model.ControlDirective{Kind: model.ControlContinue},
				},
			},
			{
				Decision: model.ModelDecision{
					Control: model.ControlDirective{Kind: model.ControlSettle},
				},
			},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	if got := len(provider.requests); got != 2 {
		t.Fatalf("provider request count = %d, want 2", got)
	}
	if got := len(provider.requests[1].Messages); got != 3 {
		t.Fatalf("second request message count = %d, want user context plus tool transcript", got)
	}
	if provider.requests[1].Messages[1].Role != model.RoleAssistant || provider.requests[1].Messages[2].Role != model.RoleTool {
		t.Fatalf("second request transcript roles = %+v", provider.requests[1].Messages)
	}
	if !strings.Contains(provider.requests[1].Messages[1].Content, "first step") {
		t.Fatalf("second request missing prior tool call transcript:\n%s", provider.requests[1].Messages[1].Content)
	}
	if !strings.Contains(provider.requests[1].Messages[2].Content, "action_succeeded") {
		t.Fatalf("second request missing prior tool result transcript:\n%s", provider.requests[1].Messages[2].Content)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventFailsWhenMaxStepsExceeded(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_2", Name: "speak", Arguments: map[string]any{"text": "two"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
		},
	}
	config := agent.DefaultConfig()
	config.MaxSteps = 2
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if err == nil || !strings.Contains(err.Error(), "max steps exceeded") {
		t.Fatalf("HandleEvent error = %v, want max steps exceeded", err)
	}
	if got := len(env.submittedActions); got != 2 {
		t.Fatalf("submitted action count = %d, want 2 before max steps terminal", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnFailed)
}

func TestHandleEventFailsWhenMaxToolCallsPerStepExceeded(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{{
			Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}},
					{ID: "call_2", Name: "speak", Arguments: map[string]any{"text": "two"}},
				},
				Control: model.ControlDirective{Kind: model.ControlContinue},
			},
		}},
	}
	config := agent.DefaultConfig()
	config.MaxToolCallsPerStep = 1
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if err == nil || !strings.Contains(err.Error(), "max tool calls per step exceeded") {
		t.Fatalf("HandleEvent error = %v, want max tool calls per step exceeded", err)
	}
	if got := len(env.submittedActions); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnFailed)
}

func TestHandleEventFailsWhenMaxToolCallsPerTurnExceeded(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{
				{ID: "call_2", Name: "speak", Arguments: map[string]any{"text": "two"}},
				{ID: "call_3", Name: "speak", Arguments: map[string]any{"text": "three"}},
			}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
		},
	}
	config := agent.DefaultConfig()
	config.MaxToolCallsPerTurn = 2
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if err == nil || !strings.Contains(err.Error(), "max tool calls per turn exceeded") {
		t.Fatalf("HandleEvent error = %v, want max tool calls per turn exceeded", err)
	}
	if got := len(env.submittedActions); got != 1 {
		t.Fatalf("submitted action count = %d, want only first step executed", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnFailed)
}

func TestHandleEventTurnTimeoutCanPreemptBudgetsWithDelayedProvider(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{delay: 200 * time.Millisecond}
	config := agent.DefaultConfig()
	config.TurnTimeout = 30 * time.Millisecond
	config.LLMTimeout = time.Second
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("HandleEvent error = %v, want context deadline exceeded", err)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnFailed)
}

func TestFailedMultiStepTurnDoesNotAppendMemory(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	store := &failRecentStore{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "side effect happened"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
		},
	}
	config := agent.DefaultConfig()
	config.MaxSteps = 1
	loop := agent.NewLoop(provider, registry, recorder, config, agent.WithMemoryStore(store))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if err == nil || !strings.Contains(err.Error(), "max steps exceeded") {
		t.Fatalf("HandleEvent error = %v, want max steps exceeded", err)
	}
	if got := len(store.appended); got != 0 {
		t.Fatalf("appended memory count = %d, want 0 for failed turn", got)
	}
}

func TestCompletedTurnAfterRejectedActionWritesOnlySuccessfulOutcomes(t *testing.T) {
	registry := newSpeakEmoteRegistry()
	env := &fakeEnvironment{
		statusByTool: map[string]protocolv1alpha2.ActionStatus{
			"emote": protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED,
		},
	}
	recorder := &recordingTraceRecorder{}
	store := &failRecentStore{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "kept"}},
					{ID: "call_2", Name: "emote", Arguments: map[string]any{"emote": "happy"}},
				},
				Control: model.ControlDirective{Kind: model.ControlSettle},
			}},
			{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig(), agent.WithMemoryStore(store))
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if got := len(store.appended); got != 1 {
		t.Fatalf("appended memory count = %d, want 1", got)
	}
	record := store.appended[0]
	if got := len(record.Outcomes); got != 1 {
		t.Fatalf("memory outcome count = %d, want 1 successful outcome", got)
	}
	if record.Outcomes[0].ToolName != "speak" {
		t.Fatalf("memory outcomes = %+v, want only speak", record.Outcomes)
	}
	if got := record.Outcomes[0].ToolArguments["text"]; got != "kept" {
		t.Fatalf("memory speak text = %v, want kept", got)
	}
}

func TestHandleEventRetriesAfterInvalidToolCallBatchWithinStepBudget(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{{ID: "call_1", Name: "missing", Arguments: map[string]any{}}},
				Control:   model.ControlDirective{Kind: model.ControlContinue},
			}},
			{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if got := len(env.submittedActions); got != 0 {
		t.Fatalf("submitted action count = %d, want 0 for invalid batch", got)
	}
	if got := len(provider.requests); got != 2 {
		t.Fatalf("provider request count = %d, want retry step", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventRetriesAfterActionResultTerminalFailure(t *testing.T) {
	for _, status := range []protocolv1alpha2.ActionStatus{
		protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED,
		protocolv1alpha2.ActionStatus_ACTION_STATUS_FAILED,
		protocolv1alpha2.ActionStatus_ACTION_STATUS_CANCELLED,
		protocolv1alpha2.ActionStatus_ACTION_STATUS_INTERRUPTED,
	} {
		t.Run(status.String(), func(t *testing.T) {
			registry := newSpeakRegistry()
			env := &fakeEnvironment{statusByTool: map[string]protocolv1alpha2.ActionStatus{"speak": status}}
			recorder := &recordingTraceRecorder{}
			provider := &scriptedProvider{
				responses: []model.Response{
					{Decision: model.ModelDecision{
						ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "try"}}},
						Control:   model.ControlDirective{Kind: model.ControlContinue},
					}},
					{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
				},
			}
			loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
			conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
			key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

			if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
				t.Fatalf("HandleEvent returned error: %v", err)
			}
			if got := len(provider.requests); got != 2 {
				t.Fatalf("provider request count = %d, want retry step", got)
			}
			assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
		})
	}
}

func TestHandleEventDoesNotSettleAfterFailedBatchEvenWhenControlSettleRequested(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{statusByTool: map[string]protocolv1alpha2.ActionStatus{
		"speak": protocolv1alpha2.ActionStatus_ACTION_STATUS_FAILED,
	}}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "fail"}}},
				Control:   model.ControlDirective{Kind: model.ControlSettle},
			}},
			{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if got := len(provider.requests); got != 2 {
		t.Fatalf("provider request count = %d, want second step despite first settle", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnCompleted)
}

func TestHandleEventFailsWhenFailureLoopExhaustsMaxSteps(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{{ID: "call_1", Name: "missing", Arguments: map[string]any{}}},
				Control:   model.ControlDirective{Kind: model.ControlSettle},
			}},
		},
	}
	config := agent.DefaultConfig()
	config.MaxSteps = 1
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))
	if err == nil || !strings.Contains(err.Error(), "max steps exceeded") {
		t.Fatalf("HandleEvent error = %v, want max steps exceeded", err)
	}
	if got := len(env.submittedActions); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertTraceContains(t, recorder.events, trace.EventTurnFailed)
}

func TestMultiStepTraceEventsShareTurnIDAndIncreaseStepIndex(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "first"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
			{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	stepEvents := traceEventsByName(recorder.events, trace.EventAgentStepStarted)
	if got := len(stepEvents); got != 2 {
		t.Fatalf("step started count = %d, want 2; events=%+v", got, recorder.events)
	}
	if stepEvents[0].TurnID == "" || stepEvents[0].TurnID != stepEvents[1].TurnID {
		t.Fatalf("step turn ids = %q, %q; want same non-empty", stepEvents[0].TurnID, stepEvents[1].TurnID)
	}
	if stepEvents[0].Fields["step_index"] != 1 || stepEvents[1].Fields["step_index"] != 2 {
		t.Fatalf("step indices = %+v, %+v; want 1 then 2", stepEvents[0].Fields, stepEvents[1].Fields)
	}
}

func TestToolBatchTraceFieldsIncludeCallCountAndConcurrency(t *testing.T) {
	registry := newSpeakEmoteRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{{
			Decision: model.ModelDecision{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}},
					{ID: "call_2", Name: "emote", Arguments: map[string]any{"emote": "happy"}},
				},
				Control: model.ControlDirective{Kind: model.ControlSettle},
			},
		}},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	batchEvents := traceEventsByName(recorder.events, trace.EventToolBatchStarted)
	if got := len(batchEvents); got != 1 {
		t.Fatalf("tool batch started count = %d, want 1; events=%+v", got, recorder.events)
	}
	fields := batchEvents[0].Fields
	if fields["tool_call_count"] != 2 {
		t.Fatalf("tool_call_count = %#v, want 2", fields["tool_call_count"])
	}
	if !strings.Contains(fmt.Sprint(fields["concurrency_modes"]), "sequential") {
		t.Fatalf("concurrency_modes = %#v, want sequential", fields["concurrency_modes"])
	}
}

func TestMultiStepTerminalEventIsUniqueAndLast(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "first"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
			{Decision: model.ModelDecision{Control: model.ControlDirective{Kind: model.ControlSettle}}},
		},
	}
	loop := agent.NewLoop(provider, registry, recorder, agent.DefaultConfig())
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	if err := loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key)); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	terminalCount := traceEventCount(recorder.events, trace.EventTurnCompleted) + traceEventCount(recorder.events, trace.EventTurnFailed)
	if terminalCount != 1 {
		t.Fatalf("terminal event count = %d, want 1; events=%+v", terminalCount, recorder.events)
	}
	if recorder.events[len(recorder.events)-1].Event != trace.EventTurnCompleted {
		t.Fatalf("last event = %q, want turn_completed; events=%+v", recorder.events[len(recorder.events)-1].Event, recorder.events)
	}
	if indexOfTrace(recorder.events, trace.EventTurnSettled) >= len(recorder.events)-1 {
		t.Fatalf("turn_settled should be non-terminal before turn_completed; events=%+v", recorder.events)
	}
}

func TestMaxStepsTraceFailureReason(t *testing.T) {
	registry := newSpeakRegistry()
	env := &fakeEnvironment{}
	recorder := &recordingTraceRecorder{}
	provider := &scriptedProvider{
		responses: []model.Response{
			{Decision: model.ModelDecision{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "speak", Arguments: map[string]any{"text": "one"}}}, Control: model.ControlDirective{Kind: model.ControlContinue}}},
		},
	}
	config := agent.DefaultConfig()
	config.MaxSteps = 1
	loop := agent.NewLoop(provider, registry, recorder, config)
	conn := agent.ConnectionContext{GameID: "fake-game", SessionID: "session:test"}
	key := session.AgentSessionKey{GameID: conn.GameID, WorldID: "world:test", EntityID: "npc:Abigail"}

	_ = loop.HandleEvent(context.Background(), env, conn, key, gameEvent("event_1", key))

	terminal := recorder.events[len(recorder.events)-1]
	if terminal.Event != trace.EventTurnFailed || terminal.Reason != "max_steps_exceeded" {
		t.Fatalf("terminal = %+v, want turn_failed reason max_steps_exceeded", terminal)
	}
}
