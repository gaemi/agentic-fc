package worldgen

import (
	"math/rand/v2"
	"sort"
)

// Cup within-season progression. The generator draws only round 1 (a
// seeded entry so lower divisions enter earlier — docs/09 §3); every later round
// is drawn here as the world runs, once its predecessor is complete. Like the
// season rollover this is a pure function of World: it reads the finished
// results and the fixtures, and mutates only World.Fixtures / NextFixtureID, so
// a resumed run rebuilds the identical bracket (NFR-2). The engine owns the
// queue and the shootout dice; this file never touches them.

// cupFixtures returns the cup fixtures of one round, in fixture-slice order.
func (w *World) cupFixtures(round int) []Fixture {
	var out []Fixture
	for _, f := range w.Fixtures {
		if f.Competition == CompetitionCup && f.Round == round {
			out = append(out, f)
		}
	}
	return out
}

// HasCupRound reports whether the round has been drawn (its fixtures exist).
func (w *World) HasCupRound(round int) bool {
	return len(w.cupFixtures(round)) > 0
}

// resultByFixture indexes finished results by fixture id (a fixture has at most
// one result). Rebuilt per call — cup draws are rare (roughly monthly).
func (w *World) resultByFixture() map[int64]*MatchResult {
	m := make(map[int64]*MatchResult, len(w.Results))
	for i := range w.Results {
		m[w.Results[i].FixtureID] = &w.Results[i]
	}
	return m
}

// CupRoundComplete reports whether every cup fixture of the round has a result.
// False for a round with no fixtures (nothing to advance from).
func (w *World) CupRoundComplete(round int) bool {
	fx := w.cupFixtures(round)
	if len(fx) == 0 {
		return false
	}
	res := w.resultByFixture()
	for _, f := range fx {
		if res[f.ID] == nil {
			return false
		}
	}
	return true
}

// CupRoundWinners returns the advancing clubs of a completed round, sorted by id
// so the set is order-stable before any draw (NFR-2). On the final round the
// slice holds the single champion.
func (w *World) CupRoundWinners(round int) []int64 {
	res := w.resultByFixture()
	var winners []int64
	for _, f := range w.cupFixtures(round) {
		if r := res[f.ID]; r != nil && r.Winner != 0 {
			winners = append(winners, r.Winner)
		}
	}
	sort.Slice(winners, func(i, j int) bool { return winners[i] < winners[j] })
	return winners
}

// DrawCupRound draws cup round `round` (>= 2) from the previous round's winners,
// adding the byes when round == 2 (they sat out round 1 — docs/09 §3). Entrants
// are id-sorted, then the stream shuffles them into pairs; new fixtures get
// monotonic ids and the round's season-shifted midweek kickoff. It returns the
// created fixtures so the engine can schedule their kickoffs, exactly as the
// season rollover primes a new season's fixtures.
func (w *World) DrawCupRound(r *rand.Rand, round, season int) []Fixture {
	entrants := w.CupRoundWinners(round - 1)
	if round == 2 {
		entrants = append(entrants, w.CupByes...)
	}
	sort.Slice(entrants, func(i, j int) bool { return entrants[i] < entrants[j] })
	r.Shuffle(len(entrants), func(i, j int) { entrants[i], entrants[j] = entrants[j], entrants[i] })

	kickoff := w.Derived.CupRoundTimes[round-1] + seasonShift(season)
	nextID := w.NextFixtureID
	var created []Fixture
	for i := 0; i+1 < len(entrants); i += 2 {
		nextID++
		f := Fixture{
			ID:          nextID,
			Competition: CompetitionCup,
			Round:       round,
			Kickoff:     kickoff,
			HomeID:      entrants[i],
			AwayID:      entrants[i+1],
		}
		w.Fixtures = append(w.Fixtures, f)
		created = append(created, f)
	}
	w.NextFixtureID = nextID + 1
	return created
}
