package engine

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// Board sacking. A club's board loses patience as its live
// confidence falls, in a legible, always-announced escalation (FR-14b): OK →
// private WARNING → public ULTIMATUM ("gain N points by <date>") → dismissal, with
// a reprieve at any step if confidence recovers. Every transition is a news item —
// never an unheralded roll. Evaluated on the CLUB (managers move; board patience
// is a club property) right after each LEAGUE result, in the queue's total order,
// so it reproduces exactly on resume. A dismissal installs a caretaker in the same
// event, so `club.manager` is never null (FR-14d).

const (
	sackOK        = "" // the zero value reads as OK
	sackWarned    = "WARNED"
	sackUltimatum = "ULTIMATUM"
)

// Sacking thresholds + ultimatum terms (tunable, docs/98).
const (
	sackWarnThreshold      = 30 // confidence ≤ this from OK → private warning
	sackUltimatumThreshold = 20 // confidence ≤ this from WARNED → public ultimatum
	sackRecoverThreshold   = 45 // confidence ≥ this → reprieve back to OK
	ultimatumDays          = 42 // the deadline window
	ultimatumPointsTarget  = 7  // points to GAIN within the window to survive
	caretakerHoneymoon     = 55 // confidence a newly installed manager starts on
)

// evaluateSacking advances a club's sacking state after a league result.
func (e *Engine) evaluateSacking(ev *sim.Event, club *worldgen.Club) {
	switch club.SackState {
	case sackWarned:
		switch {
		case club.Confidence >= sackRecoverThreshold:
			club.SackState = sackOK
			e.boardNews(ev.Due, club, "news.board.reprieve")
		case club.Confidence <= sackUltimatumThreshold:
			club.SackState = sackUltimatum
			club.UltimatumUntil = ev.Due + sim.GameTime(ultimatumDays*sim.MinutesPerDay)
			club.UltimatumStartPoints = e.clubPoints(club)
			e.boardNews(ev.Due, club, "news.board.ultimatum")
		}
	case sackUltimatum:
		switch {
		case club.Confidence >= sackRecoverThreshold:
			e.clearUltimatum(club)
			e.boardNews(ev.Due, club, "news.board.reprieve")
		case ev.Due >= club.UltimatumUntil:
			if e.clubPoints(club)-club.UltimatumStartPoints >= ultimatumPointsTarget {
				e.clearUltimatum(club)
				e.boardNews(ev.Due, club, "news.board.reprieve")
			} else {
				e.sackManager(ev, club)
			}
		}
	default: // OK
		if club.Confidence <= sackWarnThreshold {
			club.SackState = sackWarned
			e.boardNews(ev.Due, club, "news.board.warning")
		}
	}
}

// resolveEndOfSeasonUltimata settles every pending ultimatum BEFORE the season
// rollover zeroes the table: the points target is measured against
// the FINAL standings, so an ultimatum can never straddle a table reset — which
// would compare fresh-season points against a prior-season snapshot and sack a
// manager who actually met the target. Called from handleSeasonEnd before
// RolloverSeason. After it, no club is left in ULTIMATUM, so no cross-season result
// ever re-evaluates a stale window.
func (e *Engine) resolveEndOfSeasonUltimata(ev *sim.Event) {
	for i := range e.world.Clubs {
		club := &e.world.Clubs[i]
		if club.SackState != sackUltimatum {
			continue
		}
		if e.clubPoints(club)-club.UltimatumStartPoints >= ultimatumPointsTarget {
			e.clearUltimatum(club)
			e.boardNews(ev.Due, club, "news.board.reprieve")
		} else {
			e.sackManager(ev, club)
		}
	}
}

func (e *Engine) clearUltimatum(club *worldgen.Club) {
	club.SackState = sackOK
	club.UltimatumUntil = 0
	club.UltimatumStartPoints = 0
}

// sackManager dismisses the club's current manager and installs a caretaker in the
// SAME event, so the club is never unmanaged. The dismissed manager stays in the
// world, unemployed — their token and Agent binding are untouched (FR-20c).
func (e *Engine) sackManager(ev *sim.Event, club *worldgen.Club) {
	old := e.clubManager(club.ID)
	if old == nil {
		return // invariant says this can't happen, but never panic
	}
	oldName := old.Name // capture before the spawn may reallocate World.Managers
	old.ClubID = 0      // unemployed; Status stays ACTIVE, token + binding preserved
	// A sacked CARETAKER retires instead of joining the hire pool. A caretaker's stint
	// only ever ends by earning its own club (convert-in-place) or leaving — and
	// resolveVacancy already retires a caretaker displaced by a hire, so a sacking is
	// just another way the stint ends. Left ACTIVE it would be an unemployed
	// `Caretaker==true` candidate; appointed elsewhere, its decision roll would treat
	// the NEW club as a live vacancy at the cross-system seam. Mutated here,
	// before SpawnManager's append copies `old` into the reallocated slice.
	if old.Caretaker {
		old.Status = worldgen.ManagerRetired
	}

	caretakerName := e.installCaretaker(ev, club)
	e.rebuildManagerIndex()

	e.boardNewsManager(ev.Due, club, oldName, "news.board.sacked")
	e.boardNewsManager(ev.Due, club, caretakerName, "news.board.caretaker")
}

