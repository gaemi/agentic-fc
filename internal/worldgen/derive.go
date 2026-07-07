package worldgen

import "github.com/gaemi/agentic-fc/internal/sim"

// PoolBand is an inclusive Ability Pool sampling range for one division.
type PoolBand struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Derived is the deterministic structure computed from the config with no
// dice involved (docs/09 §3, stage 0).
type Derived struct {
	PromotionSlots    int            `json:"promotion_slots"` // between adjacent divisions
	Rounds            int            `json:"rounds"`          // league rounds per season
	LeagueRoundTimes  []sim.GameTime `json:"league_round_times"`
	CupBracketSize    int            `json:"cup_bracket_size"`
	CupByes           int            `json:"cup_byes"`
	CupRounds         int            `json:"cup_rounds"`
	CupRoundTimes     []sim.GameTime `json:"cup_round_times"`
	DivisionPoolBands []PoolBand     `json:"division_pool_bands"` // index 0 = tier 1
}

// qualityBands maps World quality to the division-1 pool band and the
// per-division decrement (docs/09 §4.2, tunable — registered in docs/98).
var qualityBands = map[Quality]struct {
	Min, Max, Decrement int
}{
	QualityAmateur:      {40, 90, 15},
	QualitySemiPro:      {60, 110, 18},
	QualityProfessional: {90, 150, 22},
	QualityElite:        {120, 180, 25},
}

// Band floors: deep pyramids at low quality would otherwise go negative.
const (
	poolBandFloorMin    = 5
	poolBandMinSpread   = 10
	promotionSlotShare  = 0.15 // slots = max(2, round(clubs × share)) (docs/09 §3)
	promotionSlotFloor  = 2
	unemployedPoolShare = 0.10 // managers beyond one-per-club (docs/09 §4.3)
	unemployedPoolFloor = 2
)

func deriveStructure(cfg WorldConfig) Derived {
	d := Derived{
		PromotionSlots: max(promotionSlotFloor, int(float64(cfg.ClubsPerDivision)*promotionSlotShare+0.5)),
		Rounds:         2 * (cfg.ClubsPerDivision - 1),
	}
	d.LeagueRoundTimes = leagueRoundTimes(d.Rounds)

	leagueDays := map[int]bool{}
	for _, t := range d.LeagueRoundTimes {
		leagueDays[int(int64(t)/sim.MinutesPerDay)] = true
	}
	clubs := cfg.TotalClubs()
	d.CupBracketSize = 1
	for d.CupBracketSize < clubs {
		d.CupBracketSize *= 2
	}
	d.CupByes = d.CupBracketSize - clubs
	for n := d.CupBracketSize; n > 1; n /= 2 {
		d.CupRounds++
	}
	d.CupRoundTimes = cupRoundTimes(d.CupRounds, leagueDays)

	band := qualityBands[cfg.Quality]
	d.DivisionPoolBands = make([]PoolBand, cfg.Divisions)
	for tier := 1; tier <= cfg.Divisions; tier++ {
		lo := band.Min - band.Decrement*(tier-1)
		hi := band.Max - band.Decrement*(tier-1)
		if lo < poolBandFloorMin {
			lo = poolBandFloorMin
		}
		if hi < lo+poolBandMinSpread {
			hi = lo + poolBandMinSpread
		}
		d.DivisionPoolBands[tier-1] = PoolBand{Min: lo, Max: hi}
	}
	return d
}

// unemployedPoolSize is the generated jobless-manager pool (docs/09 §4.3).
func unemployedPoolSize(totalClubs int) int {
	return max(unemployedPoolFloor, int(float64(totalClubs)*unemployedPoolShare+0.5))
}

// UnemployedPoolTarget is the size the jobless-manager pool is topped back up to
// each season by newgen (manager careers), so the job market never dries up as clubs
// hire and managers retire. It reuses the generation-time pool formula, so the
// running world's pool floor equals the world it was born with.
func UnemployedPoolTarget(w *World) int {
	return unemployedPoolSize(w.Config.TotalClubs())
}
