// Package attr implements the attribute taxonomy from docs/08-attributes.md:
// expanded visible attributes (GKs swap outfield technicals for goalkeeping),
// hidden attributes, weak-foot proficiency, and the Ability Pool cost model.
package attr

// Scale bounds for visible and hidden attributes (1–20 integer display).
const (
	ScaleMin = 1
	ScaleMax = 20
)

// Ability Pool / Potential Cap scale (0–200).
const (
	PoolMin = 0
	PoolMax = 200
)

type Visible string

// Outfield technical (GKs do not carry these).
const (
	Finishing  Visible = "FINISHING"
	LongShots  Visible = "LONG_SHOTS"
	FirstTouch Visible = "FIRST_TOUCH"
	Passing    Visible = "PASSING"
	Crossing   Visible = "CROSSING"
	Dribbling  Visible = "DRIBBLING"
	Technique  Visible = "TECHNIQUE"
	Heading    Visible = "HEADING"
	Tackling   Visible = "TACKLING"
	Marking    Visible = "MARKING"
	SetPieces  Visible = "SET_PIECES"
)

// Mental (all players).
const (
	Aggression    Visible = "AGGRESSION"
	Vision        Visible = "VISION"
	Decisions     Visible = "DECISIONS"
	Composure     Visible = "COMPOSURE"
	Concentration Visible = "CONCENTRATION"
	Positioning   Visible = "POSITIONING"
	OffBall       Visible = "OFF_BALL"
	Anticipation  Visible = "ANTICIPATION"
	WorkRate      Visible = "WORK_RATE"
	Bravery       Visible = "BRAVERY"
	Teamwork      Visible = "TEAMWORK"
	Leadership    Visible = "LEADERSHIP"
	Determination Visible = "DETERMINATION"
	Flair         Visible = "FLAIR"
)

// Physical (all players).
const (
	Acceleration   Visible = "ACCELERATION"
	Pace           Visible = "PACE"
	Agility        Visible = "AGILITY"
	Balance        Visible = "BALANCE"
	Strength       Visible = "STRENGTH"
	Stamina        Visible = "STAMINA"
	NaturalFitness Visible = "NATURAL_FITNESS"
	JumpingReach   Visible = "JUMPING_REACH"
)

// Goalkeeping (replaces the technical five for GKs).
const (
	Reflexes      Visible = "REFLEXES"
	OneOnOnes     Visible = "ONE_ON_ONES"
	Handling      Visible = "HANDLING"
	AerialReach   Visible = "AERIAL_REACH"
	CommandOfArea Visible = "COMMAND_OF_AREA"
	Communication Visible = "COMMUNICATION"
	Distribution  Visible = "DISTRIBUTION"
	Sweeping      Visible = "SWEEPING"
	Eccentricity  Visible = "ECCENTRICITY"
	Punching      Visible = "PUNCHING"
)

// Generic labels used by narrative and compatibility surfaces. They are not
// present in PoolCosts.
const (
	Defending     Visible = "DEFENDING"
	Aerial        Visible = "AERIAL"
	ShotStopping  Visible = "SHOT_STOPPING"
	AerialCommand Visible = "AERIAL_COMMAND"
)

type Hidden string

// Trajectory (4).
const (
	PotentialCap     Hidden = "POTENTIAL_CAP" // 0–200, fixed at generation
	DevelopmentSpeed Hidden = "DEVELOPMENT_SPEED"
	DeclineOnset     Hidden = "DECLINE_ONSET"
	DeclineSpeed     Hidden = "DECLINE_SPEED"
)

// Personality (8).
const (
	Professionalism Hidden = "PROFESSIONALISM"
	Ambition        Hidden = "AMBITION"
	Loyalty         Hidden = "LOYALTY"
	Temperament     Hidden = "TEMPERAMENT"
	Pressure        Hidden = "PRESSURE"
	Adaptability    Hidden = "ADAPTABILITY"
	Sportsmanship   Hidden = "SPORTSMANSHIP"
	Controversy     Hidden = "CONTROVERSY"
	Sociability     Hidden = "SOCIABILITY"
	Influence       Hidden = "INFLUENCE"
)

