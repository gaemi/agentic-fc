package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// installCaretaker force-sacks a club's manager so a caretaker takes over, and returns
// the vacant club plus the caretaker's id — the starting state for a hiring test.
func installCaretaker(t *testing.T, e *Engine) (*worldgen.Club, int64) {
	t.Helper()
	mid := employedManagers(t, e, 1)[0]
	club := e.clubs[e.managers[mid].ClubID]
	club.SackState = sackUltimatum
	club.UltimatumUntil = day(50)
	club.UltimatumStartPoints = 999
	club.Confidence = 10
	e.sackManager(&sim.Event{Due: day(51)}, club)
	ct := e.clubManager(club.ID)
	if ct == nil || !ct.Caretaker {
		t.Fatalf("caretaker not installed: %+v", ct)
	}
	return club, ct.ID
}

// TestHiringAppointsFromPool locks the external-hire path (hiring): with an
// eligible unemployed candidate available, a caretaker's vacancy resolves by
// appointing that manager — who keeps their own token/roll (only ClubID flips) —
// while the displaced caretaker retires (never pooled), the board resets to a
// honeymoon, and an appointment news is filed.
func TestHiringAppointsFromPool(t *testing.T) {
	e, _ := newEngine(t, 42)
	club, caretakerID := installCaretaker(t, e)

	// Guarantee an eligible candidate (rep clears the tier-1 floor).
	var candidate bool
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.ClubID == 0 && m.Status != worldgen.ManagerRetired && m.ID != caretakerID {
			m.Reputation = 9000
			candidate = true
			break
		}
	}
	if !candidate {
		t.Fatal("no unemployed manager available to stand as a candidate")
	}

	// Drive vacancy reviews (each ~25% to act) until the board appoints externally.
	hired := false
	for d := 52; d < 600 && !hired; d++ {
		ct := e.clubManager(club.ID)
		if ct == nil || !ct.Caretaker {
			break
		}
		hired = e.resolveVacancy(&sim.Event{Due: day(d)}, ct)
	}
	if !hired {
		t.Fatal("no external hire occurred despite an eligible high-rep candidate")
	}

	mgr := e.clubManager(club.ID)
	if mgr == nil || mgr.ID == caretakerID || mgr.Caretaker {
		t.Fatalf("club not run by an external permanent manager: %+v", mgr)
	}
	if got := e.managers[caretakerID]; got.Status != worldgen.ManagerRetired || got.ClubID != 0 {
		t.Fatalf("displaced caretaker not retired-and-unemployed: Status=%q ClubID=%d", got.Status, got.ClubID)
	}
	if club.Confidence != caretakerHoneymoon {
		t.Fatalf("confidence not reset to honeymoon: got %d want %d", club.Confidence, caretakerHoneymoon)
	}
	if !hasNews(e, "news.board.appointed") {
		t.Fatal("appointment news not filed")
	}
}

// TestHiringMakesCaretakerPermanent locks the convert-in-place path (FR-14d,
// "occasionally a caretaker earns the job permanently"): when no unemployed manager
// clears the club's reputation bar, the board keeps the caretaker — same entity, now
// a permanent manager — and files the permanent-appointment news.
func TestHiringMakesCaretakerPermanent(t *testing.T) {
	e, _ := newEngine(t, 42)
	club, caretakerID := installCaretaker(t, e)

	// Empty the candidate pool: no unemployed manager clears the tier floor.
	for i := range e.world.Managers {
		if e.world.Managers[i].ClubID == 0 {
			e.world.Managers[i].Reputation = 0
		}
	}

	for d := 52; d < 600; d++ {
		ct := e.clubManager(club.ID)
		if ct == nil || !ct.Caretaker {
			break
		}
		if e.resolveVacancy(&sim.Event{Due: day(d)}, ct) {
			t.Fatal("external hire despite an empty candidate pool")
		}
	}

	mgr := e.clubManager(club.ID)
	if mgr == nil || mgr.ID != caretakerID {
		t.Fatalf("caretaker replaced despite empty pool: %+v", mgr)
	}
	if mgr.Caretaker {
		t.Fatal("caretaker never confirmed permanent")
	}
	if !hasNews(e, "news.board.caretaker_permanent") {
		t.Fatal("permanent-appointment news not filed")
	}
}

