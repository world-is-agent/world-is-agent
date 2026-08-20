package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/llm/fake"
	"gameagent/runtime/internal/tool"
	"gameagent/runtime/internal/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestConnectRunsOneTurnWithFakeAdapter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	timeline(t, "adapter -> AdapterHello")
	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	timeline(t, "runtime -> EnvironmentReady")
	ready := recvRuntimeMessage(t, stream)
	if got := ready.GetEnvironmentReady(); got == nil || got.SessionId != "session:test" {
		t.Fatalf("environment ready = %+v, want environment session:test", got)
	}

	timeline(t, "runtime -> CapabilityRequest")
	capabilityRequest := recvRuntimeMessage(t, stream)
	if capabilityRequest.GetCapabilityRequest() == nil {
		t.Fatalf("expected capability request, got %+v", capabilityRequest.Payload)
	}

	timeline(t, "adapter -> CapabilityList(speak)")
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	timeline(t, "adapter -> GameEvent(player_interacted_with_npc)")
	if err := stream.Send(npcInteractionEventMessage()); err != nil {
		t.Fatalf("send game event: %v", err)
	}

	timeline(t, "runtime -> EventAck(ACCEPTED)")
	eventAck := recvRuntimeMessage(t, stream)
	ack := eventAck.GetEventAck()
	if ack == nil {
		t.Fatalf("expected event ack, got %+v", eventAck.Payload)
	}
	if eventAck.CorrelationId != "event_msg_1" {
		t.Fatalf("event ack correlation id = %q, want %q", eventAck.CorrelationId, "event_msg_1")
	}
	if ack.EventId != "event_1" {
		t.Fatalf("event ack id = %q, want %q", ack.EventId, "event_1")
	}
	if ack.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("event ack status = %v, want accepted", ack.Status)
	}

	timeline(t, "runtime -> ObserveRequest(npc:Linus)")
	observeRequest := recvRuntimeMessage(t, stream)
	observe := observeRequest.GetObserve()
	if observe == nil {
		t.Fatalf("expected observe request, got %+v", observeRequest.Payload)
	}
	if observe.EntityId != "npc:Linus" {
		t.Fatalf("observe entity id = %q, want %q", observe.EntityId, "npc:Linus")
	}
	if observe.WorldId != "world:test" {
		t.Fatalf("observe world id = %q, want %q", observe.WorldId, "world:test")
	}

	timeline(t, "adapter -> Observation(npc:Linus)")
	if err := stream.Send(observationMessage(observeRequest.MessageId)); err != nil {
		t.Fatalf("send observation: %v", err)
	}

	timeline(t, "runtime -> ActionRequest(speak)")
	actionRequest := recvRuntimeMessage(t, stream)
	action := actionRequest.GetAction()
	if action == nil {
		t.Fatalf("expected action request, got %+v", actionRequest.Payload)
	}
	if action.EntityId != "npc:Linus" {
		t.Fatalf("action entity id = %q, want %q", action.EntityId, "npc:Linus")
	}
	if action.WorldId != "world:test" {
		t.Fatalf("action world id = %q, want %q", action.WorldId, "world:test")
	}
	if action.Capability != "speak" {
		t.Fatalf("action capability = %q, want %q", action.Capability, "speak")
	}
	if action.ActionId == "" {
		t.Fatal("expected action id to be set")
	}
	text := action.Arguments.GetFields()["text"].GetStringValue()
	if text == "" {
		t.Fatal("expected speak action text to be set")
	}

	timeline(t, "adapter -> ActionResult(SUCCEEDED)")
	if err := stream.Send(actionResultMessage(action.ActionId)); err != nil {
		t.Fatalf("send action result: %v", err)
	}
	timeline(t, "adapter -> CloseSend")
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}

	timeline(t, "runtime -> EOF")
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("final recv error = %v, want EOF", err)
	}
}

func TestConnectForwardsDynamicEmoteToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	timeline(t, "adapter -> AdapterHello")
	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	timeline(t, "runtime -> EnvironmentReady")
	ready := recvRuntimeMessage(t, stream)
	if got := ready.GetEnvironmentReady(); got == nil || got.SessionId != "session:test" {
		t.Fatalf("environment ready = %+v, want environment session:test", got)
	}

	timeline(t, "runtime -> CapabilityRequest")
	capabilityRequest := recvRuntimeMessage(t, stream)
	if capabilityRequest.GetCapabilityRequest() == nil {
		t.Fatalf("expected capability request, got %+v", capabilityRequest.Payload)
	}

	timeline(t, "adapter -> CapabilityList(speak, emote)")
	if err := stream.Send(capabilityListWithEmoteMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "emote")

	timeline(t, "adapter -> GameEvent(player_interacted_with_npc)")
	if err := stream.Send(npcInteractionEventMessage()); err != nil {
		t.Fatalf("send game event: %v", err)
	}

	timeline(t, "runtime -> EventAck(ACCEPTED)")
	eventAck := recvRuntimeMessage(t, stream)
	if ack := eventAck.GetEventAck(); ack == nil || ack.EventId != "event_1" {
		t.Fatalf("expected event ack for event_1, got %+v", eventAck.Payload)
	}

	timeline(t, "runtime -> ObserveRequest(npc:Linus)")
	observeRequest := recvRuntimeMessage(t, stream)
	observe := observeRequest.GetObserve()
	if observe == nil {
		t.Fatalf("expected observe request, got %+v", observeRequest.Payload)
	}
	if observe.EntityId != "npc:Linus" {
		t.Fatalf("observe entity id = %q, want %q", observe.EntityId, "npc:Linus")
	}
	if observe.WorldId != "world:test" {
		t.Fatalf("observe world id = %q, want %q", observe.WorldId, "world:test")
	}

	timeline(t, "adapter -> Observation(npc:Linus)")
	if err := stream.Send(observationMessage(observeRequest.MessageId)); err != nil {
		t.Fatalf("send observation: %v", err)
	}

	timeline(t, "runtime -> ActionRequest(emote)")
	actionRequest := recvRuntimeMessage(t, stream)
	action := actionRequest.GetAction()
	if action == nil {
		t.Fatalf("expected action request, got %+v", actionRequest.Payload)
	}
	if action.EntityId != "npc:Linus" {
		t.Fatalf("action entity id = %q, want %q", action.EntityId, "npc:Linus")
	}
	if action.WorldId != "world:test" {
		t.Fatalf("action world id = %q, want %q", action.WorldId, "world:test")
	}
	if action.Capability != "emote" {
		t.Fatalf("action capability = %q, want %q", action.Capability, "emote")
	}
	if got := action.Arguments.GetFields()["emote"].GetStringValue(); got != "happy" {
		t.Fatalf("action emote = %q, want %q", got, "happy")
	}
	if action.ActionId == "" {
		t.Fatal("expected action id to be set")
	}

	timeline(t, "adapter -> ActionResult(SUCCEEDED)")
	if err := stream.Send(actionResultMessage(action.ActionId)); err != nil {
		t.Fatalf("send action result: %v", err)
	}
	timeline(t, "adapter -> CloseSend")
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}

	timeline(t, "runtime -> EOF")
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("final recv error = %v, want EOF", err)
	}
}

func TestConnectRejectsGameEventWhenEventQueueIsFull(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	_ = recvRuntimeMessage(t, stream)
	capabilityRequest := recvRuntimeMessage(t, stream)
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	const eventCount = 25
	for i := 0; i < eventCount; i++ {
		if err := stream.Send(npcInteractionEventMessageWithIDs(i)); err != nil {
			t.Fatalf("send game event %d: %v", i, err)
		}
	}

	ackCount := 0
	rejectedCount := 0
	for ackCount < eventCount {
		msg := recvRuntimeMessage(t, stream)
		ack := msg.GetEventAck()
		if ack == nil {
			continue
		}

		ackCount++
		if ack.Status == protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_REJECTED {
			rejectedCount++
			if ack.Error == nil || ack.Error.Code != "session_queue_full" {
				t.Fatalf("rejected ack error = %+v, want session_queue_full", ack.Error)
			}
		}
	}

	if rejectedCount == 0 {
		t.Fatal("expected at least one rejected event ack when event queue is full")
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}

func TestConnectRoutesDifferentNPCsToIndependentLanes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	_ = recvRuntimeMessage(t, stream)
	capabilityRequest := recvRuntimeMessage(t, stream)
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	if err := stream.Send(npcInteractionEventMessageForNPC(1, "npc:Linus", "Linus")); err != nil {
		t.Fatalf("send linus event: %v", err)
	}
	linusAck := recvRuntimeMessage(t, stream).GetEventAck()
	if linusAck == nil || linusAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("linus ack = %+v, want accepted", linusAck)
	}
	linusObserve := recvRuntimeMessage(t, stream).GetObserve()
	if linusObserve == nil || linusObserve.EntityId != "npc:Linus" {
		t.Fatalf("linus observe = %+v, want npc:Linus", linusObserve)
	}

	if err := stream.Send(npcInteractionEventMessageForNPC(2, "npc:Robin", "Robin")); err != nil {
		t.Fatalf("send robin event: %v", err)
	}
	robinAck := recvRuntimeMessage(t, stream).GetEventAck()
	if robinAck == nil || robinAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("robin ack = %+v, want accepted", robinAck)
	}
	robinObserve := recvRuntimeMessageWithin(t, stream, 300*time.Millisecond).GetObserve()
	if robinObserve == nil || robinObserve.EntityId != "npc:Robin" {
		t.Fatalf("robin observe = %+v, want npc:Robin", robinObserve)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}

func TestConnectSerializesEventsForSameNPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	_ = recvRuntimeMessage(t, stream)
	capabilityRequest := recvRuntimeMessage(t, stream)
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	if err := stream.Send(npcInteractionEventMessageWithIDs(1)); err != nil {
		t.Fatalf("send first event: %v", err)
	}
	firstAck := recvRuntimeMessage(t, stream).GetEventAck()
	if firstAck == nil || firstAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("first ack = %+v, want accepted", firstAck)
	}
	firstObserveMessage := recvRuntimeMessage(t, stream)
	firstObserve := firstObserveMessage.GetObserve()
	if firstObserve == nil || firstObserve.EntityId != "npc:Linus" {
		t.Fatalf("first observe = %+v, want npc:Linus", firstObserve)
	}

	if err := stream.Send(npcInteractionEventMessageWithIDs(2)); err != nil {
		t.Fatalf("send second event: %v", err)
	}
	secondAck := recvRuntimeMessage(t, stream).GetEventAck()
	if secondAck == nil || secondAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("second ack = %+v, want accepted", secondAck)
	}

	if err := stream.Send(npcInteractionEventMessageWithIDs(3)); err != nil {
		t.Fatalf("send third event: %v", err)
	}
	thirdAck := recvRuntimeMessage(t, stream).GetEventAck()
	if thirdAck == nil || thirdAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_REJECTED {
		t.Fatalf("third ack = %+v, want rejected", thirdAck)
	}
	if thirdAck.Error == nil || thirdAck.Error.Code != "session_queue_full" {
		t.Fatalf("third ack error = %+v, want session_queue_full", thirdAck.Error)
	}

	if err := stream.Send(observationMessage(firstObserveMessage.MessageId)); err != nil {
		t.Fatalf("send first observation: %v", err)
	}
	firstAction := recvRuntimeMessage(t, stream).GetAction()
	if firstAction == nil {
		t.Fatal("expected first action")
	}
	if err := stream.Send(actionResultMessage(firstAction.ActionId)); err != nil {
		t.Fatalf("send first action result: %v", err)
	}

	secondObserve := recvRuntimeMessageWithin(t, stream, 300*time.Millisecond).GetObserve()
	if secondObserve == nil || secondObserve.EntityId != "npc:Linus" {
		t.Fatalf("second observe = %+v, want npc:Linus", secondObserve)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}

func TestConnectDrainsQueuedEventOnDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	_ = recvRuntimeMessage(t, stream)
	capabilityRequest := recvRuntimeMessage(t, stream)
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	if err := stream.Send(npcInteractionEventMessageWithIDs(1)); err != nil {
		t.Fatalf("send first event: %v", err)
	}
	firstAck := recvRuntimeMessage(t, stream).GetEventAck()
	if firstAck == nil || firstAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("first ack = %+v, want accepted", firstAck)
	}
	firstObserve := recvRuntimeMessage(t, stream).GetObserve()
	if firstObserve == nil || firstObserve.EntityId != "npc:Linus" {
		t.Fatalf("first observe = %+v, want npc:Linus", firstObserve)
	}

	if err := stream.Send(npcInteractionEventMessageWithIDs(2)); err != nil {
		t.Fatalf("send queued event: %v", err)
	}
	secondAck := recvRuntimeMessage(t, stream).GetEventAck()
	if secondAck == nil || secondAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("second ack = %+v, want accepted", secondAck)
	}

	logs := captureStandardLog(t)
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}

	msg, err := stream.Recv()
	if err == nil {
		t.Fatalf("received runtime message after disconnect: %+v", msg.Payload)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("final recv error = %v, want EOF", err)
	}

	waitForLogContains(t, logs, `abort queued game event "event_2": connection_closed`)
}

