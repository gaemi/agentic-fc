package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// seasonEnd is the game time of the first season-end rollover, when the careers
// birthday/retirement/newgen pass runs.
func seasonEnd() sim.GameTime { return day(worldgen.DaysPerSeason) }

// TestManagerBirthdayAtSeasonBoundary locks the aging tick (manager careers): every
// pre-existing manager advances exactly one year as the season rolls over — the
// clock age-driven retirement reads from.
func TestManagerBirthdayAtSeasonBoundary(t *testing.T) {
	e, _ := newEngine(t, 42)
	id := employedManagers(t, e, 1)[0]
	before := e.managers[id].Age
	if _, err := e.RunUntil(day(worldgen.DaysPerSeason + 2)); err != nil {
		t.Fatal(err)
	}
	if after := e.managers[id].Age; after != before+1 {
		t.Fatalf("manager age did not advance across the season boundary: %d → %d", before, after)
	}
}

// TestRetirementExemptsRecentAvatar locks FR-14e and the zero-sentinel fix
// review): at a certain-retirement age, an autonomous manager (LastActiveGameTime 0)
// retires, while one whose bound Agent was recently active is exempt and survives —
// proving the exemption is gated on a real activity stamp, not the `now - 0 < window`
// trap that would spuriously exempt every manager early in the world.
func TestRetirementExemptsRecentAvatar(t *testing.T) {
	e, _ := newEngine(t, 42)
	n := len(e.world.Managers)
	ids := employedManagers(t, e, 2)
	for _, id := range ids {
		e.managers[id].Age = retireAgeCeil // certain retirement
	}
	e.managers[ids[0]].LastActiveGameTime = 0      // never active ⇒ autonomous ⇒ NOT exempt
	e.managers[ids[1]].LastActiveGameTime = day(1) // recent session ⇒ exempt

	e.processManagerCareers(&sim.Event{Due: seasonEnd()}, 2, n)

	if e.managers[ids[0]].Status != worldgen.ManagerRetired {
		t.Fatal("autonomous manager at the retirement ceiling did not retire (zero-sentinel or age gate broken)")
	}
	if e.managers[ids[1]].Status == worldgen.ManagerRetired {
		t.Fatal("manager with a live avatar binding retired — FR-14e exemption not honoured")
	}
}

// TestRetirementEmployedInstallsCaretaker locks the employed-retiree path (careers
// C, FR-14d): retiring a club's manager marks it RETIRED and installs a caretaker in
// the same tick, so the club is never left unmanaged, and files the retirement news.
func TestRetirementEmployedInstallsCaretaker(t *testing.T) {
	e, _ := newEngine(t, 42)
	n := len(e.world.Managers)
	id := employedManagers(t, e, 1)[0]
	clubID := e.managers[id].ClubID
	e.managers[id].Age = retireAgeCeil // certain retirement

	e.processManagerCareers(&sim.Event{Due: seasonEnd()}, 2, n)

	if e.managers[id].Status != worldgen.ManagerRetired || e.managers[id].ClubID != 0 {
		t.Fatalf("retired manager not marked RETIRED-and-unemployed: Status=%q ClubID=%d",
			e.managers[id].Status, e.managers[id].ClubID)
	}
	ct := e.clubManager(clubID)
	if ct == nil || !ct.Caretaker {
		t.Fatalf("no caretaker installed after an employed retirement: %+v", ct)
	}
	if !hasNews(e, "news.board.retired") {
		t.Fatal("retirement news not filed")
	}
}

// TestRetirementUnemployedNoCaretaker locks the unemployed-retiree path: a jobless
// manager simply leaves (RETIRED, no vacancy, no caretaker), still surfaced as news.
// Calls retireManager directly so no other manager's boundary retirement muddies the
// "no caretaker spawned" assertion.
func TestRetirementUnemployedNoCaretaker(t *testing.T) {
	e, _ := newEngine(t, 42)
	var m *worldgen.Manager
	for i := range e.world.Managers {
		if e.world.Managers[i].ClubID == 0 {
			m = &e.world.Managers[i]
			break
		}
	}
	if m == nil {
		t.Fatal("no unemployed manager in the generated pool")
	}
	id := m.ID
	managersBefore := len(e.world.Managers)

	e.retireManager(&sim.Event{Due: seasonEnd()}, m)
	e.rebuildManagerIndex()

	if e.managers[id].Status != worldgen.ManagerRetired {
		t.Fatal("unemployed manager not marked RETIRED")
	}
	if len(e.world.Managers) != managersBefore {
		t.Fatal("a jobless retirement spawned a caretaker — none should be installed with no club")
	}
	if !hasNews(e, "news.board.retired") {
		t.Fatal("retirement news not filed for a jobless manager")
	}
}

// TestNewgenBackfillsPool locks the newgen intake (manager careers): draining the entire
// unemployed pool and running the boundary refills it back to the target size with
// freshly generated managers.
func TestNewgenBackfillsPool(t *testing.T) {
	e, _ := newEngine(t, 42)
	target := worldgen.UnemployedPoolTarget(e.world)
	for i := range e.world.Managers {
		if e.world.Managers[i].ClubID == 0 {
			e.world.Managers[i].Status = worldgen.ManagerRetired // drain the pool
		}
	}
	before := len(e.world.Managers)

	e.backfillManagerPool(&sim.Event{Due: seasonEnd()}, 2)

	have := 0
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.ClubID == 0 && m.Status != worldgen.ManagerRetired && !m.Caretaker {
			have++
		}
	}
	if have != target {
		t.Fatalf("pool not refilled to target: have %d want %d", have, target)
	}
	if len(e.world.Managers) != before+target {
		t.Fatalf("newgen count wrong: appended %d want %d", len(e.world.Managers)-before, target)
	}
}

