package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"

	"google.golang.org/protobuf/types/known/structpb"
)

type schedulerTestEnvironment struct {
	mu           sync.Mutex
	calls        []string
	events       []string
	active       int
	maxActive    int
	statuses     map[string]protocolv1alpha2.ActionStatus
	actionErrors map[string]*protocolv1alpha2.Error
	submitErrors map[string]error
	delays       map[string]time.Duration
}

func (e *schedulerTestEnvironment) Observe(ctx context.Context, worldID string, entityID string) (*protocolv1alpha2.Observation, error) {
	return nil, nil
}

func (e *schedulerTestEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (*protocolv1alpha2.ActionResult, error) {
	label := schedulerActionLabel(req)
	e.recordStart(label)
	defer e.recordFinish(label)

	if delay := e.delay(label); delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if err := e.submitError(label); err != nil {
		return nil, err
	}

	status := e.status(label)
	output, err := structpb.NewStruct(map[string]any{"label": label})
	if err != nil {
		return nil, err
	}

	return &protocolv1alpha2.ActionResult{
		ActionId: req.GetActionId(),
		Status:   status,
		Output:   output,
		Error:    e.actionError(label),
	}, nil
}

func (e *schedulerTestEnvironment) StartAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (ActionStart, error) {
	return ActionStart{}, errors.New("StartAction is not implemented for sync scheduler tests")
}

func (e *schedulerTestEnvironment) WaitActionResult(ctx context.Context, actionID string) (*protocolv1alpha2.ActionResult, error) {
	return nil, errors.New("WaitActionResult is not implemented for sync scheduler tests")
}

func (e *schedulerTestEnvironment) CancelAction(actionID string, reason string) {}

func (e *schedulerTestEnvironment) SendTurnCompletion(ctx context.Context, completion *protocolv1alpha2.TurnCompletion) error {
	return nil
}

func (e *schedulerTestEnvironment) recordStart(label string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.calls = append(e.calls, label)
	e.events = append(e.events, "start:"+label)
	e.active++
	if e.active > e.maxActive {
		e.maxActive = e.active
	}
}

func (e *schedulerTestEnvironment) recordFinish(label string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.active--
	e.events = append(e.events, "finish:"+label)
}

func (e *schedulerTestEnvironment) status(label string) protocolv1alpha2.ActionStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.statuses == nil {
		return protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED
	}
	status := e.statuses[label]
	if status == protocolv1alpha2.ActionStatus_ACTION_STATUS_UNSPECIFIED {
		return protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED
	}
	return status
}

func (e *schedulerTestEnvironment) actionError(label string) *protocolv1alpha2.Error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.actionErrors == nil {
		return nil
	}
	return e.actionErrors[label]
}

func (e *schedulerTestEnvironment) submitError(label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.submitErrors == nil {
		return nil
	}
	return e.submitErrors[label]
}

func (e *schedulerTestEnvironment) delay(label string) time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.delays == nil {
		return 0
	}
	return e.delays[label]
}

func (e *schedulerTestEnvironment) callOrder() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]string, len(e.calls))
	copy(out, e.calls)
	return out
}

func (e *schedulerTestEnvironment) eventOrder() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]string, len(e.events))
	copy(out, e.events)
	return out
}

func (e *schedulerTestEnvironment) maxConcurrent() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.maxActive
}

func (e *schedulerTestEnvironment) activeCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.active
}

func TestSchedulerRunsSequentialCallsSerially(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "speak", "first"),
		schedulerCall("call_2", "speak", "second"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	assertStringSlice(t, env.callOrder(), []string{"first", "second"})
	assertToolResult(t, outcome.Results[0], "call_1", "speak", "succeeded", "action_succeeded")
	assertToolResult(t, outcome.Results[1], "call_2", "speak", "succeeded", "action_succeeded")
	if outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = true, want false")
	}
}

func TestSchedulerRunsParallelSafeGroupWithBoundedConcurrency(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
	)
	env := &schedulerTestEnvironment{delays: map[string]time.Duration{
		"a": 30 * time.Millisecond,
		"b": 30 * time.Millisecond,
		"c": 30 * time.Millisecond,
		"d": 30 * time.Millisecond,
	}}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_a", "sense", "a"),
		schedulerCall("call_b", "sense", "b"),
		schedulerCall("call_c", "sense", "c"),
		schedulerCall("call_d", "sense", "d"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := env.maxConcurrent(); got > 2 {
		t.Fatalf("max concurrent submissions = %d, want <= 2", got)
	}
	if got := env.maxConcurrent(); got != 2 {
		t.Fatalf("max concurrent submissions = %d, want scheduler to use concurrency 2", got)
	}
}

