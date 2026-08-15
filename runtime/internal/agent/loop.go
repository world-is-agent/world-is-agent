package agent

import (
	"context"
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"
)

// Environment 定义 Agent Loop 需要的最小环境能力。
//
// Loop 只依赖这个接口，不直接依赖 gateway 或具体游戏 Adapter。
type Environment interface {
	Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error)
	SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error)
}

// Loop 执行 Runtime MVP0 的 one-turn AgentRun。
type Loop struct {
	model model.Provider
	tools *tool.Registry
}

func NewLoop(modelProvider model.Provider, tools *tool.Registry) *Loop {
	return &Loop{
		model: modelProvider,
		tools: tools,
	}
}

// HandleEvent 处理一次 GameEvent，并在需要时执行完整的 Agent Turn。
//
// MVP0 只处理 player_interacted_with_npc：Observe once、Think once、Act once。
func (l *Loop) HandleEvent(
	ctx context.Context,
	env Environment,
	event *protocolv1alpha1.GameEvent,
) error {
	if event.EventType != "player_interacted_with_npc" {
		return nil
	}

	// Agent Loop 只理解 protocol entity，不知道 Stardew / SMAPI 的具体类型。
	entityIDs := []string{}
	for _, entity := range event.Entities {
		if entity.EntityType == "npc" {
			entityIDs = append(entityIDs, entity.EntityId)
		}
	}
	if len(entityIDs) == 0 {
		return fmt.Errorf("npc entity not found in game event")
	}

	obs, err := env.Observe(ctx, entityIDs[0])
	if err != nil {
		return err
	}

	// MVP0 先把 Observation 简单放进 prompt；后续可以拆出 prompt builder，
	// 专门格式化 GameTime、State、NearbyEntities 等字段。
	req := BuildModelRequest(event, obs, l.tools.Available())
	rep, err := l.model.Generate(ctx, req)
	if err != nil {
		return err
	}

	// Provider 必须返回结构化 ToolCall；Loop 只做统一校验和协议转换。
	err = l.tools.ValidateToolCall(entityIDs[0], rep.ToolCall)
	if err != nil {
		return err
	}
	actReq, err := tool.BuildActionRequest(entityIDs[0], rep.ToolCall)
	if err != nil {
		return err
	}
	actRep, err := env.SubmitAction(ctx, actReq)
	if err != nil {
		return err
	}
	fmt.Printf("%s once loop end", actRep.ActionId)

	return nil
}
