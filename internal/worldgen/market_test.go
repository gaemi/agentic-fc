package worldgen

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
)

func genWorld(t *testing.T, seed uint64) *World {
	t.Helper()
	res, err := Generate(PresetClassic(seed), WithTokenReader(&counterReader{}))
	if err != nil {
		t.Fatal(err)
	}
	return res.World
}

// TestSquadSizeAtTarget locks the generation baseline the autonomous market rests
// on: every club starts with exactly SquadSizeTarget senior players (youth don't
// count), so a fresh world is in neither deficit nor surplus.
func TestSquadSizeAtTarget(t *testing.T) {
	w := genWorld(t, 42)
	for i := range w.Clubs {
		if got := SquadSize(w, w.Clubs[i].ID); got != w.Config.SquadSizeTarget {
			t.Fatalf("club %d SquadSize = %d, want target %d", w.Clubs[i].ID, got, w.Config.SquadSizeTarget)
		}
	}
	if SquadSize(w, 0) != 0 {
		t.Fatal("club id 0 (free agents) must have SquadSize 0")
	}
}

func playerClubID(w *World, id int64) int64 {
	for i := range w.Players {
		if w.Players[i].ID == id {
			return w.Players[i].ClubID
		}
	}
	return 0
}

// TestSurplusListed: a fresh at-target world exposes no surplus; a strict-surplus
// club exposes exactly its lowest-pool senior excess, and only that.
func TestSurplusListed(t *testing.T) {
	w := genWorld(t, 42)
	if got := len(SurplusListed(w)); got != 0 {
		t.Fatalf("fresh at-target world exposes %d surplus, want 0", got)
	}

	// Move a free agent into club C → C is target+1 (strict surplus).
	var fa *Player
	for i := range w.Players {
		if w.Players[i].ClubID == 0 && !w.Players[i].Youth {
			fa = &w.Players[i]
			break
		}
	}
	if fa == nil {
		t.Fatal("no free agent to create a surplus with")
	}
	club := w.Clubs[0].ID
	fa.ClubID = club

	wantExcess, bestPool := int64(0), -1
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID != club || p.Youth {
			continue
		}
		if bestPool == -1 || p.AbilityPool < bestPool || (p.AbilityPool == bestPool && p.ID < wantExcess) {
			bestPool, wantExcess = p.AbilityPool, p.ID
		}
	}
	listed := SurplusListed(w)
	if !listed[wantExcess] {
		t.Fatalf("surplus club's lowest-pool senior %d is not exposed", wantExcess)
	}
	fromClub := 0
	for id := range listed {
		if playerClubID(w, id) == club {
			fromClub++
		}
	}
	if fromClub != 1 {
		t.Fatalf("a surplus of 1 exposed %d players from the club, want exactly 1", fromClub)
	}
}

// TestExplicitlyListed: a SELL directive on a senior lists them; a SELL on a youth
// does not — youth are never on the market.
func TestExplicitlyListed(t *testing.T) {
	w := genWorld(t, 42)
	if got := len(ExplicitlyListed(w)); got != 0 {
		t.Fatalf("fresh world has %d explicitly listed, want 0", got)
	}
	club := w.Clubs[0].ID
	var mgr *Manager
	for i := range w.Managers {
		if w.Managers[i].ClubID == club {
			mgr = &w.Managers[i]
			break
		}
	}
	if mgr == nil {
		t.Fatal("club has no manager")
	}
	// Two of the club's seniors; mark the second a youth to exercise exclusion.
	var senior, youth int64
	for i := range w.Players {
		p := &w.Players[i]
		if p.ClubID != club || p.Youth {
			continue
		}
		if senior == 0 {
			senior = p.ID
		} else if youth == 0 {
			youth, p.Youth = p.ID, true
			break
		}
	}
	if senior == 0 || youth == 0 {
		t.Fatal("club needs two seniors for the setup")
	}
	mgr.Mindset.Directives = append(mgr.Mindset.Directives,
		mindset.Directive{ID: "s1", Verb: mindset.VerbSell, Target: mindset.Target{Player: senior}, Strength: mindset.StrengthLean},
		mindset.Directive{ID: "s2", Verb: mindset.VerbSell, Target: mindset.Target{Player: youth}, Strength: mindset.StrengthLean})

	listed := ExplicitlyListed(w)
	if !listed[senior] {
		t.Fatal("explicitly SELL-listed senior is not on the market")
	}
	if listed[youth] {
		t.Fatal("a youth was exposed on the market via SELL — youth must never be listed")
	}
}
