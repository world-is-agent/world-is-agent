package gateway

import (
	"context"
	"fmt"
	"sync"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
)

type streamEnvironment struct {
	stream protocolv1alpha1.GameAgentGateway_ConnectServer

	sendMu sync.Mutex

	pendingMu           sync.Mutex
	pendingObservations map[string]chan observeResult
	pendingActions      map[string]chan actionResult
}

type observeResult struct {
	observation *protocolv1alpha1.Observation
	err         error
}

type actionResult struct {
	result *protocolv1alpha1.ActionResult
	err    error
}

func newStreamEnvironment(stream protocolv1alpha1.GameAgentGateway_ConnectServer) *streamEnvironment {
	return &streamEnvironment{
		stream:              stream,
		pendingObservations: make(map[string]chan observeResult),
		pendingActions:      make(map[string]chan actionResult),
	}
}

func (e *streamEnvironment) Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity id is empty")
	}

	messageID := newMessageID("observe")

	ch := make(chan observeResult, 1)

	e.pendingMu.Lock()
	e.pendingObservations[messageID] = ch
	e.pendingMu.Unlock()

	defer func() {
		e.pendingMu.Lock()
		delete(e.pendingObservations, messageID)
		e.pendingMu.Unlock()
	}()

	msg := &protocolv1alpha1.RuntimeMessage{
		MessageId: messageID,
		Payload: &protocolv1alpha1.RuntimeMessage_Observe{
			Observe: &protocolv1alpha1.ObserveRequest{
				EntityId: entityID,
			},
		},
	}

	if err := e.send(msg); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		return result.observation, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *streamEnvironment) resolveObservation(correlationID string, observation *protocolv1alpha1.Observation) {
	e.pendingMu.Lock()
	ch := e.pendingObservations[correlationID]
	e.pendingMu.Unlock()

	if ch == nil {
		return
	}

	ch <- observeResult{observation: observation}
}

func (e *streamEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("action request is nil")
	}
	if req.ActionId == "" {
		return nil, fmt.Errorf("action id is empty")
	}

	messageID := newMessageID("action")

	ch := make(chan actionResult, 1)

	e.pendingMu.Lock()
	e.pendingActions[req.ActionId] = ch
	e.pendingMu.Unlock()

	defer func() {
		e.pendingMu.Lock()
		delete(e.pendingActions, req.ActionId)
		e.pendingMu.Unlock()
	}()

	msg := &protocolv1alpha1.RuntimeMessage{
		MessageId: messageID,
		Payload: &protocolv1alpha1.RuntimeMessage_Action{
			Action: req,
		},
	}

	if err := e.send(msg); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		return result.result, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *streamEnvironment) resolveActionResult(actionID string, result *protocolv1alpha1.ActionResult) {
	e.pendingMu.Lock()
	ch := e.pendingActions[actionID]
	e.pendingMu.Unlock()

	if ch == nil {
		return
	}

	ch <- actionResult{result: result}
}

func (e *streamEnvironment) send(msg *protocolv1alpha1.RuntimeMessage) error {
	e.sendMu.Lock()
	defer e.sendMu.Unlock()

	return e.stream.Send(msg)
}

func (e *streamEnvironment) failAllPending(err error) {
	e.pendingMu.Lock()

	observationChs := make([]chan observeResult, 0, len(e.pendingObservations))
	for id, ch := range e.pendingObservations {
		observationChs = append(observationChs, ch)
		delete(e.pendingObservations, id)
	}

	actionChs := make([]chan actionResult, 0, len(e.pendingActions))
	for id, ch := range e.pendingActions {
		actionChs = append(actionChs, ch)
		delete(e.pendingActions, id)
	}

	e.pendingMu.Unlock()

	for _, ch := range observationChs {
		select {
		case ch <- observeResult{err: err}:
		default:
		}
	}

	for _, ch := range actionChs {
		select {
		case ch <- actionResult{err: err}:
		default:
		}
	}
}
