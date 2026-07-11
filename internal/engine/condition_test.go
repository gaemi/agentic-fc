package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func TestConditionRecoveryRunsDaily(t *testing.T) {
	w := &worldgen.World{Players: []worldgen.Player{
		{ID: 1, Condition: 40},
		{ID: 2, Condition: 95},
		{ID: 3, Condition: 41, Retired: true},
	}}
	q := sim.NewQueue()
	q.Schedule(&sim.Event{Due: sim.MinutesPerDay, Priority: sim.PriorityCondition,
		Kind: sim.KindWorld, Payload: worldgen.PayloadConditionTick})
	e := New(w, q, &store.MemAuditLog{})

	ev, err := e.Step()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Payload != worldgen.PayloadConditionTick {
		t.Fatalf("handled %v, want condition recovery", ev.Payload)
	}
	if got := w.Players[0].Condition; got != 54 {
		t.Fatalf("condition recovered to %d, want 54", got)
	}
	if got := w.Players[1].Condition; got != worldgen.ConditionMax {
		t.Fatalf("condition did not clamp at 100: %d", got)
	}
	if got := w.Players[2].Condition; got != 41 {
		t.Fatalf("retired player condition moved to %d", got)
	}
	next := q.Peek()
	if next == nil || next.Payload != worldgen.PayloadConditionTick || next.Due != 2*sim.MinutesPerDay {
		t.Fatalf("next daily recovery = %+v", next)
	}
}

func TestResumeBackfillsOneFutureConditionRecovery(t *testing.T) {
	w := &worldgen.World{Players: []worldgen.Player{{ID: 1, Condition: 40}}}
	q := sim.NewQueue()
	e := New(w, q, &store.MemAuditLog{})
	now := sim.GameTime(10*sim.MinutesPerDay + 123)
	e.ResumeAt(now)
	e.ResumeAt(now)

	events, _ := q.Snapshot()
	if len(events) != 1 {
		t.Fatalf("backfill scheduled %d events, want one", len(events))
	}
	ev := events[0]
	if ev.Payload != worldgen.PayloadConditionTick || ev.Due != 11*sim.MinutesPerDay {
		t.Fatalf("backfilled event = %+v, want next midnight", ev)
	}
}
