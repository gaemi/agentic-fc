package sim

import "testing"

// The queue must drain in the total order (Due, Priority, Kind, EntityID, Seq)
// regardless of insertion order — NFR-2 queue determinism.
func TestQueueTotalOrder(t *testing.T) {
	q := NewQueue()

	// Deliberately shuffled inserts, several sharing Due=100.
	q.Schedule(&Event{Due: 200, Priority: PriorityDrift, Kind: KindPlayer, EntityID: 1})
	q.Schedule(&Event{Due: 100, Priority: PriorityDrift, Kind: KindPlayer, EntityID: 7})
	q.Schedule(&Event{Due: 100, Priority: PriorityMatch, Kind: KindMatch, EntityID: 3})
	q.Schedule(&Event{Due: 100, Priority: PriorityDrift, Kind: KindPlayer, EntityID: 2})
	q.Schedule(&Event{Due: 100, Priority: PriorityWorld, Kind: KindWorld, EntityID: 0})
	q.Schedule(&Event{Due: 50, Priority: PriorityDrift, Kind: KindClub, EntityID: 9})

	type key struct {
		due  GameTime
		prio PriorityClass
		kind EntityKind
		id   int64
	}
	want := []key{
		{50, PriorityDrift, KindClub, 9},
		{100, PriorityWorld, KindWorld, 0},
		{100, PriorityMatch, KindMatch, 3},
		{100, PriorityDrift, KindPlayer, 2},
		{100, PriorityDrift, KindPlayer, 7},
		{200, PriorityDrift, KindPlayer, 1},
	}
	for i, w := range want {
		e := q.Pop()
		if e == nil {
			t.Fatalf("pop %d: queue exhausted early", i)
		}
		got := key{e.Due, e.Priority, e.Kind, e.EntityID}
		if got != w {
			t.Fatalf("pop %d: got %+v, want %+v", i, got, w)
		}
	}
	if q.Pop() != nil {
		t.Fatal("queue should be empty")
	}
}

// Two identical events must drain in schedule order (Seq tie-breaker).
func TestQueueSeqTieBreak(t *testing.T) {
	q := NewQueue()
	a := &Event{Due: 10, Priority: PriorityDrift, Kind: KindPlayer, EntityID: 5, Payload: "first"}
	b := &Event{Due: 10, Priority: PriorityDrift, Kind: KindPlayer, EntityID: 5, Payload: "second"}
	q.Schedule(a)
	q.Schedule(b)
	if got := q.Pop().Payload; got != "first" {
		t.Fatalf("got %v, want first", got)
	}
	if got := q.Pop().Payload; got != "second" {
		t.Fatalf("got %v, want second", got)
	}
}
