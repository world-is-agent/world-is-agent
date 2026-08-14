package tool

import (
	"errors"
	"fmt"
	"gameagent/runtime/internal/model"
	"strings"
)

type Registry struct {
	tools map[string]model.ToolDefinition
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]model.ToolDefinition),
	}
}

// 注册工具
func (r *Registry) RegisterEnvironmentCapabilities(capabilities []string) {
	for _, capability := range capabilities {
		//暂时只注册“speak 能力”
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

// 查询当前可用工具列表
func (r *Registry) Available() []model.ToolDefinition {
	available := make([]model.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		available = append(available, tool)
	}
	return available
}

// 判断工具name是否合法
func (r *Registry) HasTool(name string) bool {
	if _, exists := r.tools[name]; exists {
		return true
	}
	return false
}

// 判断工具param是否合法
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
