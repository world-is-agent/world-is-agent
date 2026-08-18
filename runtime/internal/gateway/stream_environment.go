package gateway

import (
	"context"
	"fmt"
	"sync"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
)

// streamEnvironment 将一条 gRPC Adapter stream 适配成 agent.Environment。
//
// Agent Loop 看到的是同步的 Observe / SubmitAction 调用，底层协议实际是
// bidirectional stream 上的异步 request / response。
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

// newStreamEnvironment 创建连接级环境状态。
//
// pendingObservations / pendingActions 必须属于单条 Connect stream，
// 避免未来多个 Adapter 连接之间互相唤醒错误的 waiter。
func newStreamEnvironment(stream protocolv1alpha1.GameAgentGateway_ConnectServer) *streamEnvironment {
	return &streamEnvironment{
		stream:              stream,
		pendingObservations: make(map[string]chan observeResult),
		pendingActions:      make(map[string]chan actionResult),
	}
}

// Observe 发送 ObserveRequest，并等待 recvLoop 通过 correlation_id 返回 Observation。
func (e *streamEnvironment) Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity id is empty")
	}

	messageID := newMessageID("observe")

	ch := make(chan observeResult, 1)

	// message_id 是这次 ObserveRequest 的协议编号；Adapter 回复时必须把它
	// 放到 correlation_id，recvLoop 才能找到正确的等待者。
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

// resolveObservation 唤醒正在等待指定 correlation_id 的 Observe 调用。
func (e *streamEnvironment) resolveObservation(correlationID string, observation *protocolv1alpha1.Observation) {
	e.pendingMu.Lock()
	ch := e.pendingObservations[correlationID]
	e.pendingMu.Unlock()

	if ch == nil {
		return
	}

	ch <- observeResult{observation: observation}
}

// failObservation 用 Adapter 返回的 Error 唤醒对应 Observe 调用。
func (e *streamEnvironment) failObservation(correlationID string, err error) {
	e.pendingMu.Lock()
	ch := e.pendingObservations[correlationID]
	e.pendingMu.Unlock()

	if ch == nil {
		return
	}

	ch <- observeResult{err: err}
}

// SubmitAction 发送 ActionRequest，并等待 Adapter 返回对应 action_id 的 ActionResult。
func (e *streamEnvironment) SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("action request is nil")
	}
	if req.ActionId == "" {
		return nil, fmt.Errorf("action id is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	messageID := newMessageID("action")

	ch := make(chan actionResult, 1)

	// ActionResult 自带 action_id，所以这里用业务动作 ID 匹配结果；
	// RuntimeMessage.message_id 只负责这条协议消息本身的身份。
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
		e.sendCancelActionBestEffort(req.ActionId, "action_timeout")
		return nil, ctx.Err()
	}
}

func (e *streamEnvironment) sendCancelActionBestEffort(actionID string, reason string) {
	msg := &protocolv1alpha1.RuntimeMessage{
		MessageId: newMessageID("cancel_action"),
		Payload: &protocolv1alpha1.RuntimeMessage_CancelAction{
			CancelAction: &protocolv1alpha1.CancelActionRequest{
				ActionId: actionID,
				Reason:   reason,
			},
		},
	}

	// Cancel 是 best-effort：action ctx 已经过期，但 cancel 仍要尽量通过连接级 stream 发出去。
	// 发送失败不改变 SubmitAction 已经超时的结果。
	_ = e.send(msg)
}

// resolveActionResult 唤醒正在等待指定 action_id 的 SubmitAction 调用。
func (e *streamEnvironment) resolveActionResult(actionID string, result *protocolv1alpha1.ActionResult) {
	e.pendingMu.Lock()
	ch := e.pendingActions[actionID]
	e.pendingMu.Unlock()

	if ch == nil {
		return
	}

	ch <- actionResult{result: result}
}

// send 串行化同一条 stream 上的 RuntimeMessage 写入。
//
// EventAck、Observe、SubmitAction 都可能从不同 goroutine 触发 Send，
// sendMu 保证这条 gRPC stream 只有一个 writer。
func (e *streamEnvironment) send(msg *protocolv1alpha1.RuntimeMessage) error {
	e.sendMu.Lock()
	defer e.sendMu.Unlock()

	return e.stream.Send(msg)
}

// failAllPending 在 Adapter stream 断开时唤醒仍在等待的 Observe / SubmitAction。
//
// 先在锁内取出 waiter，再在锁外发送错误，避免持锁写 channel 导致极端时序卡住。
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
