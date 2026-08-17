package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"
)

// Registry 保存 Runtime 当前可暴露给 Agent Loop 的工具。
//
// Adapter 上报的是 capability，Runtime 注册后才变成模型可见的 tool。
type Registry struct {
	tools map[string]model.ToolDefinition
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]model.ToolDefinition),
	}
}

// RegisterEnvironmentCapabilities 把 Adapter 的 environment-level capability 注册成 tool。
func (r *Registry) RegisterEnvironmentCapabilities(capabilities []*protocolv1alpha1.Capability) {
	for _, capability := range capabilities {

		if capability == nil {
			continue
		}
		if capability.Name == "" {
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

		r.tools[name] = model.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: inputSchemaJson,
		}
	}
}

func (r *Registry) Available() []model.ToolDefinition {
	available := make([]model.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		available = append(available, tool)
	}
	return available
}

func (r *Registry) HasTool(name string) bool {
	if _, exists := r.tools[name]; exists {
		return true
	}
	return false
}

// ValidateToolCall 校验模型返回的 ToolCall 是否能安全转成 ActionRequest。
func (r *Registry) ValidateToolCall(entityID string, call model.ToolCall) error {
	if !r.HasTool(call.Name) {
		return fmt.Errorf("tool %q is not registered", call.Name)
	}
	if call.Arguments == nil {
		return errors.New("tool arguments are missing")
	}

	return nil
}
