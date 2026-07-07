package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestSackingEscalation locks the state machine (board review): OK → WARNED →
// ULTIMATUM as confidence falls, and a reprieve back to OK when it recovers, with
// every transition filing a board news item.
func TestSackingEscalation(t *testing.T) {
	e, _ := newEngine(t, 42)
	club := e.clubs[e.managers[employedManagers(t, e, 1)[0]].ClubID]
	ev := &sim.Event{Due: day(100)}

	club.Confidence = sackWarnThreshold - 1
	e.evaluateSacking(ev, club)
	if club.SackState != sackWarned {
		t.Fatalf("state = %q, want WARNED", club.SackState)
	}

	club.Confidence = sackUltimatumThreshold - 1
	e.evaluateSacking(ev, club)
	if club.SackState != sackUltimatum {
		t.Fatalf("state = %q, want ULTIMATUM", club.SackState)
	}
	if club.UltimatumUntil != ev.Due+sim.GameTime(ultimatumDays*sim.MinutesPerDay) {
		t.Fatalf("ultimatum deadline = %d, want %d", club.UltimatumUntil, ev.Due+sim.GameTime(ultimatumDays*sim.MinutesPerDay))
	}

	club.Confidence = sackRecoverThreshold
	e.evaluateSacking(ev, club)
	if club.SackState != sackOK {
		t.Fatalf("state = %q, want OK (reprieve)", club.SackState)
	}

	for _, key := range []string{"news.board.warning", "news.board.ultimatum", "news.board.reprieve"} {
		if !hasNews(e, key) {
			t.Fatalf("no %s news filed", key)
		}
	}
}

// TestSackingInstallsCaretaker locks the dismissal (board review): a failed ultimatum
// unemploys the manager (keeping them ACTIVE + their binding), installs a caretaker
// so the club is never unmanaged, resets confidence to the honeymoon, schedules the
// caretaker's first decision roll, and files the dismissal + appointment news.
func TestSackingInstallsCaretaker(t *testing.T) {
	e, _ := newEngine(t, 42)
	mid := employedManagers(t, e, 1)[0]
	old := e.managers[mid]
	club := e.clubs[old.ClubID]
	clubID := club.ID
	managersBefore := len(e.world.Managers)

	// A failed ultimatum: deadline passed, points unreachable.
	club.SackState = sackUltimatum
	club.UltimatumUntil = day(50)
	club.UltimatumStartPoints = 999
	club.Confidence = 10
	e.evaluateSacking(&sim.Event{Due: day(51)}, club)

	if old.ClubID != 0 || old.Status != worldgen.ManagerActive {
		t.Fatalf("sacked manager: ClubID=%d Status=%q, want 0 / ACTIVE (unemployed, binding kept)", old.ClubID, old.Status)
	}
	if len(e.world.Managers) != managersBefore+1 {
		t.Fatalf("managers = %d, want %d (a caretaker spawned)", len(e.world.Managers), managersBefore+1)
	}
	ct := e.clubManager(clubID)
	if ct == nil || !ct.Caretaker || ct.ClubID != clubID {
		t.Fatalf("caretaker not installed at the club: %+v", ct)
	}
	if club.Confidence != caretakerHoneymoon || club.SackState != sackOK {
		t.Fatalf("post-sack club: confidence=%d state=%q, want %d / OK", club.Confidence, club.SackState, caretakerHoneymoon)
	}
	// The caretaker must actually manage — its first decision roll is queued.
	events, _ := e.Queue().Snapshot()
	scheduled := false
	for _, qe := range events {
		if qe.EntityID == ct.ID && qe.Payload == worldgen.PayloadDecisionRoll {
			scheduled = true
		}
	}
	if !scheduled {
		t.Fatal("caretaker's decision roll not scheduled — it would sit inert")
	}
	if !hasNews(e, "news.board.sacked") || !hasNews(e, "news.board.caretaker") {
		t.Fatal("dismissal / caretaker-appointment news not filed")
	}
}

