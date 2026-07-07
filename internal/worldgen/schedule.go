package worldgen

import (
	"math/rand/v2"
	"sort"

	"github.com/gaemi/agentic-fc/internal/sim"
)

// Stage 6 — schedule: double round-robin league fixtures on the derived
// round times (all fixtures of a match day kick off simultaneously — FR-6a),
// the seeded cup first-round draw, and each club's youth intake day
// (docs/09 §3, §4).

const fixtureIDBase = 100000

func genSchedule(w *World, r *rand.Rand) {
	nextID := int64(fixtureIDBase)
	scheduleSeasonFixtures(w, r, 1, &nextID)
	w.NextFixtureID = nextID + 1
}

// seasonShift is the game-time offset from season 1 to the given 1-based
// season — the derived round times are season-1-absolute, so a later season's
// fixtures must be pushed forward by whole seasons.
func seasonShift(season int) sim.GameTime {
	return sim.GameTime(int64(season-1) * DaysPerSeason * sim.MinutesPerDay)
}

// YouthIntakeDue is the absolute game time of a club's youth intake in a given
// 1-based season: 00:00 on the club's spring intake day (Club.YouthIntakeDay,
// re-rolled each rollover), shifted to that season. It is the single source both
// queue-priming paths read — generation for season 1, the rollover for later
// seasons — so every season schedules the intake identically (youth intake, NFR-2).
// 00:00 (like the world-calendar events) lands the intake before any same-day
// 15:00/19:30 kickoff, so a live match's player-id lineups are never disturbed
// while the intake appends to World.Players.
func YouthIntakeDue(club *Club, season int) sim.GameTime {
	return seasonShift(season) + sim.GameTime(int64(club.YouthIntakeDay)*sim.MinutesPerDay)
}

// scheduleSeasonFixtures generates one season's league round-robin and cup
// draw for the current division memberships, with kickoff times shifted to the
// given season and ids drawn monotonically from nextID. Shared by generation
// (season 1) and the season rollover, so both produce identical
// fixtures for the same (world, stream, season).
func scheduleSeasonFixtures(w *World, r *rand.Rand, season int, nextID *int64) {
	shift := seasonShift(season)
	for tier := 1; tier <= w.Config.Divisions; tier++ {
		clubs := clubsInTier(w, tier)
		ids := make([]int64, len(clubs))
		for i, c := range clubs {
			ids[i] = c.ID
		}
		r.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

		for _, fx := range roundRobin(ids) {
			*nextID++
			w.Fixtures = append(w.Fixtures, Fixture{
				ID:           *nextID,
				Competition:  CompetitionLeague,
				DivisionTier: tier,
				Round:        fx.round,
				Kickoff:      w.Derived.LeagueRoundTimes[fx.round-1] + shift,
				HomeID:       fx.home,
				AwayID:       fx.away,
			})
		}
	}

	genCupDraw(w, r, shift, nextID)

	// Youth intake day: one per club, rolled in the spring window
	// (docs/09 §3).
	span := dayYouthIntakeEnd - dayYouthIntakeStart + 1
	for i := range w.Clubs {
		w.Clubs[i].YouthIntakeDay = dayYouthIntakeStart + r.IntN(span)
	}
}

type pairing struct {
	round      int
	home, away int64
}

// roundRobin builds a double round-robin with the circle method: rounds
// 1…n-1 rotate, rounds n…2(n-1) mirror with venues swapped.
func roundRobin(ids []int64) []pairing {
	n := len(ids)
	half := n - 1
	var out []pairing
	for rd := 0; rd < half; rd++ {
		for i := 0; i < n/2; i++ {
			var a, b int64
			if i == 0 {
				a, b = ids[rd%half], ids[n-1] // the fixed seat
			} else {
				a = ids[(rd+i)%half]
				b = ids[(rd+half-i)%half]
			}
			// Alternate venues so no club hosts every week.
			home, away := a, b
			if (rd+i)%2 == 1 {
				home, away = b, a
			}
			out = append(out, pairing{round: rd + 1, home: home, away: away})
			out = append(out, pairing{round: rd + 1 + half, home: away, away: home})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].round != out[j].round {
			return out[i].round < out[j].round
		}
		if out[i].home != out[j].home {
			return out[i].home < out[j].home
		}
		return out[i].away < out[j].away
	})
	return out
}

// genCupDraw seeds entry so lower divisions enter earlier: the byes (bracket
// size − clubs) go to the best-seeded clubs — top division first, then
// last-season order (docs/09 §3). Only round 1 is drawn now; later rounds
// draw as the world runs.
func genCupDraw(w *World, r *rand.Rand, shift sim.GameTime, nextID *int64) {
	seeds := make([]*Club, len(w.Clubs))
	for i := range w.Clubs {
		seeds[i] = &w.Clubs[i]
	}
	lastPos := map[int64]int{}
	for _, table := range w.LastSeason {
		for _, row := range table {
			lastPos[row.ClubID] = row.Pos
		}
	}
	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].DivisionTier != seeds[j].DivisionTier {
			return seeds[i].DivisionTier < seeds[j].DivisionTier
		}
		if lastPos[seeds[i].ID] != lastPos[seeds[j].ID] {
			return lastPos[seeds[i].ID] < lastPos[seeds[j].ID]
		}
		return seeds[i].ID < seeds[j].ID
	})

	byes := w.Derived.CupByes
	for _, c := range seeds[:byes] {
		w.CupByes = append(w.CupByes, c.ID)
	}
	sort.Slice(w.CupByes, func(i, j int) bool { return w.CupByes[i] < w.CupByes[j] })

	entrants := make([]int64, 0, len(seeds)-byes)
	for _, c := range seeds[byes:] {
		entrants = append(entrants, c.ID)
	}
	r.Shuffle(len(entrants), func(i, j int) { entrants[i], entrants[j] = entrants[j], entrants[i] })
	for i := 0; i+1 < len(entrants); i += 2 {
		*nextID++
		w.Fixtures = append(w.Fixtures, Fixture{
			ID:          *nextID,
			Competition: CompetitionCup,
			Round:       1,
			Kickoff:     w.Derived.CupRoundTimes[0] + shift,
			HomeID:      entrants[i],
			AwayID:      entrants[i+1],
		})
	}
}
