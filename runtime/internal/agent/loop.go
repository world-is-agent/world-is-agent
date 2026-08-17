package agent

import (
	"context"
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/tool"
	"gameagent/runtime/internal/trace"
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
	model    model.Provider
	tools    *tool.Registry
	recorder trace.Recorder
}

type ConnectionContext struct {
	GameID    string
	SessionID string
}

func NewLoop(modelProvider model.Provider, tools *tool.Registry, recorder trace.Recorder) *Loop {
	if recorder == nil {
		recorder = trace.NoopRecorder{}
	}

	return &Loop{
		model:    modelProvider,
		tools:    tools,
		recorder: recorder,
	}
}

// HandleEvent 处理一次 GameEvent，并在需要时执行完整的 Agent Turn。
//
// MVP0 只处理 player_interacted_with_npc：Observe once、Think once、Act once。
func (l *Loop) HandleEvent(
	ctx context.Context,
	env Environment,
	conn ConnectionContext,
	event *protocolv1alpha1.GameEvent,
) error {
	if event.EventType != "player_interacted_with_npc" {
		return nil
	}

	// Agent Loop 只理解 protocol entity，不知道具体游戏 Adapter 的内部类型。
	entityIDs := []string{}
	for _, entity := range event.Entities {
		if entity.EntityType == "npc" {
			entityIDs = append(entityIDs, entity.EntityId)
		}
	}
	if len(entityIDs) == 0 {
		return fmt.Errorf("npc entity not found in game event")
	}

	// 为本次有效 GameEvent 创建 TurnTracer。
	turnTracer := trace.NewTurnTracer(l.recorder, trace.TurnContext{
		GameID:    conn.GameID,
		SaveID:    event.SaveId,
		SessionID: conn.SessionID,
		EventID:   event.EventId,
		EventType: event.EventType,
		EntityID:  entityIDs[0],
	})
	turnTracer.Emit(trace.EventTurnStarted, trace.EventData{})
	turnTracer.Emit(trace.EventObservationRequested, trace.EventData{})
	obs, err := env.Observe(ctx, entityIDs[0])
	if err != nil {
		turnTracer.Fail("observation", "observation_failed", err, trace.EventData{})
		return err
	}
	turnTracer.Emit(trace.EventObservationReceived, trace.EventData{})

	// MVP0 先把 Observation 简单放进 prompt；后续可以拆出 prompt builder，
	// 专门格式化 GameTime、State、NearbyEntities 等字段。
	req := BuildModelRequest(event, obs, l.tools.Available())
	turnTracer.Emit(trace.EventModelRequestStarted, trace.EventData{
		Fields: trace.Fields{
			"tool_count": len(req.Tools),
		},
	})
	rep, err := l.model.Generate(ctx, req)
	if err != nil {
		turnTracer.Fail("model", "provider_failed", err, trace.EventData{})
		return err
	}

	turnTracer.Emit(trace.EventModelResponseReceived, trace.EventData{})

	// Provider 必须返回结构化 ToolCall；Loop 只做统一校验和协议转换。
	if err := l.tools.ValidateToolCall(entityIDs[0], rep.ToolCall); err != nil {
		turnTracer.Fail("tool", "tool_call_invalid", err, trace.EventData{
			Tool: rep.ToolCall.Name,
		})
		return err
	}
	turnTracer.Emit(trace.EventToolCallSelected, trace.EventData{
		Tool: rep.ToolCall.Name,
	})

	actReq, err := tool.BuildActionRequest(entityIDs[0], rep.ToolCall)
	if err != nil {
		turnTracer.Fail("action", "action_request_build_failed", err, trace.EventData{
			Tool: rep.ToolCall.Name,
		})
		return err
	}

	turnTracer.Emit(trace.EventActionSubmitStarted, trace.EventData{
		ActionID: actReq.ActionId,
		Tool:     actReq.Capability,
	})

	actRep, err := env.SubmitAction(ctx, actReq)
	if err != nil {
		turnTracer.Fail("action", "submit_action_failed", err, trace.EventData{
			ActionID: actReq.ActionId,
			Tool:     actReq.Capability,
		})
		return err
	}

	turnTracer.Emit(trace.EventActionResultReceived, trace.EventData{
		ActionID: actRep.ActionId,
		Tool:     actReq.Capability,
		Fields: trace.Fields{
			"action_status": actRep.Status.String(),
		},
	})

	if actRep.Status != protocolv1alpha1.ActionStatus_ACTION_STATUS_SUCCEEDED {
		turnTracer.Fail("action", "action_result_failed", nil, trace.EventData{
			ActionID: actRep.ActionId,
			Tool:     actReq.Capability,
			Fields: trace.Fields{
				"action_status": actRep.Status.String(),
			},
		})
		// ActionResult 失败是 Adapter 返回的业务结果；trace 用 reason/fields 表达，不伪造底层 error。
		// MVP0 仍返回 error，让 gateway 保留一条可见日志，方便联调时排查。
		return fmt.Errorf("action result failed: %s", actRep.Status.String())
	}

	turnTracer.Complete(trace.EventData{
		ActionID: actRep.ActionId,
		Tool:     actReq.Capability,
	})

	return nil
}
