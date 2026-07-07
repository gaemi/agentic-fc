package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Club finance tick (docs/03 §3 "Club"): weekly revenue lands, wages leave,
// with modest gate variance. Cadence is weekly (registered in docs/98).
const financeTickDays = 7

func (e *Engine) handleFinanceTick(ev *sim.Event) error {
	c, ok := e.clubs[ev.EntityID]
	if !ok {
		return e.log(ev, "finance", nil, "unknown_club", 0, 0)
	}
	r := e.rollStream(ev)

	revenue := worldgen.ClubWeeklyRevenueMinor(e.world.Config, c)
	// Gate & merchandising variance: ±10%.
	revenue = revenue * int64(900+r.Int64N(201)) / 1000
	delta := revenue - c.WageBillWeeklyMinor
	c.BalanceMinor += delta

	outcome := "surplus"
	if delta < 0 {
		outcome = "deficit"
	}
	next := ev.Due + sim.GameTime(financeTickDays*sim.MinutesPerDay)
	e.reschedule(ev, next)
	return e.log(ev, "finance", map[string]any{
		"revenue_minor": revenue,
		"wages_minor":   c.WageBillWeeklyMinor,
		"balance_minor": c.BalanceMinor,
	}, outcome, next, 0)
}
