package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"
)

const (
	toolResultStatusSucceeded   = "succeeded"
	toolResultStatusInvalid     = "invalid"
	toolResultStatusSkipped     = "skipped"
	toolResultStatusRejected    = "rejected"
	toolResultStatusFailed      = "failed"
	toolResultStatusCancelled   = "cancelled"
	toolResultStatusInterrupted = "interrupted"

	toolResultCodeActionSucceeded        = "action_succeeded"
	toolResultCodeBatchValidationFailed  = "batch_validation_failed"
	toolResultCodePriorGroupFailed       = "prior_group_failed"
	toolResultCodeDuplicateToolCallID    = "duplicate_tool_call_id"
	toolResultCodeToolNotRegistered      = "tool_not_registered"
	toolResultCodeToolArgumentsMissing   = "tool_arguments_missing"
	toolResultCodeToolArgumentsInvalid   = "tool_arguments_invalid"
	toolResultCodeActionRequestInvalid   = "action_request_invalid"
	toolResultCodeNonTerminalActionState = "non_terminal_action_status"
	toolResultCodeExclusiveToolBatch     = "exclusive_tool_must_be_only_tool_call"
	toolResultCodeAsyncBatchUnsupported  = "async_batch_unsupported"
	toolResultCodeActionStartRejected    = "action_start_rejected"
)

type toolBatchScheduler struct {
	registry             *tool.Registry
	maxParallelToolCalls int
	actionTimeout        time.Duration
	actionStartTimeout   time.Duration
	asyncActionTimeout   time.Duration
	sourceEventID        string
	sourceTurnID         string
	onActionSubmit       func(plannedToolCall)
	onActionStatusUpdate func(plannedToolCall, *protocolv1alpha2.ActionStatusUpdate)
	onActionResult       func(plannedToolCall, *protocolv1alpha2.ActionResult)
}

type toolBatchOutcome struct {
	Results                []model.ToolResult
	SuccessfulActions      []completedToolAction
	HasModelVisibleFailure bool
	SettleAfterSuccess     bool
}

type completedToolAction struct {
	ToolCall     model.ToolCall
	ActionResult *protocolv1alpha2.ActionResult
	Policy       tool.ToolPolicy
}

type plannedToolCall struct {
	index   int
	call    model.ToolCall
	entry   tool.Entry
	request *protocolv1alpha2.ActionRequest
}

type parallelExecutionResult struct {
	item         plannedToolCall
	result       model.ToolResult
	actionResult *protocolv1alpha2.ActionResult
	err          error
}

func (s toolBatchScheduler) Run(
	ctx context.Context,
	env Environment,
	worldID string,
	entityID string,
	calls []model.ToolCall,
) (toolBatchOutcome, error) {
	plan, validationResults, validationFailed := s.preflight(worldID, entityID, calls)
	if validationFailed {
		return toolBatchOutcome{
			Results:                validationResults,
			HasModelVisibleFailure: true,
		}, nil
	}
	if len(plan) == 1 && plan[0].entry.Execution == tool.ExecutionAsync {
		result, actionResult, err := s.runAsyncOne(ctx, env, plan[0])
		if err != nil {
			return toolBatchOutcome{}, err
		}
		outcome := toolBatchOutcome{
			Results: []model.ToolResult{result},
		}
		if result.Status == toolResultStatusSucceeded {
			outcome.SuccessfulActions = []completedToolAction{{
				ToolCall:     plan[0].call,
				ActionResult: actionResult,
				Policy:       plan[0].entry.Policy,
			}}
			outcome.SettleAfterSuccess = plan[0].entry.Policy.SettleAfterSuccess
		} else {
			outcome.HasModelVisibleFailure = true
		}
		return outcome, nil
	}

	results := make([]model.ToolResult, len(calls))
	successfulActions := make([]completedToolAction, 0, len(calls))
	settleAfterSuccess := false
	for i := 0; i < len(plan); {
		if plan[i].entry.Concurrency == tool.ConcurrencyParallelSafe {
			end := i + 1
			for end < len(plan) && plan[end].entry.Concurrency == tool.ConcurrencyParallelSafe {
				end++
			}
			groupSuccessfulActions, failed, err := s.runParallelGroup(ctx, env, plan[i:end], results)
			if err != nil {
				successfulActions = append(successfulActions, groupSuccessfulActions...)
				settleAfterSuccess = settleAfterSuccess || completedActionsShouldSettle(groupSuccessfulActions)
				return toolBatchOutcome{Results: results, SuccessfulActions: successfulActions, SettleAfterSuccess: settleAfterSuccess}, err
			}
			successfulActions = append(successfulActions, groupSuccessfulActions...)
			settleAfterSuccess = settleAfterSuccess || completedActionsShouldSettle(groupSuccessfulActions)
			if failed {
				fillPriorGroupSkipped(results, plan[end:])
				return toolBatchOutcome{
					Results:                results,
					SuccessfulActions:      successfulActions,
					HasModelVisibleFailure: true,
					SettleAfterSuccess:     settleAfterSuccess,
				}, nil
			}
			i = end
			continue
		}

		result, actionResult, err := s.runOne(ctx, env, plan[i])
		if err != nil {
			return toolBatchOutcome{Results: results, SuccessfulActions: successfulActions, SettleAfterSuccess: settleAfterSuccess}, err
		}
		results[plan[i].index] = result
		if result.Status != toolResultStatusSucceeded {
			fillPriorGroupSkipped(results, plan[i+1:])
			return toolBatchOutcome{
				Results:                results,
				SuccessfulActions:      successfulActions,
				HasModelVisibleFailure: true,
				SettleAfterSuccess:     settleAfterSuccess,
			}, nil
		}
		action := completedToolAction{
			ToolCall:     plan[i].call,
			ActionResult: actionResult,
			Policy:       plan[i].entry.Policy,
		}
		successfulActions = append(successfulActions, action)
		settleAfterSuccess = settleAfterSuccess || action.Policy.SettleAfterSuccess
		i++
	}

	return toolBatchOutcome{Results: results, SuccessfulActions: successfulActions, SettleAfterSuccess: settleAfterSuccess}, nil
}

