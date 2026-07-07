package engine

import (
	"fmt"

	"github.com/gaemi/agentic-fc/internal/sim"
	"github.com/gaemi/agentic-fc/internal/store"
	"github.com/gaemi/agentic-fc/internal/worldgen"
)

// CalibrationReport is a deterministic aggregate over generated worlds. It is
// a development aid for tuning the match model; all averages are ×100 integers
// to keep reports stable and easy to diff.
type CalibrationReport struct {
	Seeds              []uint64       `json:"seeds"`
	HorizonDays        int            `json:"horizon_days"`
	Matches            int            `json:"matches"`
	Goals              int            `json:"goals"`
	Shots              int            `json:"shots"`
	HomeWins           int            `json:"home_wins"`
	Draws              int            `json:"draws"`
	AwayWins           int            `json:"away_wins"`
	Upsets             int            `json:"upsets"`
	ChanceTypes        map[string]int `json:"chance_types,omitempty"`
	ShotQuality        map[string]int `json:"shot_quality,omitempty"`
	AerialDuels        int            `json:"aerial_duels,omitempty"`
	AerialWins         int            `json:"aerial_wins,omitempty"`
	PressTurnovers     int            `json:"press_turnovers,omitempty"`
	SetPieceThreat     int            `json:"set_piece_threat,omitempty"`
	GoalsPerMatchX100  int            `json:"goals_per_match_x100"`
	ShotsPerMatchX100  int            `json:"shots_per_match_x100"`
	ConversionRateX100 int            `json:"conversion_rate_x100"`
}

// RunCalibration generates compact worlds for each seed, drains the queue for
// horizonDays, and aggregates finished match outcomes. It never touches
// operator data or snapshots.
func RunCalibration(seeds []uint64, horizonDays int) (CalibrationReport, error) {
	if len(seeds) == 0 {
		return CalibrationReport{}, fmt.Errorf("at least one seed is required")
	}
	if horizonDays <= 0 {
		return CalibrationReport{}, fmt.Errorf("horizonDays must be positive")
	}
	report := CalibrationReport{
		Seeds:       append([]uint64{}, seeds...),
		HorizonDays: horizonDays,
		ChanceTypes: map[string]int{},
		ShotQuality: map[string]int{},
	}
	for _, seed := range seeds {
		res, err := worldgen.Generate(worldgen.PresetCompact(seed), worldgen.WithTokenReader(&calibrationTokens{}))
		if err != nil {
			return CalibrationReport{}, err
		}
		e := New(res.World, res.Queue, &store.MemAuditLog{})
		if _, err := e.RunUntil(sim.GameTime(int64(horizonDays) * sim.MinutesPerDay)); err != nil {
			return CalibrationReport{}, err
		}
		accumulateCalibration(&report, e.World())
	}
	report.finalize()
	return report, nil
}

func accumulateCalibration(report *CalibrationReport, w *worldgen.World) {
	clubByID := make(map[int64]*worldgen.Club, len(w.Clubs))
	for i := range w.Clubs {
		clubByID[w.Clubs[i].ID] = &w.Clubs[i]
	}
	for i := range w.History {
		for j := range w.History[i].Results {
			accumulateCalibrationResult(report, clubByID, &w.History[i].Results[j])
		}
	}
	for i := range w.Results {
		accumulateCalibrationResult(report, clubByID, &w.Results[i])
	}
}

func accumulateCalibrationResult(report *CalibrationReport, clubByID map[int64]*worldgen.Club, r *worldgen.MatchResult) {
	report.Matches++
	report.Goals += r.HomeGoals + r.AwayGoals
	report.Shots += r.HomeShots + r.AwayShots
	switch {
	case r.HomeGoals > r.AwayGoals:
		report.HomeWins++
		if isCalibrationUpset(clubByID[r.HomeID], clubByID[r.AwayID]) {
			report.Upsets++
		}
	case r.HomeGoals < r.AwayGoals:
		report.AwayWins++
		if isCalibrationUpset(clubByID[r.AwayID], clubByID[r.HomeID]) {
			report.Upsets++
		}
	default:
		report.Draws++
	}
	addCounts(report.ChanceTypes, r.ChanceTypes)
	addCounts(report.ShotQuality, r.Diagnostics.ShotQuality)
	report.AerialDuels += sumCounts(r.Diagnostics.AerialDuels)
	report.AerialWins += sumCounts(r.Diagnostics.AerialWins)
	report.PressTurnovers += sumCounts(r.Diagnostics.PressTurnovers)
	report.SetPieceThreat += sumCounts(r.Diagnostics.SetPieceThreat)
}

func isCalibrationUpset(winner, loser *worldgen.Club) bool {
	if winner == nil || loser == nil {
		return false
	}
	return winner.PredictedFinish >= loser.PredictedFinish+4
}

func (r *CalibrationReport) finalize() {
	if r.Matches > 0 {
		r.GoalsPerMatchX100 = r.Goals * 100 / r.Matches
		r.ShotsPerMatchX100 = r.Shots * 100 / r.Matches
	}
	if r.Shots > 0 {
		r.ConversionRateX100 = r.Goals * 100 / r.Shots
	}
	if len(r.ChanceTypes) == 0 {
		r.ChanceTypes = nil
	}
	if len(r.ShotQuality) == 0 {
		r.ShotQuality = nil
	}
}

func addCounts(dst, src map[string]int) {
	for k, v := range src {
		dst[k] += v
	}
}

func sumCounts(src map[string]int) int {
	n := 0
	for _, v := range src {
		n += v
	}
	return n
}

type calibrationTokens struct{ n uint32 }

func (t *calibrationTokens) Read(p []byte) (int, error) {
	for i := range p {
		t.n++
		p[i] = byte((t.n * 2654435761) >> 24)
	}
	return len(p), nil
}
