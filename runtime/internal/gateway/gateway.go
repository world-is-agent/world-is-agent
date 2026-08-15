package gateway

import (
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/tool"
	"time"
)

type Server struct {
	protocolv1alpha1.UnimplementedGameAgentGatewayServer

	agentLoop *agent.Loop
	tools     *tool.Registry
}

func NewServer(agentLoop *agent.Loop, tools *tool.Registry) *Server {
	return &Server{
		agentLoop: agentLoop,
		tools:     tools,
	}
}

func (s *Server) Connect(stream protocolv1alpha1.GameAgentGateway_ConnectServer) error {
	firstMessage, err := stream.Recv()
	if err != nil {
		return err
	}

	hello := firstMessage.GetHello()
	if hello == nil {
		return fmt.Errorf("expected adapter hello as first message")
	}

	readyMessageID := newMessageID("runtime_ready")
	readyMessage := &protocolv1alpha1.RuntimeMessage{
		MessageId: readyMessageID,
		Payload: &protocolv1alpha1.RuntimeMessage_EnvironmentReady{
			EnvironmentReady: &protocolv1alpha1.EnvironmentReady{
				EnvironmentId:    hello.InstanceId,
				ServerTimeUnixMs: time.Now().UnixMilli(),
			},
		},
	}

	if err := stream.Send(readyMessage); err != nil {
		return err
	}

	capabilityRequestID := newMessageID("cap_req")
	capabilityRequestMessage := &protocolv1alpha1.RuntimeMessage{
		MessageId: capabilityRequestID,
		Payload: &protocolv1alpha1.RuntimeMessage_CapabilityRequest{
			CapabilityRequest: &protocolv1alpha1.CapabilityRequest{},
		},
	}

	if err := stream.Send(capabilityRequestMessage); err != nil {
		return err
	}

	capabilityMessage, err := stream.Recv()
	if err != nil {
		return err
	}

	capabilityList := capabilityMessage.GetCapabilities()
	if capabilityList == nil {
		return fmt.Errorf("expected capability list")
	}

	names := make([]string, 0, len(capabilityList.Capabilities))
	for _, capability := range capabilityList.Capabilities {
		if capability.Name == "" {
			continue
		}
		names = append(names, capability.Name)
	}

	s.tools.RegisterEnvironmentCapabilities(names)

	env := newStreamEnvironment(stream)
	eventCh := make(chan *protocolv1alpha1.GameEvent, 16)

	go func() {
		for event := range eventCh {
			if err := s.agentLoop.HandleEvent(stream.Context(), env, event); err != nil {
				fmt.Printf("agent loop failed: %v\n", err)
			}
		}
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			env.failAllPending(err)
			close(eventCh)
			return err
		}

		switch payload := msg.Payload.(type) {
		case *protocolv1alpha1.AdapterMessage_Event:
			if payload.Event == nil {
				continue
			}
			ack := &protocolv1alpha1.RuntimeMessage{
				MessageId:     newMessageID("event_ack"),
				CorrelationId: msg.MessageId,
				Payload: &protocolv1alpha1.RuntimeMessage_EventAck{
					EventAck: &protocolv1alpha1.EventAck{
						EventId: payload.Event.EventId,
						Status:  protocolv1alpha1.EventAckStatus_EVENT_ACK_STATUS_ACCEPTED,
					},
				},
			}

			if err := env.send(ack); err != nil {
				return err
			}

			eventCh <- payload.Event

		case *protocolv1alpha1.AdapterMessage_Observation:
			if payload.Observation == nil {
				continue
			}
			env.resolveObservation(msg.CorrelationId, payload.Observation)

		case *protocolv1alpha1.AdapterMessage_ActionResult:
			if payload.ActionResult == nil {
				continue
			}
			env.resolveActionResult(payload.ActionResult.ActionId, payload.ActionResult)
		}
	}

}
