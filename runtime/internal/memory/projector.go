package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/idgen"
	"gameagent/runtime/internal/model"
	"gameagent/runtime/internal/session"
)

var ErrProject = errors.New("project memory")

type ProjectInput struct {
	SessionKey session.AgentSessionKey
	TurnID     string

	Event        *protocolv1alpha2.GameEvent
	ToolCall     model.ToolCall
	ActionResult *protocolv1alpha2.ActionResult
	Outcomes     []ProjectOutcome
}

type ProjectOutcome struct {
	ToolCall     model.ToolCall
	ActionResult *protocolv1alpha2.ActionResult
}

type Projector struct {
	now func() time.Time
}

// NewProjector 创建 Memory Projector。
// now 可注入，方便测试稳定断言 CreatedAt。
func NewProjector(now func() time.Time) Projector {
	if now == nil {
		now = time.Now
	}
	return Projector{now: now}
}

// Project 把 Event、ToolCall 和 ActionResult 转成 MemoryRecord。
// 这是确定性投影，不调用 LLM，也不读取 Trace JSONL。
func (p Projector) Project(input ProjectInput) (Record, error) {
	if strings.TrimSpace(input.TurnID) == "" {
		return Record{}, fmt.Errorf("%w: turn_id is required", ErrProject)
	}
	if input.Event == nil {
		return Record{}, fmt.Errorf("%w: event is required", ErrProject)
	}
	if input.SessionKey.GameID == "" || input.SessionKey.WorldID == "" || input.SessionKey.EntityID == "" {
		return Record{}, fmt.Errorf("%w: session key is required", ErrProject)
	}

	outcomes, err := projectOutcomes(input)
	if err != nil {
		return Record{}, err
	}

	return Record{
		MemoryID:            idgen.New("mem"),
		SessionKey:          input.SessionKey,
		SourceTurnID:        input.TurnID,
		SourceEventID:       input.Event.GetEventId(),
		SourceEventSequence: input.Event.GetSequence(),
		EventType:           input.Event.GetEventType(),
		GameTime:            gameTimeSnapshot(input.Event.GetGameTime()),
		SourceContextFacts:  sourceContextFacts(input.Event.GetContextFacts()),
		Outcomes:            outcomes,
		CreatedAt:           p.now(),
	}, nil
}

func projectOutcomes(input ProjectInput) ([]TurnOutcome, error) {
	items := input.Outcomes
	if len(items) == 0 && hasSingleOutcomeInput(input) {
		items = []ProjectOutcome{{
			ToolCall:     input.ToolCall,
			ActionResult: input.ActionResult,
		}}
	}

	if len(items) == 0 {
		return nil, nil
	}

	outcomes := make([]TurnOutcome, 0, len(items))
	for _, item := range items {
		if item.ActionResult == nil {
			return nil, fmt.Errorf("%w: action_result is required", ErrProject)
		}
		if strings.TrimSpace(item.ToolCall.Name) == "" {
			return nil, fmt.Errorf("%w: tool name is required", ErrProject)
		}
		outcomes = append(outcomes, TurnOutcome{
			ToolName:      item.ToolCall.Name,
			ToolArguments: toolArguments(item.ToolCall),
			ActionStatus:  item.ActionResult.GetStatus().String(),
		})
	}
	return outcomes, nil
}

func hasSingleOutcomeInput(input ProjectInput) bool {
	return input.ActionResult != nil || strings.TrimSpace(input.ToolCall.Name) != "" || input.ToolCall.Arguments != nil
}

func sourceContextFacts(facts []*protocolv1alpha2.ContextFact) []SourceContextFact {
	if len(facts) == 0 {
		return nil
	}

	out := make([]SourceContextFact, 0, len(facts))
	for _, fact := range facts {
		if fact == nil {
			continue
		}
		out = append(out, SourceContextFact{
			Kind:           fact.GetKind(),
			ActorEntityID:  fact.GetActorEntityId(),
			TargetEntityID: fact.GetTargetEntityId(),
			ScopeID:        fact.GetScopeId(),
			Text:           fact.GetText(),
			Label:          fact.GetLabel(),
			Attributes:     copyMap(fact.GetAttributes().AsMap()),
		})
	}
	return out
}

func gameTimeSnapshot(gameTime *protocolv1alpha2.GameTime) *GameTimeSnapshot {
	if gameTime == nil {
		return nil
	}
	return &GameTimeSnapshot{
		Year:   gameTime.GetYear(),
		Season: gameTime.GetSeason(),
		Day:    gameTime.GetDay(),
		Hour:   gameTime.GetHour(),
		Minute: gameTime.GetMinute(),
		Tick:   gameTime.GetTick(),
	}
}

// toolArguments 将模型返回的结构化参数复制成普通 map。
// Memory 只保存 Action 结果需要的轻量摘要，不保存完整 Observation。
func toolArguments(call model.ToolCall) map[string]any {
	if call.Arguments == nil {
		return nil
	}
	return copyMap(call.Arguments)
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = copyAny(value)
	}
	return out
}

func copyAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyAny(item)
		}
		return out
	default:
		return value
	}
}
