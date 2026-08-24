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
	if input.ActionResult == nil {
		return Record{}, fmt.Errorf("%w: action_result is required", ErrProject)
	}
	if strings.TrimSpace(input.ToolCall.Name) == "" {
		return Record{}, fmt.Errorf("%w: tool name is required", ErrProject)
	}
	if input.SessionKey.GameID == "" || input.SessionKey.WorldID == "" || input.SessionKey.EntityID == "" {
		return Record{}, fmt.Errorf("%w: session key is required", ErrProject)
	}

	return Record{
		MemoryID:            idgen.New("mem"),
		SessionKey:          input.SessionKey,
		SourceTurnID:        input.TurnID,
		SourceEventID:       input.Event.GetEventId(),
		SourceEventSequence: input.Event.GetSequence(),
		EventType:           input.Event.GetEventType(),
		GameTime:            gameTimeSnapshot(input.Event.GetGameTime()),
		Outcome: TurnOutcome{
			ToolName:      input.ToolCall.Name,
			ToolArguments: toolArguments(input.ToolCall),
			ActionStatus:  input.ActionResult.GetStatus().String(),
		},
		CreatedAt: p.now(),
	}, nil
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
	out := make(map[string]any, len(call.Arguments))
	for key, value := range call.Arguments {
		out[key] = value
	}
	return out
}
