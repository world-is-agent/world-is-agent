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

	readyMessage := &protocolv1alpha1.RuntimeMessage{
		MessageId: "runtime_ready_1",
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

	capabilityRequestID := "cap_req_1"
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

	return nil
}
