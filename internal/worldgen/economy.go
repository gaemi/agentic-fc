package worldgen

import (
	"math"
	"math/rand/v2"
)

// Stage 7 — economy init: balances, wage bills, and first budgets, all in
// Crowns minor units (docs/09 §4.5). Division d's band = top band ×
// decay^(d−1), scaled by Economy scale and World quality (docs/09 §3).
// Initial values, registered in docs/98.

const (
	// Top-division annual revenue base: cr 30M (Standard, Professional).
	revenueTopDivBaseMinor = int64(3_000_000_000)
	divisionEconomyDecay   = 0.45
	wageRevenueShare       = 0.55 // of revenue available for wages
	weeksPerSeason         = 52
)

var qualityRevenueFactor = map[Quality]float64{
	QualityAmateur:      0.05,
	QualitySemiPro:      0.25,
	QualityProfessional: 1.0,
	QualityElite:        2.5,
}

var economyScaleFactor = map[EconomyScale]float64{
	EconomyAusterity: 0.6,
	EconomyStandard:  1.0,
	EconomyFlush:     1.8,
}

// divisionRevenueBaseMinor is the tier's annual revenue midpoint before the
// club's own Wealth spreads it.
func divisionRevenueBaseMinor(cfg WorldConfig, tier int) float64 {
	base := float64(revenueTopDivBaseMinor)
	base *= qualityRevenueFactor[cfg.Quality]
	base *= economyScaleFactor[cfg.Economy]
	return base * math.Pow(divisionEconomyDecay, float64(tier-1))
}

// divisionAvgWageMinor anchors stage-4 wage rolls: the wage share of tier
// revenue spread over a squad.
func divisionAvgWageMinor(cfg WorldConfig, tier int) int64 {
	perWeek := divisionRevenueBaseMinor(cfg, tier) * wageRevenueShare / weeksPerSeason
	return int64(perWeek / float64(cfg.SquadSizeTarget))
}

// ClubWeeklyRevenueMinor is the club's expected weekly revenue — the same
// model stage 7 budgets from, exposed for the engine's finance ticks.
func ClubWeeklyRevenueMinor(cfg WorldConfig, c *Club) int64 {
	wealthFactor := 0.6 + float64(c.Tendencies.Wealth-1)/19.0*0.8
	return int64(divisionRevenueBaseMinor(cfg, c.DivisionTier) * wealthFactor / weeksPerSeason)
}

// WageDemandMinor is a free agent's deterministic weekly wage ask: the
// stage-4 wage curve (avgWage × (pool/mid)^exp × reputation premium) WITHOUT the
// negotiation noise, so a signing reproduces exactly. It uses the signing tier's
// wage scale and pool band, mirroring rollWage.
func WageDemandMinor(cfg WorldConfig, d Derived, tier, pool, reputation int) int64 {
	if tier < 1 || tier > len(d.DivisionPoolBands) {
		return wageFloorMinor
	}
	band := d.DivisionPoolBands[tier-1]
	mid := float64(band.Min+band.Max) / 2
	if mid <= 0 {
		mid = 1
	}
	f := math.Pow(float64(pool)/mid, wagePoolExponent)
	f *= 0.9 + float64(reputation)/10000.0*0.4
	w := int64(float64(divisionAvgWageMinor(cfg, tier)) * f)
	w = w / wageRounding * wageRounding
	if w < wageFloorMinor {
		w = wageFloorMinor
	}
	return w
}

// transferValuationPerPoolSq prices an inter-club fee at AbilityPool² × this,
// in Crowns minor units. The quadratic makes an elite pool cost sharply more
// than its wage alone (tunable — registered in docs/98). It is an ENGINE-INTERNAL
// figure: the exact fee is a pure function of the raw pool, so publishing it
// would invert to the hidden Ability Pool — it never crosses the MCP wire (FR-22,
// hidden Ability Pool. See TransferValuationMinor.
const transferValuationPerPoolSq = 5000

// TransferValuationMinor is a contracted player's fee for an inter-club move
// a pure quadratic in Ability Pool, integer minor-units so
// nothing that reaches the world hash is a float (NFR-2). Raw-pool derived —
// engine-internal only, never surfaced to agents (FR-22).
func TransferValuationMinor(pool int) int64 {
	if pool < 0 {
		pool = 0
	}
	return int64(pool) * int64(pool) * transferValuationPerPoolSq
}

func genEconomy(w *World, r *rand.Rand) {
	for i := range w.Clubs {
		club := &w.Clubs[i]
		revenue := float64(ClubWeeklyRevenueMinor(w.Config, club) * weeksPerSeason)

		club.BalanceMinor = int64(revenue *
			(0.05 + float64(club.Tendencies.Wealth)/20.0*0.35) *
			(0.8 + r.Float64()*0.5))

		var bill int64
		for j := range w.Players {
			p := &w.Players[j]
			if p.ClubID == club.ID && p.Contract != nil {
				bill += p.Contract.WageWeeklyMinor
			}
		}
		club.WageBillWeeklyMinor = bill

		deriveClubBudgets(w.Config, club)
	}
}

// deriveClubBudgets sets the club's wage + transfer budgets for a season from
// its current tier revenue, cached wage bill, and balance — shared by generation
// (stage 7) and the season rollover (careers E) so the budget rule can't drift.
// The wage budget is floored just over the current bill (never start a season
// over cap); the transfer budget is ambition-scaled from the CURRENT balance,
// clamped at zero — a club that has run its account negative gets no kitty. At
// generation the balance is always positive, so the clamp is generation-inert
// and stage-7 output stays byte-identical. Dice-free: a pure function of World
// state, so the rollover reproduces it exactly on replay (NFR-2).
func deriveClubBudgets(cfg WorldConfig, club *Club) {
	revenue := float64(ClubWeeklyRevenueMinor(cfg, club) * weeksPerSeason)

	bill := club.WageBillWeeklyMinor
	budget := int64(revenue * wageRevenueShare / weeksPerSeason)
	if budget < bill+bill/20 { // coherence: never start a season over cap
		budget = bill + bill/20
	}
	club.WageBudgetWeeklyMinor = budget

	tb := int64(float64(club.BalanceMinor) *
		(0.2 + float64(club.Tendencies.BoardAmbition)/20.0*0.4))
	if tb < 0 {
		tb = 0
	}
	club.TransferBudgetMinor = tb
}
