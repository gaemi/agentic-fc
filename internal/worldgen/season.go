package worldgen

import (
	"fmt"
	"math/rand/v2"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// CalendarEvent is a world-calendar tick (window edge or season rollover) at a
// game time — shared by queue priming (season 1) and the season rollover
// (later seasons) so both schedule the same events for a given season.
type CalendarEvent struct {
	Due     sim.GameTime
	Payload string
}

// SeasonCalendarEvents returns the world-calendar events for a 1-based season:
// the transfer-window edges and the season-end rollover, shifted to that
// season. The season-end lands at the end of the season (day season×365),
// which is where the next rollover fires.
func SeasonCalendarEvents(season int) []CalendarEvent {
	shift := seasonShift(season)
	at := func(dayOfSeason int) sim.GameTime {
		return sim.GameTime(int64(dayOfSeason)*sim.MinutesPerDay) + shift
	}
	return []CalendarEvent{
		{at(daySummerWindowClose + 1), PayloadWindowClose},
		{at(dayWinterWindowOpen), PayloadWindowOpen},
		{at(dayWinterWindowClose + 1), PayloadWindowClose},
		{at(DaysPerSeason), PayloadSeasonEnd},
	}
}

// Season lifecycle. RolloverSeason is a pure function of the world at
// season end plus a stateless stream: it archives the final table, applies
// promotion/relegation, resets per-season state, and regenerates the next
// season's fixtures — everything it reads (the final Table, Club.DivisionTier)
// lives in the World, so a resumed run rebuilds the identical post-rollover
// state (NFR-2). The engine owns the queue and caches; this touches World only.
func (w *World) RolloverSeason(r *rand.Rand, newSeason int) {
	// Archive the finished season's standings, then move clubs between
	// divisions. Order matters: pro/rel reads the final table; the archive must
	// capture it (with the finishing positions) before the fresh table is built.
	// The permanent archive copies the tables + cup winner into
	// World.History — LastSeason only ever holds one season and is overwritten
	// here every rollover.
	tables := make([][]Standing, len(w.Table))
	for i := range w.Table {
		tables[i] = append([]Standing{}, w.Table[i]...)
	}
	// The season's full result ledger survives the reset in compact form
	// deep copies with the commentary prose dropped, so get_match
	// can serve any past fixture forever. Dice-free fold at an already-ordered
	// point; growth is linear compact facts per season (cap logged, not built).
	archived := make([]MatchResult, 0, len(w.Results))
	for i := range w.Results {
		archived = append(archived, w.Results[i].archiveCopy())
	}
	w.History = append(w.History, SeasonSummary{
		SeasonYear: newSeason - 1, FinalTables: tables, CupWinnerID: w.CupChampionID,
		Results: archived,
	})
	w.CupChampionID = 0
	w.LastSeason = w.Table
	w.applyPromotionRelegation()

	// Media predictions must track the NEW divisions after promotion/relegation so
	// board confidence judges each club against its current-tier rivals (careers
	// A1) — a stale prediction would treat a promoted side as a title favourite and
	// punish a routine loss as a choke. A labelled stream keeps this independent of
	// the fixture-regen dice. Live Confidence then rebaselines to the fresh
	// baseline (careers E), exactly how generation seeds season 1 — the board opens
	// every season at its re-derived expectation, win or bust forgotten. SackState
	// deliberately carries over: a WARNED club stays warned (no silent flip —
	// FR-14b announces transitions, and the next result reprieves or escalates it
	// through the ordinary machinery), and no club is in ULTIMATUM here because the
	// engine settles ultimata against the final table before the rollover. A
	// caretaker installed at this same boundary rebaselines with everyone else, so
	// its honeymoon confidence only survives mid-season installs (v1
	// simplification.
	pred := rng.Stream(w.Config.Seed, fmt.Sprintf("predictions/season/%d", newSeason))
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		clubs := clubsInTier(w, tier)
		derivePredictionsForTier(w, pred, clubs, len(clubs))
		for _, c := range clubs {
			c.Confidence = c.ConfidenceBaseline // season N+1 starts like season 1
		}
	}

	// Off-season finance reset (careers E): the board sets fresh wage + transfer
	// budgets from the club's post-promotion/relegation tier revenue, current
	// wage bill, and current balance. The balance itself is a running account —
	// never reset. Shared rule with generation stage 7 (deriveClubBudgets), and
	// dice-free, so a resumed run reproduces it exactly.
	for i := range w.Clubs {
		deriveClubBudgets(w.Config, &w.Clubs[i])
	}

	// Careers were already archived by ArchivePlayerSeasons — the engine runs
	// it BEFORE the retirement/contract passes zero anyone's ClubID, so every
	// record keeps the club it was earned at. Here only reset.
	for i := range w.Players {
		p := &w.Players[i]
		p.SeasonApps, p.SeasonGoals, p.RatingSumX10 = 0, 0, 0
		p.FormX10 = nil
		p.Condition, p.Sharpness = ConditionMax, ConditionMax
	}
	// Results are season-scoped: LastSeason holds the summary, and form /
	// recent-results reads must not bleed last season into the new one.
	w.Results = nil
	w.CupByes = nil
	w.Fixtures = nil

	nextID := w.NextFixtureID
	scheduleSeasonFixtures(w, r, newSeason, &nextID)
	w.NextFixtureID = nextID + 1

	w.initTable()
}

// ArchivePlayerSeasons writes every playing player's season line to its career
// ledger. The engine calls it FIRST at the season boundary —
// before the retirement and contract passes zero a departing player's ClubID —
// so the record keeps the club the season was played for;
// RolloverSeason later resets the lines it archived. Dice-free, slice order.
func ArchivePlayerSeasons(w *World, season int) {
	for i := range w.Players {
		p := &w.Players[i]
		if p.SeasonApps == 0 {
			continue // an empty season leaves no record
		}
		p.Career = append(p.Career, SeasonRecord{
			SeasonYear: season, ClubID: p.ClubID,
			Apps: p.SeasonApps, Goals: p.SeasonGoals, RatingSumX10: p.RatingSumX10,
		})
	}
}

// applyPromotionRelegation swaps the bottom clubs of each division with the top
// clubs of the one below (docs/09 §3, automatic, no playoffs). The slot count
// is the derived tunable (`Derived.PromotionSlots`, registered in docs/98), and
// the live Table is sorted by position, so the bottom slots relegate and the
// top slots of the division below promote.
func (w *World) applyPromotionRelegation() {
	for tier := 1; tier < w.Config.Divisions; tier++ {
		upper := w.Table[tier-1]
		lower := w.Table[tier]
		// Clamp to half of each division: a middle division both promotes its
		// top and relegates its bottom, so top and bottom slots must not
		// overlap (they would in a division with fewer than 2×slots clubs,
		// double-processing a middle club). Half-size guarantees disjoint sets.
		slots := w.Derived.PromotionSlots
		if slots > len(upper)/2 {
			slots = len(upper) / 2
		}
		if slots > len(lower)/2 {
			slots = len(lower) / 2
		}
		if slots <= 0 {
			continue
		}
		for _, row := range upper[len(upper)-slots:] {
			w.setClubTier(row.ClubID, tier+1)
		}
		for _, row := range lower[:slots] {
			w.setClubTier(row.ClubID, tier)
		}
	}
}

func (w *World) setClubTier(clubID int64, tier int) {
	for i := range w.Clubs {
		if w.Clubs[i].ID == clubID {
			w.Clubs[i].DivisionTier = tier
			return
		}
	}
}
