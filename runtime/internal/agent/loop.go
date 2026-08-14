package agent

import (
	"context"
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"
)

type Environment interface {
	Observe(ctx context.Context, entityID string) (*protocolv1alpha1.Observation, error)
	SubmitAction(ctx context.Context, req *protocolv1alpha1.ActionRequest) (*protocolv1alpha1.ActionResult, error)
}

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

func (l *Loop) HandleEvent(
	ctx context.Context,
	env Environment,
	event *protocolv1alpha1.GameEvent,
) error {
	if event.EventType != "player_interacted_with_npc" {
		return nil
	}

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

	prompt := fmt.Sprintf(
		"You are controlling an NPC in a game.\nObservation: %v",
		obs,
	)
	tools := l.tools.Available()
	req := model.Request{
		Prompt: prompt,
		Tools:  tools,
	}
	rep, err := l.model.Generate(ctx, req)
	if err != nil {
		return err
	}

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
