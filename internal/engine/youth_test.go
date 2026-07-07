package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// afterSpringIntake is a horizon comfortably past every club's season-1 youth
// intake window (spring, ~Mar 1 – Apr 30), so a run to here has processed exactly
// one intake per club and still sits inside season 1.
func afterSpringIntake() sim.GameTime { return day(310) }

// youthCount counts youth players at a club (clubID 0 → across the whole world).
func youthCount(e *Engine, clubID int64) int {
	n := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Youth && (clubID == 0 || p.ClubID == clubID) {
			n++
		}
	}
	return n
}

// TestYouthIntakeAddsToSquad locks the functional core of youth intake: running past
// the spring window brings each club exactly one batch of prospects — flagged Youth,
// aged 15–17, contracted, at a real club — each wired for development with a drift
// roll, and the intake is announced on the news ring.
func TestYouthIntakeAddsToSquad(t *testing.T) {
	e, _ := newEngine(t, 42)
	w := e.world
	clubs := w.Config.TotalClubs()
	batch := w.Config.YouthIntakeBatch

	before := youthCount(e, 0)
	preexisting := map[int64]bool{}
	for i := range w.Players {
		preexisting[w.Players[i].ID] = true
	}

	if _, err := e.RunUntil(afterSpringIntake()); err != nil {
		t.Fatal(err)
	}

	// Initial academies (3–5) plus a batch of 5 stay well under the cap, so no club
	// throttles its first spring: total youth grows by exactly clubs × batch.
	if got, want := youthCount(e, 0), before+clubs*batch; got != want {
		t.Fatalf("total youth after intake = %d, want %d (+%d clubs × %d)", got, want, clubs, batch)
	}

	drifts := driftRollsByPlayer(e)
	newYouth := 0
	for i := range w.Players {
		p := &w.Players[i]
		if preexisting[p.ID] {
			continue
		}
		newYouth++
		if !p.Youth {
			t.Fatalf("new player %d is not flagged Youth", p.ID)
		}
		if p.ClubID == 0 || e.clubs[p.ClubID] == nil {
			t.Fatalf("new youth %d has no valid club (ClubID=%d)", p.ID, p.ClubID)
		}
		if p.Age < 15 || p.Age > 17 {
			t.Fatalf("new youth %d age %d out of the 15–17 intake range", p.ID, p.Age)
		}
		if p.Contract == nil {
			t.Fatalf("new youth %d has no contract", p.ID)
		}
		if p.Condition != worldgen.ConditionMax || p.Sharpness != worldgen.ConditionMax {
			t.Fatalf("new youth %d not match-fresh (cond=%d sharp=%d) — intake bypasses EnsureMatchState, so it must seed its own fitness",
				p.ID, p.Condition, p.Sharpness)
		}
		if drifts[p.ID] == 0 {
			t.Fatalf("new youth %d has no drift roll queued — it would never develop", p.ID)
		}
	}
	if newYouth != clubs*batch {
		t.Fatalf("new players = %d, want %d", newYouth, clubs*batch)
	}
	if !hasNews(e, NewsYouthIntake) {
		t.Fatal("no youth-intake news filed")
	}
	// The cached wage bill must stay in step with the contracts the intake added —
	// finance ticks and transfer affordability read it.
	for i := range w.Clubs {
		c := &w.Clubs[i]
		var want int64
		for j := range w.Players {
			if p := &w.Players[j]; p.ClubID == c.ID && p.Contract != nil {
				want += p.Contract.WageWeeklyMinor
			}
		}
		if c.WageBillWeeklyMinor != want {
			t.Fatalf("club %d wage bill = %d after intake, want %d (cache drifted from contracts)",
				c.ID, c.WageBillWeeklyMinor, want)
		}
	}
}

// TestYouthIntakeExcludedFromSelection locks the "inert until graduation" contract
// (youth intake): however many prospects a club takes in, none is ever fielded — the XI
// selector skips Youth, so intake never leaks into the senior match system.
func TestYouthIntakeExcludedFromSelection(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(afterSpringIntake()); err != nil {
		t.Fatal(err)
	}
	if youthCount(e, 0) == 0 {
		t.Fatal("test vacuous: no youth intake occurred")
	}
	for i := range e.world.Clubs {
		clubID := e.world.Clubs[i].ID
		xi, _ := e.selectSquad(clubID, e.Now(), mindset.TacticalPlan{})
		for _, id := range xi {
			if p := e.players[id]; p != nil && p.Youth {
				t.Fatalf("club %d fielded youth player %d", clubID, id)
			}
		}
	}
}

