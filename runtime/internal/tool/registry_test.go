package tool

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/model"
)

func TestRegisterEnvironmentCapabilitiesUsesAdapterSchema(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
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
				registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
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

func TestRegistryLookupFindsEnvironmentTool(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			Description:     "Make the NPC say a short line of dialogue.",
			InputSchemaJson: `{"type":"object","properties":{"text":{"type":"string"}}}`,
			ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_SYNC,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL,
		},
	})

	entry, ok := registry.Lookup("speak")
	if !ok {
		t.Fatal("Lookup(speak) = false, want true")
	}
	if entry.Kind != KindEnvironment {
		t.Fatalf("Kind = %q, want environment", entry.Kind)
	}
	if entry.Concurrency != ConcurrencySequential {
		t.Fatalf("Concurrency = %q, want sequential", entry.Concurrency)
	}
	if entry.Definition.Name != "speak" {
		t.Fatalf("Definition.Name = %q, want speak", entry.Definition.Name)
	}
	if entry.Definition.Description != "Make the NPC say a short line of dialogue." {
		t.Fatalf("Definition.Description = %q", entry.Definition.Description)
	}
}

func TestRegistryMapsUnspecifiedConcurrencyToSequential(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			InputSchemaJson: `{"type":"object"}`,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_UNSPECIFIED,
		},
	})

	entry, ok := registry.Lookup("speak")
	if !ok {
		t.Fatal("Lookup(speak) = false, want true")
	}
	if entry.Concurrency != ConcurrencySequential {
		t.Fatalf("Concurrency = %q, want sequential", entry.Concurrency)
	}
}

func TestRegistryMapsParallelSafeCapability(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "sense_nearby",
			InputSchemaJson: `{"type":"object"}`,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE,
		},
	})

	entry, ok := registry.Lookup("sense_nearby")
	if !ok {
		t.Fatal("Lookup(sense_nearby) = false, want true")
	}
	if entry.Concurrency != ConcurrencyParallelSafe {
		t.Fatalf("Concurrency = %q, want parallel_safe", entry.Concurrency)
	}
}

func TestRegistryTreatsParallelSafeAsStrongAdapterCommitment(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			InputSchemaJson: `{"type":"object"}`,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE,
		},
	})

	entry, ok := registry.Lookup("speak")
	if !ok {
		t.Fatal("Lookup(speak) = false, want true")
	}
	if entry.Concurrency != ConcurrencyParallelSafe {
		t.Fatalf("Concurrency = %q, want adapter-declared parallel_safe", entry.Concurrency)
	}
}

func TestRegistryExcludesAsyncCapabilitiesFromPhase5ToolView(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "wait_for_path",
			InputSchemaJson: `{"type":"object"}`,
			ExecutionMode:   protocolv1alpha2.ExecutionMode_EXECUTION_MODE_ASYNC,
		},
	})

	if registry.HasTool("wait_for_path") {
		t.Fatal("async capability should not be visible as a Phase5 tool")
	}
	if _, ok := registry.Lookup("wait_for_path"); ok {
		t.Fatal("async capability should not have a registry entry")
	}
	if got := len(registry.Available()); got != 0 {
		t.Fatalf("Available count = %d, want 0", got)
	}
}

func TestRegistryAvailableReturnsDeterministicOrder(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{Name: "speak", InputSchemaJson: `{"type":"object"}`},
		{Name: "emote", InputSchemaJson: `{"type":"object"}`},
		{Name: "observe_detail", InputSchemaJson: `{"type":"object"}`},
	})

	available := registry.Available()
	if got := len(available); got != 3 {
		t.Fatalf("available count = %d, want 3", got)
	}
	wantNames := []string{"emote", "observe_detail", "speak"}
	for i, want := range wantNames {
		if available[i].Name != want {
			t.Fatalf("available[%d].Name = %q, want %q", i, available[i].Name, want)
		}
	}
}

func TestValidateToolCallOnlyChecksEnvelope(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "emote",
			InputSchemaJson: `{"type":"object","properties":{"emote":{"type":"string"}},"required":["emote"]}`,
		},
	})

	args := map[string]any{
		"unexpected": "adapter-validates-semantics",
	}

	err := registry.ValidateToolCall("npc:Linus", model.ToolCall{
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

func TestRegistryValidateToolCallRejectsNilArgumentMap(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{Name: "speak", InputSchemaJson: `{"type":"object"}`},
	})

	err := registry.ValidateToolCall("npc:Linus", model.ToolCall{Name: "speak"})
	if err == nil {
		t.Fatal("expected nil argument map to fail")
	}
}

func TestValidateToolCallBatchRejectsUnknownToolBeforeExecution(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{Name: "speak", InputSchemaJson: `{"type":"object"}`},
	})

	entries, err := registry.ValidateToolCallBatch("npc:Linus", []model.ToolCall{
		{Name: "speak", Arguments: map[string]any{"text": "hello"}},
		{Name: "missing", Arguments: map[string]any{}},
	})
	if err == nil {
		t.Fatal("ValidateToolCallBatch returned nil error, want unknown tool failure")
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want none on batch validation failure", entries)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want missing tool name", err)
	}
}

func TestValidateToolCallBatchRejectsInvalidArgumentsBeforeExecution(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{Name: "speak", InputSchemaJson: `{"type":"object"}`},
		{Name: "emote", InputSchemaJson: `{"type":"object"}`},
	})

	entries, err := registry.ValidateToolCallBatch("npc:Linus", []model.ToolCall{
		{Name: "speak", Arguments: map[string]any{"text": "hello"}},
		{Name: "emote"},
	})
	if err == nil {
		t.Fatal("ValidateToolCallBatch returned nil error, want invalid arguments failure")
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want none on batch validation failure", entries)
	}
	if !strings.Contains(err.Error(), "tool arguments are missing") {
		t.Fatalf("error = %v, want missing arguments", err)
	}
}

func TestValidateToolCallBatchReturnsEntriesInToolCallOrder(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "speak",
			InputSchemaJson: `{"type":"object"}`,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL,
		},
		{
			Name:            "sense_nearby",
			InputSchemaJson: `{"type":"object"}`,
			ConcurrencyMode: protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE,
		},
	})

	entries, err := registry.ValidateToolCallBatch("npc:Linus", []model.ToolCall{
		{Name: "sense_nearby", Arguments: map[string]any{}},
		{Name: "speak", Arguments: map[string]any{"text": "hello"}},
	})
	if err != nil {
		t.Fatalf("ValidateToolCallBatch returned error: %v", err)
	}
	if got := len(entries); got != 2 {
		t.Fatalf("entry count = %d, want 2", got)
	}
	if entries[0].Definition.Name != "sense_nearby" || entries[0].Concurrency != ConcurrencyParallelSafe {
		t.Fatalf("entries[0] = %+v, want sense_nearby parallel_safe", entries[0])
	}
	if entries[1].Definition.Name != "speak" || entries[1].Concurrency != ConcurrencySequential {
		t.Fatalf("entries[1] = %+v, want speak sequential", entries[1])
	}
}