// Volatility & durability (6).
const (
	Consistency   Hidden = "CONSISTENCY" // full ability in x/N matches
	BigMatchNerve Hidden = "BIG_MATCH_NERVE"
	InjuryProne   Hidden = "INJURY_PRONENESS"
	Recovery      Hidden = "RECOVERY"
	Discipline    Hidden = "DISCIPLINE"
	Versatility   Hidden = "VERSATILITY"
)

// Social (1). Reputation uses its own 0–10,000 scale.
const Reputation Hidden = "REPUTATION"

type PositionGroup string

const (
	GK PositionGroup = "GK"
	DF PositionGroup = "DF"
	MF PositionGroup = "MF"
	FW PositionGroup = "FW"
)

// PreferredFoot is visible, surfaced as a descriptor (docs/08 §2).
type PreferredFoot string

const (
	FootLeft   PreferredFoot = "LEFT"
	FootRight  PreferredFoot = "RIGHT"
	FootEither PreferredFoot = "EITHER"
)

// WeakFootProficiency is stored on Player as a 1–20 football ability. The raw
// value is treated like visible ability for own/scouted reads; public displays
// can collapse it to this descriptor.
func WeakFootDescriptor(v int) string {
	switch {
	case v >= 16:
		return "Strong"
	case v >= 11:
		return "Useful"
	case v >= 6:
		return "Limited"
	default:
		return "One-footed"
	}
}

// PositionalFamiliarity is the hidden per-position competence map (0–20),
// surfaced only as descriptors: Natural / Accomplished / Competent / Awkward
// (docs/08 §2). Keys are concrete positions, e.g. "GK", "DC", "ML", "ST".
type PositionalFamiliarity map[string]int

// FamiliarityDescriptor derives the visible label from a hidden value.
func FamiliarityDescriptor(v int) string {
	switch {
	case v >= 18:
		return "Natural"
	case v >= 13:
		return "Accomplished"
	case v >= 7:
		return "Competent"
	default:
		return "Awkward"
	}
}

// PoolCosts is the per-position Ability Pool cost table from docs/08 §4 —
// the primary balance surface (initial values, tunable). Pool consumed =
// Σ (value − 1) × weight. GKs have no outfield-technical rows; outfield
// players have no goalkeeping rows.
var PoolCosts = map[PositionGroup]map[Visible]float64{
	GK: {
		Reflexes: 2.0, OneOnOnes: 1.6, Handling: 1.6, AerialReach: 1.2, CommandOfArea: 1.2,
		Communication: 0.8, Distribution: 0.9, Sweeping: 0.8, Eccentricity: 0.1, Punching: 0.1,
		Aggression: 0.1, Vision: 0.4, Decisions: 1.6, Composure: 1.0, Concentration: 1.4,
		Positioning: 1.0, OffBall: 0.1, Anticipation: 1.2, WorkRate: 0.3, Bravery: 0.4,
		Teamwork: 0.4, Leadership: 0.4, Determination: 0.1, Flair: 0.1,
		Acceleration: 0.4, Pace: 0.5, Agility: 1.2, Balance: 0.8, Strength: 0.7,
		Stamina: 0.4, NaturalFitness: 0.1, JumpingReach: 0.5,
	},
	DF: {
		Finishing: 0.3, LongShots: 0.2, FirstTouch: 0.8, Passing: 0.9, Crossing: 0.5,
		Dribbling: 0.5, Technique: 0.7, Heading: 1.1, Tackling: 1.8, Marking: 1.7, SetPieces: 0.3,
		Aggression: 0.2, Vision: 0.7, Decisions: 1.3, Composure: 1.0, Concentration: 1.4,
		Positioning: 1.6, OffBall: 0.4, Anticipation: 1.4, WorkRate: 0.8, Bravery: 0.7,
		Teamwork: 0.5, Leadership: 0.5, Determination: 0.1, Flair: 0.1,
		Acceleration: 2.0, Pace: 2.0, Agility: 1.0, Balance: 0.9, Strength: 1.3,
		Stamina: 1.0, NaturalFitness: 0.1, JumpingReach: 1.2,
	},
	MF: {
		Finishing: 0.7, LongShots: 0.6, FirstTouch: 1.4, Passing: 1.5, Crossing: 0.8,
		Dribbling: 1.2, Technique: 1.3, Heading: 0.4, Tackling: 0.9, Marking: 0.9, SetPieces: 0.4,
		Aggression: 0.2, Vision: 1.4, Decisions: 1.4, Composure: 1.1, Concentration: 1.0,
		Positioning: 1.2, OffBall: 1.0, Anticipation: 1.2, WorkRate: 1.0, Bravery: 0.4,
		Teamwork: 0.8, Leadership: 0.5, Determination: 0.1, Flair: 0.4,
		Acceleration: 2.0, Pace: 2.0, Agility: 1.1, Balance: 1.0, Strength: 1.0,
		Stamina: 1.2, NaturalFitness: 0.1, JumpingReach: 0.5,
	},
	FW: {
		Finishing: 1.8, LongShots: 0.8, FirstTouch: 1.2, Passing: 1.0, Crossing: 0.8,
		Dribbling: 1.4, Technique: 1.2, Heading: 1.0, Tackling: 0.2, Marking: 0.2, SetPieces: 0.4,
		Aggression: 0.2, Vision: 1.1, Decisions: 1.1, Composure: 1.2, Concentration: 0.8,
		Positioning: 0.6, OffBall: 1.4, Anticipation: 1.2, WorkRate: 0.8, Bravery: 0.6,
		Teamwork: 0.6, Leadership: 0.4, Determination: 0.1, Flair: 0.5,
		Acceleration: 2.2, Pace: 2.2, Agility: 1.3, Balance: 1.0, Strength: 1.2,
		Stamina: 1.0, NaturalFitness: 0.1, JumpingReach: 0.8,
	},
}

