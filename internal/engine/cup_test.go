package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// cupHorizon runs past the last cup round (PresetCompact: rounds ~Sep 8 + 30·n,
// so the round-4 final lands ~day 159) with margin, still inside season 1.
const cupHorizon = 200

func countChampions(e *Engine) int {
	n := 0
	for _, item := range e.world.News {
		if item.Key == FeedCupChampion {
			n++
		}
	}
	return n
}

func cupResultCount(e *Engine) int {
	n := 0
	for _, r := range e.world.Results {
		if r.Competition == worldgen.CompetitionCup {
			n++
		}
	}
	return n
}

// TestCupRunsToChampion is the cup end-to-end: the bracket advances through
// every round (a 12-club PresetCompact cup: 4+4+2+1 ties over 4 rounds — byes
// enter at round 2), every tie names an advancing club, and exactly one champion
// is crowned. Without progression the world would hold only round-1's 4 ties.
func TestCupRunsToChampion(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(day(cupHorizon)); err != nil {
		t.Fatal(err)
	}

	wantTies := map[int]int{1: 4, 2: 4, 3: 2, 4: 1}
	gotTies := map[int]int{}
	for _, f := range e.world.Fixtures {
		if f.Competition == worldgen.CompetitionCup {
			gotTies[f.Round]++
		}
	}
	for round, want := range wantTies {
		if gotTies[round] != want {
			t.Fatalf("cup round %d drawn with %d ties, want %d", round, gotTies[round], want)
		}
	}

	for _, r := range e.world.Results {
		if r.Competition != worldgen.CompetitionCup {
			continue
		}
		if r.Winner == 0 {
			t.Fatalf("cup tie %d recorded no advancing club", r.FixtureID)
		}
		if r.Winner != r.HomeID && r.Winner != r.AwayID {
			t.Fatalf("cup tie %d winner %d is neither side (%d/%d)", r.FixtureID, r.Winner, r.HomeID, r.AwayID)
		}
	}
	if got := cupResultCount(e); got != 11 {
		t.Fatalf("played %d cup ties, want 11 (4+4+2+1)", got)
	}
	if got := countChampions(e); got != 1 {
		t.Fatalf("crowned %d cup champions, want exactly 1", got)
	}
}

// TestCupDeterminismAcrossTempo extends the match determinism invariant past the
// cup final: the draws (winners re-paired on a labelled stream) and shootouts
// must be tempo-independent too, so a one-drain run and a day-chunked run reach
// the identical world hash. The match determinism test stops before round 1
// kicks off, so this is the draw/shootout-specific guard (NFR-2).
func TestCupDeterminismAcrossTempo(t *testing.T) {
	const seed = 7
	ea, _ := newEngine(t, seed)
	horizon := day(cupHorizon)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if countChampions(ea) != 1 {
		t.Fatal("test vacuous: no champion crowned in the horizon")
	}

	eb, _ := newEngine(t, seed)
	for eb.Now() < horizon {
		to := eb.Now() + day(3)
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
		t.Fatalf("cup progression not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestCupResumeAtDrawSeam is the cup invariant: it snapshots at the fragile
// seam — a round has been drawn forward but has NOT yet kicked off — then proves
// the resumed run still plays that round and crowns the same champion at the same
// hash. This is where a bug would hide: the new round's fixtures live in World and
// its kickoff events live only in the persisted queue, so if either failed to
// survive the round would silently never play (FR-28, NFR-2).
func TestCupResumeAtDrawSeam(t *testing.T) {
	const seed = 99
	horizon := day(cupHorizon)

	ea, _ := newEngine(t, seed)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if countChampions(ea) != 1 {
		t.Fatal("uninterrupted run did not crown a champion — fixture bad")
	}

	// Snapshot after round 1 completes and round 2 is drawn (~day 69), but before
	// round 2 kicks off (~day 99): round 2's fixtures exist, none have played.
	eb, _ := newEngine(t, seed)
	if _, err := eb.RunUntil(day(85)); err != nil {
		t.Fatal(err)
	}
	if !eb.world.HasCupRound(2) {
		t.Fatal("test vacuous: round 2 not drawn at snapshot time")
	}
	if !eb.world.CupRoundComplete(1) {
		t.Fatal("test vacuous: round 1 not complete at snapshot time")
	}
	if got := cupResultCount(eb); got != 4 {
		t.Fatalf("at the seam expected exactly round 1's 4 results, got %d (round 2 already kicked off?)", got)
	}

	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := eb.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{
		Now: eb.Now(), World: eb.World(), Queue: events, QueueNextSeq: nextSeq,
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

	if countChampions(resumed) != 1 {
		t.Fatal("resumed run crowned no champion — the draw→kickoff seam broke")
	}
	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("cup draw-seam resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}

// TestCupShootoutResolvesLevelTie unit-tests the tie-break: a decisive score
// advances the higher scorer with no shootout, and a level score goes to a
// deterministic penalty shootout that always splits and names a real side.
func TestCupShootoutResolvesLevelTie(t *testing.T) {
	e, _ := newEngine(t, 1)

	decisive := &worldgen.LiveMatch{FixtureID: 123, HomeID: 10, AwayID: 20, HomeGoals: 2, AwayGoals: 1}
	if w, _, _, shootout := e.resolveCupWinner(decisive); w != 10 || shootout {
		t.Fatalf("decisive tie: got winner=%d shootout=%v, want 10/false", w, shootout)
	}

	level := &worldgen.LiveMatch{FixtureID: 456, HomeID: 10, AwayID: 20, HomeGoals: 1, AwayGoals: 1}
	w1, hp, ap, shootout := e.resolveCupWinner(level)
	if !shootout {
		t.Fatal("level tie must go to a shootout")
	}
	if w1 != 10 && w1 != 20 {
		t.Fatalf("shootout winner %d is neither side", w1)
	}
	if hp == ap {
		t.Fatalf("shootout ended level %d-%d — no advancing club", hp, ap)
	}
	if (w1 == 10) != (hp > ap) {
		t.Fatalf("winner %d inconsistent with pens home=%d away=%d", w1, hp, ap)
	}
	if w2, _, _, _ := e.resolveCupWinner(level); w2 != w1 {
		t.Fatalf("shootout not deterministic: %d then %d", w1, w2)
	}
}