// TestSackedCaretakerRetires locks the A2×B2 seam fix: a caretaker
// sacked before its vacancy is filled RETIRES rather than lingering as an unemployed
// Caretaker==true manager — which a later vacancy could appoint, then wrongly treat
// the new club as a live vacancy, breaking the bounded-population invariant.
func TestSackedCaretakerRetires(t *testing.T) {
	e, _ := newEngine(t, 42)
	club, caretakerID := installCaretaker(t, e)

	// Sack the caretaker itself: a failed ultimatum on its club.
	club.SackState = sackUltimatum
	club.UltimatumUntil = day(60)
	club.UltimatumStartPoints = 999
	club.Confidence = 5
	e.sackManager(&sim.Event{Due: day(61)}, club)

	if got := e.managers[caretakerID]; got.Status != worldgen.ManagerRetired {
		t.Fatalf("sacked caretaker Status=%q, want RETIRED (must not linger as a hire candidate)", got.Status)
	}
	// The invariant: no ACTIVE caretaker is ever in the unemployed pool.
	for i := range e.world.Managers {
		m := &e.world.Managers[i]
		if m.Caretaker && m.Status != worldgen.ManagerRetired && m.ClubID == 0 {
			t.Fatalf("manager %d is a pooled ACTIVE caretaker — the leak is open", m.ID)
		}
	}
}

// TestRetiredManagerRollTerminates locks the roll-chain guard (hiring): a RETIRED
// manager's decision roll is a no-op that does NOT reschedule (the chain ends), while
// an ACTIVE manager's roll reschedules as before — so a displaced caretaker stops
// consuming the queue without leaving a dangling event.
func TestRetiredManagerRollTerminates(t *testing.T) {
	e, _ := newEngine(t, 42)
	mids := employedManagers(t, e, 2)

	rolls := func(id int64) int {
		evs, _ := e.Queue().Snapshot()
		n := 0
		for _, ev := range evs {
			if ev.EntityID == id && ev.Payload == worldgen.PayloadDecisionRoll {
				n++
			}
		}
		return n
	}

	// RETIRED: the roll terminates — no new roll scheduled.
	retired := e.managers[mids[0]]
	retired.Status = worldgen.ManagerRetired
	before := rolls(mids[0])
	if err := e.handleDecisionRoll(&sim.Event{Due: day(10), EntityID: mids[0], Payload: worldgen.PayloadDecisionRoll}); err != nil {
		t.Fatal(err)
	}
	if after := rolls(mids[0]); after != before {
		t.Fatalf("retired manager's roll rescheduled (%d → %d) — chain did not terminate", before, after)
	}

	// Control: an ACTIVE manager's roll reschedules (the chain continues).
	before2 := rolls(mids[1])
	if err := e.handleDecisionRoll(&sim.Event{Due: day(10), EntityID: mids[1], Payload: worldgen.PayloadDecisionRoll}); err != nil {
		t.Fatal(err)
	}
	if after2 := rolls(mids[1]); after2 != before2+1 {
		t.Fatalf("active manager's roll did not reschedule (%d → %d)", before2, after2)
	}
}

// TestHiringDeterminismAcrossTempo is a phase invariant (NFR-2): a run that installs a
// caretaker and lets the job market resolve the vacancy reaches the identical world
// hash whether run in one shot or chunked (the tempo/resume proxy) — hiring draws a
// labelled stream keyed on club + due, never wall-clock or chunk boundaries.
func TestHiringDeterminismAcrossTempo(t *testing.T) {
	const seed = 42
	horizon := day(220)
	setup := func() *Engine {
		e, _ := newEngine(t, seed)
		mid := employedManagers(t, e, 1)[0]
		club := e.clubs[e.managers[mid].ClubID]
		e.sackManager(&sim.Event{Due: day(20)}, club) // install a caretaker up front
		return e
	}

	ea := setup()
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if !hasNews(ea, "news.board.appointed") && !hasNews(ea, "news.board.caretaker_permanent") {
		t.Fatal("test vacuous: no vacancy resolved within the horizon")
	}

	eb := setup()
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
		t.Fatalf("hiring not tempo-independent:\nA %s\nB %s", ha, hb)
	}
}

// TestHiringResumeAcrossSeam is a phase invariant (FR-28, NFR-2): a snapshot taken
// after a caretaker is installed but while the vacancy is still open round-trips
// through the store and the resumed run reaches the same hash as an uninterrupted one
// — the hire lives entirely in World + the queue, so nothing about it is lost to a
// restart.
func TestHiringResumeAcrossSeam(t *testing.T) {
	const seed = 42
	horizon := day(220)
	setup := func() *Engine {
		e, _ := newEngine(t, seed)
		mid := employedManagers(t, e, 1)[0]
		club := e.clubs[e.managers[mid].ClubID]
		e.sackManager(&sim.Event{Due: day(20)}, club)
		return e
	}

	ea := setup()
	if _, err := ea.RunUntil(horizon); err != nil {
		t.Fatal(err)
	}
	if !hasNews(ea, "news.board.appointed") && !hasNews(ea, "news.board.caretaker_permanent") {
		t.Fatal("test vacuous: no vacancy resolved within the horizon")
	}

	eb := setup()
	if _, err := eb.RunUntil(day(30)); err != nil { // caretaker installed (day 20), vacancy still open
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
		t.Fatalf("hiring resume diverged from the uninterrupted run:\nA %s\nB %s", ha, hb)
	}
}