func (s toolBatchScheduler) preflight(
	worldID string,
	entityID string,
	calls []model.ToolCall,
) ([]plannedToolCall, []model.ToolResult, bool) {
	duplicateIDs := duplicateToolCallIDs(calls)
	results := make([]model.ToolResult, len(calls))
	invalid := make([]bool, len(calls))
	plan := make([]plannedToolCall, 0, len(calls))
	hasFailure := false

	for i, call := range calls {
		if duplicateIDs[strings.TrimSpace(call.ID)] {
			results[i] = invalidToolResult(call, toolResultCodeDuplicateToolCallID, "duplicate tool call id")
			invalid[i] = true
			hasFailure = true
			continue
		}

		entry, ok := s.lookup(call.Name)
		if !ok {
			results[i] = invalidToolResult(call, toolResultCodeToolNotRegistered, fmt.Sprintf("tool %q is not registered", call.Name))
			invalid[i] = true
			hasFailure = true
			continue
		}
		if entry.Policy.ExclusivePerStep && len(calls) > 1 {
			results[i] = invalidToolResult(call, toolResultCodeExclusiveToolBatch, "tool with exclusive_per_step policy must be the only tool call in the model response")
			invalid[i] = true
			hasFailure = true
			continue
		}
		if entry.Execution == tool.ExecutionAsync && len(calls) > 1 {
			results[i] = invalidToolResult(call, toolResultCodeAsyncBatchUnsupported, "async tool must be the only tool call in the model response")
			invalid[i] = true
			hasFailure = true
			continue
		}
		if call.Arguments == nil {
			results[i] = invalidToolResult(call, toolResultCodeToolArgumentsMissing, "tool arguments are missing")
			invalid[i] = true
			hasFailure = true
			continue
		}
		if err := tool.ValidateArguments(entry.Definition.InputSchema, call.Arguments); err != nil {
			results[i] = invalidToolResult(call, toolResultCodeToolArgumentsInvalid, err.Error())
			invalid[i] = true
			hasFailure = true
			continue
		}

		actionRequest, err := tool.BuildActionRequest(tool.ActionRequestInput{
			WorldID:       worldID,
			EntityID:      entityID,
			SourceEventID: s.sourceEventID,
			SourceTurnID:  s.sourceTurnID,
			ToolCall:      call,
		})
		if err != nil {
			results[i] = invalidToolResult(call, toolResultCodeActionRequestInvalid, err.Error())
			invalid[i] = true
			hasFailure = true
			continue
		}

		plan = append(plan, plannedToolCall{
			index:   i,
			call:    call,
			entry:   entry,
			request: actionRequest,
		})
	}

	if hasFailure {
		for i, call := range calls {
			if invalid[i] {
				continue
			}
			results[i] = skippedToolResult(call, toolResultCodeBatchValidationFailed, "batch validation failed")
		}
		return nil, results, true
	}

	return plan, nil, false
}

func (s toolBatchScheduler) lookup(name string) (tool.Entry, bool) {
	if s.registry == nil {
		return tool.Entry{}, false
	}
	return s.registry.Lookup(name)
}

