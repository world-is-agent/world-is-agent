package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"google.golang.org/grpc/metadata"
)

func TestStreamEnvironmentSubmitActionRejectsNilRequest(t *testing.T) {
	env := newStreamEnvironment(nil)

	_, err := env.SubmitAction(context.Background(), nil)

	if err == nil {
		t.Fatal("expected error for nil action request")
	}
	if !strings.Contains(err.Error(), "action request is nil") {
		t.Fatalf("expected nil action request error, got %q", err.Error())
	}
}

func TestStreamEnvironmentSubmitActionRejectsEmptyActionID(t *testing.T) {
	env := newStreamEnvironment(nil)

	_, err := env.SubmitAction(context.Background(), &protocolv1alpha2.ActionRequest{})

	if err == nil {
		t.Fatal("expected error for empty action id")
	}
	if !strings.Contains(err.Error(), "action id is empty") {
		t.Fatalf("expected empty action id error, got %q", err.Error())
	}
}

func TestStreamEnvironmentSubmitActionDoesNotSendExpiredAction(t *testing.T) {
	stream := &captureStream{sent: make(chan *protocolv1alpha2.RuntimeMessage, 2)}
	env := newStreamEnvironment(stream)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := env.SubmitAction(ctx, &protocolv1alpha2.ActionRequest{
		ActionId:   "act_expired",
		EntityId:   "npc:Linus",
		Capability: "speak",
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	select {
	case msg := <-stream.sent:
		t.Fatalf("expected no runtime message for expired action, got %T", msg.Payload)
	default:
	}

	env.pendingMu.Lock()
	defer env.pendingMu.Unlock()
	if len(env.pendingActions) != 0 {
		t.Fatalf("expected no pending actions for expired action, got %d", len(env.pendingActions))
	}
}

func TestStreamEnvironmentSubmitActionSendsCancelOnContextDone(t *testing.T) {
	stream := &captureStream{sent: make(chan *protocolv1alpha2.RuntimeMessage, 2)}
	env := newStreamEnvironment(stream)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := env.SubmitAction(ctx, &protocolv1alpha2.ActionRequest{
			ActionId:   "act_1",
			EntityId:   "npc:Linus",
			Capability: "speak",
		})
		errCh <- err
	}()

	actionMsg := stream.recvSent(t)
	if actionMsg.GetAction() == nil {
		t.Fatalf("expected ActionRequest, got %T", actionMsg.Payload)
	}

	cancelMsg := stream.recvSent(t)
	cancelReq := cancelMsg.GetCancelAction()
	if cancelReq == nil {
		t.Fatalf("expected CancelActionRequest, got %T", cancelMsg.Payload)
	}
	if cancelReq.ActionId != "act_1" {
		t.Fatalf("expected cancel action_id act_1, got %q", cancelReq.ActionId)
	}
	if cancelReq.Reason != "action_timeout" {
		t.Fatalf("expected cancel reason action_timeout, got %q", cancelReq.Reason)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitAction did not return after context deadline")
	}
}

func TestStreamEnvironmentLateActionResultAfterTimeoutIsIgnored(t *testing.T) {
	stream := &captureStream{sent: make(chan *protocolv1alpha2.RuntimeMessage, 2)}
	env := newStreamEnvironment(stream)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := env.SubmitAction(ctx, &protocolv1alpha2.ActionRequest{
			ActionId:   "act_late",
			EntityId:   "npc:Linus",
			Capability: "speak",
		})
		errCh <- err
	}()

	actionMsg := stream.recvSent(t)
	if actionMsg.GetAction() == nil {
		t.Fatalf("expected ActionRequest, got %T", actionMsg.Payload)
	}
	cancelMsg := stream.recvSent(t)
	if cancelMsg.GetCancelAction() == nil {
		t.Fatalf("expected CancelActionRequest, got %T", cancelMsg.Payload)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitAction did not return after context deadline")
	}

	env.resolveActionResult("act_late", &protocolv1alpha2.ActionResult{
		ActionId: "act_late",
		Status:   protocolv1alpha2.ActionStatus_ACTION_STATUS_CANCELLED,
	})

	env.pendingMu.Lock()
	defer env.pendingMu.Unlock()
	if len(env.pendingActions) != 0 {
		t.Fatalf("expected no pending actions after timeout, got %d", len(env.pendingActions))
	}
}

