package trace

import (
	"context"
	"time"
)

type EventName string

const (
	EventTurnStarted           EventName = "turn_started"
	EventObservationRequested  EventName = "observation_requested"
	EventObservationReceived   EventName = "observation_received"
	EventModelRequestStarted   EventName = "model_request_started"
	EventModelResponseReceived EventName = "model_response_received"
	EventToolCallSelected      EventName = "tool_call_selected"
	EventActionSubmitStarted   EventName = "action_submit_started"
	EventActionResultReceived  EventName = "action_result_received"
	EventTurnCompleted         EventName = "turn_completed"
	EventTurnFailed            EventName = "turn_failed"
)

type Fields map[string]any

type EventData struct {
	ActionID string
	Tool     string
	Fields   Fields
}

type TurnContext struct {
	GameID    string
	EnvID     string
	EventID   string
	EventType string
	EntityID  string
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	TraceID       string    `json:"trace_id"`
	TurnID        string    `json:"turn_id"`
	Seq           uint32    `json:"seq"`
	Event         EventName `json:"event"`
	Time          time.Time `json:"time"`
	ElapsedMS     int64     `json:"elapsed_ms"`

	GameID    string `json:"game_id,omitempty"`
	EnvID     string `json:"env_id,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	EventType string `json:"event_type,omitempty"`
	EntityID  string `json:"entity_id,omitempty"`

	ActionID string `json:"action_id,omitempty"`
	Tool     string `json:"tool,omitempty"`

	Stage        string `json:"stage,omitempty"`
	Reason       string `json:"reason,omitempty"`
	ErrorMessage string `json:"error,omitempty"`

	Fields Fields `json:"fields,omitempty"`
}

type Recorder interface {
	Record(event Event)
	Close(ctx context.Context) error
}