func TestSchedulerUsesSequentialCallAsOrderingBarrier(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{delays: map[string]time.Duration{
		"a": 20 * time.Millisecond,
		"b": 20 * time.Millisecond,
		"c": 5 * time.Millisecond,
		"d": 5 * time.Millisecond,
		"e": 5 * time.Millisecond,
	}}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_a", "sense", "a"),
		schedulerCall("call_b", "sense", "b"),
		schedulerCall("call_c", "speak", "c"),
		schedulerCall("call_d", "sense", "d"),
		schedulerCall("call_e", "sense", "e"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	events := env.eventOrder()
	if indexOf(events, "start:c") < indexOf(events, "finish:a") || indexOf(events, "start:c") < indexOf(events, "finish:b") {
		t.Fatalf("sequential call started before prior parallel group drained: %v", events)
	}
	if indexOf(events, "start:d") < indexOf(events, "finish:c") || indexOf(events, "start:e") < indexOf(events, "finish:c") {
		t.Fatalf("later parallel group started before sequential barrier finished: %v", events)
	}
}

func TestSchedulerReturnsResultsInOriginalToolCallOrder(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
	)
	env := &schedulerTestEnvironment{delays: map[string]time.Duration{
		"slow":   30 * time.Millisecond,
		"fast":   1 * time.Millisecond,
		"medium": 10 * time.Millisecond,
	}}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 3, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_slow", "sense", "slow"),
		schedulerCall("call_fast", "sense", "fast"),
		schedulerCall("call_medium", "sense", "medium"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	gotIDs := []string{outcome.Results[0].ToolCallID, outcome.Results[1].ToolCallID, outcome.Results[2].ToolCallID}
	assertStringSlice(t, gotIDs, []string{"call_slow", "call_fast", "call_medium"})
	if outcome.Results[0].Output["label"] != "slow" || outcome.Results[1].Output["label"] != "fast" || outcome.Results[2].Output["label"] != "medium" {
		t.Fatalf("results output order = %+v", outcome.Results)
	}
}

func TestSchedulerDoesNotExecuteWhenBatchValidationFails(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "speak", "first"),
		schedulerCall("call_2", "missing", "second"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(env.callOrder()); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
}

func TestSchedulerProducesOneToolResultPerToolCallWhenBatchValidationFails(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
		schedulerCapability("emote", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "speak", "first"),
		{ID: "call_2", Name: "emote"},
		schedulerCall("call_3", "speak", "third"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(outcome.Results); got != 3 {
		t.Fatalf("result count = %d, want 3", got)
	}
	assertToolResult(t, outcome.Results[0], "call_1", "speak", "skipped", "batch_validation_failed")
	assertToolResult(t, outcome.Results[1], "call_2", "emote", "invalid", "tool_arguments_missing")
	assertToolResult(t, outcome.Results[2], "call_3", "speak", "skipped", "batch_validation_failed")
	if !outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = false, want true")
	}
}

func TestSchedulerRejectsExclusivePolicyToolMixedBatchBeforeExecution(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapabilityWithPolicy("ask_player", tool.ConcurrencySequential, tool.ToolPolicy{ExclusivePerStep: true}),
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "ask_player", "question"),
		schedulerCall("call_2", "speak", "aside"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(env.callOrder()); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertToolResult(t, outcome.Results[0], "call_1", "ask_player", "invalid", "exclusive_tool_must_be_only_tool_call")
	assertToolResult(t, outcome.Results[1], "call_2", "speak", "skipped", "batch_validation_failed")
	if strings.Contains(outcome.Results[0].Message, "ask_player") {
		t.Fatalf("exclusive policy error message should not contain capability name: %q", outcome.Results[0].Message)
	}
	if !outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = false, want true")
	}
}

