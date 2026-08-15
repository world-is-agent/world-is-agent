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

func TestParseResponseReturnsToolCall(t *testing.T) {
	resp, err := parseResponse([]byte(`{
		"choices": [
			{
				"message": {
					"tool_calls": [
						{
							"type": "function",
							"function": {
								"name": "speak",
								"arguments": "{\"text\":\"Hi, friend.\"}"
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

	if resp.ToolCall.Name != "speak" {
		t.Fatalf("tool name = %q, want speak", resp.ToolCall.Name)
	}

	text := resp.ToolCall.Arguments.Fields["text"].GetStringValue()
	if text != "Hi, friend." {
		t.Fatalf("text = %q, want Hi, friend.", text)
	}
}

func TestParseResponseRejectsMissingToolCall(t *testing.T) {
	_, err := parseResponse([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	if err == nil {
		t.Fatal("parseResponse succeeded, want error")
	}
}
