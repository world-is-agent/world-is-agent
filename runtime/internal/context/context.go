package context

import (
	"errors"
	"fmt"
	"strings"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"
)

var ErrInvalidInput = errors.New("invalid agent context input")

type AgentDescriptor struct {
	EntityID     string
	DefinitionID string
}

type AgentContext struct {
	SessionKey session.AgentSessionKey

	AgentDescriptor AgentDescriptor

	RuntimePolicy string

	RecentMemories []memory.Record

	Event       *protocolv1alpha2.GameEvent
	Observation *protocolv1alpha2.Observation

	Tools []model.ToolDefinition

	Transcript []model.Message
}

type BuildInput struct {
	SessionKey session.AgentSessionKey

	RuntimePolicy string

	RecentMemories []memory.Record

	Event       *protocolv1alpha2.GameEvent
	Observation *protocolv1alpha2.Observation

	Tools []model.ToolDefinition

	Transcript []model.Message
}

type Builder struct{}

// NewBuilder 创建 AgentContext Builder。
// Builder 本身无状态，便于在 Loop 中长期复用。
func NewBuilder() Builder {
	return Builder{}
}

// Build 负责建立 AgentContext 边界。
// 它只做结构化组装与必要校验，不负责 prompt 文本渲染。
func (Builder) Build(input BuildInput) (AgentContext, error) {
	if input.Event == nil {
		return AgentContext{}, fmt.Errorf("%w: event is required", ErrInvalidInput)
	}
	if input.Observation == nil {
		return AgentContext{}, fmt.Errorf("%w: observation is required", ErrInvalidInput)
	}
	if input.SessionKey.GameID == "" || input.SessionKey.WorldID == "" || input.SessionKey.EntityID == "" {
		return AgentContext{}, fmt.Errorf("%w: session key is required", ErrInvalidInput)
	}

	return AgentContext{
		SessionKey: input.SessionKey,
		AgentDescriptor: AgentDescriptor{
			EntityID:     input.SessionKey.EntityID,
			DefinitionID: definitionIDFromTargetEntity(input.Event),
		},
		RuntimePolicy:  input.RuntimePolicy,
		RecentMemories: append([]memory.Record(nil), input.RecentMemories...),
		Event:          input.Event,
		Observation:    input.Observation,
		Tools:          append([]model.ToolDefinition(nil), input.Tools...),
		Transcript:     copyMessages(input.Transcript),
	}, nil
}

func copyMessages(messages []model.Message) []model.Message {
	if len(messages) == 0 {
		return nil
	}

	out := make([]model.Message, len(messages))
	for i, message := range messages {
		out[i] = message
		out[i].ToolCalls = copyToolCalls(message.ToolCalls)
		out[i].ToolResults = copyToolResults(message.ToolResults)
	}
	return out
}

func copyToolCalls(calls []model.ToolCall) []model.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	out := make([]model.ToolCall, len(calls))
	for i, call := range calls {
		out[i] = call
		out[i].Arguments = copyMap(call.Arguments)
	}
	return out
}

func copyToolResults(results []model.ToolResult) []model.ToolResult {
	if len(results) == 0 {
		return nil
	}

	out := make([]model.ToolResult, len(results))
	for i, result := range results {
		out[i] = result
		out[i].Output = copyMap(result.Output)
	}
	return out
}

func copyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}

	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func definitionIDFromTargetEntity(event *protocolv1alpha2.GameEvent) string {
	if event == nil {
		return ""
	}
	targetEntityID := strings.TrimSpace(event.GetTargetEntityId())
	if targetEntityID == "" {
		return ""
	}

	for _, entity := range event.GetEntities() {
		if entity == nil {
			continue
		}
		if strings.TrimSpace(entity.GetEntityId()) != targetEntityID {
			continue
		}
		return strings.TrimSpace(entity.GetDefinitionId())
	}

	return ""
}
