package main

import (
	"context"
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
	"gameagent/runtime/internal/trace"

	"google.golang.org/grpc"
)

func main() {
	modelProvider, modelConfig, err := llm.NewProviderFromConfigFile(llm.ConfigPathFromEnv())
	if err != nil {
		log.Fatalf("load model provider failed: %v", err)
	}
	log.Printf("GameAgent model provider: %s model=%s", modelConfig.Provider, modelConfig.Model)

	toolRegistry := tool.NewRegistry()

	// 初始化 trace recorder。
	var traceRecorder trace.Recorder
	traceRecorder, err = trace.NewJSONLRecorder(
		// MVP0 trace path 依赖从 npcore/ 作为 cwd 启动；后续由 agent.json 的 trace.path 接管。
		"runtime/.local/traces.jsonl",
		trace.JSONLRecorderOptions{},
	)
	if err != nil {
		log.Printf("create trace recorder failed: %v, fallback to noop", err)
		traceRecorder = trace.NoopRecorder{}
	}
	defer traceRecorder.Close(context.Background())

	agentLoop := agent.NewLoop(modelProvider, toolRegistry, traceRecorder)
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
