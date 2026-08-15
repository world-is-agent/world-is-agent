package fake

import (
	"context"

	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/types/known/structpb"
)

var _ model.Provider = (*Provider)(nil)

type Provider struct{}

func NewProvider() Provider {
	return Provider{}
}

// Generate 返回一个固定 speak ToolCall，用于验证 Runtime 到 Adapter 的闭环。
func (Provider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	args, err := structpb.NewStruct(map[string]any{
		"text": "Hello from GameAgent Runtime by zlc",
	})

	if err != nil {
		return model.Response{}, err
	}

	return model.Response{
		ToolCall: model.ToolCall{
			Name:      "speak",
			Arguments: args,
		},
	}, nil
}