// installCaretaker spawns a caretaker to run a club whose manager just departed —
// by sacking (A2) or retirement (C) — and lands the club in the "never unmanaged"
// state (FR-14d): board confidence reset to a honeymoon, any ultimatum cleared, and
// the caretaker's first decision roll scheduled (from which it self-reschedules like
// any manager). Deterministic: a labelled stream keyed on club + due (NFR-2). Returns
// the caretaker's NAME (a copy safe across a later reallocation) for the appointment
// news; the caller must rebuildManagerIndex, since SpawnManager's append may have
// reallocated World.Managers and invalidated every held *Manager.
func (e *Engine) installCaretaker(ev *sim.Event, club *worldgen.Club) string {
	r := rng.Stream(e.world.Config.Seed, fmt.Sprintf("career/caretaker/%d@%d", club.ID, int64(ev.Due)))
	caretaker := worldgen.SpawnManager(e.world, r, club.ID, club.DivisionTier, true)
	// A new manager gets a honeymoon: reset confidence + the sacking state so the
	// caretaker isn't instantly re-sacked on the confidence it inherited. (A
	// caretaker installed AT the season boundary loses the honeymoon to the
	// rollover's rebaseline moments later — SackState stays clean, so it is not
	// re-warned before results.
	club.Confidence = caretakerHoneymoon
	e.clearUltimatum(club)
	// The caretaker starts reviewing the day after appointment (deterministic — a
	// fixed offset, no dice).
	e.scheduleDecisionRoll(caretaker.ID, ev.Due+sim.GameTime(sim.MinutesPerDay))
	return caretaker.Name
}

// scheduleDecisionRoll queues a manager's next standing review. Every ACTIVE manager
// carries exactly one such roll (primed at generation, or here for a runtime-spawned
// caretaker/newgen), so the hiring path can flip a manager's club without adding a
// second (hiring).
func (e *Engine) scheduleDecisionRoll(managerID int64, due sim.GameTime) {
	e.queue.Schedule(&sim.Event{
		Due:      due,
		Priority: sim.PriorityDecision,
		Kind:     sim.KindManager,
		EntityID: managerID,
		Payload:  worldgen.PayloadDecisionRoll,
	})
}

// clubPoints is the club's current league points from the live table (0 if not
// found — e.g. before its first result).
func (e *Engine) clubPoints(club *worldgen.Club) int {
	if club.DivisionTier < 1 || club.DivisionTier > len(e.world.Table) {
		return 0
	}
	for _, s := range e.world.Table[club.DivisionTier-1] {
		if s.ClubID == club.ID {
			return s.Points
		}
	}
	return 0
}

// boardNews files a board statement about a club (news + Console feed).
func (e *Engine) boardNews(t sim.GameTime, club *worldgen.Club, key string) {
	params := map[string]any{"club": club.Name}
	e.addNews(worldgen.NewsItem{
		GameTime: t, Category: "board", Key: key, Params: params, ClubIDs: []int64{club.ID},
	})
	e.emit(t, key, cloneParams(params))
}

// boardNewsManager files a board statement naming a manager (dismissal, caretaker
// appointment). The manager name is passed by value — the manager pointer may be
// stale after a spawn reallocated World.Managers.
func (e *Engine) boardNewsManager(t sim.GameTime, club *worldgen.Club, managerName, key string) {
	params := map[string]any{"club": club.Name, "manager": managerName}
	e.addNews(worldgen.NewsItem{
		GameTime: t, Category: "board", Key: key, Params: params, ClubIDs: []int64{club.ID},
	})
	e.emit(t, key, cloneParams(params))
}

// managerNews files a statement naming only a manager, no club — an unemployed
// manager's retirement (manager careers), which has no club to attach.
func (e *Engine) managerNews(t sim.GameTime, managerName, key string) {
	params := map[string]any{"manager": managerName}
	e.addNews(worldgen.NewsItem{
		GameTime: t, Category: "board", Key: key, Params: params,
	})
	e.emit(t, key, cloneParams(params))
}
