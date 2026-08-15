package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/agent"
	"gameagent/runtime/internal/gateway"
	"gameagent/runtime/internal/llm"
	"gameagent/runtime/internal/tool"

	"google.golang.org/grpc"
)

func main() {
	modelProvider, modelConfig, err := llm.NewProviderFromConfigFile(llm.ConfigPathFromEnv())
	if err != nil {
		log.Fatalf("load model provider failed: %v", err)
	}
	log.Printf("GameAgent model provider: %s model=%s", modelConfig.Provider, modelConfig.Model)

	toolRegistry := tool.NewRegistry()

	agentLoop := agent.NewLoop(modelProvider, toolRegistry)
	gatewayServer := gateway.NewServer(agentLoop, toolRegistry)

	grpcServer := grpc.NewServer()
	protocolv1alpha1.RegisterGameAgentGatewayServer(grpcServer, gatewayServer)

	listener, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	go func() {
		log.Println("GameAgent Runtime listening on 127.0.0.1:50051")
		if err := grpcServer.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			log.Printf("serve stopped: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop

	log.Println("shutting down GameAgent Runtime")
	grpcServer.GracefulStop()
}
