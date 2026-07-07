package engine

import (
	"fmt"
	"math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Manager hiring / the job market. A club whose job is filled by
// an auto-installed Caretaker (FR-14d) is a live VACANCY: the board interviews the
// pool of unemployed managers and, when it settles, either appoints one — the
// managerial merry-go-round (FR-14a) — or confirms the caretaker permanently. The
// binding follows the MANAGER (FR-20c): a hired manager keeps its token and its
// EXISTING decision roll (every ACTIVE manager carries exactly one from primeQueue,
// unemployed ones firing harmlessly at ClubID==0), so hiring only flips ClubID — it
// never schedules a second roll (that would double a manager's cadence and diverge
// determinism, NFR-2).
//
// Evaluated on the caretaker's own decision roll (the vacancy's natural cadence) and
// resolved BEFORE the roll's normal managerial work, so a caretaker replaced by an
// external hire can retire and end its roll chain cleanly. Fully deterministic: a
// labelled stream keyed on club + due, independent of the decision-roll dice, so a
// resumed or retempo'd run resolves the identical vacancy (NFR-2). Contention between
// two vacant clubs eyeing the same manager is settled by the queue's total order —
// the first roll claims them (ClubID set), the later roll's candidate scan skips them
// — the same first-roll-wins model as listed-only transfers.

// Hiring tunables (initial values; docs/98).
const (
	hireReviewChance = 25   // % chance a caretaker's review settles the open vacancy
	hireRepFloorTop  = 3000 // reputation a tier-1 vacancy demands of a candidate
	hireRepFloorStep = 700  // floor reduction per division tier below the top
)

// resolveVacancy runs the job market for a caretaker-run club on the caretaker's
// decision roll. It returns true ONLY when the caretaker was REPLACED by an external
// hire (and thus retired) — the caller must then end the caretaker's roll chain
// without rescheduling. A convert-in-place (caretaker made permanent) or a no-op
// (board still interviewing) returns false; the caretaker carries on as a normal
// manager on its usual cadence.
func (e *Engine) resolveVacancy(ev *sim.Event, caretaker *worldgen.Manager) bool {
	club := e.clubs[caretaker.ClubID]
	if club == nil {
		return false
	}
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("career/hire/%d@%d", club.ID, int64(ev.Due)))

	// The board doesn't fill the job every review — it interviews over weeks.
	if r.IntN(100) >= hireReviewChance {
		return false
	}

	hire := e.pickHireCandidate(club, r)
	if hire == nil {
		// No unemployed manager clears the club's reputation bar → the caretaker has
		// earned the job (FR-14d: "occasionally a caretaker earns the job permanently").
		caretaker.Caretaker = false
		e.boardNewsManager(ev.Due, club, caretaker.Name, "news.board.caretaker_permanent")
		return false
	}

	// External appointment: the manager moves in (keeping its token + existing roll),
	// the displaced caretaker retires (bounded population — a caretaker is never
	// pooled), and the board's pick starts on a fresh honeymoon.
	hireName := hire.Name // capture before any mutation; no realloc here, but mirror sacking
	hire.ClubID = club.ID
	caretaker.Status = worldgen.ManagerRetired
	caretaker.ClubID = 0
	club.Confidence = caretakerHoneymoon
	e.clearUltimatum(club)
	e.boardNewsManager(ev.Due, club, hireName, "news.board.appointed")
	return true
}

// pickHireCandidate selects an unemployed ACTIVE manager for a vacant club, weighted
// by reputation and gated by the club's tier (higher divisions demand more standing).
// Deterministic: managers are scanned in World.Managers order (append-only, so
// ascending id), and the weighted roll breaks ties by that order (NFR-2). Returns nil
// when no unemployed manager clears the bar.
func (e *Engine) pickHireCandidate(club *worldgen.Club, r *rand.Rand) *worldgen.Manager {
	floor := hireRepFloor(club.DivisionTier)
	total := 0
	var eligible []*worldgen.Manager
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		// Unemployed (ClubID==0) and ACTIVE (empty status reads as ACTIVE; only RETIRED
		// is excluded) and reputable enough for this tier.
		if m.ClubID != 0 || m.Status == worldgen.ManagerRetired || m.Reputation < floor {
			continue
		}
		eligible = append(eligible, m)
		total += m.Reputation
	}
	if len(eligible) == 0 {
		return nil
	}
	roll := r.IntN(total)
	for _, m := range eligible {
		if roll < m.Reputation {
			return m
		}
		roll -= m.Reputation
	}
	return eligible[len(eligible)-1] // unreachable: weights sum to total
}

// hireRepFloor is the reputation an unemployed manager needs to be considered for a
// vacancy in the given division tier — tier 1 the most demanding (tunable, docs/98).
func hireRepFloor(tier int) int {
	floor := hireRepFloorTop - hireRepFloorStep*(tier-1)
	if floor < 0 {
		return 0
	}
	return floor
}