// TestNextPlayerIDMonotonic locks the id allocator (youth intake): the generation
// counter equals the max generated id, and every intake id is fresh — strictly
// beyond the generation max and never a duplicate — with the counter still leading.
func TestNextPlayerIDMonotonic(t *testing.T) {
	e, _ := newEngine(t, 42)
	w := e.world
	genMax := int64(0)
	for i := range w.Players {
		if w.Players[i].ID > genMax {
			genMax = w.Players[i].ID
		}
	}
	if w.NextPlayerID != genMax {
		t.Fatalf("NextPlayerID %d != max generated player id %d", w.NextPlayerID, genMax)
	}

	if _, err := e.RunUntil(afterSpringIntake()); err != nil {
		t.Fatal(err)
	}

	seen := map[int64]bool{}
	newMax := int64(0)
	for i := range w.Players {
		id := w.Players[i].ID
		if seen[id] {
			t.Fatalf("duplicate player id %d after intake", id)
		}
		seen[id] = true
		if id > newMax {
			newMax = id
		}
	}
	if newMax <= genMax {
		t.Fatal("intake allocated no ids beyond the generation max")
	}
	if w.NextPlayerID != newMax {
		t.Fatalf("NextPlayerID %d != max id after intake %d", w.NextPlayerID, newMax)
	}
}

// TestYouthIntakeDeterminismAcrossTempo is a phase invariant (NFR-2): a run that
// spans two spring intakes and the rollover between them (which re-primes the
// intake) reaches the identical world hash whether drained in one shot or chunked —
// intake ids come from the snapshotted counter in queue order and every roll is a
// labelled stream keyed on club + due, never wall-clock or chunk boundaries.
func TestYouthIntakeDeterminismAcrossTempo(t *testing.T) {
	const seed = 42
	horizon := day(worldgen.DaysPerSeason + 310) // season-1 intake, rollover, season-2 intake

	e0, _ := newEngine(t, seed)
	initialYouth := youthCount(e0, 0)

	ea, _ := newEngine(t, seed)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	eb, _ := newEngine(t, seed)
	for eb.Now() < horizon {
		to := eb.Now() + day(7)
		if to > horizon {
			to = horizon
		}
		if _, err := eb.RunUntil(to); err != nil {
			t.Fatal(err)
		}
	}

	if youthCount(ea, 0) <= initialYouth {
		t.Fatalf("test vacuous: no net youth intake across the horizon (%d ≤ %d)", youthCount(ea, 0), initialYouth)
	}
	ha, _ := ea.World().Hash()
	hb, _ := eb.World().Hash()
	if ha != hb {
		t.Fatalf("youth intake not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestYouthIntakeResumeAtIntakeSeam is a phase invariant (FR-28, NFR-2): a snapshot
// taken partway through the spring intake window — some clubs already intaken, the
// rest still queued — round-trips through the store, and the resumed run reaches the
// same hash as an uninterrupted one. This exercises NextPlayerID and the pending
// per-club intake events surviving the snapshot.
func TestYouthIntakeResumeAtIntakeSeam(t *testing.T) {
	const seed = 42
	horizon := day(320)
	cut := day(275) // mid spring window (~Mar 1 – Apr 30)

	ea, _ := newEngine(t, seed)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if youthCount(ea, 0) == 0 {
		t.Fatal("test vacuous: no youth intake within the horizon")
	}

	eb, _ := newEngine(t, seed)
	if _, err := eb.RunUntil(cut); err != nil {
		t.Fatal(err)
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

	ha, _ := ea.World().Hash()
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("youth-intake resume diverged:\nA %s\nB %s", ha, hb)
	}
}

// driftRollsByPlayer tallies queued player-drift events per entity id.
func driftRollsByPlayer(e *Engine) map[int64]int {
	out := map[int64]int{}
	evs, _ := e.Queue().Snapshot()
	for _, ev := range evs {
		if ev.Payload == worldgen.PayloadPlayerDrift {
			out[ev.EntityID]++
		}
	}
	return out
}