func TestConnectReturnsDuplicateAckForRepeatedEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	registry := tool.NewRegistry()
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{}, agent.DefaultConfig())
	protocolv1alpha2.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		select {
		case <-serverErrCh:
		case <-time.After(time.Second):
			t.Fatal("gRPC server did not stop")
		}
	})

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	defer conn.Close()

	client := protocolv1alpha2.NewGameAgentGatewayClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := stream.Send(adapterHelloMessage()); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	_ = recvRuntimeMessage(t, stream)
	capabilityRequest := recvRuntimeMessage(t, stream)
	if err := stream.Send(capabilityListMessage(capabilityRequest.MessageId)); err != nil {
		t.Fatalf("send capability list: %v", err)
	}
	waitForTool(t, registry, "speak")

	first := npcInteractionEventMessageWithIDs(1)
	if err := stream.Send(first); err != nil {
		t.Fatalf("send first event: %v", err)
	}
	firstAck := recvRuntimeMessage(t, stream).GetEventAck()
	if firstAck == nil || firstAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
		t.Fatalf("first ack = %+v, want accepted", firstAck)
	}
	if observe := recvRuntimeMessage(t, stream).GetObserve(); observe == nil {
		t.Fatal("expected first event to start observe")
	}

	duplicate := npcInteractionEventMessageWithIDs(2)
	duplicate.GetEvent().EventId = first.GetEvent().EventId
	if err := stream.Send(duplicate); err != nil {
		t.Fatalf("send duplicate event: %v", err)
	}
	duplicateAck := recvRuntimeMessage(t, stream).GetEventAck()
	if duplicateAck == nil {
		t.Fatal("expected duplicate event ack")
	}
	if duplicateAck.EventId != first.GetEvent().EventId {
		t.Fatalf("duplicate ack event id = %q, want %q", duplicateAck.EventId, first.GetEvent().EventId)
	}
	if duplicateAck.Status != protocolv1alpha2.EventAckStatus_EVENT_ACK_STATUS_DUPLICATE {
		t.Fatalf("duplicate ack status = %v, want duplicate", duplicateAck.Status)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}

type capturedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *capturedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *capturedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func captureStandardLog(t *testing.T) *capturedLog {
	t.Helper()

	captured := &capturedLog{}
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(captured)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return captured
}

func waitForLogContains(t *testing.T, logs *capturedLog, want string) {
	t.Helper()

	deadline := time.After(time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if strings.Contains(logs.String(), want) {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("log output %q does not contain %q", logs.String(), want)
		case <-tick.C:
		}
	}
}

func timeline(t *testing.T, step string) {
	t.Helper()
	t.Logf("[timeline] %s", step)
}

func recvRuntimeMessage(t *testing.T, stream protocolv1alpha2.GameAgentGateway_ConnectClient) *protocolv1alpha2.RuntimeMessage {
	t.Helper()

	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv runtime message: %v", err)
	}
	return msg
}

func recvRuntimeMessageWithin(t *testing.T, stream protocolv1alpha2.GameAgentGateway_ConnectClient, timeout time.Duration) *protocolv1alpha2.RuntimeMessage {
	t.Helper()

	type recvResult struct {
		msg *protocolv1alpha2.RuntimeMessage
		err error
	}
	ch := make(chan recvResult, 1)
	go func() {
		msg, err := stream.Recv()
		ch <- recvResult{msg: msg, err: err}
	}()

	select {
	case result := <-ch:
		if result.err != nil {
			t.Fatalf("recv runtime message: %v", result.err)
		}
		return result.msg
	case <-time.After(timeout):
		t.Fatalf("runtime message was not received within %s", timeout)
		return nil
	}
}

