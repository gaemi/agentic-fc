package sim

import "container/heap"

// PriorityClass breaks ties between events due at the same GameTime.
// Lower drains first. Part of the total order required by NFR-2
// (docs/03-simulation-engine.md §5 "Queue determinism").
type PriorityClass uint8

const (
	PriorityWorld     PriorityClass = iota // season rollover, window open/close
	PriorityMatch                          // kickoffs, in-match moments
	PriorityDecision                       // Manager decision points
	PriorityCondition                      // injuries, recovery
	PriorityDrift                          // attribute drift, finance ticks
)

// EntityKind is the third tie-breaker component.
type EntityKind uint8

const (
	KindWorld EntityKind = iota
	KindMatch
	KindClub
	KindManager
	KindPlayer
)

// Event is a scheduled future occurrence (docs/00-glossary.md "Event").
type Event struct {
	Due      GameTime
	Priority PriorityClass
	Kind     EntityKind
	EntityID int64
	Seq      uint64 // assigned by the queue at schedule time; final tie-breaker
	Payload  any
}

// less implements the total order: (Due, Priority, Kind, EntityID, Seq).
// Tempo changes and pauses re-pace the queue but never reorder it.
func less(a, b *Event) bool {
	if a.Due != b.Due {
		return a.Due < b.Due
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.EntityID != b.EntityID {
		return a.EntityID < b.EntityID
	}
	return a.Seq < b.Seq
}

type eventHeap []*Event

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return less(h[i], h[j]) }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)        { *h = append(*h, x.(*Event)) }
func (h *eventHeap) Pop() any          { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

// Queue is the world's event queue. Not safe for concurrent use: the
// Simulation Core is a single-writer authority (docs/05-architecture.md A1).
type Queue struct {
	h       eventHeap
	nextSeq uint64
}

func NewQueue() *Queue { return &Queue{} }

// Schedule assigns the event its Seq and enqueues it.
func (q *Queue) Schedule(e *Event) {
	e.Seq = q.nextSeq
	q.nextSeq++
	heap.Push(&q.h, e)
}

// Peek returns the next due event without removing it, or nil.
func (q *Queue) Peek() *Event {
	if len(q.h) == 0 {
		return nil
	}
	return q.h[0]
}

// Pop removes and returns the next due event, or nil.
func (q *Queue) Pop() *Event {
	if len(q.h) == 0 {
		return nil
	}
	return heap.Pop(&q.h).(*Event)
}

func (q *Queue) Len() int { return len(q.h) }

// Snapshot copies out every queued event plus the next Seq for
// persistence (FR-28). Order is the internal heap layout — complete but
// arbitrary; the total order restores it.
func (q *Queue) Snapshot() ([]Event, uint64) {
	events := make([]Event, len(q.h))
	for i, e := range q.h {
		events[i] = *e
	}
	return events, q.nextSeq
}

// RestoreQueue rebuilds a queue from a snapshot. Events keep their
// original Seq values (they are part of the NFR-2 total order), so the
// drain order is identical no matter what order the snapshot listed.
func RestoreQueue(events []Event, nextSeq uint64) *Queue {
	q := &Queue{nextSeq: nextSeq}
	for i := range events {
		e := events[i]
		heap.Push(&q.h, &e)
	}
	return q
}
