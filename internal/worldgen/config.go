// Package worldgen implements the world generation pipeline from
// docs/09-world-generation.md: operator config validation, deterministic
// derivation, and the ten seeded stages. Same config + same seed ⇒ identical
// world (NFR-2); every stage consumes its own RNG stream so a change in one
// stage never perturbs another.
package worldgen

import (
	"fmt"
	"strings"

	"github.com/gaemi/agentic-fc/internal/sim"
)

// Quality scales the Ability Pool bands of every division (docs/09 §2.1).
type Quality string

const (
	QualityAmateur      Quality = "AMATEUR"
	QualitySemiPro      Quality = "SEMI_PRO"
	QualityProfessional Quality = "PROFESSIONAL"
	QualityElite        Quality = "ELITE"
)

// EconomyScale scales all money in the world (docs/09 §2.1).
type EconomyScale string

const (
	EconomyAusterity EconomyScale = "AUSTERITY"
	EconomyStandard  EconomyScale = "STANDARD"
	EconomyFlush     EconomyScale = "FLUSH"
)

// Culture is one of the configured name cultures (docs/09 §2.2).
type Culture string

const (
	CultureAnglo       Culture = "ANGLO"
	CultureLatin       Culture = "LATIN"
	CultureContinental Culture = "CONTINENTAL"
	CultureEastAsian   Culture = "EAST_ASIAN"
)

// AllCultures fixes the culture order everywhere a mix is sampled — never
// iterate a map for this (determinism).
var AllCultures = [4]Culture{CultureAnglo, CultureLatin, CultureContinental, CultureEastAsian}

// CultureMix holds percentage weights parallel to AllCultures, summing to 100.
type CultureMix [4]int

// DefaultCultureMix is Anglo 40 / Latin 25 / Continental 25 / East Asian 10
// (docs/09 §2.2, tunable).
var DefaultCultureMix = CultureMix{40, 25, 25, 10}

// NameOverrides are operator-provided identity overrides for generated worlds.
// Lists apply in deterministic generation order: clubs are tier-by-tier slots;
// managers are club managers in club order, then the unemployed pool.
type NameOverrides struct {
	ClubNames    []string `json:"club_names,omitempty"`
	ManagerNames []string `json:"manager_names,omitempty"`
}

// WorldConfig is the operator's World Config (docs/09 §2). Zero values are
// not defaulted here — build from DefaultConfig or a preset and override.
type WorldConfig struct {
	Name             string        `json:"name"` // empty ⇒ generated in stage 1
	Seed             uint64        `json:"seed"`
	Divisions        int           `json:"divisions"`          // 1–5
	ClubsPerDivision int           `json:"clubs_per_division"` // 8–24, even
	RunProfile       string        `json:"run_profile,omitempty"`
	GameSpeed        sim.Speed     `json:"game_speed"`
	Quality          Quality       `json:"quality"`
	Economy          EconomyScale  `json:"economy"`
	CultureMix       CultureMix    `json:"culture_mix"`
	IdleAcceleration int           `json:"idle_acceleration"`      // 2–64 × base
	OffseasonAccel   int           `json:"offseason_acceleration"` // 2–240 × base
	SquadSizeTarget  int           `json:"squad_size_target"`      // 20–30
	YouthIntakeBatch int           `json:"youth_intake_batch"`     // 3–8
	StartRunning     bool          `json:"start_running"`          // false = "ready"
	NameOverrides    NameOverrides `json:"name_overrides,omitempty"`
}

// DefaultConfig returns the wizard defaults (docs/09 §2.1/2.2) with the given
// seed. The default league shape is the Classic preset (2×16).
func DefaultConfig(seed uint64) WorldConfig {
	return WorldConfig{
		Seed:             seed,
		Divisions:        2,
		ClubsPerDivision: 16,
		RunProfile:       "default",
		GameSpeed:        sim.Speed15,
		Quality:          QualityProfessional,
		Economy:          EconomyStandard,
		CultureMix:       DefaultCultureMix,
		IdleAcceleration: sim.DefaultIdleAcceleration,
		OffseasonAccel:   sim.DefaultOffseasonAcceleration,
		SquadSizeTarget:  24,
		YouthIntakeBatch: 5,
	}
}

// Presets for one-key setup (docs/09 §2.1).
func PresetCompact(seed uint64) WorldConfig {
	c := DefaultConfig(seed)
	c.Divisions, c.ClubsPerDivision = 1, 12
	return c
}

func PresetClassic(seed uint64) WorldConfig { return DefaultConfig(seed) }

