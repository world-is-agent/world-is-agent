package context

import (
	"errors"
	"fmt"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"
)

var ErrInvalidInput = errors.New("invalid agent context input")

type AgentContext struct {
	SessionKey session.AgentSessionKey

	RuntimePolicy string

	RecentMemories []memory.Record

	Event       *protocolv1alpha2.GameEvent
	Observation *protocolv1alpha2.Observation

	Tools []model.ToolDefinition
}

type BuildInput struct {
	SessionKey session.AgentSessionKey

	RuntimePolicy string

	RecentMemories []memory.Record

	Event       *protocolv1alpha2.GameEvent
	Observation *protocolv1alpha2.Observation

	Tools []model.ToolDefinition
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
		SessionKey:     input.SessionKey,
		RuntimePolicy:  input.RuntimePolicy,
		RecentMemories: append([]memory.Record(nil), input.RecentMemories...),
		Event:          input.Event,
		Observation:    input.Observation,
		Tools:          append([]model.ToolDefinition(nil), input.Tools...),
	}, nil
}
