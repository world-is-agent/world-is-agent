package model

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"
)

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

func NewProviderFromEnv() Provider {
	return FakeProvider{}
}

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