func (s toolBatchScheduler) runParallelGroup(
	ctx context.Context,
	env Environment,
	group []plannedToolCall,
	results []model.ToolResult,
) ([]completedToolAction, bool, error) {
	groupCtx, cancelGroup := context.WithCancel(ctx)
	defer cancelGroup()

	limit := s.maxParallelToolCalls
	if limit <= 0 {
		limit = 1
	}

	resultCh := make(chan parallelExecutionResult, len(group))
	active := 0
	next := 0
	launchingStopped := false

	launch := func(item plannedToolCall) {
		active++
		go func() {
			result, actionResult, err := s.runOne(groupCtx, env, item)
			resultCh <- parallelExecutionResult{
				item:         item,
				result:       result,
				actionResult: actionResult,
				err:          err,
			}
		}()
	}

	for active < limit && next < len(group) {
		launch(group[next])
		next++
	}

	var firstErr error
	modelVisibleFailure := false
	successfulActionsByIndex := make(map[int]completedToolAction, len(group))
	for active > 0 {
		executed := <-resultCh
		active--

		if executed.err != nil {
			firstErr = preferTechnicalError(firstErr, executed.err)
			launchingStopped = true
			cancelGroup()
		} else {
			results[executed.item.index] = executed.result
			if executed.result.Status != toolResultStatusSucceeded {
				modelVisibleFailure = true
			} else {
				successfulActionsByIndex[executed.item.index] = completedToolAction{
					ToolCall:     executed.item.call,
					ActionResult: executed.actionResult,
					Policy:       executed.item.entry.Policy,
				}
			}
		}

		for !launchingStopped && active < limit && next < len(group) {
			launch(group[next])
			next++
		}
	}

	if firstErr != nil {
		return successfulActionsFromParallelGroup(group, successfulActionsByIndex), false, firstErr
	}
	return successfulActionsFromParallelGroup(group, successfulActionsByIndex), modelVisibleFailure, nil
}

func successfulActionsFromParallelGroup(group []plannedToolCall, actionsByIndex map[int]completedToolAction) []completedToolAction {
	successfulActions := make([]completedToolAction, 0, len(actionsByIndex))
	for _, item := range group {
		if action, ok := actionsByIndex[item.index]; ok {
			successfulActions = append(successfulActions, action)
		}
	}
	return successfulActions
}

func completedActionsShouldSettle(actions []completedToolAction) bool {
	for _, action := range actions {
		if action.Policy.SettleAfterSuccess {
			return true
		}
	}
	return false
}

func (s toolBatchScheduler) runOne(ctx context.Context, env Environment, item plannedToolCall) (model.ToolResult, *protocolv1alpha2.ActionResult, error) {
	if env == nil {
		return model.ToolResult{}, nil, errors.New("environment is nil")
	}

	actionCtx := ctx
	cancel := func() {}
	if s.actionTimeout > 0 {
		actionCtx, cancel = context.WithTimeout(ctx, s.actionTimeout)
	}
	defer cancel()

	if s.onActionSubmit != nil {
		s.onActionSubmit(item)
	}

	actionResult, err := env.SubmitAction(actionCtx, item.request)
	if err != nil {
		return model.ToolResult{}, nil, err
	}
	if actionResult == nil {
		return model.ToolResult{}, nil, errors.New("action result is nil")
	}
	if s.onActionResult != nil {
		s.onActionResult(item, actionResult)
	}

	result, err := toolResultFromActionResult(item.call, actionResult)
	return result, actionResult, err
}

func (s toolBatchScheduler) runAsyncOne(ctx context.Context, env Environment, item plannedToolCall) (model.ToolResult, *protocolv1alpha2.ActionResult, error) {
	if env == nil {
		return model.ToolResult{}, nil, errors.New("environment is nil")
	}

	startCtx := ctx
	cancelStart := func() {}
	if s.actionStartTimeout > 0 {
		startCtx, cancelStart = context.WithTimeout(ctx, s.actionStartTimeout)
	}
	defer cancelStart()

	if s.onActionSubmit != nil {
		s.onActionSubmit(item)
	}

	start, err := env.StartAction(startCtx, item.request)
	if err != nil {
		return model.ToolResult{}, nil, err
	}
	if start.Result != nil {
		if s.onActionResult != nil {
			s.onActionResult(item, start.Result)
		}
		result, err := toolResultFromActionResultWithDefaultRejectedCode(item.call, start.Result, toolResultCodeActionStartRejected)
		return result, start.Result, err
	}
	if start.Update == nil {
		return model.ToolResult{}, nil, errors.New("action start is missing status update or terminal result")
	}
	if s.onActionStatusUpdate != nil {
		s.onActionStatusUpdate(item, start.Update)
	}

	waitCtx := ctx
	cancelWait := func() {}
	if s.asyncActionTimeout > 0 {
		waitCtx, cancelWait = context.WithTimeout(ctx, s.asyncActionTimeout)
	}
	defer cancelWait()

	actionResult, err := env.WaitActionResult(waitCtx, item.request.GetActionId())
	if err != nil {
		return model.ToolResult{}, nil, err
	}
	if actionResult == nil {
		return model.ToolResult{}, nil, errors.New("action result is nil")
	}
	if s.onActionResult != nil {
		s.onActionResult(item, actionResult)
	}

	result, err := toolResultFromActionResult(item.call, actionResult)
	return result, actionResult, err
}