// WeakFootCosts follow the same "budget cost, not event weight" rule as the
// visible table. Wide/forward roles pay more because two-footed actions matter
// more often in their event mix.
var WeakFootCosts = map[PositionGroup]float64{GK: 0.2, DF: 0.5, MF: 0.8, FW: 1.0}

// PersonalityDescriptor derives the single public personality read from
// hidden attributes (docs/08 §5, precedence top-down; initial thresholds,
// tunable). Returns a stable key rendered via desc.player.* catalogs.
func PersonalityDescriptor(hidden map[Hidden]int, visible map[Visible]int) string {
	anyVisible16 := false
	for _, v := range visible {
		if v >= 16 {
			anyVisible16 = true
			break
		}
	}
	switch {
	case hidden[Professionalism] >= 18:
		return "CONSUMMATE_PROFESSIONAL"
	case hidden[Ambition] >= 16 && hidden[Professionalism] >= 12:
		return "DRIVEN"
	case hidden[Sociability] >= 16 && hidden[Influence] >= 14:
		return "TEAM_HEART"
	case hidden[Pressure] >= 17 && hidden[BigMatchNerve] >= 15:
		return "IRON_NERVED"
	case hidden[Loyalty] <= 5 && hidden[Ambition] >= 15:
		return "MERCENARY"
	case hidden[Temperament] <= 5 || hidden[Controversy] >= 16:
		return "VOLATILE"
	case hidden[InjuryProne] >= 15 && anyVisible16:
		return "GLASS_CANNON"
	case hidden[Adaptability] <= 5:
		return "HOMEBODY"
	case hidden[Influence] <= 5:
		return "WALLFLOWER"
	default:
		return "BALANCED"
	}
}

func WeakFootCost(group PositionGroup, weakFoot int) float64 {
	w := WeakFootCosts[group]
	v := weakFoot
	if v < ScaleMin {
		v = ScaleMin
	}
	if v > ScaleMax {
		v = ScaleMax
	}
	return float64((v-ScaleMin)*int(w*10+0.5)) / 10
}

// PoolCost computes the Ability Pool consumed by a full visible-attribute
// set. Weights are 0.1-grained, so the sum runs in exact deci-point integers:
// the result is identical regardless of map iteration order — float
// accumulation order must never leak into outcomes (NFR-2).
func PoolCost(group PositionGroup, values map[Visible]int) float64 {
	total := 0
	for a, v := range values {
		if w, ok := PoolCosts[group][a]; ok && v > ScaleMin {
			total += (v - ScaleMin) * int(w*10+0.5)
		}
	}
	return float64(total) / 10
}

// ProfilePoolCost adds weak-foot proficiency to the visible attribute spend.
func ProfilePoolCost(group PositionGroup, values map[Visible]int, weakFoot int) float64 {
	return PoolCost(group, values) + WeakFootCost(group, weakFoot)
}
