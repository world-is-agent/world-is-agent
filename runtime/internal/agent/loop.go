package agent

import (
	"context"
	"errors"
	"fmt"
	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	agentcontext "gameagent/runtime/internal/context"
	"gameagent/runtime/internal/idgen"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"
	"gameagent/runtime/internal/tool"
	"gameagent/runtime/internal/trace"
)

// Environment 定义 Agent Loop 需要的最小环境能力。
//
// Loop 只依赖这个接口，不直接依赖 gateway 或具体游戏 Adapter。
type Environment interface {
	Observe(ctx context.Context, worldID string, entityID string) (*protocolv1alpha2.Observation, error)
	SubmitAction(ctx context.Context, req *protocolv1alpha2.ActionRequest) (*protocolv1alpha2.ActionResult, error)
}

// Loop 执行 Runtime MVP0 的 one-turn AgentRun。
type Loop struct {
	model    model.Provider
	tools    *tool.Registry
	recorder trace.Recorder
	config   Config

	memoryStore     memory.Store
	memoryProjector memoryProjector
	contextBuilder  agentcontext.Builder
	contextRenderer agentcontext.Renderer
}

type memoryProjector interface {
	Project(memory.ProjectInput) (memory.Record, error)
}

var (
	errInvalidModelDecision = errors.New("invalid model response")
	errUnsupportedToolBatch = errors.New("multiple tool calls are not supported until scheduler")
)

type LoopOption func(*Loop)

// WithMemoryStore 覆盖 Loop 默认使用的 MemoryStore。
// 主要用于测试或未来替换成持久化 Memory backend。
func WithMemoryStore(store memory.Store) LoopOption {
	return func(loop *Loop) {
		if store == nil {
			return
		}
		loop.memoryStore = store
	}
}

// WithMemoryProjector 覆盖 Loop 默认使用的 MemoryProjector。
// 主要用于测试 Memory 投影失败等 fail-open 分支。
func WithMemoryProjector(projector interface {
	Project(memory.ProjectInput) (memory.Record, error)
}) LoopOption {
	return func(loop *Loop) {
		if projector == nil {
			return
		}
		loop.memoryProjector = projector
	}
}

type ConnectionContext struct {
	GameID    string
	SessionID string
}

// NewLoop 创建 Agent Loop。
// Phase4 在 Loop 中接入 MemoryStore、MemoryProjector、ContextBuilder 和 Renderer，
// 让一次 Agent Turn 可以读取历史 Memory 并在成功 Action 后更新 Memory。
func NewLoop(modelProvider model.Provider, tools *tool.Registry, recorder trace.Recorder, config Config, options ...LoopOption) *Loop {
	if recorder == nil {
		recorder = trace.NoopRecorder{}
	}
	config = config.WithDefaults()

	loop := &Loop{
		model:           modelProvider,
		tools:           tools,
		recorder:        recorder,
		config:          config,
		memoryStore:     memory.NewInMemoryStoreWithMaxRecords(defaultMemoryStoreMaxRecords(config.RecentMemoryLimit)),
		memoryProjector: memory.NewProjector(nil),
		contextBuilder:  agentcontext.NewBuilder(),
		contextRenderer: agentcontext.NewRenderer(agentcontext.RendererConfig{
			MemoryContextSizeLimit: config.MemoryContextSizeLimit,
		}),
	}
	for _, option := range options {
		if option != nil {
			option(loop)
		}
	}
	return loop
}

// defaultMemoryStoreMaxRecords 计算默认 InMemory backend 的保留上限。
// 保留数量必须不少于 RecentMemoryLimit，避免配置被 store 层隐式截断。
func defaultMemoryStoreMaxRecords(recentMemoryLimit int) int {
	if recentMemoryLimit > memory.DefaultMaxRecordsPerSession {
		return recentMemoryLimit
	}
	return memory.DefaultMaxRecordsPerSession
}