func waitForTool(t *testing.T, registry *tool.Registry, name string) {
	t.Helper()

	deadline := time.After(time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if registry.HasTool(name) {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected %q capability to be registered", name)
		case <-tick.C:
		}
	}
}

func adapterHelloMessage() *protocolv1alpha2.AdapterMessage {
	return &protocolv1alpha2.AdapterMessage{
		MessageId: "hello_msg_1",
		Payload: &protocolv1alpha2.AdapterMessage_Hello{
			Hello: &protocolv1alpha2.AdapterHello{
				AdapterId:       "fake-adapter",
				AdapterVersion:  "0.1.0",
				ProtocolVersion: "v1alpha2",
				GameId:          "fake-game",
				GameVersion:     "0.1.0",
				SessionId:       "session:test",
			},
		},
	}
}

func capabilityListMessage(correlationID string) *protocolv1alpha2.AdapterMessage {
	return &protocolv1alpha2.AdapterMessage{
		MessageId:     "capabilities_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha2.AdapterMessage_Capabilities{
			Capabilities: &protocolv1alpha2.CapabilityList{
				Capabilities: []*protocolv1alpha2.Capability{
					{
						Name:            "speak",
						Version:         "0.1.0",
						Description:     "Make the NPC speak.",
						InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
						ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
					},
				},
				Revision: 1,
			},
		},
	}
}

func capabilityListWithEmoteMessage(correlationID string) *protocolv1alpha2.AdapterMessage {
	return &protocolv1alpha2.AdapterMessage{
		MessageId:     "capabilities_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha2.AdapterMessage_Capabilities{
			Capabilities: &protocolv1alpha2.CapabilityList{
				Capabilities: []*protocolv1alpha2.Capability{
					{
						Name:            "speak",
						Version:         "0.1.0",
						Description:     "Make the NPC speak.",
						InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
						ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
					},
					{
						Name:            "emote",
						Version:         "0.1.0",
						Description:     "Make the NPC play an emote bubble.",
						InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string","enum":["happy","sad","surprised","neutral"]}},"required":["emote"],"additionalProperties":false}`,
						ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
					},
				},
				Revision: 1,
			},
		},
	}
}

func npcInteractionEventMessage() *protocolv1alpha2.AdapterMessage {
	return npcInteractionEventMessageWithIDs(1)
}

func npcInteractionEventMessageWithIDs(index int) *protocolv1alpha2.AdapterMessage {
	return npcInteractionEventMessageForNPC(index, "npc:Linus", "Linus")
}

func npcInteractionEventMessageForNPC(index int, entityID string, displayName string) *protocolv1alpha2.AdapterMessage {
	return &protocolv1alpha2.AdapterMessage{
		MessageId: "event_msg_" + strconv.Itoa(index),
		Payload: &protocolv1alpha2.AdapterMessage_Event{
			Event: &protocolv1alpha2.GameEvent{
				EventId:        "event_" + strconv.Itoa(index),
				EventType:      "player_interacted_with_npc",
				WorldId:        "world:test",
				TargetEntityId: entityID,
				Entities: []*protocolv1alpha2.EntityRef{
					{
						EntityId:    "player:local",
						EntityType:  "player",
						DisplayName: "Player",
					},
					{
						EntityId:    entityID,
						EntityType:  "npc",
						DisplayName: displayName,
					},
				},
				Sequence: 1,
			},
		},
	}
}

func observationMessage(correlationID string) *protocolv1alpha2.AdapterMessage {
	state, err := structpb.NewStruct(map[string]any{
		"npc_name": "Linus",
		"location": "Mountain",
		"weather":  "sunny",
	})
	if err != nil {
		panic(err)
	}

	return &protocolv1alpha2.AdapterMessage{
		MessageId:     "observation_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha2.AdapterMessage_Observation{
			Observation: &protocolv1alpha2.Observation{
				EntityId: "npc:Linus",
				WorldId:  "world:test",
				Revision: 1,
				State:    state,
			},
		},
	}
}

func actionResultMessage(actionID string) *protocolv1alpha2.AdapterMessage {
	return &protocolv1alpha2.AdapterMessage{
		MessageId: "action_result_msg_1",
		Payload: &protocolv1alpha2.AdapterMessage_ActionResult{
			ActionResult: &protocolv1alpha2.ActionResult{
				ActionId: actionID,
				Status:   protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED,
			},
		},
	}
}
