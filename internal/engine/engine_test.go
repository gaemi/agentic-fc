package engine

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// tokens is a deterministic entropy source (tokens never affect outcomes).
type tokens struct{ n uint32 }

func (t *tokens) Read(p []byte) (int, error) {
	for i := range p {
		t.n++
		p[i] = byte((t.n * 2654435761) >> 24)
	}
	return len(p), nil
}

func newEngine(t *testing.T, seed uint64) (*Engine, *store.MemAuditLog) {
	t.Helper()
	res, err := worldgen.Generate(worldgen.PresetCompact(seed), worldgen.WithTokenReader(&tokens{}))
	if err != nil {
		t.Fatal(err)
	}
	audit := &store.MemAuditLog{}
	return New(res.World, res.Queue, audit), audit
}

func day(d int) sim.GameTime { return sim.GameTime(int64(d) * sim.MinutesPerDay) }

// TestTempoIndependence locks the tempo invariant (docs/03 §4, NFR-2):
// same seed + config ⇒ identical world hash AND identical audit trail after
// N game-days, no matter how the run is paced or chunked.
func TestTempoIndependence(t *testing.T) {
	const horizon = 30

	// Run A: one big drain.
	ea, auditA := newEngine(t, 99)
	if _, err := ea.RunUntil(day(horizon)); err != nil {
		t.Fatal(err)
	}

	// Run B: day-by-day chunks.
	eb, auditB := newEngine(t, 99)
	for d := 1; d <= horizon; d++ {
		if _, err := eb.RunUntil(day(d)); err != nil {
			t.Fatal(err)
		}
	}

	// Run C: real-time runner with an instant sleeper at a different speed.
	ec, auditC := newEngine(t, 99)
	runner := NewRunner(ec, Pacer{Speed: sim.Speed60, IdleAcceleration: 8}, func(context.Context, time.Duration) error { return nil }, nil)
	if err := runner.Run(context.Background(), day(horizon)); err != nil {
		t.Fatal(err)
	}

	ha, _ := ea.World().Hash()
	hb, _ := eb.World().Hash()
	hc, _ := ec.World().Hash()
	if ha != hb || ha != hc {
		t.Fatalf("pacing changed outcomes:\nA %s\nB %s\nC %s", ha, hb, hc)
	}
	compareAudits(t, auditA, auditB)
	compareAudits(t, auditA, auditC)
}

func compareAudits(t *testing.T, a, b *store.MemAuditLog) {
	t.Helper()
	if len(a.Entries) != len(b.Entries) {
		t.Fatalf("audit lengths differ: %d vs %d", len(a.Entries), len(b.Entries))
	}
	for i := range a.Entries {
		x, y := a.Entries[i], b.Entries[i]
		if x.GameTime != y.GameTime || x.EntityKind != y.EntityKind ||
			x.EntityID != y.EntityID || x.Category != y.Category ||
			x.Outcome != y.Outcome || x.NextRoll != y.NextRoll ||
			x.MindsetVersion != y.MindsetVersion || !bytes.Equal(x.Factors, y.Factors) {
			t.Fatalf("audit diverges at %d:\n%+v\n%+v", i, x, y)
		}
	}
}

// TestDriftRespectsContracts: after a season of drift, every player still
// honors the attribute scale, the cost model, and the Potential Cap.
func TestDriftRespectsContracts(t *testing.T) {
	e, audit := newEngine(t, 7)
	if _, err := e.RunUntil(day(180)); err != nil {
		t.Fatal(err)
	}
	w := e.World()
	var grew, declined int
	for _, en := range audit.Entries {
		if en.Category != "drift" {
			continue
		}
		if len(en.Outcome) > 5 && en.Outcome[:5] == "grew_" {
			grew++
		}
		if len(en.Outcome) > 9 && en.Outcome[:9] == "declined_" {
			declined++
		}
	}
	if grew == 0 || declined == 0 {
		t.Fatalf("half a season produced grew=%d declined=%d; drift is dead", grew, declined)
	}
	for i := range w.Players {
		p := &w.Players[i]
		for a, v := range p.Visible {
			if v < attr.ScaleMin || v > attr.ScaleMax {
				t.Fatalf("player %d %s = %d out of scale", p.ID, a, v)
			}
		}
		cost := attr.ProfilePoolCost(p.Group, p.Visible, p.WeakFoot)
		if p.AbilityPool > p.PotentialCap {
			t.Fatalf("player %d broke the Potential Cap: pool %d > cap %d (cost %.1f)", p.ID, p.AbilityPool, p.PotentialCap, cost)
		}
		if p.AbilityPool != int(cost+0.5) {
			t.Fatalf("player %d pool %d out of sync with cost %.1f", p.ID, p.AbilityPool, cost)
		}
	}
}

