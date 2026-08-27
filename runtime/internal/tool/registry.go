package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/model"
)

// Registry 保存 Runtime 当前可暴露给 Agent Loop 的工具。
//
// Adapter 上报的是 capability，Runtime 注册后才变成模型可见的 tool。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Entry
}

type Kind string

const (
	KindEnvironment Kind = "environment"
)

type ConcurrencyMode string

const (
	ConcurrencySequential   ConcurrencyMode = "sequential"
	ConcurrencyParallelSafe ConcurrencyMode = "parallel_safe"
)

type Entry struct {
	Definition  model.ToolDefinition
	Kind        Kind
	Concurrency ConcurrencyMode
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Entry),
	}
}

// RegisterEnvironmentCapabilities 把 Adapter 的 environment-level capability 注册成 tool。
func (r *Registry) RegisterEnvironmentCapabilities(capabilities []*protocolv1alpha2.Capability) {
	for _, capability := range capabilities {

		if capability == nil {
			continue
		}
		if capability.Name == "" {
			continue
		}
		if capability.GetExecutionMode() == protocolv1alpha2.ExecutionMode_EXECUTION_MODE_ASYNC {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(capability.InputSchemaJson), &raw); err != nil {
			fmt.Printf("skip capability %q: invalid input_schema_json: %v\n", capability.Name, err)
			continue
		}

		name := capability.Name
		description := capability.Description
		inputSchemaJson := capability.InputSchemaJson

		r.mu.Lock()
		r.tools[name] = Entry{
			Definition: model.ToolDefinition{
				Name:        name,
				Description: description,
				InputSchema: inputSchemaJson,
			},
			Kind:        KindEnvironment,
			Concurrency: concurrencyModeFromCapability(capability),
		}
		r.mu.Unlock()
	}
}

func (r *Registry) Available() []model.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	available := make([]model.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		available = append(available, tool.Definition)
	}
	sort.Slice(available, func(i, j int) bool {
		return available[i].Name < available[j].Name
	})
	return available
}

func (r *Registry) HasTool(name string) bool {
	_, exists := r.Lookup(name)
	return exists
}

func (r *Registry) Lookup(name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.tools[name]
	return entry, exists
}

func concurrencyModeFromCapability(capability *protocolv1alpha2.Capability) ConcurrencyMode {
	if capability.GetConcurrencyMode() == protocolv1alpha2.CapabilityConcurrencyMode_CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE {
		return ConcurrencyParallelSafe
	}
	return ConcurrencySequential
}