func toolResultFromActionResult(call model.ToolCall, actionResult *protocolv1alpha2.ActionResult) (model.ToolResult, error) {
	switch actionResult.GetStatus() {
	case protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED:
		return model.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Status:     toolResultStatusSucceeded,
			Code:       toolResultCodeActionSucceeded,
			Output:     actionResultOutput(actionResult),
		}, nil
	case protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED:
		return terminalActionToolResult(call, actionResult, toolResultStatusRejected), nil
	case protocolv1alpha2.ActionStatus_ACTION_STATUS_FAILED:
		return terminalActionToolResult(call, actionResult, toolResultStatusFailed), nil
	case protocolv1alpha2.ActionStatus_ACTION_STATUS_CANCELLED:
		return terminalActionToolResult(call, actionResult, toolResultStatusCancelled), nil
	case protocolv1alpha2.ActionStatus_ACTION_STATUS_INTERRUPTED:
		return terminalActionToolResult(call, actionResult, toolResultStatusInterrupted), nil
	default:
		return model.ToolResult{}, fmt.Errorf("%s: %s", toolResultCodeNonTerminalActionState, actionResult.GetStatus().String())
	}
}

func toolResultFromActionResultWithDefaultRejectedCode(call model.ToolCall, actionResult *protocolv1alpha2.ActionResult, defaultRejectedCode string) (model.ToolResult, error) {
	if actionResult.GetStatus() != protocolv1alpha2.ActionStatus_ACTION_STATUS_REJECTED ||
		strings.TrimSpace(actionResult.GetError().GetCode()) != "" {
		return toolResultFromActionResult(call, actionResult)
	}
	result, err := toolResultFromActionResult(call, actionResult)
	if err != nil {
		return model.ToolResult{}, err
	}
	result.Code = defaultRejectedCode
	return result, nil
}

func terminalActionToolResult(call model.ToolCall, actionResult *protocolv1alpha2.ActionResult, status string) model.ToolResult {
	code := strings.TrimSpace(actionResult.GetError().GetCode())
	if code == "" {
		code = "action_" + status
	}
	return model.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Status:     status,
		Code:       code,
		Message:    truncateMessage(actionResult.GetError().GetMessage(), 120),
		Output:     actionResultOutput(actionResult),
	}
}

func actionResultOutput(actionResult *protocolv1alpha2.ActionResult) map[string]any {
	if actionResult.GetOutput() == nil {
		return nil
	}
	return actionResult.GetOutput().AsMap()
}

func fillPriorGroupSkipped(results []model.ToolResult, plan []plannedToolCall) {
	for _, item := range plan {
		results[item.index] = skippedToolResult(item.call, toolResultCodePriorGroupFailed, "prior group failed")
	}
}

func invalidToolResult(call model.ToolCall, code string, message string) model.ToolResult {
	return model.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Status:     toolResultStatusInvalid,
		Code:       code,
		Message:    truncateMessage(message, 120),
	}
}

func skippedToolResult(call model.ToolCall, code string, message string) model.ToolResult {
	return model.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Status:     toolResultStatusSkipped,
		Code:       code,
		Message:    truncateMessage(message, 120),
	}
}

func duplicateToolCallIDs(calls []model.ToolCall) map[string]bool {
	counts := make(map[string]int, len(calls))
	for _, call := range calls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			continue
		}
		counts[id]++
	}

	duplicates := make(map[string]bool)
	for id, count := range counts {
		if count > 1 {
			duplicates[id] = true
		}
	}
	return duplicates
}

func preferTechnicalError(current error, candidate error) error {
	if current == nil {
		return candidate
	}
	if errors.Is(current, context.Canceled) && !errors.Is(candidate, context.Canceled) {
		return candidate
	}
	return current
}

func truncateMessage(message string, maxRunes int) string {
	message = strings.TrimSpace(message)
	if maxRunes <= 0 || utf8.RuneCountInString(message) <= maxRunes {
		return message
	}

	runes := []rune(message)
	return string(runes[:maxRunes])
}