func TestSchedulerPreflightsBuildActionRequestBeforeAnyExecution(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "speak", "first"),
		{ID: "call_2", Name: "speak", Arguments: map[string]any{"bad": make(chan int)}},
		schedulerCall("call_3", "speak", "third"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(env.callOrder()); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertToolResult(t, outcome.Results[0], "call_1", "speak", "skipped", "batch_validation_failed")
	assertToolResult(t, outcome.Results[1], "call_2", "speak", "invalid", "action_request_invalid")
	assertToolResult(t, outcome.Results[2], "call_3", "speak", "skipped", "batch_validation_failed")
}

func TestSchedulerRejectsDuplicateToolCallIDsDuringPreflight(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_1", "speak", "first"),
		schedulerCall("call_1", "speak", "second"),
		schedulerCall("call_3", "speak", "third"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(env.callOrder()); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertToolResult(t, outcome.Results[0], "call_1", "speak", "invalid", "duplicate_tool_call_id")
	assertToolResult(t, outcome.Results[1], "call_1", "speak", "invalid", "duplicate_tool_call_id")
	assertToolResult(t, outcome.Results[2], "call_3", "speak", "skipped", "batch_validation_failed")
}

func TestSchedulerValidatesArgumentsAgainstInputSchemaBeforeExecution(t *testing.T) {
	registry := schedulerRegistry(schedulerCapabilityWithSchema(
		"speak",
		tool.ConcurrencySequential,
		`{"type":"object","properties":{"mood":{"type":"string","enum":["happy","sad"]}},"required":["mood"],"additionalProperties":false}`,
	))
	env := &schedulerTestEnvironment{}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		{ID: "call_bad", Name: "speak", Arguments: map[string]any{"mood": "angry"}},
		{ID: "call_good", Name: "speak", Arguments: map[string]any{"mood": "happy"}},
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	if got := len(env.callOrder()); got != 0 {
		t.Fatalf("submitted action count = %d, want 0", got)
	}
	assertToolResult(t, outcome.Results[0], "call_bad", "speak", "invalid", "tool_arguments_invalid")
	assertToolResult(t, outcome.Results[1], "call_good", "speak", "skipped", "batch_validation_failed")
	if !outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = false, want true")
	}
}

