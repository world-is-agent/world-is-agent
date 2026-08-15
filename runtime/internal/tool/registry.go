package tool

import (
	"errors"
	"fmt"
	"gameagent/runtime/internal/model"
	"strings"
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
func (r *Registry) RegisterEnvironmentCapabilities(capabilities []string) {
	for _, capability := range capabilities {
		// MVP0 只允许 speak，避免模型调用 Adapter 尚未实现的能力。
		if capability != "speak" {
			continue
		}

		r.tools["speak"] = model.ToolDefinition{
			Name:        "speak",
			Description: "Make the NPC say a short line of dialogue.",
			InputSchema: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
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
		return errors.New("speak arguments are missing")
	}

	value := call.Arguments.Fields["text"]
	if value == nil {
		return errors.New("speak text is missing")
	}

	text := strings.TrimSpace(value.GetStringValue())
	if text == "" {
		return errors.New("speak text is empty")
	}

	if len(text) > 300 {
		return errors.New("speak text is too long")
	}

	return nil
}
