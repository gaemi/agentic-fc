package worldgen

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/sim"
)

// cupWorld builds a minimal world holding round-1 cup fixtures and their
// results, so the pure draw/winner helpers can be tested without a full sim.
// Round 1 has four ties; each result's Winner is the passed-in advancing club.
func cupWorld(winners []int64, byes []int64) *World {
	w := &World{}
	w.Config.Divisions = 1
	w.Derived.CupRounds = 4
	w.Derived.CupRoundTimes = []sim.GameTime{1000, 2000, 3000, 4000}
	w.CupByes = byes
	w.NextFixtureID = 500
	for i, win := range winners {
		id := int64(100 + i)
		home := int64(1000 + i*2)
		away := int64(1001 + i*2)
		w.Fixtures = append(w.Fixtures, Fixture{
			ID: id, Competition: CompetitionCup, Round: 1, HomeID: home, AwayID: away,
		})
		w.Results = append(w.Results, MatchResult{
			FixtureID: id, Competition: CompetitionCup, HomeID: home, AwayID: away, Winner: win,
		})
	}
	return w
}

// TestCupRoundCompleteAndWinners locks the gating: a round is complete only once
// every tie has a result, and the winners come back id-sorted (order-stable
// before any draw — NFR-2).
func TestCupRoundCompleteAndWinners(t *testing.T) {
	w := cupWorld([]int64{103, 101, 100, 102}, nil)

	if !w.CupRoundComplete(1) {
		t.Fatal("round 1 should be complete: every tie has a result")
	}
	if w.CupRoundComplete(2) {
		t.Fatal("round 2 has no fixtures — cannot be complete")
	}

	// Drop one result: the round is no longer complete.
	partial := cupWorld([]int64{103, 101, 100, 102}, nil)
	partial.Results = partial.Results[:3]
	if partial.CupRoundComplete(1) {
		t.Fatal("round 1 with a tie unresolved must not be complete")
	}

	got := w.CupRoundWinners(1)
	want := []int64{100, 101, 102, 103}
	if len(got) != len(want) {
		t.Fatalf("winners %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("winners %v not id-sorted, want %v", got, want)
		}
	}
}

// TestDrawCupRoundPairsWinners is the pure-draw invariant: round 2 pairs the
// round-1 winners together with the byes (who sat out round 1), every entrant
// appears exactly once, new fixtures carry monotonic ids from NextFixtureID and
// the round's season-shifted kickoff, and NextFixtureID advances past them.
func TestDrawCupRoundPairsWinners(t *testing.T) {
	winners := []int64{100, 101, 102, 103}
	byes := []int64{200, 201, 202, 203}
	w := cupWorld(winners, byes)

	created := w.DrawCupRound(rng.Stream(1, "cup/test/draw"), 2, 1)

	if len(created) != 4 {
		t.Fatalf("round 2 created %d ties, want 4 (4 winners + 4 byes)", len(created))
	}

	seen := map[int64]int{}
	var maxID int64
	for _, f := range created {
		if f.Competition != CompetitionCup || f.Round != 2 {
			t.Fatalf("created fixture wrong comp/round: %+v", f)
		}
		if f.Kickoff != w.Derived.CupRoundTimes[1]+seasonShift(1) {
			t.Fatalf("round 2 kickoff %d, want %d", f.Kickoff, w.Derived.CupRoundTimes[1]+seasonShift(1))
		}
		seen[f.HomeID]++
		seen[f.AwayID]++
		if f.ID > maxID {
			maxID = f.ID
		}
		if f.ID <= 500 {
			t.Fatalf("fixture id %d not drawn above NextFixtureID (500)", f.ID)
		}
	}

	for _, id := range append(append([]int64{}, winners...), byes...) {
		if seen[id] != 1 {
			t.Fatalf("entrant %d appears %d times in the round-2 draw, want exactly 1", id, seen[id])
		}
	}
	if len(seen) != 8 {
		t.Fatalf("round 2 has %d distinct entrants, want 8", len(seen))
	}
	if w.NextFixtureID != maxID+1 {
		t.Fatalf("NextFixtureID %d, want %d (past the drawn ties)", w.NextFixtureID, maxID+1)
	}
}

// TestDrawCupRoundIsDeterministic confirms the labelled draw stream reproduces
// the same pairings on replay (NFR-2): two draws from the same seed/label pair
// identically.
func TestDrawCupRoundIsDeterministic(t *testing.T) {
	winners := []int64{100, 101, 102, 103}
	byes := []int64{200, 201, 202, 203}

	a := cupWorld(winners, byes)
	b := cupWorld(winners, byes)
	ca := a.DrawCupRound(rng.Stream(5, "cup/s1/r2/draw"), 2, 1)
	cb := b.DrawCupRound(rng.Stream(5, "cup/s1/r2/draw"), 2, 1)

	if len(ca) != len(cb) {
		t.Fatalf("draw sizes differ: %d vs %d", len(ca), len(cb))
	}
	for i := range ca {
		if ca[i].HomeID != cb[i].HomeID || ca[i].AwayID != cb[i].AwayID {
			t.Fatalf("tie %d differs: %d-%d vs %d-%d", i, ca[i].HomeID, ca[i].AwayID, cb[i].HomeID, cb[i].AwayID)
		}
	}
}
