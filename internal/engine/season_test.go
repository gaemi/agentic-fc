package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// newEngineCfg builds an engine over a chosen config — season tests need a
// multi-division world (PresetCompact is a single division, so no pro/rel).
func newEngineCfg(t *testing.T, cfg worldgen.WorldConfig) *Engine {
	t.Helper()
	res, err := worldgen.Generate(cfg, worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	return New(res.World, res.Queue, &store.MemAuditLog{})
}

func clubTier(w *worldgen.World, id int64) int {
	for i := range w.Clubs {
		if w.Clubs[i].ID == id {
			return w.Clubs[i].DivisionTier
		}
	}
	return 0
}

// TestSeasonRolloverCrossBoundary is the rollover invariant gate: it runs past
// the season-1/season-2 boundary (day 365) so the rollover fires AND season-2
// fixtures actually play, then proves the run is identical (a) one drain vs
// day-chunks and (b) snapshotted before the boundary then resumed — the
// resumed run must fire the rollover itself and reach the same hash. A horizon
// under day 365 would exercise none of this.
func TestSeasonRolloverCrossBoundary(t *testing.T) {
	const seed = 31
	horizon := sim.GameTime(430 * sim.MinutesPerDay) // season 2 underway

	ea := newEngineCfg(t, worldgen.DefaultConfig(seed))
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	s2 := 0
	for _, r := range ea.world.Results {
		if worldgen.DateOf(r.Kickoff).Season == 2 {
			s2++
		}
	}
	if s2 == 0 {
		t.Fatal("test vacuous: no season-2 matches played — rollover not exercised")
	}

	// (a) day-chunked drain to the same horizon.
	eb := newEngineCfg(t, worldgen.DefaultConfig(seed))
	for eb.Now() < horizon {
		to := eb.Now() + day(1)
		if to > horizon {
			to = horizon
		}
		if _, err := eb.RunUntil(to); err != nil {
			t.Fatal(err)
		}
	}
	ha, _ := ea.World().Hash()
	hb, _ := eb.World().Hash()
	if ha != hb {
		t.Fatalf("rollover not tempo-independent:\nA %s\nB %s", ha, hb)
	}

	// (b) snapshot BEFORE the boundary (day 360) so the resumed run fires the
	// rollover itself, then run to the horizon.
	ec := newEngineCfg(t, worldgen.DefaultConfig(seed))
	if _, err := ec.RunUntil(sim.GameTime(360 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := ec.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: ec.Now(), World: ec.World(), Queue: events, QueueNextSeq: nextSeq,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := fstore.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	resumed := New(snap.World, sim.RestoreQueue(snap.Queue, snap.QueueNextSeq), &store.MemAuditLog{})
	resumed.ResumeAt(snap.Now)
	if _, err := resumed.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	hc, _ := resumed.World().Hash()
	if ha != hc {
		t.Fatalf("resume across the rollover diverged:\nA %s\nC %s", ha, hc)
	}
}

// TestPromotionRelegation locks the pro/rel rule: the bottom `slots` of a
// division swap with the top `slots` of the one below at season end.
func TestPromotionRelegation(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(13))

	// Late June, after May's final round: the table is settled but the rollover
	// (day 365) hasn't fired yet.
	if _, err := e.RunUntil(sim.GameTime(364 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	div1 := append([]worldgen.Standing{}, e.world.Table[0]...)
	div2 := append([]worldgen.Standing{}, e.world.Table[1]...)
	slots := e.world.Derived.PromotionSlots
	relegated := div1[len(div1)-slots:]
	promoted := div2[:slots]

	// Cross the rollover.
	if _, err := e.RunUntil(sim.GameTime(366 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	for _, row := range relegated {
		if got := clubTier(e.world, row.ClubID); got != 2 {
			t.Fatalf("club %d finished %d in div 1 but is now in division %d, want 2", row.ClubID, row.Pos, got)
		}
	}
	for _, row := range promoted {
		if got := clubTier(e.world, row.ClubID); got != 1 {
			t.Fatalf("club %d finished %d in div 2 but is now in division %d, want 1", row.ClubID, row.Pos, got)
		}
	}
}
