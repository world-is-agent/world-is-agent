package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const DefaultQueueSize = 1

var (
	ErrLaneClosed = errors.New("execution lane is closed")
	ErrLaneFull   = errors.New("execution lane queue is full")
)

type AbortReason string

const AbortReasonConnectionClosed AbortReason = "connection_closed"

type Task struct {
	ID       string
	Admitted <-chan struct{}
	Run      func(context.Context)
	Abort    func(AbortReason)
}

type ExecutionLane struct {
	ctx    context.Context
	cancel context.CancelFunc

	queue chan Task
	done  chan struct{}

	mu     sync.Mutex
	closed bool
}

func NewExecutionLane(parent context.Context, queueSize int) (*ExecutionLane, error) {
	if queueSize <= 0 {
		return nil, fmt.Errorf("queue size must be positive")
	}
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	lane := &ExecutionLane{
		ctx:    ctx,
		cancel: cancel,
		queue:  make(chan Task, queueSize),
		done:   make(chan struct{}),
	}

	go lane.run()

	return lane, nil
}

func (l *ExecutionLane) Enqueue(task Task) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrLaneClosed
	}

	select {
	case l.queue <- task:
		return nil
	default:
		return ErrLaneFull
	}
}

func (l *ExecutionLane) Close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	l.mu.Unlock()

	l.cancel()
}

func (l *ExecutionLane) Done() <-chan struct{} {
	return l.done
}

func (l *ExecutionLane) run() {
	defer func() {
		l.closeAdmission()
		l.drainQueued(AbortReasonConnectionClosed)
		close(l.done)
	}()

	for {
		select {
		case <-l.ctx.Done():
			return
		case task := <-l.queue:
			if l.ctx.Err() != nil {
				task.abort(AbortReasonConnectionClosed)
				return
			}
			if !l.waitAdmitted(task) {
				task.abort(AbortReasonConnectionClosed)
				return
			}
			if l.ctx.Err() != nil {
				task.abort(AbortReasonConnectionClosed)
				return
			}
			task.run(l.ctx)
		}
	}
}

func (l *ExecutionLane) waitAdmitted(task Task) bool {
	if task.Admitted == nil {
		return true
	}

	select {
	case <-task.Admitted:
		return true
	case <-l.ctx.Done():
		return false
	}
}

func (l *ExecutionLane) drainQueued(reason AbortReason) {
	for {
		select {
		case task := <-l.queue:
			task.abort(reason)
		default:
			return
		}
	}
}

func (l *ExecutionLane) closeAdmission() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
}

func (t Task) run(ctx context.Context) {
	if t.Run != nil {
		t.Run(ctx)
	}
}

func (t Task) abort(reason AbortReason) {
	if t.Abort != nil {
		t.Abort(reason)
	}
}

type LaneStore struct {
	ctx context.Context

	queueSize int
	lanes     map[AgentSessionKey]*ExecutionLane

	mu     sync.Mutex
	closed bool
}

func NewLaneStore(parent context.Context, queueSize int) (*LaneStore, error) {
	if queueSize <= 0 {
		return nil, fmt.Errorf("queue size must be positive")
	}
	if parent == nil {
		parent = context.Background()
	}

	return &LaneStore{
		ctx:       parent,
		queueSize: queueSize,
		lanes:     make(map[AgentSessionKey]*ExecutionLane),
	}, nil
}

func (s *LaneStore) GetOrCreate(key AgentSessionKey) (*ExecutionLane, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrLaneClosed
	}

	if lane := s.lanes[key]; lane != nil {
		return lane, nil
	}

	lane, err := NewExecutionLane(s.ctx, s.queueSize)
	if err != nil {
		return nil, err
	}
	s.lanes[key] = lane
	return lane, nil
}

func (s *LaneStore) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true

	lanes := make([]*ExecutionLane, 0, len(s.lanes))
	for _, lane := range s.lanes {
		lanes = append(lanes, lane)
	}
	s.mu.Unlock()

	for _, lane := range lanes {
		lane.Close()
	}
}
