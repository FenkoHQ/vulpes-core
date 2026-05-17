package observability

import (
	"context"
	"sync"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

type Queue struct {
	mu        sync.Mutex
	events    []capabilities.GatewayEvent
	cap       int
	batch     int
	interval  time.Duration
	overflow  string
	observers []capabilities.Observer
	closed    chan struct{}
	done      chan struct{}
}

func NewQueue(size, batch int, interval time.Duration, overflow string, observers []capabilities.Observer) *Queue {
	if size <= 0 {
		size = 10000
	}
	if batch <= 0 {
		batch = 100
	}
	if interval <= 0 {
		interval = time.Second
	}
	q := &Queue{cap: size, batch: batch, interval: interval, overflow: overflow, observers: observers, closed: make(chan struct{}), done: make(chan struct{})}
	go q.run()
	return q
}

func (q *Queue) Enqueue(ev capabilities.GatewayEvent) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.events) >= q.cap {
		switch q.overflow {
		case "block", "fail":
			return false
		default:
			copy(q.events, q.events[1:])
			q.events[len(q.events)-1] = ev
			return true
		}
	}
	q.events = append(q.events, ev)
	return true
}

func (q *Queue) Close(ctx context.Context) error {
	close(q.closed)
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Queue) run() {
	defer close(q.done)
	t := time.NewTicker(q.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			q.flush(context.Background())
		case <-q.closed:
			q.flush(context.Background())
			return
		}
	}
}

func (q *Queue) flush(ctx context.Context) {
	for {
		batch := q.takeBatch()
		if len(batch) == 0 {
			return
		}
		for _, obs := range q.observers {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = obs.Emit(ctx, batch)
			cancel()
		}
	}
}

func (q *Queue) takeBatch() []capabilities.GatewayEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.events) == 0 {
		return nil
	}
	n := min(q.batch, len(q.events))
	out := make([]capabilities.GatewayEvent, n)
	copy(out, q.events[:n])
	q.events = append(q.events[:0], q.events[n:]...)
	return out
}
