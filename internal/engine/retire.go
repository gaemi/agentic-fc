package engine

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Manager population dynamics. At each season boundary every
// existing manager has a birthday and then rolls age-driven retirement (FR-14e),
// and the unemployed pool is topped back up with newly generated managers so the
// job market never runs dry (docs/02 §2.4, §3.1 — "the merry-go-round never runs
// dry"). Retirement is exempt for a manager whose bound Agent has been active
// recently: the exemption lapses 2 game-years after the last session on the token.
//
// The engine is deterministic and cannot see live gateway sessions, so the
// liveness signal is a per-manager GAME-time stamp (Manager.LastActiveGameTime)
// that the gateway writes on each accepted authenticated call. In a
// zero-agent world that field is never written — it stays zero, the sentinel for
// "never active ⇒ autonomous ⇒ never exempt" — so retirement always fires. Every
// draw here is a labelled stream keyed on manager id / club / season, independent
// of the rollover dice, so a resumed or retempo'd run reproduces the identical
// population (NFR-2). All of it runs inside the season-end handler, in the queue's
// total order.

// Retirement age curve + the avatar grace period (tunable, docs/98). Below the
// floor a manager never retires; at or above the ceiling it is certain; between,
// the chance rises linearly. Integer-only — no float on the world hash (NFR-2).
const (
	retireAgeFloor = 60 // below this age a manager never retires
	retireAgeCeil  = 72 // at/above this age retirement is certain
)

// retirementExemptionWindow is FR-14e's avatar grace period: a manager whose bound
// Agent has made an authenticated call within this much GAME time is exempt from
// retirement. A game-year is one season (docs/02), so 2 game-years is two seasons.
const retirementExemptionWindow = sim.GameTime(int64(2*worldgen.DaysPerSeason) * sim.MinutesPerDay)

// retirementChance is the percentage chance a manager of the given age retires this
// season, clamped to [0, 100] by the floor/ceiling.
func retirementChance(age int) int {
	if age < retireAgeFloor {
		return 0
	}
	if age >= retireAgeCeil {
		return 100
	}
	return (age - retireAgeFloor) * 100 / (retireAgeCeil - retireAgeFloor)
}

// processManagerCareers advances the manager population at the season boundary. It
// runs AFTER end-of-season ultimata (so a just-sacked manager is already in its
// settled state) and BEFORE RolloverSeason. `n` is the manager count captured at
// season end, before this tick spawned any caretaker or newgen: only indices
// [0, n) are the managers present at boundary start — anything appended here is
// brand new and is neither aged nor retirement-rolled this tick, so no manager is
// ever processed twice in one boundary.
func (e *Engine) processManagerCareers(ev *sim.Event, season, n int) {
	for i := 0; i < n; i++ {
		e.world.Managers[i].Age++
	}
	// Each manager rolls independently on its own id-keyed stream, so
	// the outcome is order-free and reproduces regardless of how many others retire
	// first. Retiring an EMPLOYED manager appends a caretaker (beyond n) and may
	// reallocate World.Managers, so re-fetch by index each iteration.
	for i := 0; i < n; i++ {
		m := &e.world.Managers[i]
		if m.Status == worldgen.ManagerRetired {
			continue // already gone this tick (e.g. a caretaker sacked by ultimatum)
		}
		if e.avatarExempt(m, ev.Due) {
			continue
		}
		if e.rollsRetirement(m, season) {
			e.retireManager(ev, m)
		}
	}
	e.backfillManagerPool(ev, season)
	e.rebuildManagerIndex() // single rebuild after all appends (retirements + newgen)
}

// avatarExempt reports whether a live Agent binding shields the manager from
// retirement. Zero LastActiveGameTime means the token has never carried an
// authenticated call (autonomous) → never exempt; otherwise the manager is exempt
// until FR-14e's grace period lapses. The zero check is load-bearing: without it,
// early in the world (now < the window) every autonomous manager would spuriously
// satisfy `now - 0 < window` and nothing would ever retire.
func (e *Engine) avatarExempt(m *worldgen.Manager, now sim.GameTime) bool {
	return m.LastActiveGameTime != 0 && now-m.LastActiveGameTime < retirementExemptionWindow
}

// rollsRetirement decides whether a manager retires this season on its own labelled
// stream (career/retire/<id>@<season>), independent of every other roll.
func (e *Engine) rollsRetirement(m *worldgen.Manager, season int) bool {
	chance := retirementChance(m.Age)
	if chance == 0 {
		return false
	}
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("career/retire/%d@%d", m.ID, season))
	return r.IntN(100) < chance
}

// retireManager retires m: it is marked RETIRED (its token then rejects calls with
// MANAGER_RETIRED and Focus regen stops, FR-14e — enforced at the gateway; its next
// decision roll hits the RETIRED guard and terminates the chain). An EMPLOYED
// retiree additionally leaves a vacancy that an installed caretaker fills at once
// (FR-14d, never unmanaged), mirroring a sacking. The manager's fields are mutated
// BEFORE installCaretaker's spawn may reallocate World.Managers (the pointer then
// goes stale — the same capture-first discipline as sackManager). The caller
// rebuilds the manager index.
func (e *Engine) retireManager(ev *sim.Event, m *worldgen.Manager) {
	name := m.Name
	clubID := m.ClubID
	m.Status = worldgen.ManagerRetired
	m.ClubID = 0
	if clubID == 0 {
		e.managerNews(ev.Due, name, "news.board.retired")
		return
	}
	club := e.clubs[clubID]
	caretakerName := e.installCaretaker(ev, club)
	e.boardNewsManager(ev.Due, club, name, "news.board.retired")
	e.boardNewsManager(ev.Due, club, caretakerName, "news.board.caretaker")
}

// backfillManagerPool tops the unemployed pool back up to its target size with
// newgen managers so the job market never dries up as clubs hire and managers
// retire. Newgen enter unemployed (ClubID 0) at a rolled tier with full reputation
// for that tier — no unemployment haircut, so fresh entrants reliably clear the
// hiring reputation floors (hiring) and tier-1 vacancies don't all default to a
// permanent caretaker. Each newgen draws its own labelled stream and gets a
// staggered first decision roll, so a later hire finds the "exactly one roll per
// manager" invariant intact.
func (e *Engine) backfillManagerPool(ev *sim.Event, season int) {
	target := worldgen.UnemployedPoolTarget(e.world)
	have := 0
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.ClubID == 0 && m.Status != worldgen.ManagerRetired && !m.Caretaker {
			have++
		}
	}
	for k := 0; have+k < target; k++ {
		r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("career/newgen/%d/%d", season, k))
		tier := 1 + r.IntN(e.world.Config.Divisions)
		m := worldgen.SpawnManager(e.world, r, 0, tier, false)
		// Stagger the first roll off the same stream so day-one after intake isn't a
		// thundering herd (mirrors generation's primeQueue).
		e.scheduleDecisionRoll(m.ID, ev.Due+sim.GameTime(r.Int64N(3*sim.MinutesPerDay)))
	}
}