// TestNewgenGetsDecisionRoll locks the careers-C × B2 seam: a newgen manager is
// spawned WITH a decision roll (like every generated manager), so if it is later
// hired the "exactly one self-rescheduling roll per manager" invariant holds and it
// actually reviews. Without this a hired newgen would sit inert forever.
func TestNewgenGetsDecisionRoll(t *testing.T) {
	e, _ := newEngine(t, 42)
	for i := range e.world.Managers {
		if e.world.Managers[i].ClubID == 0 {
			e.world.Managers[i].Status = worldgen.ManagerRetired
		}
	}
	before := len(e.world.Managers)

	e.backfillManagerPool(&sim.Event{Due: seasonEnd()}, 2)

	// Every appended newgen must have exactly one decision roll queued.
	evs, _ := e.Queue().Snapshot()
	for i := before; i < len(e.world.Managers); i++ {
		id := e.world.Managers[i].ID
		rolls := 0
		for _, ev := range evs {
			if ev.EntityID == id && ev.Payload == worldgen.PayloadDecisionRoll {
				rolls++
			}
		}
		if rolls != 1 {
			t.Fatalf("newgen manager %d has %d decision rolls, want exactly 1", id, rolls)
		}
	}
}

// TestManagerCareersDeterminismAcrossTempo is a phase invariant (NFR-2): a run that
// churns the manager population across a season boundary (forced retirements +
// caretaker installs + newgen backfill) reaches the identical world hash whether run
// in one shot or chunked — every draw is a labelled stream keyed on id/club/season,
// never wall-clock or chunk boundaries.
func TestManagerCareersDeterminismAcrossTempo(t *testing.T) {
	const seed = 42
	horizon := day(worldgen.DaysPerSeason + 30)

	ha := runCareersChurn(t, seed, func(e *Engine) {
		if _, err := e.RunUntil(horizon); err != nil {
			t.Fatal(err)
		}
	})
	hb := runCareersChurn(t, seed, func(e *Engine) {
		for e.Now() < horizon {
			to := e.Now() + day(7)
			if to > horizon {
				to = horizon
			}
			if _, err := e.RunUntil(to); err != nil {
				t.Fatal(err)
			}
		}
	})
	if ha != hb {
		t.Fatalf("manager careers not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestManagerCareersResumeAcrossSeasonBoundary is a phase invariant (FR-28, NFR-2):
// a snapshot taken before the boundary round-trips through the store and the resumed
// run — which executes the whole birthday/retirement/newgen pass — reaches the same
// hash as an uninterrupted run. It also asserts the avatar exemption READ survives
// the round-trip (the exempt manager is not retired) and that the run actually
// churned (retirements happened), so the invariant isn't proven over a no-op.
func TestManagerCareersResumeAcrossSeasonBoundary(t *testing.T) {
	const seed = 42
	horizon := day(worldgen.DaysPerSeason + 30)
	exemptID := careersChurnExemptID(t, seed)

	ea := careersChurnEngine(t, seed)
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if !hasNews(ea, "news.board.retired") {
		t.Fatal("test vacuous: no retirement occurred within the horizon")
	}
	if ea.managers[exemptID].Status == worldgen.ManagerRetired {
		t.Fatal("exempt avatar manager retired — exemption not honoured across the run")
	}

	eb := careersChurnEngine(t, seed)
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
	hb, _ := resumed.World().Hash()
	if ha != hb {
		t.Fatalf("manager-careers rollover resume diverged:\nA %s\nB %s", ha, hb)
	}
}

// careersChurnEngine builds an engine primed so the first season boundary MUST churn
// the manager population: four employed managers are pushed to the certain-retirement
// age, and the first of them is given a live avatar binding so it is exempt (the rest
// retire, each leaving a vacancy + caretaker). Deterministic for a given seed.
func careersChurnEngine(t *testing.T, seed uint64) *Engine {
	t.Helper()
	e, _ := newEngine(t, seed)
	ids := employedManagers(t, e, 4)
	for _, id := range ids {
		e.managers[id].Age = retireAgeCeil
	}
	e.managers[ids[0]].LastActiveGameTime = day(1) // exempt: a recent session
	return e
}

// careersChurnExemptID returns the id careersChurnEngine exempts, recomputed from a
// throwaway engine (deterministic: same seed ⇒ same first employed manager).
func careersChurnExemptID(t *testing.T, seed uint64) int64 {
	t.Helper()
	e, _ := newEngine(t, seed)
	return employedManagers(t, e, 4)[0]
}

// runCareersChurn builds the churn engine, runs it via drive, and returns the world
// hash.
func runCareersChurn(t *testing.T, seed uint64, drive func(*Engine)) string {
	t.Helper()
	e := careersChurnEngine(t, seed)
	drive(e)
	h, err := e.World().Hash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}