func TestStreamEnvironmentFailObservationUnblocksObserve(t *testing.T) {
	stream := &captureStream{sent: make(chan *protocolv1alpha2.RuntimeMessage, 1)}
	env := newStreamEnvironment(stream)
	wantErr := fmt.Errorf("adapter observe failed")

	resultCh := make(chan error, 1)
	go func() {
		_, err := env.Observe(context.Background(), "world:test", "npc:Linus")
		resultCh <- err
	}()

	sent := stream.recvSent(t)
	observe := sent.GetObserve()
	if observe == nil {
		t.Fatalf("expected ObserveRequest, got %T", sent.Payload)
	}
	if observe.WorldId != "world:test" {
		t.Fatalf("observe world id = %q, want %q", observe.WorldId, "world:test")
	}
	if observe.EntityId != "npc:Linus" {
		t.Fatalf("observe entity id = %q, want %q", observe.EntityId, "npc:Linus")
	}

	env.failObservation(sent.MessageId, wantErr)

	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("expected observe error")
		}
		if !strings.Contains(err.Error(), wantErr.Error()) {
			t.Fatalf("expected %q, got %q", wantErr.Error(), err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("observe was not unblocked")
	}
}

func TestAdapterErrorExposesFailureReason(t *testing.T) {
	err := adapterError{
		code:    "world_mismatch",
		message: "current world changed",
	}

	if err.FailureReason() != "world_mismatch" {
		t.Fatalf("failure reason = %q, want world_mismatch", err.FailureReason())
	}
	if !strings.Contains(err.Error(), "world_mismatch") {
		t.Fatalf("error string = %q, want code included", err.Error())
	}
}

func TestStreamEnvironmentRejectsObservationScopeMismatch(t *testing.T) {
	stream := &captureStream{sent: make(chan *protocolv1alpha2.RuntimeMessage, 1)}
	env := newStreamEnvironment(stream)

	resultCh := make(chan error, 1)
	go func() {
		_, err := env.Observe(context.Background(), "world:test", "npc:Linus")
		resultCh <- err
	}()

	sent := stream.recvSent(t)
	env.resolveObservation(sent.MessageId, &protocolv1alpha2.Observation{
		EntityId: "npc:Linus",
		WorldId:  "world:other",
	})

	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("expected observation scope mismatch error")
		}
		if !strings.Contains(err.Error(), "observation_scope_mismatch") {
			t.Fatalf("expected observation_scope_mismatch, got %q", err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("observe was not unblocked")
	}
}

type captureStream struct {
	sent chan *protocolv1alpha2.RuntimeMessage
}

func (s *captureStream) Send(msg *protocolv1alpha2.RuntimeMessage) error {
	if s.sent == nil {
		s.sent = make(chan *protocolv1alpha2.RuntimeMessage, 1)
	}
	s.sent <- msg
	return nil
}

func (s *captureStream) Recv() (*protocolv1alpha2.AdapterMessage, error) {
	return nil, fmt.Errorf("recv not implemented")
}

func (s *captureStream) recvSent(t *testing.T) *protocolv1alpha2.RuntimeMessage {
	t.Helper()

	select {
	case msg := <-s.sent:
		return msg
	case <-time.After(time.Second):
		t.Fatal("runtime message was not sent")
		return nil
	}
}

func (s *captureStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *captureStream) SendHeader(metadata.MD) error {
	return nil
}

func (s *captureStream) SetTrailer(metadata.MD) {}

func (s *captureStream) Context() context.Context {
	return context.Background()
}

func (s *captureStream) SendMsg(any) error {
	return nil
}

func (s *captureStream) RecvMsg(any) error {
	return nil
}
