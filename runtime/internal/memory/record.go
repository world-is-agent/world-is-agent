package memory

import (
	"time"

	"gameagent/runtime/internal/session"
)

type Record struct {
	MemoryID string

	SessionKey session.AgentSessionKey

	SourceTurnID        string
	SourceEventID       string
	SourceEventSequence uint64

	EventType string

	GameTime *GameTimeSnapshot

	Outcome TurnOutcome

	CreatedAt time.Time
}

type GameTimeSnapshot struct {
	Year   int32
	Season int32
	Day    int32
	Hour   int32
	Minute int32
	Tick   int64
}

type TurnOutcome struct {
	ToolName      string
	ToolArguments map[string]any

	ActionStatus string
}
