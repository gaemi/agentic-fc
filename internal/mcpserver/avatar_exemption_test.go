package mcpserver

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/sim"
)

// TestAvatarActivityStamp locks manager careers (FR-14e): each ACCEPTED authenticated
// call marks the manager's token active at the current GAME time
// (Manager.LastActiveGameTime), which the engine's season-boundary retirement pass
// reads to exempt a live Avatar. A REJECTED call must NOT update the stamp — only the
// logged/accepted path marks activity, so the signal stays reproducible on replay
// (the input log records exactly the accepted calls). The two phases run at distinct
// game times so a stray stamp on the rejected call would be detectable.
func TestAvatarActivityStamp(t *testing.T) {
	g, host, _, _ := newGateway(t)
	mid, _ := employedManager(t, g)
	if g.managers[mid].LastActiveGameTime != 0 {
		t.Fatalf("fresh manager should be at the zero sentinel, got %d", g.managers[mid].LastActiveGameTime)
	}

	// (1) An accepted call stamps the current game time.
	if _, err := host.eng.RunUntil(sim.GameTime(sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	t1 := host.Engine().Now()
	if t1 == 0 {
		t.Fatal("clock did not advance; a zero stamp is indistinguishable from the sentinel")
	}
	if env := g.getNews(mid, "s1", getNewsIn{Scope: "world", Limit: 5}); env["ok"] != true {
		t.Fatalf("accepted call rejected unexpectedly: %v", env)
	}
	if got := g.managers[mid].LastActiveGameTime; got != t1 {
		t.Fatalf("accepted call stamp = %d, want the current game time %d", got, t1)
	}

	// (2) A rejected call (insufficient focus) at a LATER time must leave the stamp
	// untouched — a stray stamp would advance it from t1 to t2.
	if _, err := host.eng.RunUntil(sim.GameTime(4 * sim.MinutesPerDay)); err != nil {
		t.Fatal(err)
	}
	t2 := host.Engine().Now()
	if t2 == t1 {
		t.Fatal("clock did not advance between phases")
	}
	// No manager spawns in this short a run (a sacking needs a season of decline), so
	// g.managers is length-stable and this pointer stays valid through the next call.
	m := g.managers[mid]
	m.FocusBalance = 0    // zero the balance …
	m.FocusRegenMark = t2 // … and pin regen to now so syncFocus adds no headroom
	if env := g.getLeague(mid, "s1", getLeagueIn{Division: 1}); env["ok"] == true {
		t.Fatalf("expected an insufficient-focus rejection, got ok: %v", env)
	}
	if got := g.managers[mid].LastActiveGameTime; got != t1 {
		t.Fatalf("rejected call moved the activity stamp to %d — only accepted calls should stamp (want %d)", got, t1)
	}
}
