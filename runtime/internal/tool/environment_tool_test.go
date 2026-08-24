package tool

import (
	"testing"

	"gameagent/runtime/internal/model"
)

func TestBuildActionRequestConvertsToolCallArgumentMapToProtocolStruct(t *testing.T) {
	req, err := BuildActionRequest("world:test", "npc:Linus", model.ToolCall{
		ID:        "call_1",
		Name:      "speak",
		Arguments: map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("BuildActionRequest returned error: %v", err)
	}

	if req.Arguments == nil {
		t.Fatal("Arguments is nil")
	}
	if text := req.Arguments.Fields["text"].GetStringValue(); text != "hello" {
		t.Fatalf("text = %q, want hello", text)
	}
}
