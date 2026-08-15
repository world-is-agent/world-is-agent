package model

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"
)

// Provider 抽象一次模型生成调用。
//
// 不同厂商可以有不同 tool calling 协议，但进入 Agent Loop 前必须统一成 Response。
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Prompt string
	Tools  []ToolDefinition
}

type Response struct {
	ToolCall ToolCall
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema string
}

type ToolCall struct {
	Name      string
	Arguments *structpb.Struct
}

type FakeProvider struct{}

// NewProviderFromEnv 根据环境变量选择模型 Provider。
//
// MVP0 先固定返回 FakeProvider；后续再扩展 fake / openai 等 provider 选择。
func NewProviderFromEnv() Provider {
	return FakeProvider{}
}

// Generate 返回一个固定 speak ToolCall，用于验证 Runtime 到 Adapter 的闭环。
func (FakeProvider) Generate(ctx context.Context, req Request) (Response, error) {
	args, err := structpb.NewStruct(map[string]any{
		"text": "Hello from GameAgent Runtime by zlc",
	})

	if err != nil {
		return Response{}, err
	}

	return Response{
		ToolCall: ToolCall{
			Name:      "speak",
			Arguments: args,
		},
	}, nil
}
