package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/types/known/structpb"
)

const defaultEndpoint = "https://api.openai.com/v1/responses"

var _ model.Provider = (*Provider)(nil)

type Provider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

type Option func(*Provider)

func WithBaseURL(baseURL string) Option {
	return func(p *Provider) {
		if baseURL == "" {
			return
		}
		p.endpoint = strings.TrimRight(baseURL, "/") + "/responses"
	}
}

func NewProvider(apiKey string, modelName string, opts ...Option) *Provider {
	provider := &Provider{
		apiKey:   apiKey,
		model:    modelName,
		endpoint: defaultEndpoint,
		client:   http.DefaultClient,
	}

	for _, opt := range opts {
		opt(provider)
	}

	return provider
}

func (p *Provider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if p.apiKey == "" {
		return model.Response{}, errors.New("openai api key is empty")
	}

	body, err := p.buildRequest(req)
	if err != nil {
		return model.Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return model.Response{}, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return model.Response{}, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return model.Response{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return model.Response{}, fmt.Errorf("openai response failed: %s", string(respBody))
	}

	return parseResponse(respBody)
}

func (p *Provider) buildRequest(req model.Request) ([]byte, error) {
	tools := make([]map[string]any, 0, len(req.Tools))

	for _, tool := range req.Tools {
		var schema map[string]any
		if err := json.Unmarshal([]byte(tool.InputSchema), &schema); err != nil {
			return nil, err
		}
		schema = normalizeStrictSchema(schema)

		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  schema,
			"strict":      true,
		})
	}

	payload := map[string]any{
		"model":       p.model,
		"input":       buildInput(req.Messages),
		"tools":       tools,
		"tool_choice": "required",
	}

	if req.System != "" {
		payload["instructions"] = req.System
	}

	return json.Marshal(payload)
}

func normalizeStrictSchema(schema map[string]any) map[string]any {
	normalized := make(map[string]any, len(schema)+1)
	for key, value := range schema {
		normalized[key] = value
	}

	if normalized["type"] == "object" {
		if _, exists := normalized["additionalProperties"]; !exists {
			normalized["additionalProperties"] = false
		}
	}

	return normalized
}

func parseResponse(data []byte) (model.Response, error) {
	var raw struct {
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return model.Response{}, err
	}

	for _, item := range raw.Output {
		if item.Type != "function_call" {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
			return model.Response{}, err
		}

		structArgs, err := structpb.NewStruct(args)
		if err != nil {
			return model.Response{}, err
		}

		return model.Response{
			ToolCall: model.ToolCall{
				Name:      item.Name,
				Arguments: structArgs,
			},
		}, nil
	}

	return model.Response{}, fmt.Errorf("openai response has no function_call")
}

func buildInput(messages []model.Message) []map[string]string {
	input := make([]map[string]string, 0, len(messages))

	for _, message := range messages {
		input = append(input, map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		})
	}

	return input
}
