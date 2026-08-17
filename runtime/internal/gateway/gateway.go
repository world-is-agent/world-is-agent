package gateway

import (
	"errors"
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/tool"
	"io"
	"time"
)

// Server 实现 Runtime 侧的 GameAgentGateway gRPC 服务。
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

// Connect 管理一条 Adapter stream 的完整生命周期。
//
// 它先完成 Environment Bootstrap，发现 Adapter 提供的 capabilities，
// 然后进入 recvLoop 持续分发 GameEvent / Observation / ActionResult。
func (s *Server) Connect(stream protocolv1alpha1.GameAgentGateway_ConnectServer) error {
	firstMessage, err := stream.Recv()
	if err != nil {
		return err
	}

	hello := firstMessage.GetHello()
	if hello == nil {
		return fmt.Errorf("expected adapter hello as first message")
	}

	// EnvironmentReady 只表示协议连接已经建立；Runtime 是否能执行
	// AgentRun，还要等 capability discovery 完成。
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

	// MVP0 先发现 environment-level capability；未来如果不同 entity
	// 能力不同，再把 CapabilityRequest 细化到 entity_id 维度。
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

	s.tools.RegisterEnvironmentCapabilities(capabilityList.Capabilities)

	env := newStreamEnvironment(stream)
	eventCh := make(chan *protocolv1alpha1.GameEvent, 16)

	// AgentRun 不能直接在 recvLoop 中执行：HandleEvent 会等待
	// Observation / ActionResult，而这些回复也必须由 recvLoop 接收。
	go func() {
		for event := range eventCh {
			if err := s.agentLoop.HandleEvent(stream.Context(), env, event); err != nil {
				fmt.Printf("agent loop failed: %v\n", err)
			}
		}
	}()

	// recvLoop 只负责接收和分发 AdapterMessage，避免被单次 AgentRun 阻塞。
	for {
		msg, err := stream.Recv()
		if err != nil {
			env.failAllPending(err)
			close(eventCh)
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		// Event 会启动新的 AgentRun；Observation / ActionResult 用来唤醒
		// 已经在 streamEnvironment 中等待的同步调用。
		switch payload := msg.Payload.(type) {
		case *protocolv1alpha1.AdapterMessage_Event:
			if payload.Event == nil {
				continue
			}
			// EventAck 表示 Runtime 已接收该 GameEvent；真正的 action
			// 执行结果会在后续 ActionResult 中体现。
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

		case *protocolv1alpha1.AdapterMessage_Error:
			if payload.Error == nil {
				continue
			}
			env.failObservation(msg.CorrelationId, fmt.Errorf("adapter error %s: %s", payload.Error.Code, payload.Error.Message))
		}
	}

}
