package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

func TestFormationBands(t *testing.T) {
	cases := []struct {
		formation  string
		df, mf, fw int
	}{
		{"4-3-3", 4, 3, 3},
		{"4-4-2", 4, 4, 2},
		{"4-2-3-1", 4, 5, 1},
		{"4-4-1-1", 4, 5, 1},
		{"3-5-2", 3, 5, 2},
		{"5-4-1", 5, 4, 1},
		{"", 4, 4, 2},         // unset plan
		{"whatever", 4, 4, 2}, // not a shape
		{"2-2-2", 4, 4, 2},    // wrong outfield total
		{"4-3-2-1", 4, 5, 1},  // four bands sum 10
		{"4--3-3", 4, 4, 2},   // malformed
		{"4-3-0-3", 4, 4, 2},  // zero band
	}
	for _, tc := range cases {
		df, mf, fw := formationBands(tc.formation)
		if df != tc.df || mf != tc.mf || fw != tc.fw {
			t.Fatalf("%q -> %d-%d-%d, want %d-%d-%d", tc.formation, df, mf, fw, tc.df, tc.mf, tc.fw)
		}
	}
}

// The XI takes the tactical plan's shape when the squad allows it, falls
// back gracefully when a band is depleted, and the bench always carries a
// spare keeper while one exists.
func TestSelectSquadHonoursFormationShape(t *testing.T) {
	e, _ := newEngine(t, 11)
	club := e.world.Clubs[0].ID
	at := firstKickoff(e)

	countGroups := func(ids []int64) map[attr.PositionGroup]int {
		counts := map[attr.PositionGroup]int{}
		for _, id := range ids {
			counts[e.players[id].Group]++
		}
		return counts
	}

	for _, tc := range []struct {
		formation  string
		df, mf, fw int
	}{
		{"4-3-3", 4, 3, 3},
		{"5-4-1", 5, 4, 1},
		{"3-5-2", 3, 5, 2},
	} {
		xi, bench := e.selectSquad(club, at, mindset.TacticalPlan{Formation: tc.formation})
		if len(xi) != 11 {
			t.Fatalf("%s: XI = %d players", tc.formation, len(xi))
		}
		got := countGroups(xi)
		if got[attr.GK] != 1 || got[attr.DF] != tc.df || got[attr.MF] != tc.mf || got[attr.FW] != tc.fw {
			t.Fatalf("%s: XI shape %v, want 1/%d/%d/%d", tc.formation, got, tc.df, tc.mf, tc.fw)
		}
		if len(bench) == 0 || e.players[bench[0]].Group != attr.GK {
			t.Fatalf("%s: bench should open with the spare keeper, got %v", tc.formation, bench)
		}
	}

	// Deplete the back line to two fit defenders: the XI must stay at 11,
	// keeping both defenders and borrowing the rest from other groups.
	kept := 0
	for i := range e.world.Players {
		p := &e.world.Players[i]
		if p.ClubID != club || p.Youth || p.Group != attr.DF {
			continue
		}
		if kept < 2 {
			kept++
			continue
		}
		p.InjuredUntil = at + 10
	}
	xi, _ := e.selectSquad(club, at, mindset.TacticalPlan{Formation: "4-4-2"})
	if len(xi) != 11 {
		t.Fatalf("depleted back line: XI = %d players, want 11", len(xi))
	}
	got := countGroups(xi)
	if got[attr.DF] != 2 || got[attr.GK] != 1 {
		t.Fatalf("depleted back line: shape %v, want both fit defenders kept", got)
	}
}

// Injury replacements are role-aware: an injured keeper takes the bench
// keeper, an injured outfielder never burns the reserved backup keeper while
// an outfield body remains, and the keeper comes on only as the last body.
func TestInjuryReplacementRespectsRoles(t *testing.T) {
	e, _ := newEngine(t, 13)
	club := e.world.Clubs[0].ID
	opp := e.world.Clubs[1].ID
	at := firstKickoff(e)
	xi, bench := e.selectSquad(club, at, mindset.TacticalPlan{Formation: "4-4-2"})
	if len(bench) == 0 || e.players[bench[0]].Group != attr.GK {
		t.Fatalf("bench sanity: want reserved keeper first, got %v", bench)
	}
	lm := &worldgen.LiveMatch{
		FixtureID: 910001, Competition: worldgen.CompetitionLeague,
		HomeID: club, AwayID: opp, Kickoff: at, Clock: 30,
		HomeXI: xi, HomeBench: bench,
	}

	// Injured outfielder: the replacement must be an outfielder.
	if rep := e.bestFitOnBench(lm, club, at, false); rep == 0 || e.players[rep].Group == attr.GK {
		t.Fatalf("outfield injury should bring on an outfielder, got %v", rep)
	}
	// Injured keeper: the replacement must be the bench keeper.
	if rep := e.bestFitOnBench(lm, club, at, true); rep == 0 || e.players[rep].Group != attr.GK {
		t.Fatalf("keeper injury should bring on the bench keeper, got %v", rep)
	}

	// With every bench outfielder already used, the keeper is the emergency
	// body rather than playing short.
	for _, id := range bench {
		if e.players[id].Group != attr.GK {
			lm.Subs = append(lm.Subs, worldgen.SubEvent{Minute: 40, ClubID: club, Off: xi[5], On: id})
			xi = append(xi, id) // keep Off ids unique enough for the used map
		}
	}
	if rep := e.bestFitOnBench(lm, club, at, false); rep == 0 || e.players[rep].Group != attr.GK {
		t.Fatalf("exhausted outfield bench should fall back to the keeper, got %v", rep)
	}
}
