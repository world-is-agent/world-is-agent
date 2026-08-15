package openai

import (
	"encoding/json"
	"testing"

	"gameagent/runtime/internal/model"
)

func TestBuildRequestAddsAdditionalPropertiesForStrictToolSchema(t *testing.T) {
	provider := NewProvider("test-key", "gpt-5-mini")

	body, err := provider.buildRequest(model.Request{
		System: "You are controlling an NPC.",
		Messages: []model.Message{
			{
				Role:    model.RoleUser,
				Content: "Say hello as Linus.",
			},
		},
		Tools: []model.ToolDefinition{
			{
				Name:        "speak",
				Description: "Make the NPC say a short line of dialogue.",
				InputSchema: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}

	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", payload["tools"])
	}

	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool has unexpected shape: %#v", tools[0])
	}
	if got := tool["strict"]; got != true {
		t.Fatalf("strict = %v, want true", got)
	}

	parameters, ok := tool["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters has unexpected shape: %#v", tool["parameters"])
	}
	if got := parameters["additionalProperties"]; got != false {
		t.Fatalf("additionalProperties = %v, want false", got)
	}
}

func TestParseResponseReturnsToolCall(t *testing.T) {
	resp, err := parseResponse([]byte(`{
		"output": [
			{
				"type": "function_call",
				"name": "speak",
				"arguments": "{\"text\":\"Hello.\"}"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parseResponse failed: %v", err)
	}

	if resp.ToolCall.Name != "speak" {
		t.Fatalf("tool name = %q, want speak", resp.ToolCall.Name)
	}
	text := resp.ToolCall.Arguments.Fields["text"].GetStringValue()
	if text != "Hello." {
		t.Fatalf("text = %q, want Hello.", text)
	}
}
