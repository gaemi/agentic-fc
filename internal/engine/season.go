package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// handleSeasonEnd runs the season rollover. The PayloadSeasonEnd
// event fires in the off-season (after May's last round), so no match is live
// and the rollover has the world to itself. It mutates World through the pure
// RolloverSeason (promotion/relegation, archive, stat reset, new fixtures),
// rebuilds the fixture caches to exactly what New() would build, primes the new
// season's kickoffs and calendar events, and announces the turn of the season.
func (e *Engine) handleSeasonEnd(ev *sim.Event) error {
	// The event at day N×365 marks the start of season N+1: DateOf gives that
	// new season number, and the rollover generates its fixtures.
	newSeason := worldgen.DateOf(ev.Due).Season

	// Manager count BEFORE this boundary spawns any caretaker or newgen: only these
	// existing managers age and roll retirement. Captured up front,
	// before ultimata append their caretakers, so the retirement pass never touches
	// a manager installed this same tick.
	managersAtSeasonEnd := len(e.world.Managers)

	// Archive every player's season line FIRST — before retirement or a
	// contract lapse zeroes a departing player's ClubID, so each career record
	// keeps the club it was earned at.
	worldgen.ArchivePlayerSeasons(e.world, newSeason-1)

	// Settle any pending ultimatum against the FINAL table BEFORE the rollover
	// zeroes it, so a deadline can never straddle the season boundary and sack a
	// manager on a stale cross-season points comparison.
	e.resolveEndOfSeasonUltimata(ev)

	// Manager population dynamics (manager careers): birthdays, age-driven retirement, and
	// newgen backfill. Runs after ultimata (so a just-sacked manager is settled) and
	// before the rollover; draws labelled streams independent of the rollover dice.
	e.processManagerCareers(ev, newSeason, managersAtSeasonEnd)

	// Player population dynamics (player lifecycle): birthdays, youth graduation,
	// age-driven retirement. Before the rollover so retirement-shed wages are out
	// of the bill cache when the new season's budgets derive from it (careers E).
	e.processPlayerCareers(ev, newSeason)

	// Contract expiries: renew-or-lapse every deal ending with the
	// finished season. After the player pass (a retiree's contract is already
	// gone) and before the rollover (repriced/shed wages must precede budget
	// derivation; DivisionTier is still the finished season's tier here).
	e.processContractExpiries(ev, newSeason-1)

	// Stateless, season-specific stream (world/0/season_rollover@<due>): each
	// season draws independent dice, and a replay reproduces them exactly.
	r := e.rollStream(ev)
	e.world.RolloverSeason(r, newSeason)
	e.buildFixtureIndex()
	e.primeSeasonEvents(newSeason)

	e.emitCalendar(ev.Due, worldgen.PayloadSeasonEnd)
	e.issueCalendarAlerts(ev.Due, "SEASON_ENDED")
	if key, params := calendarKeyParams(ev.Due, worldgen.PayloadSeasonEnd); key != "" {
		e.addNews(worldgen.NewsItem{
			GameTime: ev.Due, Category: "board", Key: key, Params: params,
		})
	}
	return e.log(ev, "world", map[string]any{"season": newSeason}, "rollover", 0, 0)
}

// primeSeasonEvents schedules the new season's kickoffs and calendar events
// (window edges + the next season-end). Entity ticks self-reschedule and carry
// over untouched, so only these non-self-scheduling events need re-priming. The
// order is fixed (fixtures in slice order, then calendar) so queue sequence
// numbers are deterministic (NFR-2).
func (e *Engine) primeSeasonEvents(season int) {
	for i := range e.world.Fixtures {
		e.queue.Schedule(&sim.Event{
			Due:      e.world.Fixtures[i].Kickoff,
			Priority: sim.PriorityMatch,
			Kind:     sim.KindMatch,
			EntityID: e.world.Fixtures[i].ID,
			Payload:  worldgen.PayloadKickoff,
		})
	}
	for _, ce := range worldgen.SeasonCalendarEvents(season) {
		e.queue.Schedule(&sim.Event{
			Due:      ce.Due,
			Priority: sim.PriorityWorld,
			Kind:     sim.KindWorld,
			Payload:  ce.Payload,
		})
	}
	// Youth intake, re-primed for the new season (youth intake). Club.YouthIntakeDay was
	// re-rolled by RolloverSeason, so this schedules the fresh spring days. Clubs in
	// slice order matches generation's primeQueue, keeping queue Seq deterministic.
	for i := range e.world.Clubs {
		e.queue.Schedule(&sim.Event{
			Due:      worldgen.YouthIntakeDue(&e.world.Clubs[i], season),
			Priority: sim.PriorityDrift,
			Kind:     sim.KindClub,
			EntityID: e.world.Clubs[i].ID,
			Payload:  worldgen.PayloadYouthIntake,
		})
	}
}
