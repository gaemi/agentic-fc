package worldgen

import "testing"

// TestDeriveClubBudgets locks the shared season-budget rule (careers E): the
// wage budget is the wage share of current-tier revenue floored just over the
// cached bill, and the transfer budget is ambition-scaled from the current
// balance, clamped at zero. Generation and the season rollover both call this
// helper, so these are the semantics every season starts on.
func TestDeriveClubBudgets(t *testing.T) {
	cfg := DefaultConfig(1)

	club := &Club{
		DivisionTier: 1,
		Tendencies:   Tendencies{Wealth: 10, BoardAmbition: 10},
	}

	// Ambition-scaled kitty from a positive balance: balance × (0.2 + 10/20×0.4).
	club.BalanceMinor = 1_000_000_000
	deriveClubBudgets(cfg, club)
	if want := int64(float64(club.BalanceMinor) * 0.4); club.TransferBudgetMinor != want {
		t.Fatalf("transfer budget = %d, want %d (balance × ambition factor)", club.TransferBudgetMinor, want)
	}
	if club.WageBudgetWeeklyMinor <= 0 {
		t.Fatalf("wage budget = %d, want > 0", club.WageBudgetWeeklyMinor)
	}

	// A negative balance yields NO kitty, never a negative one — the clamp is the
	// rollover-side policy (a club can run its account negative via finance
	// deficits; generation always starts positive, so the clamp is inert there).
	club.BalanceMinor = -1_000_000_000
	deriveClubBudgets(cfg, club)
	if club.TransferBudgetMinor != 0 {
		t.Fatalf("transfer budget = %d for a negative balance, want 0", club.TransferBudgetMinor)
	}

	// A bill above the revenue share floors the wage budget at bill + 5%, so a
	// club never opens a season already over cap (the same coherence rule
	// generation applies).
	club.BalanceMinor = 0
	club.WageBillWeeklyMinor = 1 << 40 // dwarfs any tier revenue
	deriveClubBudgets(cfg, club)
	if want := club.WageBillWeeklyMinor + club.WageBillWeeklyMinor/20; club.WageBudgetWeeklyMinor != want {
		t.Fatalf("wage budget = %d, want %d (bill + 5%% floor)", club.WageBudgetWeeklyMinor, want)
	}
}