// TestFinanceTicks: balances move weekly under the revenue model and every
// tick is audited with named factors.
func TestFinanceTicks(t *testing.T) {
	e, audit := newEngine(t, 11)
	if _, err := e.RunUntil(day(28)); err != nil {
		t.Fatal(err)
	}
	ticks := map[int64]int{}
	for _, en := range audit.Entries {
		if en.Category == "finance" {
			ticks[en.EntityID]++
			if len(en.Factors) == 0 {
				t.Fatal("finance tick without factors")
			}
		}
	}
	for _, c := range e.World().Clubs {
		if ticks[c.ID] < 3 || ticks[c.ID] > 5 {
			t.Errorf("club %d ticked %d times in 28 days, want ~4", c.ID, ticks[c.ID])
		}
	}
}

// TestDecisionAuditStampsMindsetVersion (FR-16e): manager decision rolls
// record the Mindset version they consumed.
func TestDecisionAuditStampsMindsetVersion(t *testing.T) {
	e, audit := newEngine(t, 13)
	mgr := &e.World().Managers[0]
	if err := mgr.Mindset.SetPriorities(mgr.Mindset.Priorities); err != nil {
		t.Fatal(err)
	} // bump version to 1
	if _, err := e.RunUntil(day(10)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, en := range audit.Entries {
		if en.Category == "decision" && en.EntityID == mgr.ID {
			found = true
			if en.MindsetVersion != mgr.Mindset.Version {
				t.Fatalf("decision recorded version %d, mindset is %d",
					en.MindsetVersion, mgr.Mindset.Version)
			}
		}
	}
	if !found {
		t.Fatal("no decision roll for manager 0 in 10 days")
	}
}

// TestMatchWindowTempo: league-wide windows at base speed, in-season gaps are
// idle, and the fixtureless calendar is off-season.
func TestMatchWindowTempo(t *testing.T) {
	e, _ := newEngine(t, 5)
	kickoff := e.kickoffs[0]
	if got := e.TempoAt(kickoff); got != sim.TempoMatch {
		t.Fatalf("tempo at kickoff = %s, want MATCH", got)
	}
	if got := e.TempoAt(kickoff + MatchWindowMinutes - 1); got != sim.TempoMatch {
		t.Fatalf("tempo at window end-1 = %s, want MATCH", got)
	}
	if got := e.TempoAt(kickoff + MatchWindowMinutes); got != sim.TempoIdle {
		t.Fatalf("tempo after window = %s, want IDLE", got)
	}
	if got := e.TempoAt(kickoff - 1); got != sim.TempoOffseason {
		t.Fatalf("tempo before first kickoff = %s, want OFFSEASON", got)
	}
	last := e.kickoffs[len(e.kickoffs)-1]
	if got := e.TempoAt(last + MatchWindowMinutes); got != sim.TempoOffseason {
		t.Fatalf("tempo after final window = %s, want OFFSEASON", got)
	}
}

// TestPacerMath: docs/02 §5.2 examples — at 15×, a 2-game-hour match window
// ≈ 8 real minutes; an idle game day at 16× base = 6 real minutes.
func TestPacerMath(t *testing.T) {
	p := Pacer{Speed: sim.Speed15, IdleAcceleration: sim.DefaultIdleAcceleration, OffseasonAcceleration: sim.DefaultOffseasonAcceleration}
	if got := time.Duration(MatchWindowMinutes) * p.RealPerGameMinute(sim.TempoMatch); got != 8*time.Minute {
		t.Fatalf("match window = %s real, want 8m", got)
	}
	if got := time.Duration(sim.MinutesPerDay) * p.RealPerGameMinute(sim.TempoIdle); got != 6*time.Minute {
		t.Fatalf("idle day = %s real, want 6m", got)
	}
	wantOffseason := time.Duration(sim.MinutesPerDay) * (time.Minute / time.Duration(15*sim.DefaultOffseasonAcceleration))
	if got := time.Duration(sim.MinutesPerDay) * p.RealPerGameMinute(sim.TempoOffseason); got != wantOffseason {
		t.Fatalf("off-season day = %s real, want %s", got, wantOffseason)
	}
	if got := p.RealPerGameMinute(sim.TempoPaused); got != 0 {
		t.Fatalf("paused must not advance, got %s", got)
	}
}

// TestRealDurationIntegration: a stretch containing exactly one match
// window costs window-at-base + rest-at-idle.
func TestRealDurationIntegration(t *testing.T) {
	e, _ := newEngine(t, 5)
	p := Pacer{Speed: sim.Speed15, IdleAcceleration: 4}
	k := e.kickoffs[0]
	for _, candidate := range e.kickoffs {
		if candidate > e.kickoffs[0]+MatchWindowMinutes {
			k = candidate
			break
		}
	}
	from, to := k-60, k+MatchWindowMinutes+60
	want := time.Duration(120)*p.RealPerGameMinute(sim.TempoIdle) +
		time.Duration(MatchWindowMinutes)*p.RealPerGameMinute(sim.TempoMatch)
	if got := e.RealDuration(p, from, to); got != want {
		t.Fatalf("integrated duration %s, want %s", got, want)
	}
}

// TestPauseFreezesWorld (A11): SetPaused(true) is a barrier — once it
// returns, the game clock must not advance until resume, no matter what
// the runner was doing.
func TestPauseFreezesWorld(t *testing.T) {
	e, _ := newEngine(t, 17)
	var mu sync.RWMutex
	r := NewRunner(e, Pacer{Speed: sim.Speed60, IdleAcceleration: 8}, func(context.Context, time.Duration) error { return nil }, &mu)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Run(ctx, day(365))
	}()

	// Let it advance, then pause. SetPaused must return only once frozen.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.RLock()
		now := e.Now()
		mu.RUnlock()
		if now > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner never advanced")
		}
	}
	r.SetPaused(true)
	mu.RLock()
	frozen := e.Now()
	mu.RUnlock()
	time.Sleep(50 * time.Millisecond)
	mu.RLock()
	after := e.Now()
	mu.RUnlock()
	if after != frozen {
		t.Fatalf("world advanced while paused: %s → %s", frozen, after)
	}

	r.SetPaused(false)
	deadline = time.Now().Add(2 * time.Second)
	for {
		mu.RLock()
		now := e.Now()
		mu.RUnlock()
		if now > frozen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner never resumed")
		}
	}
	cancel()
	<-done
}

// TestQueueNeverStarves: the self-scheduling loop keeps every recurring
// system alive — after a long run there are still future drift, finance,
// and decision events queued.
func TestQueueNeverStarves(t *testing.T) {
	e, _ := newEngine(t, 3)
	if _, err := e.RunUntil(day(60)); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for {
		ev := e.Queue().Pop()
		if ev == nil {
			break
		}
		kinds[ev.Payload.(string)] = true
	}
	for _, want := range []string{
		worldgen.PayloadPlayerDrift, worldgen.PayloadFinanceTick, worldgen.PayloadDecisionRoll,
	} {
		if !kinds[want] {
			t.Errorf("no future %s events after 60 days — system starved", want)
		}
	}
}