// TestSackingSurvivesSnapshot locks the sack's resume-safety: a world that has
// dismissed a manager and installed a caretaker round-trips through a snapshot with
// an identical hash — all the new state (unemployed manager, caretaker entity,
// cleared sacking fields) lives in World and is integer/string only.
func TestSackingSurvivesSnapshot(t *testing.T) {
	e, _ := newEngine(t, 42)
	club := e.clubs[e.managers[employedManagers(t, e, 1)[0]].ClubID]
	club.SackState = sackUltimatum
	club.UltimatumUntil = day(50)
	club.UltimatumStartPoints = 999
	club.Confidence = 10
	e.sackManager(&sim.Event{Due: day(51)}, club)

	before, err := e.World().Hash()
	if err != nil {
		t.Fatal(err)
	}
	fstore := &store.FileStore{Dir: t.TempDir()}
	events, nextSeq := e.Queue().Snapshot()
	if err := fstore.SaveSnapshot(&store.Snapshot{Now: e.Now(), World: e.World(), Queue: events, QueueNextSeq: nextSeq}); err != nil {
		t.Fatal(err)
	}
	snap, err := fstore.LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	after, err := snap.World.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("post-sack world did not survive snapshot round-trip:\n%s\n%s", before, after)
	}
}

// TestUltimatumResolvedAtSeasonEnd locks the cross-season fix: a
// pending ultimatum is settled against the final table at season end — a club that
// met the points target is reprieved (keeps its manager), one that fell short is
// sacked — so nothing straddles the rollover's table reset.
func TestUltimatumResolvedAtSeasonEnd(t *testing.T) {
	e, _ := newEngine(t, 42)
	mids := employedManagers(t, e, 2)
	a := e.clubs[e.managers[mids[0]].ClubID]
	a.SackState = sackUltimatum
	a.UltimatumStartPoints = e.clubPoints(a) - ultimatumPointsTarget // gained exactly the target
	b := e.clubs[e.managers[mids[1]].ClubID]
	b.SackState = sackUltimatum
	b.UltimatumStartPoints = e.clubPoints(b) // gained nothing

	managersBefore := len(e.world.Managers)
	e.resolveEndOfSeasonUltimata(&sim.Event{Due: day(worldgen.DaysPerSeason)})

	if a.SackState != sackOK || e.managers[mids[0]].ClubID != a.ID {
		t.Fatalf("club that met the target not reprieved: state=%q mgr.ClubID=%d", a.SackState, e.managers[mids[0]].ClubID)
	}
	if b.SackState != sackOK || len(e.world.Managers) != managersBefore+1 {
		t.Fatalf("club that fell short not sacked: state=%q managers=%d want %d", b.SackState, len(e.world.Managers), managersBefore+1)
	}
}

// TestUltimatumClearedByRollover locks the wiring: a pending ultimatum whose
// deadline would fall in a later season is resolved at the rollover, so no club is
// left in ULTIMATUM to be re-evaluated against a zeroed next-season table.
func TestUltimatumClearedByRollover(t *testing.T) {
	e, _ := newEngine(t, 42)
	club := e.clubs[e.managers[employedManagers(t, e, 1)[0]].ClubID]
	club.SackState = sackUltimatum
	club.UltimatumUntil = day(worldgen.DaysPerSeason * 3) // far future — would straddle the reset
	club.UltimatumStartPoints = 0
	if _, err := e.RunUntil(day(worldgen.DaysPerSeason + 2)); err != nil { // past the season-1 rollover
		t.Fatal(err)
	}
	for i := range e.world.Clubs {
		if e.world.Clubs[i].SackState == sackUltimatum {
			t.Fatalf("club %d still in ULTIMATUM after the rollover — a deadline straddled the table reset", e.world.Clubs[i].ID)
		}
	}
}

func hasNews(e *Engine, key string) bool {
	for i := range e.world.News {
		if e.world.News[i].Key == key {
			return true
		}
	}
	return false
}
