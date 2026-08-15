package deepseek

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

const defaultEndpoint = "https://api.deepseek.com/chat/completions"

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
		p.endpoint = strings.TrimRight(baseURL, "/") + "/chat/completions"
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

// Generate 调用 DeepSeek Chat Completions，并把 tool_call 转回 Runtime 统一模型。
func (p *Provider) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if p.apiKey == "" {
		return model.Response{}, errors.New("deepseek api key is empty")
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
		return model.Response{}, fmt.Errorf("deepseek response failed: %s", string(respBody))
	}

	return parseResponse(respBody)
}

func (p *Provider) buildRequest(req model.Request) ([]byte, error) {
	if len(req.Tools) == 0 {
		return nil, errors.New("deepseek provider requires at least one tool")
	}

	tools := make([]map[string]any, 0, len(req.Tools))
	for _, tool := range req.Tools {
		var schema map[string]any
		if err := json.Unmarshal([]byte(tool.InputSchema), &schema); err != nil {
			return nil, fmt.Errorf("invalid input schema for tool %q: %w", tool.Name, err)
		}

		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  schema,
			},
		})
	}

	payload := map[string]any{
		"model":    p.model,
		"messages": buildMessages(req),
		"tools":    tools,
		"stream":   false,
	}

	return json.Marshal(payload)
}

func buildMessages(req model.Request) []map[string]string {
	messages := make([]map[string]string, 0, len(req.Messages)+1)

	if req.System != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": req.System,
		})
	}

	for _, message := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		})
	}

	return messages
}

func parseResponse(data []byte) (model.Response, error) {
	var raw struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return model.Response{}, err
	}

	if raw.Error != nil {
		return model.Response{}, fmt.Errorf("deepseek error %s: %s", raw.Error.Code, raw.Error.Message)
	}

	for _, choice := range raw.Choices {
		for _, call := range choice.Message.ToolCalls {
			if call.Type != "" && call.Type != "function" {
				continue
			}
			if call.Function.Name == "" {
				return model.Response{}, errors.New("deepseek function_call name is empty")
			}

			var args map[string]any
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return model.Response{}, err
			}

			structArgs, err := structpb.NewStruct(args)
			if err != nil {
				return model.Response{}, err
			}

			return model.Response{
				ToolCall: model.ToolCall{
					Name:      call.Function.Name,
					Arguments: structArgs,
				},
			}, nil
		}
	}

	return model.Response{}, errors.New("deepseek response has no function tool_call")
}
