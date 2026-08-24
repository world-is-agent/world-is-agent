package deepseek

import (
	"encoding/json"
	"testing"

	"gameagent/runtime/internal/model"
)

func TestBuildRequestUsesDeepSeekChatCompletionsShape(t *testing.T) {
	provider := NewProvider("test-key", "deepseek-v4-flash")

	body, err := provider.buildRequest(model.Request{
		System: "You are controlling an NPC in a game.",
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

	if got := payload["model"]; got != "deepseek-v4-flash" {
		t.Fatalf("model = %v, want deepseek-v4-flash", got)
	}
	if _, exists := payload["tool_choice"]; exists {
		t.Fatal("tool_choice should be omitted for DeepSeek thinking models")
	}

	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want system and user messages", payload["messages"])
	}

	systemMessage, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("system message has unexpected shape: %#v", messages[0])
	}
	if got := systemMessage["role"]; got != "system" {
		t.Fatalf("system message role = %v, want system", got)
	}

	userMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("user message has unexpected shape: %#v", messages[1])
	}
	if got := userMessage["role"]; got != "user" {
		t.Fatalf("user message role = %v, want user", got)
	}

	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", payload["tools"])
	}

	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool has unexpected shape: %#v", tools[0])
	}
	if got := tool["type"]; got != "function" {
		t.Fatalf("tool.type = %v, want function", got)
	}

	function, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool.function has unexpected shape: %#v", tool["function"])
	}
	if got := function["name"]; got != "speak" {
		t.Fatalf("function.name = %v, want speak", got)
	}
}

func TestParseResponseReturnsModelDecisionToolCalls(t *testing.T) {
	resp, err := parseResponse([]byte(`{
		"choices": [
			{
				"message": {
					"tool_calls": [
						{
							"id": "native_call_1",
							"type": "function",
							"function": {
								"name": "speak",
								"arguments": "{\"text\":\"Hi, friend.\"}"
							}
						},
						{
							"type": "function",
							"function": {
								"name": "emote",
								"arguments": "{\"emote\":\"happy\"}"
							}
						}
					]
				}
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parseResponse failed: %v", err)
	}

	if resp.Decision.Control.Kind != model.ControlContinue {
		t.Fatalf("control = %q, want continue", resp.Decision.Control.Kind)
	}
	if got := len(resp.Decision.ToolCalls); got != 2 {
		t.Fatalf("tool call count = %d, want 2", got)
	}
	if resp.Decision.ToolCalls[0].ID != "native_call_1" {
		t.Fatalf("first call id = %q, want native_call_1", resp.Decision.ToolCalls[0].ID)
	}
	if resp.Decision.ToolCalls[1].ID != "deepseek_call_2" {
		t.Fatalf("second call id = %q, want deepseek_call_2", resp.Decision.ToolCalls[1].ID)
	}
	if resp.Decision.ToolCalls[0].Name != "speak" {
		t.Fatalf("first tool name = %q, want speak", resp.Decision.ToolCalls[0].Name)
	}

	if text := resp.Decision.ToolCalls[0].Arguments["text"]; text != "Hi, friend." {
		t.Fatalf("text = %v, want Hi, friend.", text)
	}
	if emote := resp.Decision.ToolCalls[1].Arguments["emote"]; emote != "happy" {
		t.Fatalf("emote = %v, want happy", emote)
	}
}

func TestParseResponseNoToolCallSettles(t *testing.T) {
	resp, err := parseResponse([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	if err != nil {
		t.Fatalf("parseResponse failed: %v", err)
	}

	if resp.Decision.Control.Kind != model.ControlSettle {
		t.Fatalf("control = %q, want settle", resp.Decision.Control.Kind)
	}
	if len(resp.Decision.ToolCalls) != 0 {
		t.Fatalf("tool calls = %+v, want none", resp.Decision.ToolCalls)
	}
}

func TestParseResponseStripsSettleSentinel(t *testing.T) {
	resp, err := parseResponse([]byte(`{
		"choices": [
			{
				"message": {
					"tool_calls": [
						{
							"id": "settle_call",
							"type": "function",
							"function": {
								"name": "__gameagent_settle",
								"arguments": "{\"reason\":\"done\"}"
							}
						}
					]
				}
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parseResponse failed: %v", err)
	}

	if resp.Decision.Control.Kind != model.ControlSettle {
		t.Fatalf("control = %q, want settle", resp.Decision.Control.Kind)
	}
	if resp.Decision.Control.Reason != "done" {
		t.Fatalf("control reason = %q, want done", resp.Decision.Control.Reason)
	}
	if len(resp.Decision.ToolCalls) != 0 {
		t.Fatalf("sentinel leaked as tool call: %+v", resp.Decision.ToolCalls)
	}
}