// HandleEvent 处理一次 GameEvent，并在需要时执行完整的 Agent Turn。
//
// Phase4 的主流程仍是 Observe once、Think once、Act once；
// 新增的 Memory / Context 只负责提供跨 Turn 的短期上下文，不改变 Action 执行协议。
func (l *Loop) HandleEvent(
	ctx context.Context,
	env Environment,
	conn ConnectionContext,
	key session.AgentSessionKey,
	event *protocolv1alpha2.GameEvent,
) error {
	if key.EntityID == "" {
		return fmt.Errorf("agent session entity id is empty")
	}
	if key.WorldID == "" {
		return fmt.Errorf("agent session world id is empty")
	}
	ctx, cancelTurn := context.WithTimeout(ctx, l.config.TurnTimeout)
	defer cancelTurn()

	turnID := idgen.New("turn")
	// 为本次有效 GameEvent 创建 TurnTracer。
	turnTracer := trace.NewTurnTracerWithID(l.recorder, trace.TurnContext{
		GameID:    key.GameID,
		WorldID:   key.WorldID,
		SessionID: conn.SessionID,
		EventID:   event.EventId,
		EventType: event.EventType,
		EntityID:  key.EntityID,
	}, turnID)
	turnTracer.Emit(trace.EventTurnStarted, trace.EventData{})
	turnTracer.Emit(trace.EventObservationRequested, trace.EventData{})

	observeCtx, cancelObserve := context.WithTimeout(ctx, l.config.ObserveTimeout)
	obs, err := env.Observe(observeCtx, key.WorldID, key.EntityID)
	cancelObserve()

	if err != nil {
		reason := "observation_failed"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "observe_timeout"
		} else if reasoner, ok := err.(interface{ FailureReason() string }); ok {
			reason = reasoner.FailureReason()
		}
		turnTracer.Fail("observation", reason, err, trace.EventData{})
		return err
	}
	turnTracer.Emit(trace.EventObservationReceived, trace.EventData{})

	recentMemories := l.loadRecentMemories(ctx, turnTracer, key)
	agentCtx, err := l.contextBuilder.Build(agentcontext.BuildInput{
		SessionKey:     key,
		RuntimePolicy:  BuildSystemPrompt(l.config.Prompt),
		Event:          event,
		Observation:    obs,
		RecentMemories: recentMemories,
		Tools:          l.tools.Available(),
	})
	if err != nil {
		turnTracer.Fail("context", "context_build_failed", err, trace.EventData{})
		return err
	}
	req, err := l.contextRenderer.Render(agentCtx)
	if err != nil {
		turnTracer.Fail("context", "context_render_failed", err, trace.EventData{})
		return err
	}
	turnTracer.Emit(trace.EventModelRequestStarted, trace.EventData{
		Fields: trace.Fields{
			"tool_count": len(req.Tools),
		},
	})
	modelCtx, cancelLLM := context.WithTimeout(ctx, l.config.LLMTimeout)
	rep, err := l.model.Generate(modelCtx, req)
	cancelLLM()
	if err != nil {
		reason := "provider_failed"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "provider_timeout"
		}
		turnTracer.Fail("model", reason, err, trace.EventData{})
		return err
	}

	turnTracer.Emit(trace.EventModelResponseReceived, trace.EventData{})

	toolCall, shouldAct, err := selectExecutableToolCall(rep.Decision)
	if err != nil {
		stage := "model"
		reason := "invalid_model_response"
		if errors.Is(err, errUnsupportedToolBatch) {
			stage = "tool"
			reason = "tool_batch_unsupported"
		}
		turnTracer.Fail(stage, reason, err, trace.EventData{})
		return err
	}

	if !shouldAct {
		turnTracer.Complete(trace.EventData{})
		return nil
	}

	// 当前 Loop 只执行单 ToolCall；Loop 只做统一校验和协议转换。
	if err := l.tools.ValidateToolCall(key.EntityID, toolCall); err != nil {
		turnTracer.Fail("tool", "tool_call_invalid", err, trace.EventData{
			Tool: toolCall.Name,
		})
		return err
	}
	turnTracer.Emit(trace.EventToolCallSelected, trace.EventData{
		Tool: toolCall.Name,
	})

	actReq, err := tool.BuildActionRequest(key.WorldID, key.EntityID, toolCall)
	if err != nil {
		turnTracer.Fail("action", "action_request_build_failed", err, trace.EventData{
			Tool: toolCall.Name,
		})
		return err
	}

	turnTracer.Emit(trace.EventActionSubmitStarted, trace.EventData{
		ActionID: actReq.ActionId,
		Tool:     actReq.Capability,
	})

	actionCtx, cancelAction := context.WithTimeout(ctx, l.config.ActionTimeout)
	actRep, err := env.SubmitAction(actionCtx, actReq)
	cancelAction()

	if err != nil {
		reason := "submit_action_failed"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "action_timeout"
		}
		turnTracer.Fail("action", reason, err, trace.EventData{
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

	if actRep.Status != protocolv1alpha2.ActionStatus_ACTION_STATUS_SUCCEEDED {
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

	l.updateMemory(ctx, turnTracer, key, turnID, event, toolCall, actRep)
	turnTracer.Complete(trace.EventData{
		ActionID: actRep.ActionId,
		Tool:     actReq.Capability,
	})

	return nil
}

func selectExecutableToolCall(decision model.ModelDecision) (model.ToolCall, bool, error) {
	switch decision.Control.Kind {
	case model.ControlSettle:
	case model.ControlContinue:
	default:
		return model.ToolCall{}, false, fmt.Errorf("%w: control is unspecified", errInvalidModelDecision)
	}

	switch len(decision.ToolCalls) {
	case 0:
		if decision.Control.Kind == model.ControlSettle {
			return model.ToolCall{}, false, nil
		}
		return model.ToolCall{}, false, fmt.Errorf("%w: continue control requires one tool call", errInvalidModelDecision)
	case 1:
		return decision.ToolCalls[0], true, nil
	default:
		return model.ToolCall{}, false, fmt.Errorf("%w: got %d", errUnsupportedToolBatch, len(decision.ToolCalls))
	}
}

// loadRecentMemories 在模型调用前加载当前 AgentSession 的短期记忆。
// Memory 是 peripheral：读取失败不能影响 Observe -> LLM -> Action 的主链路。
func (l *Loop) loadRecentMemories(
	ctx context.Context,
	turnTracer trace.TurnTracer,
	key session.AgentSessionKey,
) []memory.Record {
	if !l.config.MemoryEnabledValue() || l.memoryStore == nil {
		turnTracer.Emit(trace.EventContextLoaded, trace.EventData{
			Fields: trace.Fields{
				"memory_enabled": false,
				"memory_count":   0,
			},
		})
		return nil
	}

	records, err := l.memoryStore.Recent(ctx, key, l.config.RecentMemoryLimit)
	if err != nil {
		turnTracer.Emit(trace.EventContextLoadFailed, trace.EventData{
			Fields: trace.Fields{
				"memory_enabled": true,
				"reason":         err.Error(),
			},
		})
		return nil
	}

	turnTracer.Emit(trace.EventContextLoaded, trace.EventData{
		Fields: trace.Fields{
			"memory_enabled": true,
			"memory_count":   len(records),
			"memory_ids":     memoryIDs(records),
		},
	})
	return records
}

// updateMemory 在 Action 成功后把本轮结果写入短期记忆。
// 这里不记录失败 Turn，因为 Phase4 的 P0 Memory 只表达已成功发生的行为连续性。
func (l *Loop) updateMemory(
	ctx context.Context,
	turnTracer trace.TurnTracer,
	key session.AgentSessionKey,
	turnID string,
	event *protocolv1alpha2.GameEvent,
	toolCall model.ToolCall,
	actionResult *protocolv1alpha2.ActionResult,
) {
	if !l.config.MemoryEnabledValue() || l.memoryStore == nil || l.memoryProjector == nil {
		return
	}

	record, err := l.memoryProjector.Project(memory.ProjectInput{
		SessionKey:   key,
		TurnID:       turnID,
		Event:        event,
		ToolCall:     toolCall,
		ActionResult: actionResult,
	})
	if err != nil {
		turnTracer.Emit(trace.EventContextUpdateFailed, trace.EventData{
			Fields: trace.Fields{
				"reason": err.Error(),
			},
		})
		return
	}

	if err := l.memoryStore.Append(ctx, record); err != nil {
		turnTracer.Emit(trace.EventContextUpdateFailed, trace.EventData{
			Fields: trace.Fields{
				"memory_id": record.MemoryID,
				"reason":    err.Error(),
			},
		})
		return
	}

	turnTracer.Emit(trace.EventContextUpdated, trace.EventData{
		Fields: trace.Fields{
			"memory_id": record.MemoryID,
		},
	})
}

// memoryIDs 提取 MemoryRecord ID，用于 trace 中记录本轮加载了哪些 Memory。
func memoryIDs(records []memory.Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.MemoryID != "" {
			ids = append(ids, record.MemoryID)
		}
	}
	return ids
}
