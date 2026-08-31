package tool

import (
	"fmt"
	"sync"
	"testing"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"

	"google.golang.org/protobuf/types/known/structpb"
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

func TestRegistryParsesToolPolicyExtensions(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "ask_player",
			InputSchemaJson: `{"type":"object"}`,
			Extensions:      toolPolicyExtensions(t, true, true),
		},
	})

	entry, ok := registry.Lookup("ask_player")
	if !ok {
		t.Fatal("Lookup(ask_player) = false, want true")
	}
	if !entry.Policy.ExclusivePerStep {
		t.Fatal("ExclusivePerStep = false, want true")
	}
	if !entry.Policy.SettleAfterSuccess {
		t.Fatal("SettleAfterSuccess = false, want true")
	}
}

func TestRegistryDefaultsMissingToolPolicyToZeroValue(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{Name: "speak", InputSchemaJson: `{"type":"object"}`},
	})

	entry, ok := registry.Lookup("speak")
	if !ok {
		t.Fatal("Lookup(speak) = false, want true")
	}
	if entry.Policy.ExclusivePerStep {
		t.Fatal("ExclusivePerStep = true, want false")
	}
	if entry.Policy.SettleAfterSuccess {
		t.Fatal("SettleAfterSuccess = true, want false")
	}
}

func TestRegistrySkipsInvalidToolPolicyMetadata(t *testing.T) {
	registry := NewRegistry()
	extensions, err := structpb.NewStruct(map[string]any{
		"gameagent": map[string]any{
			"tool_policy": map[string]any{
				"exclusive_per_step": "yes",
			},
		},
	})
	if err != nil {
		t.Fatalf("build extensions: %v", err)
	}

	registry.RegisterEnvironmentCapabilities([]*protocolv1alpha2.Capability{
		{
			Name:            "ask_player",
			InputSchemaJson: `{"type":"object"}`,
			Extensions:      extensions,
		},
	})

	if registry.HasTool("ask_player") {
		t.Fatal("invalid tool policy metadata should skip capability registration")
	}
}

func toolPolicyExtensions(t *testing.T, exclusivePerStep bool, settleAfterSuccess bool) *structpb.Struct {
	t.Helper()

	extensions, err := structpb.NewStruct(map[string]any{
		"gameagent": map[string]any{
			"tool_policy": map[string]any{
				"exclusive_per_step":   exclusivePerStep,
				"settle_after_success": settleAfterSuccess,
			},
		},
	})
	if err != nil {
		t.Fatalf("build extensions: %v", err)
	}
	return extensions
}
