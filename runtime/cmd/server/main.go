package main

import (
	"log"
	"net"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/gateway"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"

	"google.golang.org/grpc"
)

func main() {
	modelProvider := model.NewProviderFromEnv()

	toolRegistry := tool.NewRegistry()

	agentLoop := agent.NewLoop(modelProvider, toolRegistry)
	gatewayServer := gateway.NewServer(agentLoop, toolRegistry)

	grpcServer := grpc.NewServer()
	protocolv1alpha1.RegisterGameAgentGatewayServer(grpcServer, gatewayServer)

	listener, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	log.Println("GameAgent Runtime listening on 127.0.0.1:50051")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("serve failed: %v", err)
	}

}
