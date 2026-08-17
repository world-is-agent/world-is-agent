package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
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
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{})
	protocolv1alpha1.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

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

	client := protocolv1alpha1.NewGameAgentGatewayClient(conn)
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
	if got := ready.GetEnvironmentReady(); got == nil || got.EnvironmentId != "env:test" {
		t.Fatalf("environment ready = %+v, want environment env:test", got)
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
	if ack.Status != protocolv1alpha1.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED {
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
	loop := agent.NewLoop(fake.NewProvider(), registry, trace.NoopRecorder{})
	protocolv1alpha1.RegisterGameAgentGatewayServer(grpcServer, NewServer(loop, registry))

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

	client := protocolv1alpha1.NewGameAgentGatewayClient(conn)
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
	if got := ready.GetEnvironmentReady(); got == nil || got.EnvironmentId != "env:test" {
		t.Fatalf("environment ready = %+v, want environment env:test", got)
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

func timeline(t *testing.T, step string) {
	t.Helper()
	t.Logf("[timeline] %s", step)
}

func recvRuntimeMessage(t *testing.T, stream protocolv1alpha1.GameAgentGateway_ConnectClient) *protocolv1alpha1.RuntimeMessage {
	t.Helper()

	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv runtime message: %v", err)
	}
	return msg
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

func adapterHelloMessage() *protocolv1alpha1.AdapterMessage {
	return &protocolv1alpha1.AdapterMessage{
		MessageId: "hello_msg_1",
		Payload: &protocolv1alpha1.AdapterMessage_Hello{
			Hello: &protocolv1alpha1.AdapterHello{
				AdapterId:       "fake-adapter",
				AdapterVersion:  "0.1.0",
				ProtocolVersion: "v1alpha1",
				GameId:          "fake-game",
				GameVersion:     "0.1.0",
				InstanceId:      "env:test",
				SaveId:          "save:test",
			},
		},
	}
}

func capabilityListMessage(correlationID string) *protocolv1alpha1.AdapterMessage {
	return &protocolv1alpha1.AdapterMessage{
		MessageId:     "capabilities_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha1.AdapterMessage_Capabilities{
			Capabilities: &protocolv1alpha1.CapabilityList{
				Capabilities: []*protocolv1alpha1.Capability{
					{
						Name:            "speak",
						Version:         "0.1.0",
						Description:     "Make the NPC speak.",
						InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
						ExecutionMode:   protocolv1alpha1.ExecutionMode_EXECUTION_MODE_SYNC,
					},
				},
				Revision: 1,
			},
		},
	}
}

func capabilityListWithEmoteMessage(correlationID string) *protocolv1alpha1.AdapterMessage {
	return &protocolv1alpha1.AdapterMessage{
		MessageId:     "capabilities_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha1.AdapterMessage_Capabilities{
			Capabilities: &protocolv1alpha1.CapabilityList{
				Capabilities: []*protocolv1alpha1.Capability{
					{
						Name:            "speak",
						Version:         "0.1.0",
						Description:     "Make the NPC speak.",
						InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
						ExecutionMode:   protocolv1alpha1.ExecutionMode_EXECUTION_MODE_SYNC,
					},
					{
						Name:            "emote",
						Version:         "0.1.0",
						Description:     "Make the NPC play an emote bubble.",
						InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string","enum":["happy","sad","surprised","neutral"]}},"required":["emote"],"additionalProperties":false}`,
						ExecutionMode:   protocolv1alpha1.ExecutionMode_EXECUTION_MODE_SYNC,
					},
				},
				Revision: 1,
			},
		},
	}
}

func npcInteractionEventMessage() *protocolv1alpha1.AdapterMessage {
	return &protocolv1alpha1.AdapterMessage{
		MessageId: "event_msg_1",
		Payload: &protocolv1alpha1.AdapterMessage_Event{
			Event: &protocolv1alpha1.GameEvent{
				EventId:   "event_1",
				EventType: "player_interacted_with_npc",
				Entities: []*protocolv1alpha1.EntityRef{
					{
						EntityId:    "player:local",
						EntityType:  "player",
						DisplayName: "Player",
					},
					{
						EntityId:    "npc:Linus",
						EntityType:  "npc",
						DisplayName: "Linus",
					},
				},
				Sequence: 1,
			},
		},
	}
}

func observationMessage(correlationID string) *protocolv1alpha1.AdapterMessage {
	state, err := structpb.NewStruct(map[string]any{
		"npc_name": "Linus",
		"location": "Mountain",
		"weather":  "sunny",
	})
	if err != nil {
		panic(err)
	}

	return &protocolv1alpha1.AdapterMessage{
		MessageId:     "observation_msg_1",
		CorrelationId: correlationID,
		Payload: &protocolv1alpha1.AdapterMessage_Observation{
			Observation: &protocolv1alpha1.Observation{
				EntityId: "npc:Linus",
				Revision: 1,
				State:    state,
			},
		},
	}
}

func actionResultMessage(actionID string) *protocolv1alpha1.AdapterMessage {
	return &protocolv1alpha1.AdapterMessage{
		MessageId: "action_result_msg_1",
		Payload: &protocolv1alpha1.AdapterMessage_ActionResult{
			ActionResult: &protocolv1alpha1.ActionResult{
				ActionId: actionID,
				Status:   protocolv1alpha1.ActionStatus_ACTION_STATUS_SUCCEEDED,
			},
		},
	}
}