func TestSchedulerSkipsLaterGroupsAfterPriorGroupFailure(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{
		statuses: map[string]protocolv1alpha2.ActionStatus{
			"fail": protocolv1alpha2.ActionStatus_ACTION_STATUS_FAILED,
		},
		actionErrors: map[string]*protocolv1alpha2.Error{
			"fail": {Code: "adapter_failed", Message: "adapter rejected action"},
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_a", "sense", "a"),
		schedulerCall("call_fail", "speak", "fail"),
		schedulerCall("call_later", "sense", "later"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	assertStringSlice(t, env.callOrder(), []string{"a", "fail"})
	assertToolResult(t, outcome.Results[0], "call_a", "sense", "succeeded", "action_succeeded")
	assertToolResult(t, outcome.Results[1], "call_fail", "speak", "failed", "adapter_failed")
	assertToolResult(t, outcome.Results[2], "call_later", "sense", "skipped", "prior_group_failed")
	if !outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = false, want true")
	}
}

func TestSchedulerDrainsParallelGroupBeforeSkippingLaterGroupsOnModelVisibleFailure(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{
		delays: map[string]time.Duration{
			"success": 20 * time.Millisecond,
			"failed":  10 * time.Millisecond,
		},
		statuses: map[string]protocolv1alpha2.ActionStatus{
			"failed": protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED,
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_success", "sense", "success"),
		schedulerCall("call_failed", "sense", "failed"),
		schedulerCall("call_later", "speak", "later"),
	})
	if err != nil {
		t.Fatalf("Run returned technical error: %v", err)
	}

	assertContainsAll(t, env.callOrder(), []string{"success", "failed"})
	assertNotContains(t, env.callOrder(), "later")
	assertToolResult(t, outcome.Results[0], "call_success", "sense", "succeeded", "action_succeeded")
	assertToolResult(t, outcome.Results[1], "call_failed", "sense", "rejected", "action_rejected")
	assertToolResult(t, outcome.Results[2], "call_later", "speak", "skipped", "prior_group_failed")
	if !outcome.HasModelVisibleFailure {
		t.Fatal("HasModelVisibleFailure = false, want true")
	}
	if got := len(outcome.SuccessfulActions); got != 1 {
		t.Fatalf("successful action count = %d, want 1", got)
	}
}

func TestSchedulerReturnsCompletedSiblingActionsBeforeParallelTechnicalError(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
	)
	env := &schedulerTestEnvironment{
		delays: map[string]time.Duration{
			"fatal": 10 * time.Millisecond,
		},
		submitErrors: map[string]error{
			"fatal": errors.New("adapter transport closed"),
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_success", "sense", "success"),
		schedulerCall("call_fatal", "sense", "fatal"),
	})
	if err == nil {
		t.Fatal("Run returned nil error, want technical failure")
	}

	assertContainsAll(t, env.callOrder(), []string{"success", "fatal"})
	assertToolResult(t, outcome.Results[0], "call_success", "sense", "succeeded", "action_succeeded")
	if got := len(outcome.SuccessfulActions); got != 1 {
		t.Fatalf("successful action count = %d, want 1", got)
	}
	if outcome.SuccessfulActions[0].ToolCall.ID != "call_success" {
		t.Fatalf("successful action = %+v, want call_success", outcome.SuccessfulActions[0])
	}
}

func TestSchedulerReturnsCompletedActionsBeforeSequentialTechnicalError(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{
		submitErrors: map[string]error{
			"fatal": errors.New("adapter transport closed"),
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	outcome, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_success", "sense", "success"),
		schedulerCall("call_fatal", "sense", "fatal"),
	})
	if err == nil {
		t.Fatal("Run returned nil error, want technical failure")
	}

	assertStringSlice(t, env.callOrder(), []string{"success", "fatal"})
	assertToolResult(t, outcome.Results[0], "call_success", "sense", "succeeded", "action_succeeded")
	if got := len(outcome.SuccessfulActions); got != 1 {
		t.Fatalf("successful action count = %d, want 1", got)
	}
	if outcome.SuccessfulActions[0].ToolCall.ID != "call_success" {
		t.Fatalf("successful action = %+v, want call_success", outcome.SuccessfulActions[0])
	}
}

func TestSchedulerDrainsStartedWorkersBeforeTechnicalFailureTerminal(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
	)
	env := &schedulerTestEnvironment{
		delays: map[string]time.Duration{
			"fatal": 10 * time.Millisecond,
			"slow":  100 * time.Millisecond,
		},
		submitErrors: map[string]error{
			"fatal": errors.New("adapter transport closed"),
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_fatal", "sense", "fatal"),
		schedulerCall("call_slow", "sense", "slow"),
		schedulerCall("call_later", "sense", "later"),
	})
	if err == nil {
		t.Fatal("Run returned nil error, want technical failure")
	}

	assertContainsAll(t, env.callOrder(), []string{"fatal", "slow"})
	assertNotContains(t, env.callOrder(), "later")
	if env.activeCount() != 0 {
		t.Fatalf("active submissions = %d, want 0 after scheduler returns", env.activeCount())
	}
	events := env.eventOrder()
	if indexOf(events, "finish:slow") == -1 {
		t.Fatalf("slow worker was not drained before return: %v", events)
	}
}

func TestPreferTechnicalErrorKeepsRealErrorOverCancellation(t *testing.T) {
	canceled := context.Canceled
	transportClosed := errors.New("adapter transport closed")

	if got := preferTechnicalError(canceled, transportClosed); got != transportClosed {
		t.Fatalf("preferred error = %v, want transport error", got)
	}
	if got := preferTechnicalError(transportClosed, canceled); got != transportClosed {
		t.Fatalf("preferred error = %v, want first transport error", got)
	}
}

func TestTerminalActionToolResultUsesStatusCodeWhenAdapterCodeIsEmpty(t *testing.T) {
	result, err := toolResultFromActionResult(
		schedulerCall("call_rejected", "sense", "rejected"),
		&protocolv1alpha2.ActionResult{
			Status: protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED,
			Error:  &protocolv1alpha2.Error{},
		},
	)
	if err != nil {
		t.Fatalf("toolResultFromActionResult returned error: %v", err)
	}

	assertToolResult(t, result, "call_rejected", "sense", "rejected", "action_rejected")
}

