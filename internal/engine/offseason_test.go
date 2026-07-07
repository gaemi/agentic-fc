package engine

import (
	"testing"

	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// TestOffSeasonResets locks careers slice E: crossing the season boundary
// rebaselines every club's live confidence to its freshly re-derived baseline
// (season N+1 starts exactly like season 1), re-derives wage + transfer budgets
// from the post-promotion/relegation tier, and carries a WARNED sacking state
// over WITHOUT a silent flip to OK — while a negative balance yields a zero,
// never negative, transfer kitty. Deterministic by seed: mutations are injected
// before the boundary and the world runs to day 365 exactly (the rollover tick).
func TestOffSeasonResets(t *testing.T) {
	e := newEngineCfg(t, worldgen.DefaultConfig(31))
	if _, err := e.RunUntil(day(364)); err != nil {
		t.Fatal(err)
	}

	// A club the board has already warned, deep in trouble: the boundary must
	// rebaseline its confidence but keep the warning (FR-14b — a reprieve is
	// announced through results, never silently applied at the boundary).
	warned := &e.world.Clubs[0]
	warned.SackState = sackWarned
	warned.Confidence = 12
	// A club that has run its account deep into the red: the new season's
	// transfer budget must clamp to zero, not go negative. Deep enough that the
	// final week's revenue cannot flip the sign.
	broke := &e.world.Clubs[1]
	broke.BalanceMinor = -1 << 50

	if _, err := e.RunUntil(day(365)); err != nil { // the rollover tick itself
		t.Fatal(err)
	}
	if got := worldgen.DateOf(e.Now()).Season; got != 2 {
		t.Fatalf("test vacuous: season = %d at day 365, want 2 (rollover not fired)", got)
	}

	for i := range e.world.Clubs {
		c := &e.world.Clubs[i]
		// Confidence opens season 2 at the re-derived baseline for every club —
		// no season-2 league result has played at day 365 00:00, so nothing has
		// moved it yet.
		if c.Confidence != c.ConfidenceBaseline {
			t.Errorf("club %d confidence %d != baseline %d after rollover", c.ID, c.Confidence, c.ConfidenceBaseline)
		}
		// The wage-bill cache the budget rule trusts must equal the real
		// contracts (transfers + youth intake maintain it; a drift here would
		// poison every season's budgets).
		var bill int64
		for j := range e.world.Players {
			p := &e.world.Players[j]
			if p.ClubID == c.ID && p.Contract != nil {
				bill += p.Contract.WageWeeklyMinor
			}
		}
		if c.WageBillWeeklyMinor != bill {
			t.Errorf("club %d cached wage bill %d != contract sum %d", c.ID, c.WageBillWeeklyMinor, bill)
		}
		// Fresh budgets follow the shared rule: floored wage budget, ambition-
		// scaled non-negative kitty.
		if floor := bill + bill/20; c.WageBudgetWeeklyMinor < floor {
			t.Errorf("club %d wage budget %d under the bill+5%% floor %d", c.ID, c.WageBudgetWeeklyMinor, floor)
		}
		if c.TransferBudgetMinor < 0 {
			t.Errorf("club %d transfer budget %d is negative", c.ID, c.TransferBudgetMinor)
		}
		if c.SackState == sackUltimatum {
			t.Errorf("club %d still in ULTIMATUM across the boundary — end-of-season settlement missed it", c.ID)
		}
	}

	if warned.SackState != sackWarned {
		t.Fatalf("warned club's SackState = %q after rollover, want WARNED carried over (no silent reprieve)", warned.SackState)
	}
	if broke.TransferBudgetMinor != 0 {
		t.Fatalf("broke club's transfer budget = %d, want 0 (clamped, not negative)", broke.TransferBudgetMinor)
	}
}
