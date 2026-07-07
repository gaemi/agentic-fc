package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestPlayerBirthdaysAtSeasonBoundary locks the aging unfreeze (player
// lifecycle): every active player is one year older across the boundary, youth
// included — while a retired player's age is frozen at its retirement age.
func TestPlayerBirthdaysAtSeasonBoundary(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	frozen := &e.world.Players[0]
	e.retirePlayer(&sim.Event{Due: e.Now()}, frozen)
	frozenAge := frozen.Age

	ages := make(map[int64]int, len(e.world.Players))
	for i := range e.world.Players {
		ages[e.world.Players[i].ID] = e.world.Players[i].Age
	}
	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ID == frozen.ID {
			continue
		}
		if !p.Retired && p.Age != ages[p.ID]+1 {
			t.Fatalf("player %d age %d after the boundary, want %d (+1)", p.ID, p.Age, ages[p.ID]+1)
		}
	}
	if frozen.Age != frozenAge {
		t.Fatalf("retired player aged %d → %d — retirement must freeze age", frozenAge, frozen.Age)
	}
}

// TestYouthGraduation locks the academy → senior seam: after a boundary no
// academy player is at or past the graduation age (everyone of age flipped to
// senior), at least one graduation actually happened, and the graduates joined
// the senior squad count. Count-only news filed per club (FR-22).
func TestYouthGraduation(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	// A 17-year-old academy prospect must graduate at this boundary; a younger
	// one must stay. Pin one of each so the assertion can't be vacuous. The
	// graduate's deal must run past this season — a graduate whose academy
	// contract expires at the same boundary faces the senior renew-or-lapse
	// and may legitimately be released (the contracts phase), which would fail
	// the squad-count assertion for the wrong reason.
	var turning, staying int64
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if !p.Youth || p.ClubID == 0 {
			continue
		}
		if p.Age == youthGraduationAge-1 && turning == 0 &&
			p.Contract != nil && p.Contract.ExpirySeasonYear >= 2 {
			turning = p.ID
		}
		if p.Age < youthGraduationAge-1 && staying == 0 {
			staying = p.ID
		}
	}
	if turning == 0 || staying == 0 {
		t.Fatalf("test vacuous: no youth aged %d and %d found before the boundary", youthGraduationAge-1, youthGraduationAge-2)
	}
	seniorBefore := worldgen.SquadSize(e.world, e.players[turning].ClubID)

	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.Youth && p.Age >= youthGraduationAge {
			t.Fatalf("player %d is still youth at age %d — graduation missed them", p.ID, p.Age)
		}
	}
	grad := e.players[turning]
	if grad.Youth {
		t.Fatalf("player %d turned %d but did not graduate", turning, grad.Age)
	}
	if got := worldgen.SquadSize(e.world, grad.ClubID); got <= seniorBefore {
		t.Fatalf("senior squad %d after graduation, want > %d — graduates must count", got, seniorBefore)
	}
	if e.players[staying].Youth != true {
		t.Fatalf("underage prospect %d graduated early", staying)
	}
	if !hasNews(e, NewsYouthGraduated) {
		t.Fatal("no graduation news filed")
	}
}

// TestPlayerRetirement locks the retirement mechanics: a veteran certain to
// retire leaves the club with its wage shed from the bill cache, its contract
// gone, named news filed — and its drift chain terminated (the queued roll dies
// at the RETIRED guard without mutating the player or rescheduling).
func TestPlayerRetirement(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	var vet *worldgen.Player
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != 0 && !p.Youth && p.Contract != nil {
			vet = p
			break
		}
	}
	vet.Age = playerRetireAgeCeil + 5 // retirement certain at this boundary
	clubID := vet.ClubID

	if _, err := e.RunUntil(day(365)); err != nil {
		t.Fatal(err)
	}
	if !vet.Retired || vet.ClubID != 0 || vet.Contract != nil {
		t.Fatalf("veteran not retired cleanly: retired=%v club=%d contract=%v", vet.Retired, vet.ClubID, vet.Contract)
	}
	var bill int64
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID == clubID && p.Contract != nil {
			bill += p.Contract.WageWeeklyMinor
		}
	}
	if e.clubs[clubID].WageBillWeeklyMinor != bill {
		t.Fatalf("bill cache %d != contract sum %d after retirement — wage not shed", e.clubs[clubID].WageBillWeeklyMinor, bill)
	}
	if !hasNews(e, newsPlayerRetired) {
		t.Fatal("no retirement news filed")
	}

	// The guard must eat the queued drift roll WITHOUT mutating the retiree —
	// plant a recognizable condition and let the pending roll fire.
	vet.Condition = 41
	if _, err := e.RunUntil(day(365 + 30)); err != nil {
		t.Fatal(err)
	}
	if vet.Condition != 41 {
		t.Fatalf("retired player's condition moved 41 → %d — the drift guard must sit before the recovery writes", vet.Condition)
	}
	events, _ := e.Queue().Snapshot()
	for _, ev := range events {
		if ev.EntityID == vet.ID && ev.Payload == worldgen.PayloadPlayerDrift {
			t.Fatal("retired player still has a drift roll queued — the chain must terminate")
		}
	}
}

