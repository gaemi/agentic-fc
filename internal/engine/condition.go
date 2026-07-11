package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Condition recovery is a daily world event rather than part of the much
// slower, jittered attribute-drift cycle. One event updates the fixed player
// slice in world order, keeping the queue compact and single-writer ordering
// deterministic.
const conditionRecoveryPerDay = 14

func (e *Engine) handleConditionRecovery(ev *sim.Event) error {
	recovered := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Retired || p.Condition >= worldgen.ConditionMax {
			continue
		}
		p.Condition = clampInt(p.Condition+conditionRecoveryPerDay, 0, worldgen.ConditionMax)
		recovered++
	}
	next := ev.Due + sim.MinutesPerDay
	e.reschedule(ev, next)
	return e.log(ev, "condition", map[string]any{"players_recovered": recovered}, "daily_recovery", next, 0)
}

// ensureConditionRecoveryTick migrates snapshots written before the daily
// recovery event existed. It is idempotent and schedules only the next future
// midnight, so resuming never drains a backlog of synthetic past events.
func (e *Engine) ensureConditionRecoveryTick() {
	events, _ := e.queue.Snapshot()
	for i := range events {
		if events[i].Payload == worldgen.PayloadConditionTick {
			return
		}
	}
	next := (e.now/sim.MinutesPerDay + 1) * sim.MinutesPerDay
	e.queue.Schedule(&sim.Event{
		Due:      next,
		Priority: sim.PriorityCondition,
		Kind:     sim.KindWorld,
		Payload:  worldgen.PayloadConditionTick,
	})
}
