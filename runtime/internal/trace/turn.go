package trace

import (
	"gameagent/runtime/internal/idgen"
	"sync"
	"time"
)

// Emit      发普通阶段事件
// Complete  发 turn_completed
// Fail      发 turn_failed
type TurnTracer interface {
	Emit(name EventName, data EventData)
	Complete(data EventData)
	Fail(stage string, reason string, err error, data EventData)
}

// mu       保护 seq / closed
// recorder 真正消费 Event 的对象
// ctx      game_id/save_id/session_id/event_id/entity_id 等固定上下文
// turnID   本轮 turn ID
// traceID  Phase2 先等于 turnID
// start    计算 elapsed_ms 的起点
// seq      本轮事件序号
// closed   terminal 发出后关闭
type turnTracer struct {
	mu       sync.Mutex
	recorder Recorder

	ctx     TurnContext
	turnID  string
	traceID string

	start  time.Time
	seq    uint32
	closed bool
}

func NewTurnTracer(recorder Recorder, ctx TurnContext) TurnTracer {
	turnID := newTurnID()
	return &turnTracer{
		recorder: recorder,
		ctx:      ctx,
		turnID:   turnID,
		traceID:  turnID,
		start:    time.Now(),
	}
}

func newTurnID() string {
	return idgen.New("turn")
}

func (t *turnTracer) Emit(name EventName, data EventData) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}

	t.recordLocked(name, "", "", nil, data)
}

func (t *turnTracer) Complete(data EventData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	t.recordLocked(EventTurnCompleted, "", "", nil, data)
	t.closed = true
}

func (t *turnTracer) Fail(stage string, reason string, err error, data EventData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	t.recordLocked(EventTurnFailed, stage, reason, err, data)
	t.closed = true
}

// recorder.Record 必须是非阻塞 Observer；recordLocked 会在持有 turn 锁时调用它。
// 如果未来实现同步 IO recorder，应先改为锁外 Record。
func (t *turnTracer) recordLocked(
	name EventName,
	stage string,
	reason string,
	err error,
	data EventData,
) {
	t.seq++

	now := time.Now()
	event := Event{
		SchemaVersion: 1,
		TraceID:       t.traceID,
		TurnID:        t.turnID,
		Seq:           t.seq,
		Event:         name,
		Time:          now,
		ElapsedMS:     now.Sub(t.start).Milliseconds(),

		GameID:    t.ctx.GameID,
		SaveID:    t.ctx.SaveID,
		SessionID: t.ctx.SessionID,
		EventID:   t.ctx.EventID,
		EventType: t.ctx.EventType,
		EntityID:  t.ctx.EntityID,

		ActionID: data.ActionID,
		Tool:     data.Tool,

		Stage:  stage,
		Reason: reason,
		Fields: data.Fields,
	}

	if err != nil {
		event.ErrorMessage = err.Error()
	}

	if t.recorder != nil {
		t.recorder.Record(event)
	}
}
