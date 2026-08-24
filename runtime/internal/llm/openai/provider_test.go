package openai

import (
	"encoding/json"
	"strings"
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
		Controls: []model.ControlDefinition{
			{Kind: model.ControlSettle, Description: "Finish the turn without an environment action."},
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
	if got := payload["tool_choice"]; got != "auto" {
		t.Fatalf("tool_choice = %v, want auto", got)
	}
	instructions, _ := payload["instructions"].(string)
	if !strings.Contains(instructions, "__gameagent_settle") {
		t.Fatalf("instructions missing settle sentinel:\n%s", instructions)
	}
}

func TestBuildRequestMapsToolTranscriptToProviderSafeInput(t *testing.T) {
	provider := NewProvider("test-key", "gpt-5-mini")

	body, err := provider.buildRequest(model.Request{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "current context"},
			{Role: model.RoleAssistant, Content: `[{"tool_call_id":"call_1","name":"speak"}]`},
			{Role: model.RoleTool, Content: `[{"tool_call_id":"call_1","status":"succeeded","code":"action_succeeded"}]`},
		},
	})
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	input := payload["input"].([]any)
	toolTranscript := input[2].(map[string]any)
	if got := toolTranscript["role"]; got != "user" {
		t.Fatalf("tool transcript role = %v, want provider-safe user", got)
	}
	if content := toolTranscript["content"].(string); !strings.Contains(content, "action_succeeded") {
		t.Fatalf("tool transcript content missing result:\n%s", content)
	}
}

func TestParseResponseReturnsModelDecisionToolCalls(t *testing.T) {
	resp, err := parseResponse([]byte(`{
		"output": [
			{
				"type": "function_call",
				"call_id": "native_call_1",
				"name": "speak",
				"arguments": "{\"text\":\"Hello.\"}"
			},
			{
				"type": "function_call",
				"name": "emote",
				"arguments": "{\"emote\":\"happy\"}"
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
	if resp.Decision.ToolCalls[1].ID != "openai_call_2" {
		t.Fatalf("second call id = %q, want openai_call_2", resp.Decision.ToolCalls[1].ID)
	}
	if resp.Decision.ToolCalls[0].Name != "speak" {
		t.Fatalf("first tool name = %q, want speak", resp.Decision.ToolCalls[0].Name)
	}
	if text := resp.Decision.ToolCalls[0].Arguments["text"]; text != "Hello." {
		t.Fatalf("text = %v, want Hello.", text)
	}
	if emote := resp.Decision.ToolCalls[1].Arguments["emote"]; emote != "happy" {
		t.Fatalf("emote = %v, want happy", emote)
	}
}

func TestParseResponseNoToolCallSettles(t *testing.T) {
	resp, err := parseResponse([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`))
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
		"output": [
			{
				"type": "function_call",
				"call_id": "settle_call",
				"name": "__gameagent_settle",
				"arguments": "{\"reason\":\"done\"}"
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
