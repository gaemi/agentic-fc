package engine

import (
	"math/rand/v2"
	"testing"

	"github.com/gaemi/agentic-fc/internal/attr"
	"github.com/gaemi/agentic-fc/internal/mindset"
	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestCardVerdict locks the booking outcomes: a straight red is a RED, a
// second yellow upgrades to a RED under its own commentary key, and a first
// yellow stays a YELLOW.
func TestCardVerdict(t *testing.T) {
	lm := &worldgen.LiveMatch{}
	if d, k := cardVerdict(lm, 7, true); d != "RED" || k != "comment.card.red" {
		t.Fatalf("straight red = %s/%s", d, k)
	}
	if d, k := cardVerdict(lm, 7, false); d != "YELLOW" || k != "comment.card.yellow" {
		t.Fatalf("first booking = %s/%s", d, k)
	}
	lm.Cards = append(lm.Cards, worldgen.MatchEvent{PlayerID: 7, Detail: "YELLOW"})
	if d, k := cardVerdict(lm, 7, false); d != "RED" || k != "comment.card.secondyellow" {
		t.Fatalf("second yellow = %s/%s, want RED/comment.card.secondyellow", d, k)
	}
	// Another player's yellow must not upgrade this one's first booking.
	if d, _ := cardVerdict(lm, 8, false); d != "YELLOW" {
		t.Fatalf("player 8 first booking = %s, want YELLOW", d)
	}
}

// TestRatingsRedSwallowsYellows locks the aggregation fix: a second
// yellow leaves YELLOW + RED entries in the ledger, but the rating penalty is
// ratingRedX10 once — not the sum.
func TestRatingsRedSwallowsYellows(t *testing.T) {
	e, _ := newEngine(t, 42)
	clubA, clubB := e.world.Clubs[0].ID, e.world.Clubs[1].ID
	xiA, _ := e.selectSquad(clubA, day(1), mindset.TacticalPlan{})
	xiB, _ := e.selectSquad(clubB, day(1), mindset.TacticalPlan{})
	lm := &worldgen.LiveMatch{
		FixtureID: 991, HomeID: clubA, AwayID: clubB,
		HomeXI: xiA, AwayXI: xiB, HomeGoals: 1, AwayGoals: 1,
	}
	victim := xiA[len(xiA)-1] // an outfielder — keeps clean-sheet logic out of it
	lm.Cards = append(lm.Cards,
		worldgen.MatchEvent{Minute: 30, PlayerID: victim, ClubID: clubA, Detail: "YELLOW"},
		worldgen.MatchEvent{Minute: 60, PlayerID: victim, ClubID: clubA, Detail: "RED"},
	)
	got := e.ratings(lm)[victim]
	want := clampInt(ratingBaseX10+ratingRedX10, ratingMinX10, ratingMaxX10)
	if got != want {
		t.Fatalf("second-yellow rating = %d, want %d (red once, yellows swallowed)", got, want)
	}
}

// discretionaryFixture builds a controlled live match for one club: real
// players from a generated world, clock parked in the discretionary window.
func discretionaryFixture(t *testing.T, e *Engine, clock int) (*worldgen.LiveMatch, int64) {
	t.Helper()
	clubID := e.world.Clubs[0].ID
	xi, bench := e.selectSquad(clubID, day(1), mindset.TacticalPlan{})
	if len(xi) < 11 || len(bench) == 0 {
		t.Fatal("fixture needs a full XI and a bench")
	}
	oppXI, oppBench := e.selectSquad(e.world.Clubs[1].ID, day(1), mindset.TacticalPlan{})
	return &worldgen.LiveMatch{
		FixtureID: 992, HomeID: clubID, AwayID: e.world.Clubs[1].ID,
		HomeXI: xi, AwayXI: oppXI, HomeBench: bench, AwayBench: oppBench,
		Clock: clock,
	}, clubID
}

// TestFatigueSubFires locks the fatigue path: a starter whose derived in-match
// condition sits under the threshold is withdrawn dice-free for the best
// outfield bench body, stamped FATIGUE — and the keeper is never the one
// pulled, however tired.
func TestFatigueSubFires(t *testing.T) {
	e, _ := newEngine(t, 42)
	lm, clubID := discretionaryFixture(t, e, tacticalSubFromMinute)

	// Everyone fresh: no fatigue candidate, and any change would be tactical.
	var tired *worldgen.Player
	for _, pid := range lm.HomeXI {
		p := e.players[pid]
		if p.Group != attr.GK {
			tired = p
			break
		}
	}
	gk := e.players[lm.HomeXI[0]]
	if gk.Group != attr.GK {
		t.Fatal("test premise: XI[0] is the keeper")
	}
	tired.Condition = fatigueSubThreshold - 1 + conditionDrainPlay*lm.Clock/matchFullTimeMinutes
	gk.Condition = 0 // the tiredest body on the pitch — and untouchable

	r := rand.New(rand.NewPCG(1, 1)) // fatigue is dice-free; the source is inert
	e.considerDiscretionarySub(lm, sim.GameTime(day(1)), r, clubID)
	if len(lm.Subs) != 1 {
		t.Fatalf("subs = %+v, want exactly one fatigue change", lm.Subs)
	}
	s := lm.Subs[0]
	if s.Off != tired.ID || s.On == 0 || s.Reason != "FATIGUE" {
		t.Fatalf("fatigue sub misrecorded: %+v (want off=%d, a replacement, FATIGUE)", s, tired.ID)
	}
	if on := e.players[s.On]; on.Group == attr.GK {
		t.Fatal("a discretionary change brought a keeper on for an outfield slot")
	}
}

// TestDiscretionaryReserveRule locks the last-card reserve: with one sub left
// before subReserveUntil nothing voluntary happens; past it the change fires.
// And a side that never finds an upgrade appends nothing — a discretionary
// path never records an uncovered withdrawal.
func TestDiscretionaryReserveRule(t *testing.T) {
	e, _ := newEngine(t, 42)
	lm, clubID := discretionaryFixture(t, e, subReserveUntil-1)
	var tired *worldgen.Player
	for _, pid := range lm.HomeXI {
		if p := e.players[pid]; p.Group != attr.GK {
			tired = p
			break
		}
	}
	tired.Condition = 0 // deep under the threshold at any clock

	// Burn subs down to the last one.
	lm.Subs = append(lm.Subs,
		worldgen.SubEvent{Minute: 50, ClubID: clubID, Off: lm.HomeXI[2], On: lm.HomeBench[0]},
		worldgen.SubEvent{Minute: 55, ClubID: clubID, Off: lm.HomeXI[3], On: lm.HomeBench[1]},
	)
	r := rand.New(rand.NewPCG(1, 1))
	e.considerDiscretionarySub(lm, sim.GameTime(day(1)), r, clubID)
	if got := len(lm.Subs); got != 2 {
		t.Fatalf("reserve violated: %d subs before minute %d", got, subReserveUntil)
	}

	lm.Clock = subReserveUntil
	e.considerDiscretionarySub(lm, sim.GameTime(day(1)), r, clubID)
	if got := len(lm.Subs); got != 3 {
		t.Fatalf("past the reserve minute the fatigue change must fire, subs = %d", got)
	}
	if last := lm.Subs[2]; last.Reason != "FATIGUE" || last.On == 0 {
		t.Fatalf("reserve-released sub misrecorded: %+v", last)
	}

	// All spent: nothing more, ever.
	lm.Clock = 85
	e.considerDiscretionarySub(lm, sim.GameTime(day(1)), r, clubID)
	if got := len(lm.Subs); got != 3 {
		t.Fatalf("a fourth substitution slipped through: %d", got)
	}
}

// TestDiscretionaryNeverShort locks the separation of paths: with an
// empty bench a voluntary change simply does not happen — no SubEvent, no
// shrinking side. Only the injury path may record On == 0.
func TestDiscretionaryNeverShort(t *testing.T) {
	e, _ := newEngine(t, 42)
	lm, clubID := discretionaryFixture(t, e, tacticalSubFromMinute)
	lm.HomeBench = nil
	for _, pid := range lm.HomeXI {
		if p := e.players[pid]; p.Group != attr.GK {
			p.Condition = 0
		}
	}
	r := rand.New(rand.NewPCG(1, 1))
	e.considerDiscretionarySub(lm, sim.GameTime(day(1)), r, clubID)
	if len(lm.Subs) != 0 {
		t.Fatalf("a discretionary change fired with no bench: %+v", lm.Subs)
	}
	if got := len(lm.OnPitch(clubID)); got != len(lm.HomeXI) {
		t.Fatalf("on-pitch shrank to %d without any event", got)
	}
}

// TestMatchDisciplineIntegration runs real football and locks the emergent
// facts: reds occur and eject (the sent-off player takes no further part in
// that match), discretionary subs occur, and every one of them brought a
// replacement on.
func TestMatchDisciplineIntegration(t *testing.T) {
	e, _ := newEngine(t, 42)
	if _, err := e.RunUntil(day(200)); err != nil {
		t.Fatal(err)
	}
	reds, voluntary := 0, 0
	for _, res := range e.world.Results {
		redAt := map[int64]int{}
		for _, c := range res.Cards {
			if c.Detail == "RED" {
				reds++
				redAt[c.PlayerID] = c.Minute
			}
		}
		for _, s := range res.Scorers {
			if m, off := redAt[s.PlayerID]; off && s.Minute > m {
				t.Fatalf("fixture %d: player %d scored at %d' after a red at %d'",
					res.FixtureID, s.PlayerID, s.Minute, m)
			}
		}
		for _, s := range res.Subs {
			if m, off := redAt[s.Off]; off && s.Minute > m {
				t.Fatalf("fixture %d: sent-off player %d was substituted at %d' after a red at %d'",
					res.FixtureID, s.Off, s.Minute, m)
			}
			switch s.Reason {
			case "FATIGUE", "TACTICAL":
				voluntary++
				if s.On == 0 {
					t.Fatalf("fixture %d: discretionary sub went short: %+v", res.FixtureID, s)
				}
				if s.Minute < tacticalSubFromMinute {
					t.Fatalf("fixture %d: discretionary sub at %d', before the window", res.FixtureID, s.Minute)
				}
			case "INJURY", "":
				// Injury withdrawals may go short; pre-reason snapshots are blank.
			default:
				t.Fatalf("fixture %d: unknown sub reason %q", res.FixtureID, s.Reason)
			}
		}
	}
	if reds == 0 {
		t.Fatal("test vacuous: no red card in 200 days of football")
	}
	if voluntary == 0 {
		t.Fatal("test vacuous: no discretionary substitution in 200 days")
	}
}