func PresetDeep(seed uint64) WorldConfig {
	c := DefaultConfig(seed)
	c.Divisions, c.ClubsPerDivision = 3, 16
	return c
}

func PresetSprawling(seed uint64) WorldConfig {
	c := DefaultConfig(seed)
	c.Divisions, c.ClubsPerDivision = 4, 20
	return c
}

const maxCustomNameLen = 64

// Normalized returns the canonical config persisted in snapshots. It trims
// operator-provided display names without otherwise changing generation choices.
func (c WorldConfig) Normalized() WorldConfig {
	c.Name = strings.TrimSpace(c.Name)
	c.NameOverrides.ClubNames = normalizeNameList(c.NameOverrides.ClubNames)
	c.NameOverrides.ManagerNames = normalizeNameList(c.NameOverrides.ManagerNames)
	return c
}

func normalizeNameList(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = strings.TrimSpace(name)
	}
	return out
}

// Validate checks every configured range from docs/09 §2 (stage 0).
func (c WorldConfig) Validate() error {
	c = c.Normalized()
	if c.Divisions < 1 || c.Divisions > 5 {
		return fmt.Errorf("divisions %d out of range 1–5", c.Divisions)
	}
	if c.ClubsPerDivision < 8 || c.ClubsPerDivision > 24 {
		return fmt.Errorf("clubs per division %d out of range 8–24", c.ClubsPerDivision)
	}
	if c.ClubsPerDivision%2 != 0 {
		return fmt.Errorf("clubs per division %d must be even", c.ClubsPerDivision)
	}
	switch c.GameSpeed {
	case sim.Speed5, sim.Speed15, sim.Speed30, sim.Speed60:
	default:
		return fmt.Errorf("game speed %d not in the fixed tier set", c.GameSpeed)
	}
	switch c.RunProfile {
	case "", "default", "fast", "slow", "custom":
	default:
		return fmt.Errorf("unknown run profile %q", c.RunProfile)
	}
	switch c.Quality {
	case QualityAmateur, QualitySemiPro, QualityProfessional, QualityElite:
	default:
		return fmt.Errorf("unknown world quality %q", c.Quality)
	}
	switch c.Economy {
	case EconomyAusterity, EconomyStandard, EconomyFlush:
	default:
		return fmt.Errorf("unknown economy scale %q", c.Economy)
	}
	sum := 0
	for i, w := range c.CultureMix {
		if w < 0 {
			return fmt.Errorf("culture mix weight for %s is negative", AllCultures[i])
		}
		sum += w
	}
	if sum != 100 {
		return fmt.Errorf("culture mix sums to %d, want 100", sum)
	}
	if c.IdleAcceleration < 2 || c.IdleAcceleration > 64 {
		return fmt.Errorf("idle acceleration %d out of range 2–64", c.IdleAcceleration)
	}
	if c.OffseasonAccel < 2 || c.OffseasonAccel > 240 {
		return fmt.Errorf("offseason acceleration %d out of range 2–240", c.OffseasonAccel)
	}
	if c.SquadSizeTarget < 20 || c.SquadSizeTarget > 30 {
		return fmt.Errorf("squad size target %d out of range 20–30", c.SquadSizeTarget)
	}
	if c.YouthIntakeBatch < 3 || c.YouthIntakeBatch > 8 {
		return fmt.Errorf("youth intake batch %d out of range 3–8", c.YouthIntakeBatch)
	}
	if err := validateNameOverrides("club", c.NameOverrides.ClubNames, c.TotalClubs()); err != nil {
		return err
	}
	if err := validateNameOverrides("manager", c.NameOverrides.ManagerNames, c.TotalClubs()+unemployedPoolSize(c.TotalClubs())); err != nil {
		return err
	}
	return nil
}

// TotalClubs is Divisions × ClubsPerDivision.
func (c WorldConfig) TotalClubs() int { return c.Divisions * c.ClubsPerDivision }

func validateNameOverrides(kind string, names []string, max int) error {
	if len(names) > max {
		return fmt.Errorf("%s name overrides = %d, max %d", kind, len(names), max)
	}
	seen := map[string]bool{}
	for i, name := range names {
		if name == "" {
			return fmt.Errorf("%s name override %d is empty", kind, i+1)
		}
		if len([]rune(name)) > maxCustomNameLen {
			return fmt.Errorf("%s name override %d exceeds %d characters", kind, i+1, maxCustomNameLen)
		}
		if strings.ContainsAny(name, "\r\n\t") {
			return fmt.Errorf("%s name override %d contains unsupported control characters", kind, i+1)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate %s name override %q", kind, name)
		}
		seen[key] = true
	}
	return nil
}
