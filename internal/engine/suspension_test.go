package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// A red card at full time starts a ban, the banned player disappears from
// squad selection, sitting out one completed club fixture serves it, another
// club's fixture serves nothing, and the news desk reports the ban.
func TestRedCardSuspensionLifecycle(t *testing.T) {
	e, _ := newEngine(t, 7)
	home := e.world.Clubs[0].ID
	away := e.world.Clubs[1].ID
	at := firstKickoff(e)

	homeXI, homeBench := e.selectSquad(home, at, mindset.TacticalPlan{})
	awayXI, _ := e.selectSquad(away, at, mindset.TacticalPlan{})
	if len(homeXI) != 11 || len(awayXI) != 11 {
		t.Fatalf("selection sanity: XI sizes %d/%d", len(homeXI), len(awayXI))
	}
	sentOff := homeXI[3]

	issuing := &worldgen.LiveMatch{
		FixtureID: 900001, Competition: worldgen.CompetitionLeague,
		HomeID: home, AwayID: away, Kickoff: at, Clock: 90,
		HomeXI: homeXI, HomeBench: homeBench, AwayXI: awayXI,
		Cards: []worldgen.MatchEvent{{Minute: 60, PlayerID: sentOff, ClubID: home, Detail: "RED"}},
	}
	e.applySuspensions(issuing, at)
	p := e.players[sentOff]
	if p.SuspendedMatches != suspensionMatchesRed {
		t.Fatalf("red card should start a %d-match ban, got %d", suspensionMatchesRed, p.SuspendedMatches)
	}
	if !hasNews(e, "news.player.suspended") {
		t.Fatal("the ban was not announced as news")
	}

	xi2, bench2 := e.selectSquad(home, at, mindset.TacticalPlan{})
	for _, id := range append(append([]int64{}, xi2...), bench2...) {
		if id == sentOff {
			t.Fatal("a suspended player was selected for the matchday squad")
		}
	}

	// A yellow booking alone must not ban anyone.
	booked := awayXI[2]
	yellowOnly := &worldgen.LiveMatch{
		FixtureID: 900002, Competition: worldgen.CompetitionLeague,
		HomeID: home, AwayID: away, Kickoff: at, Clock: 90,
		HomeXI: xi2, AwayXI: awayXI,
		Cards: []worldgen.MatchEvent{{Minute: 30, PlayerID: booked, ClubID: away, Detail: "YELLOW"}},
	}
	e.applySuspensions(yellowOnly, at)
	if e.players[booked].SuspendedMatches != 0 {
		t.Fatal("a yellow card must not start a ban")
	}
	// That completed club fixture (which the banned player sat out) also
	// served his ban.
	if p.SuspendedMatches != 0 {
		t.Fatalf("sitting out a completed club fixture should serve the ban, got %d left", p.SuspendedMatches)
	}

	// Another club's fixture serves nothing.
	p.SuspendedMatches = 1
	otherHome := e.world.Clubs[2].ID
	otherAway := e.world.Clubs[3].ID
	oXI, _ := e.selectSquad(otherHome, at, mindset.TacticalPlan{})
	oAwayXI, _ := e.selectSquad(otherAway, at, mindset.TacticalPlan{})
	elsewhere := &worldgen.LiveMatch{
		FixtureID: 900003, Competition: worldgen.CompetitionLeague,
		HomeID: otherHome, AwayID: otherAway, Kickoff: at, Clock: 90,
		HomeXI: oXI, AwayXI: oAwayXI,
	}
	e.applySuspensions(elsewhere, at)
	if p.SuspendedMatches != 1 {
		t.Fatalf("an unrelated fixture must not serve the ban, got %d left", p.SuspendedMatches)
	}

	// Discipline follows the player (real-football rule): after a transfer
	// the ban is served by the NEW club's completed fixtures, and the old
	// club's fixtures no longer touch it.
	p.ClubID = otherHome
	former := &worldgen.LiveMatch{
		FixtureID: 900004, Competition: worldgen.CompetitionLeague,
		HomeID: home, AwayID: away, Kickoff: at, Clock: 90,
		HomeXI: xi2, AwayXI: awayXI,
	}
	e.applySuspensions(former, at)
	if p.SuspendedMatches != 1 {
		t.Fatalf("the former club's fixture must not serve a transferred ban, got %d left", p.SuspendedMatches)
	}
	e.applySuspensions(elsewhere, at)
	if p.SuspendedMatches != 0 {
		t.Fatalf("the new club's fixture should serve the transferred ban, got %d left", p.SuspendedMatches)
	}
	p.ClubID = home
}
