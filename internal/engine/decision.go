package engine

import (
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Manager decision roll — the standing review cadence. The decision
// machinery itself (selection, transfers, contract calls) arrives with
// later systems; this phase establishes the cadence, the Mindset-version
// stamping (FR-16e), and the audit shape every future decision reuses.
const (
	decisionIntervalMinDays  = 2
	decisionIntervalSpanDays = 4

	// Disposition drift (FR-16b): ~2 pts/game-week ⇒ one point per axis
	// per half-week of accumulated credit. Applied on the decision
	// cadence — no separate event chain to duplicate.
	driftMinutesPerPoint = int64(7 * sim.MinutesPerDay / 2)
)

// ApplyDispositionDrift converts game time elapsed since the manager's
// drift anchor into whole drift points (remainder carries as credit
// minutes) and re-anchors at now. Shared by the decision cadence and the
// gateway's update_disposition (which must apply accrued drift BEFORE
// re-targeting — otherwise repeated calls would erase partial progress).
func ApplyDispositionDrift(m *worldgen.Manager, now sim.GameTime) int {
	if len(m.Mindset.Disposition.Target) == 0 {
		m.DriftAnchor = now
		m.DriftCreditMinutes = 0
		return 0
	}
	credit := m.DriftCreditMinutes + int64(now-m.DriftAnchor)
	moved := 0
	for credit >= driftMinutesPerPoint && len(m.Mindset.Disposition.Target) > 0 {
		credit -= driftMinutesPerPoint
		moved += m.Mindset.DriftDispositionStep()
	}
	m.DriftCreditMinutes = credit
	m.DriftAnchor = now
	return moved
}

func (e *Engine) handleDecisionRoll(ev *sim.Event) error {
	m, ok := e.managers[ev.EntityID]
	if !ok {
		return e.log(ev, "decision", nil, "unknown_manager", 0, 0)
	}
	// A RETIRED manager no longer reviews, and we do NOT reschedule — so its roll
	// chain terminates here (careers: a caretaker displaced by a hire below, and slice
	// C retirements later). Defensive against a stray queued roll; the displaced
	// caretaker path early-returns directly.
	if m.Status == worldgen.ManagerRetired {
		return e.log(ev, "decision", map[string]any{"club_id": m.ClubID}, "retired", 0, m.Mindset.Version)
	}
	// A caretaker runs a live vacancy (hiring): the board may fill it this review.
	// Resolve it FIRST — an external appointment retires the caretaker, so its roll
	// chain must end here (no reschedule) while the hired manager keeps its own roll.
	// A convert-in-place or no-op returns false: the caretaker carries on below.
	if m.Caretaker {
		filled := m.ClubID // capture before an external hire clears the caretaker's ClubID
		if e.resolveVacancy(ev, m) {
			return e.log(ev, "decision", map[string]any{"club_id": filled}, "caretaker_replaced", 0, m.Mindset.Version)
		}
	}
	r := e.rollStream(ev)

	outcome := "reviewed_no_action"
	if ApplyDispositionDrift(m, ev.Due) > 0 {
		outcome = "disposition_drift"
		// Manager decisions surface in the news with their Mindset
		// version (FR-16e, docs/11 §4).
		item := worldgen.NewsItem{
			GameTime: ev.Due, Category: "decision", Key: "news.decision.shift",
			Params: map[string]any{
				"manager":         m.Name,
				"club":            e.clubName(m.ClubID),
				"mindset_version": m.Mindset.Version,
			},
		}
		if m.ClubID != 0 {
			item.ClubIDs = []int64{m.ClubID}
		}
		e.addNews(item)
	}

	// Transfer window open → a signing may complete here. Explicit SIGN
	// directives take priority; failing those, an autonomous squad-need buy fires
	// for a club below its target (slice 2b). One transaction per roll; both use
	// their own paths, so the reschedule dice below are unchanged.
	if e.considerSignings(ev, m) {
		outcome = "signing"
	} else if e.assessSquadNeeds(ev, m) {
		outcome = "auto_signing"
	}

	next := ev.Due +
		sim.GameTime(int64(decisionIntervalMinDays)*sim.MinutesPerDay) +
		sim.GameTime(r.Int64N(int64(decisionIntervalSpanDays)*sim.MinutesPerDay))
	e.reschedule(ev, next)
	return e.log(ev, "decision", map[string]any{
		"club_id": m.ClubID,
	}, outcome, next, m.Mindset.Version)
}
