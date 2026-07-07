package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestInjuriesAreReal locks the injury-system behavior: knocks now sideline
// players. Over an opening stretch of the season some injury must occur
// (churn), every injured player carries a future InjuredUntil plus a history
// record whose band is one of the three legal tokens, injury news is filed,
// and at least one match recorded a substitution whose sub-on earned a rating
// and an appearance.
func TestInjuriesAreReal(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(day(120)); err != nil {
		t.Fatal(err)
	}

	injured := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if len(p.Injuries) == 0 {
			continue
		}
		injured++
		for _, rec := range p.Injuries {
			switch rec.Band {
			case "DAYS", "WEEKS", "MONTH":
			default:
				t.Fatalf("player %d injury band %q is not a legal token", p.ID, rec.Band)
			}
			if rec.SeasonYear != 1 {
				t.Fatalf("player %d injury season %d, want 1", p.ID, rec.SeasonYear)
			}
		}
	}
	if injured == 0 {
		t.Fatal("test vacuous: no injuries in 120 days of football")
	}
	if !hasNews(e, "news.injury.days") && !hasNews(e, "news.injury.weeks") && !hasNews(e, "news.injury.month") {
		t.Fatal("injuries occurred but no injury news was filed")
	}

	subbed := false
	for _, r := range e.world.Results {
		for _, s := range r.Subs {
			if s.On == 0 {
				continue
			}
			subbed = true
			if _, ok := r.RatingsX10[s.On]; !ok {
				t.Fatalf("sub-on %d has no rating in fixture %d", s.On, r.FixtureID)
			}
			if p := e.players[s.On]; p != nil && p.SeasonApps == 0 {
				t.Fatalf("sub-on %d earned no appearance", s.On)
			}
		}
	}
	if !subbed {
		t.Fatal("injuries occurred but no substitution was ever made")
	}
}

// TestSelectSquadSkipsInjured locks the whole recovery model — a timestamp
// comparison: a player injured past kickoff is neither in the XI nor on the
// bench, and is selectable again once the clock passes InjuredUntil.
func TestSelectSquadSkipsInjured(t *testing.T) {
	e, _ := newEngine(t, 42)
	clubID := e.world.Clubs[0].ID
	xi, _ := e.selectSquad(clubID, day(10), mindset.TacticalPlan{})
	if len(xi) == 0 {
		t.Fatal("no XI in a fresh world")
	}
	star := e.players[xi[0]]
	star.InjuredUntil = day(20)

	xi2, bench2 := e.selectSquad(clubID, day(10), mindset.TacticalPlan{})
	for _, id := range append(append([]int64{}, xi2...), bench2...) {
		if id == star.ID {
			t.Fatal("an injured player was selected")
		}
	}
	xi3, _ := e.selectSquad(clubID, day(21), mindset.TacticalPlan{})
	found := false
	for _, id := range xi3 {
		if id == star.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("a healed player was not selected again — recovery must be the timestamp alone")
	}
}

// TestWithdrawShortWhenNoBench locks the no-replacement path: with an empty
// bench the withdrawal records On==0 and the side simply plays short — the
// on-pitch set shrinks, participants don't grow, and no phantom sub is used.
func TestWithdrawShortWhenNoBench(t *testing.T) {
	e, _ := newEngine(t, 42)
	clubID := e.world.Clubs[0].ID
	xi, _ := e.selectSquad(clubID, day(1), mindset.TacticalPlan{})
	lm := &worldgen.LiveMatch{
		FixtureID: 999, HomeID: clubID, AwayID: e.world.Clubs[1].ID,
		HomeXI: xi, Clock: 30, // no bench set
	}
	e.withdrawInjured(lm, clubID, xi[0], sim.GameTime(day(1)))
	if len(lm.Subs) != 1 || lm.Subs[0].On != 0 || lm.Subs[0].Off != xi[0] {
		t.Fatalf("short-side withdrawal misrecorded: %+v", lm.Subs)
	}
	if got := len(lm.OnPitch(clubID)); got != len(xi)-1 {
		t.Fatalf("on-pitch = %d after an uncovered withdrawal, want %d", got, len(xi)-1)
	}
	if got := len(lm.Participants(clubID)); got != len(xi) {
		t.Fatalf("participants = %d, want %d (nobody new took the pitch)", got, len(xi))
	}
	if lm.SubsUsed(clubID) != 0 {
		t.Fatal("an uncovered withdrawal must not consume a substitution")
	}
}