// TestRetiredExcludedFromMarket locks the signability gate: a retired player
// shares ClubID 0 with real free agents, so both market paths — the autonomous
// buyer's scan and an explicit SIGN directive — must skip on the flag alone.
func TestRetiredExcludedFromMarket(t *testing.T) {
	e, _ := newEngine(t, 42)
	buyer := &e.world.Clubs[0]
	buyer.WageBudgetWeeklyMinor, buyer.TransferBudgetMinor = 1<<50, 1<<50

	best := e.bestAffordableTarget(buyer, map[int64]bool{})
	if best == nil {
		t.Fatal("no affordable target in a fresh world")
	}
	best.Retired = true
	if again := e.bestAffordableTarget(buyer, map[int64]bool{}); again != nil && again.ID == best.ID {
		t.Fatal("autonomous scan picked a retired player")
	}

	d := mindset.Directive{
		Verb: mindset.VerbSign, Target: mindset.Target{Player: best.ID},
		Strength: mindset.StrengthAbsolute,
	}
	// day 10 sits inside the open pre-season window (see transfer tests).
	if e.attemptSigning(&sim.Event{Due: day(10)}, buyer, d) {
		t.Fatal("explicit SIGN completed on a retired player")
	}
}

// TestShortSquadSurvivesBoundary locks the forced-collapse case: a club
// gutted by retirements below a full XI still crosses the boundary and plays
// its season-2 fixtures (selectXI fields what exists; the autonomous market
// refills deficits on its own cadence). Thin squads are a supported state:
// the world must never stall on one.
func TestShortSquadSurvivesBoundary(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}
	victim := e.world.Clubs[0].ID
	kept := 0
	ev := &sim.Event{Due: e.Now()}
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != victim || p.Youth {
			continue
		}
		if kept < 8 {
			kept++
			continue
		}
		e.retirePlayer(ev, p)
	}
	if got := worldgen.SquadSize(e.world, victim); got != 8 {
		t.Fatalf("setup: squad = %d, want 8", got)
	}

	horizon := day(430) // season-2 rounds underway (cross-boundary test's horizon)
	if _, err := e.RunUntil(horizon); err != nil {
		t.Fatalf("world stalled on a thin squad: %v", err)
	}
	played := 0
	for _, r := range e.world.Results {
		if worldgen.DateOf(r.Kickoff).Season == 2 && (r.HomeID == victim || r.AwayID == victim) {
			played++
		}
	}
	if played == 0 {
		t.Fatal("the gutted club played no season-2 match — the fixture path choked on a short XI")
	}
}

// TestPlayerLifecycleDeterminismAcrossTempo is the phase invariant (NFR-2): a
// run whose player population churns across the boundary — birthdays, natural
// graduations and retirements everywhere — reaches the identical hash one-shot
// vs day-chunked. Every retirement draw is an id+season-keyed stream and the
// passes run in slice order, so chunking cannot reorder anything.
func TestPlayerLifecycleDeterminismAcrossTempo(t *testing.T) {
	const seed = 42
	horizon := day(worldgen.DaysPerSeason + 30)

	ea := newEngineCfg(t, worldgen.DefaultConfig(seed))
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if !churned(ea) {
		t.Fatal("test vacuous: no graduation or retirement within the horizon")
	}
	eb := newEngineCfg(t, worldgen.DefaultConfig(seed))
	for eb.Now() < horizon {
		to := eb.Now() + day(7)
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
		t.Fatalf("player lifecycle not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestPlayerLifecycleResumeAcrossSeasonBoundary is the phase invariant (FR-28,
// NFR-2): a snapshot taken before the boundary resumes, runs the whole
// birthday/graduation/retirement pass itself, and lands on the same hash as the
// uninterrupted run — with real churn, so the invariant isn't proven on a no-op.
func TestPlayerLifecycleResumeAcrossSeasonBoundary(t *testing.T) {
	const seed = 42
	horizon := day(worldgen.DaysPerSeason + 30)

	ea := newEngineCfg(t, worldgen.DefaultConfig(seed))
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if !churned(ea) {
		t.Fatal("test vacuous: no graduation or retirement within the horizon")
	}

	eb := newEngineCfg(t, worldgen.DefaultConfig(seed))
	if _, err := eb.RunUntil(day(worldgen.DaysPerSeason - 5)); err != nil {
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
	hc, _ := resumed.World().Hash()
	if ha != hc {
		t.Fatalf("resume across the lifecycle boundary diverged:\nA %s\nC %s", ha, hc)
	}
}

// churned reports whether the run produced at least one graduation AND one
// retirement — the churn floor the determinism invariants require.
func churned(e *Engine) bool {
	return hasNews(e, NewsYouthGraduated) &&
		(hasNews(e, newsPlayerRetired) || hasNews(e, newsPlayerRetiredFree))
}
