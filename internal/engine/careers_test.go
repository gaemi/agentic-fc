package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/rng"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestRebuildManagerIndexAfterSpawn locks the pointer-safety half of the
// lifecycle infra: appending a runtime manager to World.Managers can reallocate
// the slice, so the engine's id→pointer index must be rebuilt — after which the
// new manager is present and every indexed pointer aliases the live slice.
func TestRebuildManagerIndexAfterSpawn(t *testing.T) {
	e, _ := newEngine(t, 42)
	before := len(e.managers)

	r := rng.Stream(e.world.Config.Seed, "career/spawn/test")
	m := worldgen.SpawnManager(e.world, r, e.world.Clubs[0].ID, 1, true)
	newID := m.ID

	e.rebuildManagerIndex()
	if len(e.managers) != before+1 {
		t.Fatalf("manager index size %d, want %d", len(e.managers), before+1)
	}
	got, ok := e.managers[newID]
	if !ok || got.ID != newID || !got.Caretaker {
		t.Fatalf("spawned manager not indexed: ok=%v %+v", ok, got)
	}
	// The indexed pointer must alias World.Managers — writes go through.
	got.Reputation = 4242
	if e.world.Managers[len(e.world.Managers)-1].Reputation != 4242 {
		t.Fatal("indexed pointer does not alias World.Managers (stale after realloc)")
	}
}

// TestMoveConfidence locks the confidence-movement table (board confidence): a favorite
// is cut for a shock loss and an underdog rewarded for a shock win; an even draw
// nudges up; the floor holds at 1.
func TestMoveConfidence(t *testing.T) {
	fav := &worldgen.Club{PredictedFinish: 1, Confidence: 50}   // strong (low finish)
	weak := &worldgen.Club{PredictedFinish: 10, Confidence: 50} // weak (gap 9 ≥ 4)
	moveConfidence(fav, weak, 0, 1)                             // favorite loses
	moveConfidence(weak, fav, 1, 0)                             // underdog wins
	if fav.Confidence != 45 {
		t.Fatalf("favorite confidence after a shock loss = %d, want 45 (−5)", fav.Confidence)
	}
	if weak.Confidence != 56 {
		t.Fatalf("underdog confidence after a shock win = %d, want 56 (+6)", weak.Confidence)
	}

	a := &worldgen.Club{PredictedFinish: 5, Confidence: 50}
	b := &worldgen.Club{PredictedFinish: 6, Confidence: 50} // gap 1 < 4 → even
	moveConfidence(a, b, 2, 2)                              // even draw
	if a.Confidence != 51 {
		t.Fatalf("even draw confidence = %d, want 51 (+1)", a.Confidence)
	}

	low := &worldgen.Club{PredictedFinish: 1, Confidence: 3}
	moveConfidence(low, weak, 0, 1) // favorite loss −5 → −2 → floored
	if low.Confidence != 1 {
		t.Fatalf("confidence floor = %d, want 1", low.Confidence)
	}
}

// TestConfidenceMovesOnLeagueResult is the A1 integration check: live confidence
// starts at the baseline and shifts once league matches play, staying in range.
func TestConfidenceMovesOnLeagueResult(t *testing.T) {
	e, _ := newEngine(t, 42)
	base := map[int64]int{}
	for i := range e.world.Clubs {
		c := &e.world.Clubs[i]
		if c.Confidence != c.ConfidenceBaseline {
			t.Fatalf("club %d live confidence %d != baseline %d at generation", c.ID, c.Confidence, c.ConfidenceBaseline)
		}
		base[c.ID] = c.Confidence
	}
	if _, err := e.RunUntil(day(60)); err != nil { // past the first league round (~mid-Aug)
		t.Fatal(err)
	}
	moved := 0
	for i := range e.world.Clubs {
		c := &e.world.Clubs[i]
		if c.Confidence != base[c.ID] {
			moved++
		}
		if c.Confidence < 1 || c.Confidence > 100 {
			t.Fatalf("club %d confidence %d out of [1,100]", c.ID, c.Confidence)
		}
	}
	if moved == 0 {
		t.Fatal("no club's confidence moved after a league round played")
	}
}

// TestPredictionsRederivedAtRollover locks the cross-season fix:
// after promotion/relegation moves clubs between divisions, each division's
// PredictedFinish must be a fresh permutation {1..n} of its CURRENT clubs — a
// stale carry-over would duplicate ranks (a promoted club keeping its old-tier
// rank), inverting the confidence incentive from season 2 on.
func TestPredictionsRederivedAtRollover(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(day(worldgen.DaysPerSeason + 2)); err != nil { // past the first rollover
		t.Fatal(err)
	}
	for tier := 1; tier <= e.world.Config.Divisions; tier++ {
		var finishes []int
		for i := range e.world.Clubs {
			if e.world.Clubs[i].DivisionTier == tier {
				finishes = append(finishes, e.world.Clubs[i].PredictedFinish)
			}
		}
		seen := map[int]bool{}
		for _, f := range finishes {
			if f < 1 || f > len(finishes) {
				t.Fatalf("tier %d PredictedFinish %d out of [1,%d]", tier, f, len(finishes))
			}
			if seen[f] {
				t.Fatalf("tier %d has a duplicate PredictedFinish %d — predictions not re-derived after promotion/relegation", tier, f)
			}
			seen[f] = true
		}
	}
}