func TestSchedulerFailsOnNonTerminalActionStatus(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("speak", tool.ConcurrencySequential),
	)
	env := &schedulerTestEnvironment{
		statuses: map[string]protocolv1alpha2.ActionStatus{
			"pending": protocolv1alpha2.ActionStatus_ACTION_STATUS_PENDING,
		},
	}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 2, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_pending", "speak", "pending"),
	})
	if err == nil {
		t.Fatal("Run returned nil error, want non-terminal status failure")
	}
	if !strings.Contains(err.Error(), "non_terminal_action_status") {
		t.Fatalf("error = %v, want non_terminal_action_status", err)
	}
}

func TestSchedulerHonorsMaxParallelToolCalls(t *testing.T) {
	registry := schedulerRegistry(
		schedulerCapability("sense", tool.ConcurrencyParallelSafe),
	)
	env := &schedulerTestEnvironment{delays: map[string]time.Duration{
		"a": 15 * time.Millisecond,
		"b": 15 * time.Millisecond,
		"c": 15 * time.Millisecond,
	}}
	scheduler := toolBatchScheduler{registry: registry, maxParallelToolCalls: 1, actionTimeout: time.Second}

	_, err := scheduler.Run(context.Background(), env, "world:test", "npc:Linus", []model.ToolCall{
		schedulerCall("call_a", "sense", "a"),
		schedulerCall("call_b", "sense", "b"),
		schedulerCall("call_c", "sense", "c"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := env.maxConcurrent(); got != 1 {
		t.Fatalf("max concurrent submissions = %d, want 1", got)
	}
}

func schedulerRegistry(capabilities ...*protocolv1alpha2.Capability) *tool.Registry {
	registry := tool.NewRegistry()
	registry.RegisterEnvironmentCapabilities(capabilities)
	return registry
}

func schedulerCapability(name string, concurrency tool.ConcurrencyMode) *protocolv1alpha2.Capability {
	return schedulerCapabilityWithSchema(name, concurrency, `{"type":"object"}`)
}

func schedulerCapabilityWithSchema(name string, concurrency tool.ConcurrencyMode, inputSchema string) *protocolv1alpha2.Capability {
	mode := protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL
	if concurrency == tool.ConcurrencyParallelSafe {
		mode = protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE
	}
	return &protocolv1alpha2.Capability{
		Name:            name,
		InputSchemaJson: inputSchema,
		ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
		ConcurrencyMode: mode,
	}
}

func schedulerCapabilityWithPolicy(name string, concurrency tool.ConcurrencyMode, policy tool.ToolPolicy) *protocolv1alpha2.Capability {
	capability := schedulerCapability(name, concurrency)
	capability.Extensions = schedulerToolPolicyExtensions(policy)
	return capability
}

func schedulerToolPolicyExtensions(policy tool.ToolPolicy) *structpb.Struct {
	extensions, err := structpb.NewStruct(map[string]any{
		"gameagent": map[string]any{
			"tool_policy": map[string]any{
				"exclusive_per_step":   policy.ExclusivePerStep,
				"settle_after_success": policy.SettleAfterSuccess,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return extensions
}

func schedulerCall(id string, name string, label string) model.ToolCall {
	return model.ToolCall{
		ID:        id,
		Name:      name,
		Arguments: map[string]any{"label": label},
	}
}

func schedulerActionLabel(req *protocolv1alpha2.ActionRequest) string {
	if req.GetArguments() == nil {
		return req.GetCapability()
	}
	value := req.GetArguments().GetFields()["label"]
	if value == nil {
		return req.GetCapability()
	}
	label := strings.TrimSpace(value.GetStringValue())
	if label == "" {
		return req.GetCapability()
	}
	return label
}

func assertToolResult(t *testing.T, got model.ToolResult, callID string, name string, status string, code string) {
	t.Helper()

	if got.ToolCallID != callID || got.Name != name || got.Status != status || got.Code != code {
		t.Fatalf("ToolResult = %+v, want id=%s name=%s status=%s code=%s", got, callID, name, status, code)
	}
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("slice = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice = %v, want %v", got, want)
		}
	}
}

func assertContainsAll(t *testing.T, got []string, want []string) {
	t.Helper()

	for _, expected := range want {
		if indexOf(got, expected) == -1 {
			t.Fatalf("slice = %v, want to contain %q", got, expected)
		}
	}
}

func assertNotContains(t *testing.T, got []string, unwanted string) {
	t.Helper()

	if indexOf(got, unwanted) != -1 {
		t.Fatalf("slice = %v, should not contain %q", got, unwanted)
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
