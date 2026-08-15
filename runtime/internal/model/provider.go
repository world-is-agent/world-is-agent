package model

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"
)

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Provider 抽象一次模型生成调用。
//
// 不同厂商可以有不同 tool calling 协议，但进入 Agent Loop 前必须统一成 Response。
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDefinition
}

type Message struct {
	Role    Role
	Content string
}

type Role string

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema string
}

type Response struct {
	ToolCall ToolCall
}

type ToolCall struct {
	Name      string
	Arguments *structpb.Struct
}
