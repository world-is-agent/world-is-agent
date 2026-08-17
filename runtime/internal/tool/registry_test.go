package tool

import (
	"fmt"
	"sync"
	"testing"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestRegisterEnvironmentCapabilitiesUsesAdapterSchema(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha1.Capability{
		nil,
		{
			Name:            "",
			InputSchemaJson: `{"type":"object"}`,
		},
		{
			Name:            "broken",
			Description:     "Malformed schema should be skipped.",
			InputSchemaJson: `{`,
		},
		{
			Name:            "emote",
			Description:     "Make the NPC show an emote.",
			InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string"}},"required":["emote"]}`,
		},
	})

	if registry.HasTool("broken") {
		t.Fatal("malformed capability schema should not be registered")
	}

	if !registry.HasTool("emote") {
		t.Fatal("expected emote capability to be registered")
	}

	available := registry.Available()
	if len(available) != 1 {
		t.Fatalf("available tool count = %d, want 1", len(available))
	}

	got := available[0]
	if got.Name != "emote" {
		t.Fatalf("tool name = %q, want emote", got.Name)
	}
	if got.Description != "Make the NPC show an emote." {
		t.Fatalf("tool description = %q", got.Description)
	}
	if got.InputSchema != `{"type":"object","properties":{"emote":{"type":"string"}},"required":["emote"]}` {
		t.Fatalf("tool schema = %q", got.InputSchema)
	}
}

func TestRegistryAllowsConcurrentRegisterAndRead(t *testing.T) {
	registry := NewRegistry()

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()

			for i := 0; i < 200; i++ {
				registry.RegisterEnvironmentCapabilities([]*protocolv1alpha1.Capability{
					{
						Name:            fmt.Sprintf("tool_%d_%d", worker, i),
						InputSchemaJson: `{"type":"object"}`,
					},
				})
				_ = registry.Available()
				_ = registry.HasTool("tool_0_0")
			}
		}(worker)
	}

	wg.Wait()
}

func TestValidateToolCallOnlyChecksEnvelope(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha1.Capability{
		{
			Name:            "emote",
			InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string"}},"required":["emote"]}`,
		},
	})

	args, err := structpb.NewStruct(map[string]any{
		"unexpected": "adapter-validates-semantics",
	})
	if err != nil {
		t.Fatalf("NewStruct failed: %v", err)
	}

	err = registry.ValidateToolCall("npc:Linus", model.ToolCall{
		Name:      "emote",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("ValidateToolCall returned error: %v", err)
	}

	err = registry.ValidateToolCall("npc:Linus", model.ToolCall{
		Name:      "missing",
		Arguments: args,
	})
	if err == nil {
		t.Fatal("expected unregistered tool to fail")
	}

	err = registry.ValidateToolCall("npc:Linus", model.ToolCall{
		Name: "emote",
	})
	if err == nil {
		t.Fatal("expected nil arguments to fail")
	}
}
